package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"tianxuan/internal/agent/cache"
	"tianxuan/internal/agent/offload"
	"tianxuan/internal/archive"
	tiancontext "tianxuan/internal/context"
	"tianxuan/internal/diff"
	"tianxuan/internal/event"
	"tianxuan/internal/evidence"
	"tianxuan/internal/jobs"
	"tianxuan/internal/learning"
	"tianxuan/internal/memory"
	"tianxuan/internal/nilutil"
	"tianxuan/internal/planmode"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// Asker puts structured multiple-choice questions to the user and blocks for the
// answers. The agent consults it for the `ask` tool. It is interface-shaped so
// the agent stays independent of the frontend; a nil asker means no interactive
// user (headless runs), where `ask` returns a "decide for yourself" result. The
// interactive frontends wire the controller in as the Asker.
type Asker interface {
	Ask(ctx context.Context, questions []event.AskQuestion) ([]event.AskAnswer, error)
}

// callContextKey carries the executing tool call's identity into Execute.
type callContextKey struct{}

// callContext is the per-call context a tool can read. parentID is the call being
// executed and sink is the agent's event sink (the `task` tool uses both to nest
// a sub-agent's events under this call); asker lets the `ask` tool reach the user.
type callContext struct {
	parentID string
	sink     event.Sink
	asker    Asker
}

// withCallContext stamps ctx with the executing call's ID, the agent's sink, and
// the asker. executeOne sets this before every Execute; `task` reads it (via
// CallContext) to nest sub-agent events, and `ask` reads the asker to prompt.
func withCallContext(ctx context.Context, parentID string, sink event.Sink, asker Asker) context.Context {
	return context.WithValue(ctx, callContextKey{}, callContext{parentID: parentID, sink: sink, asker: asker})
}

// CallContext returns the executing call's ID, the agent's sink, and the asker,
// if the context was set by an agent's executeOne. ok is false for a plain
// context (headless tool tests, calls made outside the run loop).
func CallContext(ctx context.Context) (parentID string, sink event.Sink, asker Asker, ok bool) {
	cc, ok := ctx.Value(callContextKey{}).(callContext)
	if !ok {
		return "", nil, nil, false
	}
	return cc.parentID, cc.sink, cc.asker, true
}

// StepResult records the outcome of a single complete_step call during a turn.
type StepResult struct {
	Step   string // step name from complete_step args
	Status string // "success", "error", "blocked"
	Result string // result field from complete_step args (truncated)
}

// TurnResult is a structured result produced by an AgentRunner after one turn.
// It lets upstream callers (e.g. Hermes) consume execution outcomes without
// having to extract them post-hoc from the agent's session.
type TurnResult struct {
	Plan          string       // the plan that was executed (empty for non-Hermes turns)
	FilesCreated  []string     // paths of files newly created this turn (vs. modified)
	FilesModified []string     // paths of files written/edited/moved/deleted this turn
	Summary       string       // agent's final conclusion (last assistant message)
	Success       bool         // true = no tool errors encountered this turn
	Errors        []string     // tool error messages collected during execution (max 5)
	StepResults   []StepResult // per-step outcomes from complete_step calls
}

// Runner carries out one task turn. AgentRunner satisfies it.
// Returns a structured TurnResult even on error so callers can inspect partial results.
type Runner interface {
	Run(ctx context.Context, input string) (*TurnResult, error)
}

// Gate decides, per tool call, whether it may run. The agent consults it at
// execute time (after the plan-mode gate). It is interface-shaped so the agent
// stays independent of the permission package and of how "ask" is resolved
// (silently in headless runs, interactively in the chat TUI). A nil gate means
// no gating �� every call runs, preserving behaviour for callers that don't wire
// one in. reason is fed back to the model when allow is false; a non-nil err
// (e.g. ctx cancelled awaiting approval) is treated as a block for that call.
type Gate interface {
	Check(ctx context.Context, toolName string, args json.RawMessage, readOnly bool) (allow bool, reason string, err error)
}

// ToolHooks fires user-configured shell hooks around each tool call. PreToolUse
// runs before the call and may block it (block=true; message is the reason fed
// back to the model); PostToolUse runs after and only surfaces output to the
// user (it can't block). It is interface-shaped so the agent stays independent
// of the hook package �� a nil hooks field disables hook firing entirely.
type ToolHooks interface {
	PermissionRequest(ctx context.Context, name string, args json.RawMessage) (allow bool, modifiedArgs json.RawMessage, reason string)
	PreToolUse(ctx context.Context, name string, args json.RawMessage) (block bool, message string)
	PostToolUse(ctx context.Context, name string, args json.RawMessage, result string)
	// PostLLMCall fires after each model turn completes (streaming finishes)
	// but before reasoning_content is stored. It returns the (possibly
	// translated) reasoning string �� the original when no hook is configured.
	// HasPostLLMCall reports whether such a hook exists, so the agent keeps
	// streaming reasoning live when none is wired up.
	PostLLMCall(ctx context.Context, reasoning string, turn int) string
	HasPostLLMCall() bool
	// SubagentStop fires when a `task` sub-agent finishes (foreground). PreCompact
	// fires just before a compaction pass and returns extra summary guidance (its
	// hooks' stdout) to fold into the summary prompt; "" when no hook contributes.
	SubagentStop(ctx context.Context, last string)
	PreCompact(ctx context.Context, trigger string) string
}

// AgentRunner drives a single task: a Provider, a tool Registry, and a Session
// wired into the main loop. In ModeDirect it runs the model directly; in
// ModePlanner it delegates classification and planning to a Planner before
// handing off to the executor (itself).
// KeepPolicy is a bitmask controlling which messages are preserved verbatim
// during compaction. Zero means no special retention — only digest summaries
// and small user turns are kept.
type KeepPolicy int

const (
	// KeepErrors preserves tool results that start with "error:" or "blocked:"
	// so critical failure information (build errors, test failures) is never
	// summarized away — the model needs those details to fix the problem.
	KeepErrors KeepPolicy = 1 << iota
	// KeepUserMarked preserves user messages prefixed with [[keep]], [keep],
	// <keep>, or <!-- keep --> markers, letting the user pin facts that must
	// survive compaction.
	KeepUserMarked
	// KeepProtected preserves tool results from protected-tool list (e.g.
	// read_skill, memory_search) whose outputs are foundational context that
	// must survive compaction. Pattern borrowed from opencode.
	KeepProtected
)

type AgentRunner struct {
	prov    provider.Provider
	tools   *tool.Registry
	session *Session
	sessMu  sync.Mutex // guards the session pointer for external Session()/SetSession

	// === dispatcher ===
	dispatcher  *ToolDispatcher             // centralized pre-execution checks
	ctxMgr      *tiancontext.ContextManager // V3.0: TCCA kernel (nil = legacy mode)
	maxSteps    int
	temperature float64
	pricing     *provider.Pricing

	// sink receives the turn's typed event stream (reasoning/text deltas, tool
	// dispatch/results, usage, notices). The agent no longer formats output
	// itself �� a frontend's Sink decides how to render. Never nil; New defaults
	// it to event.Discard.
	sink event.Sink

	// lastUsage caches the most recent per-turn telemetry the provider reported so
	// the CLI can expose a context gauge without re-scraping the usage line. The
	// run loop writes it while a frontend's status line reads it, so it is atomic.
	lastUsage atomic.Pointer[provider.Usage]

	// sessCacheHit/sessCacheMiss accumulate cache tokens across every API call
	// this session, so frontends can show the aggregate hit-rate (��hit/��(hit+miss))
	// �� a steadier, cost-oriented number than the single-turn rate. They are NOT
	// reset on compaction (compaction only rewrites session.Messages), so the
	// aggregate never craters when the prefix is summarized away. Atomic: the run
	// loop accumulates them while the status line reads them.
	sessCacheHit  atomic.Int64
	sessCacheMiss atomic.Int64
	// lastPrefixShape records the previous request's cacheable prefix
	// so usage events can explain prefix churn on the next request.
	lastPrefixShape      PrefixShape
	prefixFingerprintSet bool

	// V5.31: output_continue.go
	lenContCount    int
	invalidOutCount int

	// V10.89: 工具失败的结构化反馈 — 防止模型在工具连续失败后
	// 低效循环。跨轮次累计，成功率高的轮次自动重置。
	toolFeedbackCount int

	// V5.31: 重复检测（repeat_detect.go）
	repeatSig   string
	repeatCount int
	steerCount  int             // V8.0 P0-3: consecutive all-fail batches for mid-turn steer
	dedupHashes map[string]bool // V8.0 P0-2: deterministic pruning (tool+args+result → seen)

	// V10.27: 后台任务启停循环检测 — 防止模型反复 start-bash → kill_shell
	// 而不读取输出。跨轮次累计，仅 bash_output/wait/前台 bash 重置计数。
	bgStartKillStreak    int  // 连续启停循环计数（跨轮次累计）
	bgJobStartedThisTurn bool // 本轮启动了后台 bash 任务
	bgOutputReadThisTurn bool // 本轮读取了后台任务输出（bash_output / wait）
	bgJobKilledThisTurn  bool // 本轮杀掉了后台任务（kill_shell）

	// V10.28: stale anchor 编辑守卫 — 同一轮内编辑文件后必须重新 read_file，
	// 防止 old_string 锚点过时。追踪每轮写入和读取的文件路径。
	staleWrittenFiles map[string]bool // 本轮已写入的文件路径
	staleReadFiles    map[string]bool // 本轮已读取的文件路径

	// V10.13: 成功循环检测 — 移植自 Reasonix repeatedSuccessBlock。
	// 检测写工具在同一用户轮次中重复成功调用，阈值 2 次后阻止。
	repeatSuccessCounts map[string]int

	// V6.0: 回忆提醒开关（recall_reminder.go）
	recallReminderFired bool

	// Stop gates (stop_gate.go) — triple gate for solo mode, skipped in plannerMode.
	// taskGate checks incomplete canonical todos, goalGate verifies session goal,
	// verifyGate nudges the model to run tests. All three re-enter at most 3 times.
	verifyGateFired bool // Gate: orchestrate verify fired
	disableVerify   bool // suppress verify nudge (for sub-agents)
	taskGateReentry int  // V10.87: reentry counter for taskGate (cap 3)
	goalGateReentry int  // V10.87: reentry counter for goalGate (cap 3)

	// V6.0 P7: session goal (set via /goal), enforced by stop gate
	goal string

	// gate, when non-nil, is the per-call permission gate consulted in
	// executeOne. nil disables gating entirely.
	// MUST be set before Run() starts — executeOne is called from concurrent
	// goroutines (executeBatch → runParallel), and SetGate does not lock.
	// The happens-before guarantee: Controller.EnableInteractiveApproval calls
	// SetGate before dispatching Send(), which starts the run loop. The run loop
	// spawns goroutines only after the gate is written, so the write is visible
	// to all concurrent readers. A nil gate means no gating.
	gate Gate

	// hooks fires PreToolUse / PostToolUse / PermissionRequest / SubagentStop
	// hooks around tool calls. Set once during New() and never mutated afterwards,
	// so concurrent reads from executeOne goroutines are safe without a lock.
	// nil disables all hook firing.
	hooks ToolHooks

	// asker lets the `ask` tool put questions to the user. Set via SetAsker
	// before the run loop starts (same happens-before contract as gate).
	// nil in headless runs. Safe for concurrent reads.
	asker Asker

	// onPreEdit, when non-nil, is called with a writer tool's previewed change
	// just before it runs �� the seam the checkpoint store uses to snapshot a
	// file's pre-edit content. Only fires for non-ReadOnly tools that implement
	// tool.Previewer (so bash, whose targets are unknowable, is never tracked).
	// Set via SetPreEditHook.
	onPreEdit func(diff.Change)

	// pendingDiffs collects writer tool diffs for post-turn injection.
	pendingDiffs []diff.Change

	// patternExtractor learns from recurring tool errors across sessions.
	patternExtractor interface {
		Extract(toolName, result string) *learning.Pattern
		SaveStore() error
	}

	// jobs, when non-nil, is the session's background-job manager. executeOne
	// stamps it onto each tool call's context so the background tools (bash
	// run_in_background, task run_in_background, bash_output/kill_shell/wait) can
	// reach it. nil leaves those tools to degrade gracefully.
	jobs *jobs.Manager

	// evidence is a per-user-turn ledger of host-observed tool receipts. It lets
	// complete_step validate that cited evidence happened before the claim.
	evidence *evidence.Ledger

	// memQueue, when non-nil, lets the remember/forget tools fold a turn-tail note
	// about a just-made memory change into the next turn, so it applies this
	// session without touching the cache-stable prefix. Set via SetMemoryQueue.
	memQueue     memory.Queue
	sessionSaver memory.SessionSaver
	promoter     memory.SessionFactPromoter

	// archive, when non-nil, records session messages to persistent storage
	// for cross-session Dream/Distill analysis (V7.0).
	archive *archive.Store
	// sessionID is the current session identifier for archive recording.
	sessionID string

	// compaction groups context-window and compression settings (V5.0: truncation only).
	compaction CompactionConfig
	keepPolicy KeepPolicy // V10.0: messages to retain verbatim during compaction

	// V7.0 DSR: compact stuck detection �� when the kept tail alone exceeds the
	// trigger threshold, compaction can never reduce the prompt below it. After 2
	// consecutive compactions that fail to get below the trigger, we pause
	// auto-compaction and emit a warning.
	consecutiveCompacts int
	compactStuck        bool

	// activeSchemas, when non-nil, overrides the full tool registry for this
	// session. Set by the controller after GoalRouter classifies the task.
	activeSchemas   []provider.ToolSchema
	activeSchemasMu sync.RWMutex

	// storm tracks repeated failures to detect death spirals (V3.0).
	storm StormBreaker

	// V5.11: ����Ŀ¼ָ�ơ������� stream() ʱ��¼������ÿ�ֱȽϡ�
	// ��⹤�߼��仯��additive/breaking����breaking ʱ emit Warning��
	lastToolFingerprint    ToolCatalogFingerprint
	lastToolFingerprintSet bool

	// V5.13: �������籩��·���������ͬ turn ���ظ����ã���ǰԤ����
	paramStorm *ParamStormBreaker

	// V5.15: Ԥ���ſء���׷�ٻỰ�ۼƷ��ã�80%����/100%��ϡ�
	budgetGate *BudgetGate
	// lspManager runs LSP diagnostics on files modified by writer tools
	// and injects results so the model can fix compilation errors.
	lspManager interface {
		Diagnostics(ctx context.Context, file string) (string, error)
	}

	// auditFunc, when non-nil, is called after each tool execution for
	// audit trail logging (V3.2).
	auditFunc func(tool string, taskKind string, readOnly bool, outcome string, errMsg string, outputLen int, durationMs int64)

	// preOutcomes collects results of read-only tool calls that were pre-executed
	// during stream() before the full batch. Keyed by tool call ID. executeBatch
	// skips calls already present here. Protected by preMu.
	preOutcomes map[string]toolOutcome
	preMu       sync.Mutex
	preWG       sync.WaitGroup

	// tc caches read-only tool results (file reads) to avoid redundant disk IO
	// within a turn. Write operations auto-invalidate. Thread-safe.
	tc *cache.Cache

	// steerQueue holds mid-turn user messages queued while the agent is
	// running. Each is consumed once per loop iteration, persisted to the
	// session for history replay, and sent to the model as guidance (not a
	// new task). (Design adopted from DeepSeek-Reasonix-V1.12)
	steerMu       sync.Mutex
	steerQueue    []string
	steerConsumed bool

	// todoState is the host canonical task list: the latest successful
	// todo_write with completions applied by complete_step. Unlike the per-turn
	// ledger it survives turn boundaries and compaction, so the final-answer
	// gate sees an unfinished plan a later turn would otherwise hide.
	// (Design adopted from DeepSeek-Reasonix-V1.12)
	todoMu    sync.Mutex
	todoState []evidence.TodoItem

	// hostAdvanceSeq guarantees unique tool IDs across turns: every
	// emitTodoState call increments it so the frontend always sees a fresh
	// dispatch.
	hostAdvanceSeq atomic.Int64

	// responseLanguage is the runtime final-answer language preference
	// ("auto"|"zh"|"en"), stored as an atomic.Value for lock-free reads
	// from the hot stream path. Set via SetResponseLanguage.
	// (Design adopted from DeepSeek-Reasonix-V1.12)

	responseLanguage atomic.Value // string

	// reasoningLanguage is the runtime visible-reasoning language preference
	// ("auto"|"zh"|"en"), stored as an atomic.Value.
	// Set via SetReasoningLanguage.
	// (Design adopted from DeepSeek-Reasonix-V1.12)
	reasoningLanguage atomic.Value // string

	// plannerMode skips executor-specific logic — turn preferences,
	// todo rebuild, steer, repeat detection, bg cycle detection,
	// and grace round (V10.46).
	plannerMode bool

	// planModeGate, when true, refuses any tool call whose ReadOnly() is false.
	// Ported from DeepSeek-Reasonix planmode.Policy.
	planModeGate atomic.Bool

	// planModePolicy carries the policy parameters for plan-mode tool gating.
	planModePolicy planmode.Policy

	// offloadStore manages context offloading: large tool outputs are saved to
	// disk and replaced with compact references to keep the context window lean.
	// nil when offloading is disabled (OffloadDir empty).
	offloadStore *offload.Store
	// offloadThresholdChars is the character threshold above which results are
	// offloaded. Zero means use the default.
	offloadThresholdChars int
}

// SetActiveSchemas installs a tool subset for this session. Pass nil to revert
// to the full registry. Called by the controller after GoalRouter classification.
// Thread-safe: may be called while stream() reads activeSchemas.
func (a *AgentRunner) SetActiveSchemas(schemas []provider.ToolSchema) {
	a.activeSchemasMu.Lock()
	a.activeSchemas = schemas
	a.activeSchemasMu.Unlock()
}

// SetPlanMode flips the read-only plan-mode gate. While true, executeOne refuses
// any non-read-only tool call using planmode.Policy.Decide.
// Ported from DeepSeek-Reasonix.
func (a *AgentRunner) SetPlanMode(v bool) {
	a.planModeGate.Store(v)
}

// SetPlanModePolicy installs the plan-mode tool safety policy.
func (a *AgentRunner) SetPlanModePolicy(p planmode.Policy) {
	a.planModePolicy = p
}

// planModeBlocked checks whether a tool call is blocked by the plan-mode gate.
func (a *AgentRunner) planModeBlocked(name string, readOnly, untrusted bool, safety planmode.PlanSafety, args json.RawMessage) (bool, string) {
	decision := a.planModePolicy.Decide(planmode.Call{
		Name:      name,
		ReadOnly:  readOnly,
		Untrusted: untrusted,
		Safety:    safety,
		Args:      args,
	})
	if !decision.Blocked {
		return false, ""
	}
	return true, decision.Message
}

// SetGate installs the per-call permission gate. MUST be called before the
// run loop starts — executeOne reads gate from concurrent goroutines and
// SetGate does not lock. The happens-before guarantee is provided by the
// caller (Controller) wiring the gate before dispatching the first Send().
// nil disables gating.
func (a *AgentRunner) SetGate(g Gate) {
	if nilutil.IsNil(g) {
		g = nil
	}
	a.gate = g
}

// SetAsker installs the asker the `ask` tool uses to question the user.
// Interactive frontends wire one in; headless runs leave it nil.
func (a *AgentRunner) SetAsker(as Asker) { a.asker = as }

// MergeRuntimePrompt 将运行时上下文合并到系统提示词（L1）末尾，
// 取代原 L2 注入方案。合并后消息前缀永不改变，DeepSeek 可自然缓存。
// 必须在首轮 stream() 调用前调用一次。
func (a *AgentRunner) MergeRuntimePrompt(content string) {
	content = strings.TrimSpace(content)
	if content == "" || len(a.session.Messages) == 0 || a.session.Messages[0].Role != provider.RoleSystem {
		return
	}
	a.session.Messages[0].Content += "\n\n" + content
}
func (a *AgentRunner) SetGoal(g string) { a.goal = g }

// SetMemoryQueue installs the sink the remember/forget tools use to apply a
// memory change in the current session. The controller wires itself in.
func (a *AgentRunner) SetMemoryQueue(q memory.Queue) { a.memQueue = q }

// SetSessionSaver installs the sink the remember tool uses when session=true.
func (a *AgentRunner) SetSessionSaver(s memory.SessionSaver) { a.sessionSaver = s }

// SetPromoter installs the sink the promote_session_facts tool uses.
func (a *AgentRunner) SetPromoter(p memory.SessionFactPromoter) { a.promoter = p }

// SetArchive installs the session archive store for cross-session Dream/Distill.
// nil disables archiving. V7.0.
func (a *AgentRunner) SetLSPManager(m interface {
	Diagnostics(ctx context.Context, file string) (string, error)
}) {
	a.lspManager = m
}

// Sink returns the current event sink. SetSink replaces it.
func (a *AgentRunner) Sink() event.Sink { return a.sink }

// SetSink replaces the agent's event sink. Callers must ensure no concurrent
// Run() — this is intended for one-time setup or between-turn sink wrapping.
func (a *AgentRunner) SetSink(s event.Sink) { a.sink = s }

func (a *AgentRunner) SetPatternExtractor(e interface {
	Extract(toolName, result string) *learning.Pattern
	SaveStore() error
}) {
	a.patternExtractor = e
}
func (a *AgentRunner) SetArchive(ar *archive.Store, sessionID string) {
	a.archive = ar
	a.sessionID = sessionID
}

// SetOffload enables context offloading for this session. dir is the parent
// directory for offloaded files; a session-specific subdirectory is created
// automatically. Pass an empty dir to disable. thresholdChars is the output
// size above which results are offloaded (0 = default).
func (a *AgentRunner) SetOffload(dir string, thresholdChars int) {
	if dir == "" {
		a.offloadStore = nil
		return
	}
	s, err := offload.NewStore(dir, a.sessionID)
	if err != nil {
		a.offloadStore = nil
		return
	}
	a.offloadStore = s
	a.offloadThresholdChars = thresholdChars
}

// CloseOffload cleans up the offload store, deleting all offloaded files.
// Safe to call when offloading is disabled (no-op).
func (a *AgentRunner) CloseOffload() {
	if a.offloadStore != nil {
		_ = a.offloadStore.RemoveAll()
		a.offloadStore = nil
	}
}

// SetPreEditHook installs the pre-edit snapshot hook (see onPreEdit). The
// controller wires it to its per-session checkpoint store; nil disables capture.
func (a *AgentRunner) SetPreEditHook(fn func(diff.Change)) { a.onPreEdit = fn }

// PendingDiffs returns the file changes recorded during the current turn.
// Used by the WorkspacePanel to show session-level file modifications.
func (a *AgentRunner) PendingDiffs() []diff.Change {
	a.preMu.Lock()
	defer a.preMu.Unlock()
	out := make([]diff.Change, len(a.pendingDiffs))
	copy(out, a.pendingDiffs)
	return out
}

// Session returns the agent's current conversation, useful for persistence
// hooks that need to read the message log between turns. sessMu serialises this
// pointer read against SetSession, so a frontend (serve's concurrent /history and
// /new handlers) can't race the swap. The run loop touches a.session directly and
// only swaps it via SetSession while idle, so its reads need no lock.
func (a *AgentRunner) Session() *Session {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	return a.session
}

// SetSession replaces the agent's conversation wholesale. Used by
// `tianxuan chat --resume` to load a saved JSONL transcript before the first turn,
// so the model picks up exactly where it left off. Callers serialise it against a
// running turn (it only fires while idle); sessMu guards the pointer swap itself.
func (a *AgentRunner) SetSession(s *Session) {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	a.session = s
	// V8.3.2: reset prefix fingerprint baseline for the new session.
	// verifyPrefix compares L1/L2/tools hashes against the saved baseline;
	// a fresh session has different L1 content, so we must let it re-establish
	// the baseline rather than panic on mismatch.
	a.prefixFingerprintSet = false
	a.lastToolFingerprintSet = false
	// V8.4.1: reset session-level cache counters to prevent cross-session
	// accumulation from producing hit rates > 100%. sessCacheHit/sessCacheMiss
	// increment on every API call and must reset when starting a new session.
	a.sessCacheHit.Store(0)
	a.sessCacheMiss.Store(0)
	// cacheBreakCount removed (Phase 3)
}

// LastUsage returns the most recent per-turn token telemetry the provider
// reported (nil if no turn has run yet). The TUI uses it to show a context
// gauge alongside the prompt; the actual cache decisions still live inside
// maybeCompact.
func (a *AgentRunner) LastUsage() *provider.Usage { return a.lastUsage.Load() }

// SessionCache returns the cumulative cache hit/miss prompt tokens across every
// API call this session �� the basis for the status line's aggregate hit-rate.
func (a *AgentRunner) SessionCache() (hit, miss int) {
	return int(a.sessCacheHit.Load()), int(a.sessCacheMiss.Load())
}

// ContextWindow returns the configured context-window size in tokens. 0
// means compaction is disabled for this agent.
func (a *AgentRunner) CacheBreakCount() int {
	return a.compaction.CompactCount
}

// systemPrompt returns the concatenated system messages (L1 + L2).
func (a *AgentRunner) systemPrompt() string {
	var b strings.Builder
	for _, m := range a.session.Messages {
		if m.Role == provider.RoleSystem {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(m.Content)
		}
	}
	return b.String()
}

func (a *AgentRunner) ContextWindow() int { return a.compaction.Window }

// CompactRatio returns the fraction of the window at which auto-compaction
// fires (e.g. 0.8). The status line uses it to show headroom to the next compact.
func (a *AgentRunner) CompactRatio() float64 { return a.compaction.Ratio }

// Provider returns the LLM provider this runner uses.
func (a *AgentRunner) Provider() provider.Provider { return a.prov }

// Registry returns the tool registry.
func (a *AgentRunner) Registry() *tool.Registry { return a.tools }

// New constructs an AgentRunner. MaxSteps <= 0 means no cap �� the run loop
// continues until the model gives a final answer, the context is cancelled, or
// the provider errors (compaction keeps the context bounded). A nil sink is
// replaced with event.Discard so the agent can always emit unconditionally.
func New(prov provider.Provider, tools *tool.Registry, session *Session, opts Options, sink event.Sink) *AgentRunner {
	// Build CompactionConfig from opts.Compaction.
	comp := opts.Compaction
	if comp.Window == 0 {
		comp.Window = opts.ContextWindow
	}
	if comp.Ratio <= 0 {
		comp.Ratio = defaultCompactRatio
	}
	if comp.RecentKeep <= 0 {
		comp.RecentKeep = minRecentKeep
	}
	// V10.11: KeepProtected is enabled by default so foundational context from
	// read_skill, memory_search, and remember tools survives compaction.
	if comp.KeepPolicy == 0 {
		comp.KeepPolicy = KeepProtected
	}
	if nilutil.IsNil(sink) {
		sink = event.Discard
	}
	gate := opts.Gate
	if nilutil.IsNil(gate) {
		gate = nil
	}
	hooks := opts.Hooks
	if nilutil.IsNil(hooks) {
		hooks = nil
	}
	r := &AgentRunner{
		prov:          prov,
		tools:         tools,
		session:       session,
		maxSteps:      opts.MaxSteps,
		temperature:   opts.Temperature,
		pricing:       opts.Pricing,
		sink:          sink,
		gate:          gate,
		hooks:         hooks,
		jobs:          opts.Jobs,
		evidence:      evidence.NewLedger(),
		compaction:    comp,
		keepPolicy:    comp.KeepPolicy,
		dispatcher:    opts.Dispatcher,
		ctxMgr:        opts.CtxMgr,
		auditFunc:     opts.AuditFunc,
		tc:            cache.New(-1), // V5.8: session �����棬mtime У�������
		goal:          opts.Goal,     // V6.0 P7: �ỰĿ��
		disableVerify: opts.DisableVerify,
		plannerMode:   opts.PlannerMode,
		planModePolicy: planmode.Policy{
			AllowedTools:     opts.PlanModeAllowedTools,
			ReadOnlyCommands: opts.PlanModeReadOnlyCommands,
		},
	}
	r.evidence.SetStrictVerification(opts.StrictEvidence)
	// V5.13: �������籩��·��
	if opts.ParamStorm != nil {
		r.paramStorm = NewParamStormBreaker(*opts.ParamStorm)
	}
	// V5.15: Ԥ���ſ�
	if opts.BudgetLimit > 0 {
		r.budgetGate = NewBudgetGate(opts.BudgetLimit)
	}
	// V5.17: Ӧ��ģ�����ø���ѹ����ֵ
	if opts.ModelProfile != nil {
		ApplyModelProfile(&r.compaction, opts.ModelProfile)
	}
	// V10.57: sub-agent cache alignment — when TemplatePrefix is set, append
	// it to the LAST system message instead of prepending. This keeps L1 bytes
	// at the front (shared with parent → cache hit) while TemplatePrefix follows
	// (shared among same-kind sub-agents).
	if opts.TemplatePrefix != "" {
		// Find the last system message and append to it.
		for i := len(r.session.Messages) - 1; i >= 0; i-- {
			if r.session.Messages[i].Role == provider.RoleSystem {
				r.session.Messages[i].Content += "\n\n" + opts.TemplatePrefix
				break
			}
		}
	}
	// V5.30: override tools JSON sent to API for cache alignment with parent.
	if opts.ActiveSchemas != nil {
		r.activeSchemas = opts.ActiveSchemas
	}
	return r
}

// Run executes one turn with the single-model path (V5.0: Planner removed).
// plan-mode gating is consistent. Call after construction.

// filteredSchemas returns a reduced tool schema list for analysis-only
// inputs. IMPORTANT: intentionally NOT called in runDirect() — DeepSeek prefix
// cache requires immutable tools across a session. Available for session-level use.
// When the input suggests code review/reading/explaining (no write
// intent), writer tools are omitted to save prompt tokens (~15-25% savings).
// Returns nil when no filtering is needed (full tool set).
func (a *AgentRunner) filteredSchemas(input string) []provider.ToolSchema {
	// Only filter for substantial inputs (>25 chars) — single words/commands
	// like "explore" or "review" should not trigger filtering (too ambiguous).
	if len(input) <= 25 {
		return nil
	}

	lower := strings.ToLower(input)

	// Development patterns: create, write, implement, fix, refactor, build
	devKeywords := []string{
		"create", "write", "implement", "fix", "refactor", "change",
		"add", "remove", "delete", "update", "modify", "build",
		"optimize", "migrate", "convert", "deploy",
		"实现", "修复", "重构", "创建", "添加", "删除",
		"修改", "优化", "迁移", "构建",
	}
	for _, kw := range devKeywords {
		if strings.Contains(lower, kw) {
			return nil // full tool set for development tasks
		}
	}

	// Analysis-only patterns (must have at least one match)
	analysisKeywords := []string{
		"review", "explain", "analyze", "analyse",
		"审查", "分析", "解释",
	}
	hasAnalysis := false
	for _, kw := range analysisKeywords {
		if strings.Contains(lower, kw) {
			hasAnalysis = true
			break
		}
	}
	if !hasAnalysis {
		return nil
	}

	// Filter to read-only + meta tools for analysis tasks.
	return a.tools.FilteredSchemas([]string{
		"read_file", "ls", "glob", "grep",
		"git_status", "git_diff", "git_log",
		"lsp_definition", "lsp_references", "lsp_hover", "lsp_diagnostics",
		"web_search", "web_fetch",
		"todo_write", "complete_step",
		"task", "ask",
	})
}

// shouldMidTurnSteer detects error spirals during tool execution and injects
// a corrective hint. Returns true if a steer was injected (caller should
// continue the loop to let the model respond to the hint).
func (a *AgentRunner) shouldMidTurnSteer(calls []provider.ToolCall, results []string) bool {
	if len(calls) == 0 {
		return false
	}

	failed := 0
	blockedCount := 0
	for _, r := range results {
		if strings.HasPrefix(r, "blocked:") {
			blockedCount++ // V8.0.5: plan-mode blocks are normal, not failures
		} else if strings.HasPrefix(r, "error:") || strings.HasPrefix(r, "tool panic:") ||
			strings.HasPrefix(r, "suppressed:") {
			failed++
		}
	}
	// If all calls were blocked by plan mode, that is normal.
	if blockedCount == len(results) {
		a.steerCount = 0
		return false
	}
	// All non-blocked calls failed → likely wrong approach.
	if failed == len(results)-blockedCount && failed >= 2 {
		a.steerCount++
	} else {
		a.steerCount = 0
		return false
	}

	// Inject steer after 1st all-fail batch (gentle) or 3rd (firm).
	if a.steerCount == 1 {
		a.session.Add(provider.Message{Role: provider.RoleUser,
			Content: "[System note: all tool calls in the last batch failed. " +
				"Try a different approach — use read-only tools first to understand " +
				"the situation, break the task into smaller steps, or ask the user " +
				"for clarification if you are unsure. Do NOT retry the same calls.]"})
		return true
	}
	if a.steerCount >= 3 {
		a.session.Add(provider.Message{Role: provider.RoleUser,
			Content: "[System note: you have been stuck for several rounds. " +
				"STOP and re-assess. Read relevant files with read_file first. " +
				"Ask the user a clarifying question with the ask tool. " +
				"Do NOT continue the current approach.]"})
		a.steerCount = 0 // reset after firm steer
		return true
	}

	return false
}

// checkBgStartKillCycle detects repeated start→kill patterns on background bash
// jobs without reading output in between. When the model starts a background job
// and immediately kills it (same turn, no bash_output/wait), it wastes a full API
// round per cycle. After 3 such cycles without recovery, a corrective nudge is
// injected to break the loop. Resets on any foreground bash or output-read.
// Returns true if a nudge was injected (caller should continue the loop).
func (a *AgentRunner) checkBgStartKillCycle() bool {
	// Only track when the pattern appears: started AND killed in the same turn
	// without reading output.
	if !a.bgJobStartedThisTurn || !a.bgJobKilledThisTurn {
		return false
	}
	// If output was read this turn too, this is normal usage — not a cycle.
	if a.bgOutputReadThisTurn {
		return false
	}
	// Same-turn start→kill without reading output.
	a.bgStartKillStreak++

	const threshold = BgStartKillStreakThreshold
	if a.bgStartKillStreak < threshold {
		return false
	}

	// Inject corrective nudge.
	a.session.Add(provider.Message{Role: provider.RoleUser,
		Content: "[System note: you have started background bash jobs and immediately " +
			"killed them without reading their output for " + fmt.Sprintf("%d", a.bgStartKillStreak) +
			" consecutive cycles. This wastes API turns. For short commands like 'go test', " +
			"use foreground bash (omit run_in_background) so you can see the result " +
			"directly. If you must use a background job, call bash_output or wait to " +
			"read its output before deciding to kill it. Do NOT start another background " +
			"job then immediately kill it again.]"})
	a.bgStartKillStreak = 0 // reset after nudge
	return true
}

// ProvName returns the provider model name for diagnostic display.
func (a *AgentRunner) ProvName() string { return a.prov.Name() }

// SetCtxMgr wires the TCCA context kernel.
func (a *AgentRunner) SetCtxMgr(m *tiancontext.ContextManager) {
	a.ctxMgr = m
	if a.dispatcher != nil {
		a.dispatcher.SetObserver(m)
	}
}

// StormBreaker tracks repeated failures to detect death spirals (V3.0 Phase 4).
// tokPerChar derives a tokens-per-character ratio from the last turn's real
// usage so per-message estimates track the provider's tokenizer without a
// local one. Falls back to ~4 chars/token before any usage is known.
func (a *AgentRunner) tokPerChar() float64 {
	if u := a.lastUsage.Load(); u != nil && u.PromptTokens > 0 {
		if c := charsOfMessages(a.session.Messages); c > 0 {
			if r := float64(u.PromptTokens) / float64(c); r > 0.05 && r < 2 {
				return r
			}
		}
	}
	return fallbackTokPerChar
}

// msgChars counts the characters sent to the provider for one message ��
// content plus tool-call names and arguments, but not reasoning (stripped on
// send).
// (Design adopted from DeepSeek-Reasonix-V1.12)
func (a *AgentRunner) Steer(text string) {
	a.steerMu.Lock()
	defer a.steerMu.Unlock()
	a.steerQueue = append(a.steerQueue, text)
	a.steerConsumed = false
}

// SteerConsumed returns true when the steer queue became empty after the last consume.
func (a *AgentRunner) SteerConsumed() bool {
	a.steerMu.Lock()
	defer a.steerMu.Unlock()
	return a.steerConsumed
}

func (a *AgentRunner) consumeSteer() (string, bool) {
	a.steerMu.Lock()
	defer a.steerMu.Unlock()
	if len(a.steerQueue) == 0 {
		return "", false
	}
	t := a.steerQueue[0]
	a.steerQueue = a.steerQueue[1:]
	a.steerConsumed = len(a.steerQueue) == 0
	return t, true
}

func (a *AgentRunner) steerQueueLen() int {
	a.steerMu.Lock()
	defer a.steerMu.Unlock()
	return len(a.steerQueue)
}

// finalReadinessCheck verifies that the model's claim of completion is backed
// by host-observable evidence. Returns reason string if blocked, empty if ok.
// (Design adopted from DeepSeek-Reasonix-V1.12, simplified for tianxuan)
func (a *AgentRunner) finalReadinessCheck() (blocked bool, reason string) {
	if a.evidence == nil {
		return false, ""
	}
	// Check for unverified completed todos: the model marked a todo as
	// "completed" but never ran complete_step for it.
	unverified, hasBaseline := a.evidence.UnverifiedCompletedTodos(nil)
	if hasBaseline && len(unverified) > 0 {
		names := make([]string, len(unverified))
		for i, m := range unverified {
			names[i] = m.ActiveForm
		}
		return true, fmt.Sprintf("complete_step missing for: %s", strings.Join(names, ", "))
	}
	return false, ""
}

// finalReadinessRetryMessage generates a retry prompt when the final-answer
// readiness check blocks completion.
// (Design adopted from DeepSeek-Reasonix-V1.12)
