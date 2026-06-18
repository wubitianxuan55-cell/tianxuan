package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"tianxuan/internal/archive"
	tiancontext "tianxuan/internal/context"
	"tianxuan/internal/diff"
	"tianxuan/internal/event"
	"tianxuan/internal/evidence"
	"tianxuan/internal/jobs"
	"tianxuan/internal/learning"
	"tianxuan/internal/memory"
	"tianxuan/internal/nilutil"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// maxToolOutputBytes caps a single tool result before it goes into the model's
// context. ~32KB is roughly 8K tokens �� enough for a full file read or a busy
// grep, while preventing one accidental "read this 5 MB log" from blowing the
// window before the next compaction runs.
const maxToolOutputBytes = 48 * 1024

// Renderer redraws the assistant's final-answer text as styled output. It is
// applied only after a turn's text stream completes, so the user sees raw
// markdown stream live, then a single redraw replaces it with formatted
// output. The renderer is intentionally interface-shaped so the agent stays
// independent of the cli's markdown library choice. Consumed by TextSink.
type Renderer interface {
	Render(text string) string
}

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

// Runner carries out one task turn. AgentRunner satisfies it.
type Runner interface {
	Run(ctx context.Context, input string) error
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
type AgentRunner struct {
	prov        provider.Provider
	tools       *tool.Registry
	session     *Session
	sessMu      sync.Mutex // guards the session pointer for external Session()/SetSession

	// === dispatcher ===
	dispatcher *ToolDispatcher              // centralized pre-execution checks
	ctxMgr     *tiancontext.ContextManager   // V3.0: TCCA kernel (nil = legacy mode)
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
	cacheBreakCount atomic.Int64 // V5.30: ������Ѵ���
	sessCacheMiss atomic.Int64


	// V5.31: ����������Ƚضϼ�����output_continue.go��
	lenContCount    int
	invalidOutCount int

	// V5.31: 重复检测（repeat_detect.go）
	repeatSig   string
	repeatCount int
	steerCount   int    // V8.0 P0-3: consecutive all-fail batches for mid-turn steer
	dedupHashes  map[string]bool // V8.0 P0-2: deterministic pruning (tool+args+result → seen)

	// V6.0: 回忆提醒开关（recall_reminder.go）
	recallReminderFired bool

	// V7.0: ��բ�Ŷ�����������stop_gate.go��
	taskGateReentry  int  // Gate 1: unfinished task reentries
	goalGateReentry  int  // Gate 2: goal-judge reentries
	verifyGateFired  bool // Gate 3: orchestrate verify fired

	// V6.0 P7: �ỰĿ�꣨/goal ���ã�������ֹͣբ����֤
	goal string

	// V6.0 P3: agent ����ģʽ��"explore"|"develop"|"orchestrate"��
	agentMode string

	// planMode, when true, refuses any tool call whose ReadOnly() is false.
	// The system prompt and tool list never change with the toggle so the
	// prompt-cache prefix stays valid; the gating happens at execute time
	// and the model sees a "blocked" result it can adapt to. Toggled from
	// the outside via SetPlanMode.
	planMode atomic.Bool

	// gate, when non-nil, is the per-call permission gate consulted after the
	// plan-mode check. nil disables gating entirely.
	gate Gate

	// hooks, when non-nil, fires PreToolUse / PostToolUse shell hooks around each
	// tool call. nil disables hook firing.
	hooks ToolHooks

	// asker, when non-nil, lets the `ask` tool put questions to the user. nil in
	// headless runs (no interactive user). Set via SetAsker.
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
	memQueue memory.Queue

	// archive, when non-nil, records session messages to persistent storage
	// for cross-session Dream/Distill analysis (V7.0).
	archive *archive.Store
	// sessionID is the current session identifier for archive recording.
	sessionID string

	// compaction groups context-window and compression settings (V5.0: truncation only).
	compaction CompactionConfig

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

	// V5.9: ������Ѽ�⡪��ÿ�� stream() ����ǰ��Ա�ǰ׺��ϣ��
	// �� cache_read �½� >5% �� >2000 tokens ʱ�������ԭ��
	cacheBreakDetector cacheBreakDetector


	// V5.10: ImmutablePrefix ָ�ơ������� stream() ʱ���� L1+L2+tools ��
	// SHA256 ָ�ƣ�����ÿ��У�顣Ư�� �� panic�����⾲Ĭ�ƻ����档
	prefixFingerprint    string
	prefixFingerprintSet bool


	// V5.11: ����Ŀ¼ָ�ơ������� stream() ʱ��¼������ÿ�ֱȽϡ�
	// ��⹤�߼��仯��additive/breaking����breaking ʱ emit Warning��
	lastToolFingerprint    ToolCatalogFingerprint
	lastToolFingerprintSet bool

	// V5.13: �������籩��·���������ͬ turn ���ظ����ã���ǰԤ����
	paramStorm *ParamStormBreaker

	// V5.14: �Զ�ģ��·�ɡ���flash ģ�� provider��nil=�����Զ�·�ɣ���
	flashProv provider.Provider
	routeHistory *RouteHistory
	// V5.14: ��ǰ turn ʹ�õ� provider��·�ɺ�ѡ������
	activeProv provider.Provider

	// V7.5: �Ự��·���Զ�������һ�ξ���·�ɺ�������
	// ÿ�� runDirect ���´˱�־������� AutoRouteProvider��
	autoRouteLocked    bool
	autoRouteDecision provider.Provider

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
	tc *toolCache
}

// SetActiveSchemas installs a tool subset for this session. Pass nil to revert
// to the full registry. Called by the controller after GoalRouter classification.
// Thread-safe: may be called while stream() reads activeSchemas.
func (a *AgentRunner) SetActiveSchemas(schemas []provider.ToolSchema) {
	a.activeSchemasMu.Lock()
	a.activeSchemas = schemas
	a.activeSchemasMu.Unlock()
}

// SetPlanMode flips the read-only gate. While true, executeOne refuses any
// non-ReadOnly tool the model calls and returns a "blocked" result instead of
// running it. The cache-friendly bits �� system prompt, tools schema, message
// history �� are left untouched, so the toggle costs nothing in cache hits.
func (a *AgentRunner) SetPlanMode(v bool) { a.planMode.Store(v) }

// SetGate installs the per-call permission gate. Used by `tianxuan chat` to swap the
// headless gate built in setup for an interactive one that prompts the user;
// nil disables gating. Safe to call before the run loop starts.
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

// SetFlashProvider ��װ flash ģ�� provider �����Զ�·�� (V5.14)��
// �� nil �����Զ�·�ɡ�
func (a *AgentRunner) SetFlashProvider(p provider.Provider) { a.flashProv = p }

// SetGoal sets the session-level stopping condition (V6.0 P7).
// When non-empty, the stop gate checks whether the model's final answer
// satisfies the goal before allowing the agent to stop.
func (a *AgentRunner) SetGoal(g string) { a.goal = g }

// SetAgentMode switches the runtime mode (V6.0 P3).
// "explore" = read-only research, "develop" = full tools, "orchestrate" = plan��execute��verify.
func (a *AgentRunner) SetAgentMode(mode string) {
	a.agentMode = mode
	switch mode {
	case "explore":
		a.planMode.Store(true)
	case "develop":
		a.planMode.Store(false)
	case "orchestrate":
		a.planMode.Store(true) // starts in plan mode; auto-flipped after plan approval
	}
}

// SetTaskKind is a no-op in V5.0 (Learner removed). Kept for ToolFilterExecutor interface.
func (a *AgentRunner) SetTaskKind(kind string) {}

// CompactNow is a no-op in V5.0 (LLM compact removed �� use truncation instead).
func (a *AgentRunner) CompactNow(ctx context.Context, instructions string) error { return nil }

// SummarizeFrom is a no-op in V5.0.
func (a *AgentRunner) SummarizeFrom(ctx context.Context, boundary int) error { return nil }

// SummarizeUpTo is a no-op in V5.0.
func (a *AgentRunner) SummarizeUpTo(ctx context.Context, boundary int) error { return nil }

// SetMemoryQueue installs the sink the remember/forget tools use to apply a
// memory change in the current session. The controller wires itself in.
func (a *AgentRunner) SetMemoryQueue(q memory.Queue) { a.memQueue = q }

// SetArchive installs the session archive store for cross-session Dream/Distill.
// nil disables archiving. V7.0.
func (a *AgentRunner) SetLSPManager(m interface {
	Diagnostics(ctx context.Context, file string) (string, error)
}) {
	a.lspManager = m
}

func (a *AgentRunner) SetPatternExtractor(e interface {
	Extract(toolName, result string) *learning.Pattern
	SaveStore() error
}) {
	a.patternExtractor = e
}

// runPostToolDiffPreview formats collected writer diffs as unified diff text
// and injects them as a system message so the model sees its own changes.
func (a *AgentRunner) runPostToolDiffPreview(ctx context.Context) {
	if len(a.pendingDiffs) == 0 {
		return
	}
	var sb strings.Builder
	sb.WriteString("Changes made in this step:\n")
	for _, ch := range a.pendingDiffs {
		if ch.Diff == "" {
			continue
		}
		fmt.Fprintf(&sb, "\n--- %s (%s, +%d -%d)\n%s\n",
			ch.Path, string(ch.Kind), ch.Added, ch.Removed, ch.Diff)
	}
	text := strings.TrimRight(sb.String(), "\n")
	if text == "" {
		return
	}
	a.session.Add(provider.Message{
		Role:    provider.RoleSystem,
		Content: text,
	})
	a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
		Text: "diff: " + fmt.Sprintf("%d file(s) changed", len(a.pendingDiffs))})
	a.pendingDiffs = nil
}

// runPostToolDiagnostics runs LSP diagnostics on files modified by writer tools

// in the current batch and injects results into the session. The model sees
// compilation errors before the next turn, reducing fix cycles.
func (a *AgentRunner) runPostToolDiagnostics(ctx context.Context, calls []provider.ToolCall) {
	if a.lspManager == nil || len(calls) == 0 {
		return
	}

	// Collect unique file paths from writer tool calls
	seen := map[string]bool{}
	var files []string
	for _, call := range calls {
		switch call.Name {
		case "edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol", "notebook_edit":
			path := extractFilePath(call.Name, string(call.Arguments))
			if path != "" && !seen[path] {
				seen[path] = true
				files = append(files, path)
			}
		}
	}
	if len(files) == 0 {
		return
	}

	var results []string
	for _, f := range files {
		diag, err := a.lspManager.Diagnostics(ctx, f)
		if err != nil || diag == "" {
			continue
		}
		results = append(results, diag)
	}
	if len(results) == 0 {
		return
	}

	// Inject diagnostics as a single synthetic system message
	text := "LSP diagnostics after edit:\n" + strings.Join(results, "\n")
	a.session.Add(provider.Message{
		Role:    provider.RoleSystem,
		Content: text,
	})
	a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
		Text: "diagnostics: " + fmt.Sprintf("%d file(s) with issues", len(results))})
}

func (a *AgentRunner) SetArchive(ar *archive.Store, sessionID string) {
	a.archive = ar
	a.sessionID = sessionID
}

// SetPreEditHook installs the pre-edit snapshot hook (see onPreEdit). The
// controller wires it to its per-session checkpoint store; nil disables capture.
func (a *AgentRunner) SetPreEditHook(fn func(diff.Change)) { a.onPreEdit = fn }

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
	return int(a.cacheBreakCount.Load())
}


func (a *AgentRunner) ContextWindow() int { return a.compaction.Window }

// CompactRatio returns the fraction of the window at which auto-compaction
// fires (e.g. 0.8). The status line uses it to show headroom to the next compact.
func (a *AgentRunner) CompactRatio() float64 { return a.compaction.Ratio }

// CompactNow runs one compaction pass immediately, regardless of the
// Options configures an AgentRunner.
type Options struct {
	MaxSteps    int
	Temperature float64
	Pricing     *provider.Pricing // optional, for per-turn cost display

	// Gate is the per-call permission gate. nil disables gating.
	Gate Gate

	// Hooks fires PreToolUse / PostToolUse shell hooks around tool calls. nil
	// disables hook firing.
	Hooks ToolHooks

	// Jobs is the session's background-job manager (nil disables background tools).
	Jobs *jobs.Manager

	// Context management. ContextWindow <= 0 disables compaction. CompactRatio
	// is the trigger fraction; RecentKeep is the minimum recent messages kept
	// verbatim (the tail is otherwise token-bounded). Both fall back to defaults.
	ContextWindow int
	CompactRatio  float64  // deprecated: use Compaction.Ratio
	RecentKeep    int      // deprecated: use Compaction.RecentKeep
	ArchiveDir    string   // deprecated: use Compaction.ArchiveDir
	// L2Dir is the directory for L2 ring persistence (.tianxuan/l2).
	// Deprecated: use Compaction.DetailDir (V3.0).
	L2Dir string
	// Compaction groups compaction settings (V3.0). When set, overrides the
	// deprecated individual fields above.
	Compaction CompactionConfig
	// Dispatcher is the centralized pre-execution check pipeline (V2.4).
	// nil means the agent uses inline checks (backward compatible).
	Dispatcher *ToolDispatcher
	// CtxMgr is the TCCA context kernel (V3.0). When set, the agent uses it
	// for prompt assembly and tool filtering instead of inline logic.
	CtxMgr *tiancontext.ContextManager
	// AuditFunc, when non-nil, is called after every tool execution with a
	// summary of the call. V3.2: foundational audit trail.
	AuditFunc func(tool string, taskKind string, readOnly bool, outcome string, errMsg string, outputLen int, durationMs int64)

	// ParamStorm enables parameter-level duplicate tool call detection (V5.13).
	// nil disables; non-nil provides WindowSize/Threshold/ExemptTools.
	ParamStorm *ParamStormOptions

	// AutoRoute enables heuristic model routing (V5.14).
	// When true, simple inputs route to flash, complex ones to pro.
	// Requires a flash provider to be set via SetFlashProvider().
	AutoRoute bool

	// BudgetLimit is the per-session cost budget in yuan (V5.15).
	// <=0 means unlimited. When set, the agent tracks cumulative cost
	// and warns at 80%% / blocks at 100%%.
	BudgetLimit float64

	// ModelProfile overrides compaction thresholds for specific models (V5.17).
	// nil means use defaults from CompactionConfig.
	ModelProfile *ModelProfile

	// TemplatePrefix is the sub-agent template prefix injected before the
	// user message in spawned agents. Same-class sub-agents share the same
	// template bytes �� DeepSeek prefix cache hits across sub-agent invocations.
	TemplatePrefix string
	// ActiveSchemas are the filtered tool schemas for sub-agents (V5.30).
	// When set, RunSubAgent uses these as the tools JSON field so the
	// prefix cache includes the same tools the parent sends.
	ActiveSchemas []provider.ToolSchema
	RuntimePrompt string
	// Goal is the session-level stopping condition (V6.0 P7). When non-empty,
	// the stop gate checks whether the model's final answer satisfies the goal.
	Goal string
}

// New constructs an AgentRunner. MaxSteps <= 0 means no cap �� the run loop
// continues until the model gives a final answer, the context is cancelled, or
// the provider errors (compaction keeps the context bounded). A nil sink is
// replaced with event.Discard so the agent can always emit unconditionally.
func New(prov provider.Provider, tools *tool.Registry, session *Session, opts Options, sink event.Sink) *AgentRunner {
	// Build CompactionConfig from individual fields (backward compat) or from opts.Compaction.
	comp := opts.Compaction
	if comp.Window == 0 {
		comp.Window = opts.ContextWindow
	}
	if comp.Ratio <= 0 {
		comp.Ratio = opts.CompactRatio
		if comp.Ratio <= 0 {
			comp.Ratio = defaultCompactRatio
		}
	}
	if comp.RecentKeep <= 0 {
		comp.RecentKeep = opts.RecentKeep
		if comp.RecentKeep <= 0 {
			comp.RecentKeep = minRecentKeep
		}
	}
	if comp.ArchiveDir == "" {
		comp.ArchiveDir = opts.ArchiveDir
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
		jobs:       opts.Jobs,
		evidence:   evidence.NewLedger(),
		compaction: comp,
		dispatcher: opts.Dispatcher,
		ctxMgr:     opts.CtxMgr,
		auditFunc:  opts.AuditFunc,
		tc:         newToolCache(-1), // V5.8: session �����棬mtime У�������
		goal:       opts.Goal,        // V6.0 P7: �ỰĿ��
	}
	// V5.13: �������籩��·��
	if opts.ParamStorm != nil {
		r.paramStorm = NewParamStormBreaker(*opts.ParamStorm)
	}
	r.routeHistory = NewRouteHistory()
	r.activeProv = prov // Ĭ��ʹ���� provider
	// V5.15: Ԥ���ſ�
	if opts.BudgetLimit > 0 {
		r.budgetGate = NewBudgetGate(opts.BudgetLimit)
	}
	// V5.17: Ӧ��ģ�����ø���ѹ����ֵ
	if opts.ModelProfile != nil {
		ApplyModelProfile(&r.compaction, opts.ModelProfile)
	}
	return r
}

// Run executes one turn with the single-model path (V5.0: Planner removed).
func (a *AgentRunner) Run(ctx context.Context, input string) error {
	return a.runDirect(ctx, input)
}

// runDirect is the original single-model execution path.
func (a *AgentRunner) runDirect(ctx context.Context, input string) error {
	// V3.3: generate trace ID for this turn
	traceID := NewTraceID()
	ctx = WithTraceID(ctx, traceID)

	if a.evidence != nil {
		a.evidence.Reset()
	}
	a.sink.Emit(event.Event{Kind: event.TurnStarted})
	a.session.Add(provider.Message{Role: provider.RoleUser, Content: input})

	// V8.0 P0-1: reset tool filter from previous turn (prefix must be immutable).
	a.activeSchemasMu.Lock()
	a.activeSchemas = nil
	a.activeSchemasMu.Unlock()



	// V7.5: �Ự��·���Զ�������һ�ξ���·�ɺ����
	if a.flashProv != nil {
		if !a.autoRouteLocked {
						// V7.4: history-aware routing (heuristic + learned)
			heuristic := AutoRouteProvider(input, a.prov, a.flashProv)
			useFlash := heuristic == a.flashProv
			if a.routeHistory != nil {
				useFlash = a.routeHistory.ShouldRouteToFlash(input, useFlash)
			}
			if useFlash && a.flashProv != nil {
				a.activeProv = a.flashProv
			} else {
				a.activeProv = a.prov
			}
			modelName := "pro"
			if a.activeProv == a.flashProv {
				modelName = "flash"
			}
			defer func() {
				if a.routeHistory != nil {
					a.routeHistory.Record(input, modelName, false)
				}
			}()
			a.autoRouteLocked = true
			a.autoRouteDecision = a.activeProv
		} else {
			a.activeProv = a.autoRouteDecision
		}
	} else {
		a.activeProv = a.prov
	}

	// V4.2: reset pre-execution cache and tool result cache for new turn
	a.preMu.Lock()
	a.preOutcomes = make(map[string]toolOutcome)
		a.dedupHashes = nil // V8.0 P0-2: reset dedup hashes each turn
		a.steerCount = 0 // V8.0 P0-3: reset steer counter each turn
	a.pendingDiffs = nil
	a.preMu.Unlock()
	// V5.13: ���ò������籩��·������ turn = ����ͼ��
	if a.paramStorm != nil {
		a.paramStorm.Reset()
	}
	// V5.8: ���� clear()�������� mtime У���Զ�ʧЧ������Ŀ

	// V6.0: �����ٻ����ѡ���������ʾģ�ͼ�����м���
			a.maybeRecallReminder()

			for step := 0; a.maxSteps <= 0 || step < a.maxSteps; step++ {
		text, reasoning, signature, calls, usage, err := a.stream(ctx, step+1)
		if err != nil {
			a.preWG.Wait() // drain any in-flight pre-execution goroutines before returning
			return err
		}

		// V6.0 P1: ������Ƚضϡ���finish_reason="length" ���޹��ߵ���ʱע�� nudge
		if a.maybeContinueOutputLength(usage, calls) {
			continue
		}
		// V6.0 P1: ��Ч������ԡ�����˼��/������غ�����
		if a.maybeRetryInvalidOutput(text, reasoning, calls) {
			continue
		}

		if usage != nil && usage.TotalTokens > 0 {
			a.sink.Emit(event.Event{Kind: event.Usage, Usage: usage, Pricing: a.pricing,
				SessionHit: int(a.sessCacheHit.Load()), SessionMiss: int(a.sessCacheMiss.Load())})
			// V5.15: Ԥ���ſء�������ۼƷ���
			if a.budgetGate != nil {
				status := a.budgetGate.Check(a.pricing, usage)
				if status == BudgetWarn {
					a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
						Text: a.budgetGate.StatusMessage()})
				}
				if status == BudgetBlock {
					a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
						Text: a.budgetGate.StatusMessage()})
					a.preWG.Wait()
					return fmt.Errorf("budget exceeded: %s", a.budgetGate.StatusMessage())
				}
			}
		}
		if msg, ok := finishReasonMessage(usage); ok {
			a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: msg})
		}

		// Keep reasoning_content on the assistant turn for display and session
		// archive. It is NOT re-uploaded to the API: the openai provider drops it
		// when building the request, since re-sent reasoning is billable prompt
		// input for no cache or coherence gain.
		a.session.Add(provider.Message{
			Role:               provider.RoleAssistant,
			Content:            text,
			ReasoningContent:   reasoning,
			ReasoningSignature: signature,
			ToolCalls:          calls,
		})

		// V7.0: archive the assistant turn for cross-session analysis
		if a.archive != nil {
			tcJSON := "[]"
			if len(calls) > 0 {
				if b, err := json.Marshal(calls); err == nil {
					tcJSON = string(b)
				}
			}
			a.archive.RecordMessage(a.sessionID, string(provider.RoleAssistant), text, tcJSON, step+1)
		}

		if len(calls) == 0 {
			// V7.0: ��բ��ֹͣ������ֹģ����ǰֹͣ
			// Gate 1: task gate �� ���δ�������
			if a.taskGate() {
				continue
			}
			// Gate 2: goal gate �� ���� judge ģ����֤Ŀ��
			if a.goalGate() {
				continue
			}
			// Gate 3: verify gate �� orchestrate ģʽ��֤
			if a.verifyGate() {
				continue
			}
			return nil // all gates passed
		}

		// V4.2: wait for stream() pre-execution goroutines to finish before
		// dispatching the full batch �� avoids races and double-execution.
		a.preWG.Wait()
		results := a.executeBatch(ctx, calls)
		// V8.0 P0-2: deterministic pruning — skip duplicate tool results.
		// Hash (toolName + args + result) to detect identical outcomes.
		for i, call := range calls {
			// Skip suppressed calls (already have placeholder result).
			if strings.HasPrefix(results[i], "suppressed:") {
				a.session.Add(provider.Message{
					Role:       provider.RoleTool,
					Content:    results[i],
					ToolCallID: call.ID,
					Name:       call.Name,
				})
				continue
			}
			// Compute dedup key: toolName | first 64 chars of args | first 64 chars of result
			dk := call.Name + "|" + truncateStr(call.Arguments, 64) + "|" + truncateStr(results[i], 64)
			if a.dedupHashes == nil {
				a.dedupHashes = make(map[string]bool)
			}
			if a.dedupHashes[dk] {
				// Same tool + args + result already seen — skip full result.
				a.session.Add(provider.Message{
					Role:       provider.RoleTool,
					Content:    "[cached — same as previous " + call.Name + " call]",
					ToolCallID: call.ID,
					Name:       call.Name,
				})
			} else {
				a.dedupHashes[dk] = true
				a.session.Add(provider.Message{
					Role:       provider.RoleTool,
					Content:    results[i],
					ToolCallID: call.ID,
					Name:       call.Name,
				})
			}
		}

		// V8.0 P0-3: mid-turn steer — detect error patterns and inject corrective hints.
		if a.shouldMidTurnSteer(calls, results) {
			continue // steer injected, skip compaction and continue loop
		}


		// V6.0 P2: �ظ������⡪������ 3 ����ͬ���ߵ���ʱע�� nudge
		if a.detectRepeatedSteps(calls) {
			continue // nudge injected, skip compaction and continue loop
		}

			// V7.1: no mid-turn compaction �� cache grows monotonically within each turn
	}
	// Only reached when a positive maxSteps guard is configured. The work so far
	// is already in the session, so the user can just send another message to pick
	// up where it left off.
	return fmt.Errorf("paused after %d tool-call rounds (agent.max_steps) �� the work so far is saved; send another message to continue, or set max_steps higher or to 0 for no limit", a.maxSteps)
}

// SetDispatcherPlanMode wires the planMode atomic into the dispatcher so
// plan-mode gating is consistent. Call after construction.


// filteredSchemas returns a reduced tool schema list for analysis-only
// inputs. When the input suggests code review/reading/explaining (no write
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

	// Count tool failures (results starting with "error:", "blocked:", or tool panic).
	failed := 0
	for _, r := range results {
		if strings.HasPrefix(r, "error:") || strings.HasPrefix(r, "blocked:") ||
			strings.HasPrefix(r, "tool panic:") || strings.HasPrefix(r, "suppressed:") {
			failed++
		}
	}

	// All calls failed → likely wrong approach or tool misuse.
	if failed == len(calls) && failed >= 2 {
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

func (a *AgentRunner) SetDispatcherPlanMode() {
	if a.dispatcher != nil {
		a.dispatcher.planMode = &a.planMode
	}
}

// SetCtxMgr wires the TCCA context kernel (V3.0 Phase 5).
func (a *AgentRunner) SetCtxMgr(m *tiancontext.ContextManager) {
	a.ctxMgr = m
	if a.dispatcher != nil {
		a.dispatcher.SetObserver(m)
	}
}

// StormBreaker tracks repeated failures to detect death spirals (V3.0 Phase 4).
type StormBreaker struct {
	Sig   string // per-turn fixation signature
	Count int    // consecutive identical failures
}

// extractFilePath extracts a file path from tool call arguments for edit tools.
// Returns "" if no path can be extracted.

// truncateStr returns s truncated to maxLen chars. Used for dedup key building.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
func extractFilePath(name string, args string) string {
	// Common keys for file paths in tool arguments.
	keys := []string{`"path"`, `"file_path"`, `"source"`, `"destination"`}
	lower := strings.ToLower(args)
	for _, key := range keys {
		idx := strings.Index(lower, key)
		if idx < 0 {
			continue
		}
		// Find the value after the key:  "path": "value"
		rest := args[idx+len(key):]
		colon := strings.Index(rest, ":")
		if colon < 0 {
			continue
		}
		val := strings.TrimSpace(rest[colon+1:])
		// Strip quotes
		val = strings.Trim(val, `"`)
		// Take until comma or closing brace
		if end := strings.IndexAny(val, `,}`); end >= 0 {
			val = val[:end]
		}
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"`)
		if val != "" {
			return val
		}
	}
	return ""
}

// Agent is a backward-compatible alias for AgentRunner.
type Agent = AgentRunner

// stream runs one completion, emitting reasoning and text deltas as typed
// events and collecting complete tool calls. A Message event closes the text
// stream so a sink can re-render the streamed raw text as styled markdown. The
// accumulated text and reasoning are also returned so the caller can round-trip
// reasoning on the next turn.
func (a *AgentRunner) stream(ctx context.Context, turn int) (string, string, string, []provider.ToolCall, *provider.Usage, error) {
	// Build tools and messages. L4 conversation messages always come from
	// Session �� NOT from ctxMgr.AssemblePrompt() which reads FlowLayer.
	// FlowLayer is only updated during compact, so reading it on every turn
	// would send stale/empty messages to the model (information pollution).
	tools := a.tools.Schemas()
	a.activeSchemasMu.RLock()
	if a.activeSchemas != nil {
		tools = a.activeSchemas
	}
	a.activeSchemasMu.RUnlock()
	msgs := a.session.Messages

	// V5.10: ImmutablePrefix 校验——验证前缀指纹，确保缓存稳定。
	a.verifyPrefix(msgs, tools)

	ch, err := a.activeProv.Stream(ctx, provider.Request{
		Messages:    msgs,
		Tools:       tools,
		Temperature: a.temperature,
	})
	if err != nil {
		return "", "", "", nil, nil, err
	}

	// A PostLLMCall hook rewrites the whole reasoning block, so when one is wired
	// up we buffer reasoning silently and emit the transformed text once after the
	// stream. With no such hook the reasoning streams live, chunk by chunk, as
	// before �� the common case must not lose its live "thinking��" display.
	transformReasoning := a.hooks != nil && a.hooks.HasPostLLMCall()

	var text, reasoning strings.Builder
	var signature string // provider-issued proof for the reasoning (Anthropic thinking)
	var calls []provider.ToolCall
	var usage *provider.Usage
	for chunk := range ch {
		switch chunk.Type {
		case provider.ChunkReasoning:
			reasoning.WriteString(chunk.Text)
			if chunk.Signature != "" {
				signature = chunk.Signature
			}
			if chunk.Text != "" && !transformReasoning {
				a.sink.Emit(event.Event{Kind: event.Reasoning, Text: chunk.Text})
			}
		case provider.ChunkText:
			text.WriteString(chunk.Text)
			a.sink.Emit(event.Event{Kind: event.Text, Text: chunk.Text})
		case provider.ChunkToolCallStart:
			// Surface the tool card as soon as the call begins �� before its
			// (possibly large) arguments finish streaming �� so the user sees it
			// working instead of a stall. executeBatch emits the full dispatch
			// (with args) once the call completes; the frontend merges by ID.
			if tc := chunk.ToolCall; tc != nil {
				a.sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{
					ID: tc.ID, Name: tc.Name, ReadOnly: a.toolReadOnly(tc.Name), Partial: true,
				}})
			}
		case provider.ChunkToolCall:
			calls = append(calls, *chunk.ToolCall)
			// V4.2: pre-execute read-only tools as each call completes streaming,
			// overlapping tool execution with the model's remaining output.
			// Skip complete_step/todo_write (ordering-dependent on prior calls)
			// and calls with empty IDs (lookup key collision).
			if tc := chunk.ToolCall; tc != nil && tc.ID != "" && a.toolReadOnly(tc.Name) &&
				tc.Name != "complete_step" && tc.Name != "todo_write" {
				a.preWG.Add(1)
				go func(call provider.ToolCall) {
					defer a.preWG.Done()
					o := a.executeOne(ctx, call)
					a.preMu.Lock()
					a.preOutcomes[call.ID] = o
					a.preMu.Unlock()
				}(*tc)
			}
		case provider.ChunkUsage:
			usage = chunk.Usage
			a.lastUsage.Store(chunk.Usage)
			a.sessCacheHit.Add(int64(chunk.Usage.CacheHitTokens)); chunk.Usage.SessionCacheHitTokens = int(a.sessCacheHit.Load())
			a.sessCacheMiss.Add(int64(chunk.Usage.CacheMissTokens)); chunk.Usage.SessionCacheMissTokens = int(a.sessCacheMiss.Load())
			// V5.9: ��⻺�����
			if reason := a.cacheBreakDetector.check(chunk.Usage); reason != "" {
				a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: reason})
			}
		case provider.ChunkError:
			return "", "", "", nil, nil, chunk.Err
		}
	}
	// With a PostLLMCall hook, the live stream was suppressed above; transform the
	// full reasoning now and emit it once so the sink never sees the untranslated
	// text. Without a hook this is skipped �� the chunk-by-chunk events already fired.
	original := reasoning.String()
	display := original
	if transformReasoning && original != "" {
		display = a.hooks.PostLLMCall(ctx, original, turn)
		if display != "" {
			a.sink.Emit(event.Event{Kind: event.Reasoning, Text: display})
		}
	}
	// Store the transformed reasoning �� except when a provider signature pins it to
	// the original text (Anthropic extended thinking). That signed thinking block is
	// replayed verbatim on the next tool-call turn; re-uploading transformed text
	// under the original signature is rejected, so keep the original for storage
	// while the user still sees the transformed version live.
	stored := display
	if signature != "" {
		stored = original
	}
	// Close the text stream: a sink may re-render the streamed raw text as
	// styled markdown now that it is complete. Reasoning rides along so the sink
	// has the full chain if it wants it.
	if text.Len() > 0 || display != "" {
		a.sink.Emit(event.Event{Kind: event.Message, Text: text.String(), Reasoning: display})
	}
	return text.String(), stored, signature, calls, usage, nil
}

// repairArguments applies Tool-Call-Repair fixes (flatten wrappers, scavenge
// JSON strings, truncate oversized strings) to a tool call's arguments in-place.
// Called once per call in executeBatch; executeOne no longer repeats it.
func repairArguments(args *string, readOnly bool) {
	if *args == "" {
		return
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(*args), &raw); err != nil {
		return
	}
	opts := ToolArgumentRepairOptions{PreserveLongStrings: !readOnly}
	repaired := RepairDispatchToolArguments(raw, opts)
	if len(repaired.Notes) > 0 {
		if fixed, err := json.Marshal(repaired.Arguments); err == nil {
			*args = string(fixed)
		}
	}
}

// executeBatch runs a set of tool calls, parallelising read-only calls (all
// writers serialised). Repair runs first (once per call), then paramStorm
// detection, then execution. executeOne no longer repeats repair.
// ToolResult events are emitted after the batch in call order.
func (a *AgentRunner) executeBatch(ctx context.Context, calls []provider.ToolCall) []string {
	// V7.7.1: repair always runs first — executeOne no longer repeats it.
	for i := range calls {
		t, ok := a.tools.Get(calls[i].Name)
		repairArguments(&calls[i].Arguments, ok && !t.ReadOnly())
	}

	// V5.13: param storm breaker — after repair, inspect for duplicate patterns.
	suppressed := make([]bool, len(calls))
	if a.paramStorm != nil {
		for i := range calls {
			c := &calls[i]
			t, ok := a.tools.Get(c.Name)
			readOnly := ok && t.ReadOnly()
			res := a.paramStorm.Inspect(c.Name, c.Arguments, readOnly)
			if res.Suppress {
				suppressed[i] = true
				a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
					Text: "param storm breaker: " + res.Reason})
			}
		}
	}

	// V7.7.1: repair already done above; executeOne skips it.

	for i, c := range calls {
		if suppressed[i] {
			continue // ������ ToolDispatch �¼�
		}
		t, ok := a.tools.Get(c.Name)
		a.sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{
			ID:       c.ID,
			Name:     c.Name,
			Args:     c.Arguments,
			ReadOnly: ok && t.ReadOnly(),
		}})
	}

	results := make([]string, len(calls))
	outcomes := make([]toolOutcome, len(calls))
	// V5.13 fix: Ԥ����䱻���Ƶ��õĽ��
	for i := range calls {
		if suppressed[i] {
			results[i] = "suppressed: duplicate tool call (param storm breaker)"
			outcomes[i] = toolOutcome{output: results[i], errMsg: "suppressed by param storm breaker"}
		}
	}
	run := func(i int) {
		if suppressed[i] {
			return
		}
		// V4.2: skip calls already pre-executed during stream()
		a.preMu.Lock()
		pre, hasPre := a.preOutcomes[calls[i].ID]
		a.preMu.Unlock()
		if hasPre {
			outcomes[i] = pre
			results[i] = pre.output
			return
		}
		defer func() {
			if r := recover(); r != nil {
				results[i] = fmt.Sprintf("tool panic: %v", r)
				outcomes[i] = toolOutcome{output: results[i], errMsg: fmt.Sprintf("panic: %v", r)}
			}
		}()
		outcomes[i] = a.executeOne(ctx, calls[i])
		results[i] = outcomes[i].output

		// V7.4: learn from tool errors across sessions
		if a.patternExtractor != nil {
			if p := a.patternExtractor.Extract(calls[i].Name, results[i]); p != nil {
				a.patternExtractor.SaveStore()
			}
		}
	}

	for _, batch := range partitionToolCalls(a.tools, calls) {
		if batch.parallel && batch.end-batch.start > 1 {
			runParallel(batch.start, batch.end, run)
			continue
		}
		for i := batch.start; i < batch.end; i++ {
			run(i)
		}
	}

	for i, c := range calls {
		o := outcomes[i]
		t, ok := a.tools.Get(c.Name)
		a.sink.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{
			ID:        c.ID,
			Name:      c.Name,
			Args:      c.Arguments,
			Output:    o.output,
			Err:       o.errMsg,
			ReadOnly:  ok && t.ReadOnly(),
			Truncated: o.truncated,
		}})
		if o.truncated && o.truncMsg != "" {
			a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Text: o.truncMsg})
		}
	}
	a.applyStormBreaker(calls, outcomes, results)
	return results
}

type toolCallBatch struct {
	start    int
	end      int
	parallel bool
}

// partitionToolCalls keeps provider order while letting contiguous known
// read-only tools run together. Unknown and writer tools are single-call serial
// batches so they cannot reorder around reads or produce surprising errors.
// complete_step and todo_write are read-only but never join a parallel run: they
// read the turn's evidence ledger, so every prior call's receipt must be recorded
// before they run.
// V6.0: partitionToolCalls groups non-conflicting tools into parallel batches.
// Read-only tools (not complete_step/todo_write) form contiguous parallel batches.
// Writer tools targeting DIFFERENT files are also grouped into parallel batches.
// task/complete_step/todo_write/bash are always serial (each its own batch).
func partitionToolCalls(r *tool.Registry, calls []provider.ToolCall) []toolCallBatch {
	var batches []toolCallBatch
	for i := 0; i < len(calls); {
		// Read-only tools: contiguous parallel batches (existing behavior)
		if parallelisable(r, calls[i].Name) {
			start := i
			i++
			for i < len(calls) && parallelisable(r, calls[i].Name) {
				i++
			}
			batches = append(batches, toolCallBatch{start: start, end: i, parallel: true})
			continue
		}
		key := getConflictKey(calls[i])
		if key == "" || key[0] == '!' {
			// Global conflict key or empty = always serial
			batches = append(batches, toolCallBatch{start: i, end: i + 1})
			i++
			continue
		}
		// Non-conflicting writers: group consecutive calls with DIFFERENT file paths
		used := map[string]bool{key: true}
		start := i
		i++
		for i < len(calls) && !parallelisable(r, calls[i].Name) {
			k := getConflictKey(calls[i])
			if k == "" || k[0] == '!' || used[k] {
				break
			}
			used[k] = true
			i++
		}
		batches = append(batches, toolCallBatch{start: start, end: i, parallel: i-start > 1})
	}
	return batches
}

func parallelisable(r *tool.Registry, name string) bool {
	if name == "complete_step" || name == "todo_write" {
		return false
	}
	t, ok := r.Get(name)
	return ok && t.ReadOnly()
}

// V6.0: getConflictKey returns a conflict key for a tool call.
// Two calls with the same key cannot run in parallel.
// Prefix ! marks global conflict keys (always serial).
func getConflictKey(call provider.ToolCall) string {
	switch call.Name {
	case "task", "explore", "research", "review", "security_review",
		"run_skill", "install_skill":
		return "!spawn"
	case "complete_step", "todo_write":
		return "!ledger"
	case "edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol":
		path := extractFilePath(call.Name, call.Arguments)
		if path != "" {
			return "file:" + path
		}
		return "!write"
	case "bash", "bash_output":
		return "!bash"
	case "read_file":
		path := extractFilePath(call.Name, call.Arguments)
		if path != "" {
			return "read:" + path
		}
		return ""
	default:
		return ""
	}
}

// V6.0: extractArgsPath extracts the target file path from tool call arguments.

func runParallel(start, end int, run func(int)) {
	const maxParallel = 8
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for i := start; i < end; i++ {
		i := i
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			run(i)
		}()
	}
	wg.Wait()
}

// stormBreakThreshold is how many times in a row the same tool may fail the same
// way before the loop stops echoing the raw error back and instead returns a
// directive to change approach. Two natural self-corrections are healthy; the
// third identical failure is a death-spiral �� the dominant case being a tool call
// whose arguments are truncated at the output-token ceiling, which the model then
// re-emits (re-worded but still over-long), truncating the same way again.
const stormBreakThreshold = 3

// applyStormBreaker detects a run of identically-failing turns and, past the
// threshold, rewrites the model-facing result (results[0]) into a directive to
// change approach. It keys on each call's (tool, error) �� not its args �� because a
// stuck model reworks the arguments cosmetically while failing identically (see
// the stormSig field doc). A turn is a fixation candidate only when every one of
// its calls errored and none was merely blocked by plan mode / permissions (those
// carry a clear, distinct message the model can already act on). Any success, any
// block, or a different batch shape is varied work, so it resets the counter. This
// covers both the single-call spiral and a repeated multi-call batch. The hard
// maxSteps guard remains the ultimate backstop; this just keeps the loop from
// burning that whole budget bouncing off the same failure.
func (a *AgentRunner) applyStormBreaker(calls []provider.ToolCall, outcomes []toolOutcome, results []string) {
	sig, ok := batchStormSignature(calls, outcomes)
	if !ok {
		a.storm.Sig, a.storm.Count = "", 0
		return
	}
	if sig != a.storm.Sig {
		a.storm.Sig, a.storm.Count = sig, 1
		return
	}
	a.storm.Count++
	if a.storm.Count < stormBreakThreshold {
		return
	}

	// Phase 4: analyze the failure pattern and inject corrective context
	// as a user-role message (turn tail) �� never touching the cache-stable prefix.
	det := NewDetector()
	if report := det.Analyze(a.storm.Sig, outcomes); report != nil {
		hintMsg := "[System note: " + report.Hint + "]"
		a.session.Add(provider.Message{Role: provider.RoleUser, Content: hintMsg})
	}

	subject := fmt.Sprintf("%q", calls[0].Name)
	short := calls[0].Name
	if len(calls) > 1 {
		subject = fmt.Sprintf("this batch of %d tool calls", len(calls))
		short = fmt.Sprintf("a batch of %d calls", len(calls))
	}
	guardMsg := fmt.Sprintf(
		"\n\n[loop guard] %s has now failed %d times in a row with the same error. Re-sending it �� even with the wording changed �� will not help: the calls keep failing the same way. Change approach: if an argument is being truncated, write less in one call and split the work into several smaller calls; otherwise fix the arguments, use a different tool, or explain the blocker in your final answer.",
		subject, a.storm.Count)
	for i := range results {
		results[i] = outcomes[i].output + guardMsg
	}
	a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: fmt.Sprintf(
		"loop guard: %s failed %d�� the same way �� nudging the model to change approach",
		short, a.storm.Count)})
}

// batchStormSignature returns a per-turn fixation signature �� each call's
// (name, error) in order �� and ok=true only when every call errored and none was
// merely blocked. ok=false (any success or block) means the turn made varied
// progress, so the caller resets the counter. Keying on the error rather than the
// args is deliberate: a stuck model reworks the arguments while failing the same
// way, so identical-args matching would miss the loop.
func batchStormSignature(calls []provider.ToolCall, outcomes []toolOutcome) (string, bool) {
	if len(calls) == 0 {
		return "", false
	}
	var sb strings.Builder
	for i := range calls {
		if outcomes[i].errMsg == "" || outcomes[i].blocked {
			return "", false
		}
		sb.WriteString(calls[i].Name)
		sb.WriteByte(0)
		sb.WriteString(outcomes[i].errMsg)
		sb.WriteByte(0)
	}
	return sb.String(), true
}

// toolOutcome is one tool call's result, split into the model-facing output and
// the display-facing notice bits. errMsg is the short failure reason (empty on
// success) �� a refused call, an unknown tool, or an execution error �� so a sink
// renders the result as failed ("? name <errMsg>" / a red card) instead of OK;
// blocked narrows that to a refusal (plan mode / permission). truncMsg is set
// (without the "�� " prefix) when the output was head+tailed.
type toolOutcome struct {
	output    string
	blocked   bool
	errMsg    string
	truncated bool
	truncMsg  string
}

// executeOne runs a single tool call. It is pure with respect to the event sink
// �� the caller emits ToolDispatch/ToolResult �� so it is safe to invoke from
// parallel goroutines.
func (a *AgentRunner) executeOne(ctx context.Context, call provider.ToolCall) toolOutcome {
	t, ok := a.tools.Get(call.Name)
	if !ok {
		return toolOutcome{
			output: fmt.Sprintf("error: unknown tool %q", call.Name),
			errMsg: fmt.Sprintf("unknown tool %q", call.Name),
		}
	}


	// V2.4: centralized pre-execution checks via dispatcher (or inline fallback)
	if a.dispatcher != nil {
		cr := a.dispatcher.Check(ctx, call.Name, json.RawMessage(call.Arguments), t.ReadOnly())
		if !cr.Allowed {
			return toolOutcome{
				output:  cr.Reason,
				blocked: cr.Blocked,
				errMsg:  cr.Reason,
			}
		}
	} else {
		// Inline fallback �� same as original, for backward compatibility.
		// V8.0 P2-12: PermissionRequest hooks run before plan mode gate.
		if a.hooks != nil {
			allow, modifiedArgs, reason := a.hooks.PermissionRequest(ctx, call.Name, json.RawMessage(call.Arguments))
			if !allow {
				return toolOutcome{
					output:  "blocked by PermissionRequest hook: " + reason,
					blocked: true,
					errMsg:  "blocked by PermissionRequest hook",
				}
			}
			if len(modifiedArgs) > 0 {
				call.Arguments = string(modifiedArgs)
			}
		}
		// V8.0.3: plan-mode bash safety — allow safe commands through.
		if a.planMode.Load() && call.Name == "bash" {
			reason := planBashCheck(json.RawMessage(call.Arguments))
			if reason == "" {
				goto planBashAllowed
			}
			return toolOutcome{
				output:  "blocked: " + reason,
				blocked: true,
				errMsg:  "blocked: unsafe bash in plan mode",
			}
		}
		if a.planMode.Load() && !t.ReadOnly() {
			return toolOutcome{
				output:  fmt.Sprintf("blocked: %q is a writer tool and plan mode is read-only. Keep exploring with read-only tools, then write your plan as your reply. The user will be asked to approve it before any changes are made.", call.Name),
				blocked: true,
				errMsg:  "blocked: plan mode is read-only",
			}
		}
	planBashAllowed:
		if a.gate != nil {
			allow, reason, err := a.gate.Check(ctx, call.Name, json.RawMessage(call.Arguments), t.ReadOnly())
			if err != nil {
				return toolOutcome{
					output:  fmt.Sprintf("blocked: %s (%v)", reason, err),
					blocked: true,
					errMsg:  fmt.Sprintf("blocked: %v", err),
				}
			}
			if !allow {
				return toolOutcome{
					output:  "blocked: " + reason,
					blocked: true,
					errMsg:  "blocked by permission policy",
				}
			}
		}
		// PreToolUse hooks run after permission is granted but before the call.
		if a.hooks != nil {
			if block, msg := a.hooks.PreToolUse(ctx, call.Name, json.RawMessage(call.Arguments)); block {
				if msg == "" {
					msg = "blocked by a PreToolUse hook"
				}
				return toolOutcome{
					output:  "blocked: " + msg,
					blocked: true,
					errMsg:  "blocked by PreToolUse hook",
				}
			}
		}
	}
	// Checkpoint the file this writer is about to change, so the turn can be
	// rewound. Fires after all gating (the edit is cleared to run) and only for
	// tools that can describe their change; a Preview error means the edit will
	// likely fail anyway, so we skip rather than snapshot a stale state.
	if a.onPreEdit != nil && !t.ReadOnly() {
		if pv, ok := t.(tool.Previewer); ok {
			if change, perr := pv.Preview(json.RawMessage(call.Arguments)); perr == nil {
				a.onPreEdit(change)
				a.pendingDiffs = append(a.pendingDiffs, change)
			}
		}
	}
	// V4.2: tool result cache �� avoid redundant disk IO for repeat file reads
	if call.Name == "read_file" && a.tc != nil {
		var ra struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
		}
		if err := json.Unmarshal(json.RawMessage(call.Arguments), &ra); err == nil && ra.Path != "" {
			if cached, ok := a.tc.get(ra.Path, ra.Offset); ok {
				return toolOutcome{output: cached}
			}
		}
	}

	cctx := withCallContext(ctx, call.ID, a.sink, a.asker)
	if a.evidence != nil {
		cctx = evidence.WithLedger(cctx, a.evidence)
	}
	if a.jobs != nil {
		cctx = jobs.WithManager(cctx, a.jobs)
	}
	if a.memQueue != nil {
		cctx = memory.WithQueue(cctx, a.memQueue)
	}
	start := time.Now()
	result, err := t.Execute(cctx, json.RawMessage(call.Arguments))
	duration := time.Since(start).Milliseconds()

	// V4.2: cache successful file reads; invalidate writes
	if a.tc != nil {
		switch call.Name {
		case "read_file":
			if err == nil {
				var ra struct {
					Path   string `json:"path"`
					Offset int    `json:"offset"`
				}
				if json.Unmarshal(json.RawMessage(call.Arguments), &ra) == nil && ra.Path != "" {
					a.tc.set(ra.Path, ra.Offset, result)
				}
			}
		case "edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol":
			var wa struct{ Path string `json:"path"` }
			if json.Unmarshal(json.RawMessage(call.Arguments), &wa) == nil && wa.Path != "" {
				a.tc.invalidatePath(wa.Path)
			}
		}
	}

	// V3.2: audit trail �� log every tool execution
	if a.auditFunc != nil {
		outcome := "success"
		errMsg := ""
		if err != nil {
			outcome = "error"
			errMsg = err.Error()
		}
		a.auditFunc(call.Name, "", t.ReadOnly(), outcome, errMsg, len(result), duration)
	}

	// V3.0: notify workspace observer of successful edits.
	if err == nil && !t.ReadOnly() && a.dispatcher != nil {
		if path := extractFilePath(call.Name, call.Arguments); path != "" {
			a.dispatcher.NotifyEdit(path)
		}
	}

	if a.evidence != nil {
		if call.Name == "complete_step" {
			if err == nil {
				a.evidence.Record(evidence.ReceiptFromToolCall(call.Name, json.RawMessage(call.Arguments), true, t.ReadOnly()))
			}
		} else {
			a.evidence.Record(evidence.ReceiptFromToolCall(call.Name, json.RawMessage(call.Arguments), err == nil, t.ReadOnly()))
		}
	}
	// PostToolUse hooks observe the result (they can't block); fired whether the
	// call succeeded or errored, since the tool did run.
	if a.hooks != nil {
		a.hooks.PostToolUse(ctx, call.Name, json.RawMessage(call.Arguments), result)
	}
	if err != nil {
		body, truncMsg := truncateToolOutput(fmt.Sprintf("error: %v\n%s", err, result))
		return toolOutcome{output: body, errMsg: firstLine(err.Error()), truncated: truncMsg != "", truncMsg: truncMsg}
	}
	// A foreground `task` sub-agent just finished �� its result is the final answer.
	// (A backgrounded one returns a "Started��" string and stops later in a job, so
	// it doesn't fire here.) SubagentStop lets a hook react to delegated work.
	if a.hooks != nil && call.Name == "task" && !isBackgroundTaskCall(call.Arguments) {
		a.hooks.SubagentStop(ctx, result)
	}
	// V5.8: ����ѹ�����߽�����ٽضϣ����ٽ���ģ�͵����� token
	result = SmartCompress(call.Name, result)
	body, truncMsg := truncateToolOutput(result)
	return toolOutcome{output: body, truncated: truncMsg != "", truncMsg: truncMsg}
}

// isBackgroundTaskCall reports whether a `task` call set run_in_background, so a
// fire-and-return dispatch isn't mistaken for a sub-agent that has stopped.
func isBackgroundTaskCall(args string) bool {
	var p struct {
		RunInBackground bool `json:"run_in_background"`
	}
	_ = json.Unmarshal([]byte(args), &p)
	return p.RunInBackground
}

// toolReadOnly reports a tool's ReadOnly classification by name (false for an
// unknown tool), for stamping early ToolDispatch events.
func (a *AgentRunner) toolReadOnly(name string) bool {
	t, ok := a.tools.Get(name)
	return ok && t.ReadOnly()
}

// firstLine returns s up to its first newline �� a one-line failure summary for
// the display Err, while the full error stays in the model-facing output.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// truncateToolOutput V5.10: ����Ϊ Kun ������άѹ����������+�ֽ���+token���㡣
// �����ź��У�error/fatal/panic�ȣ������� base64��ѹ���ظ��У��۵��հ��С�
// ��ȫȷ���ԣ���Ӱ�컺��ǰ׺��
func truncateToolOutput(s string) (string, string) {
	return truncateToolOutputWith(s, maxToolResultLines, maxToolOutputBytes)
}

const (
	maxToolResultLines = 320 // ���������
)

// truncateToolOutputWith �������汾�����ڲ��ԡ�
func truncateToolOutputWith(s string, maxLines, maxBytes int) (string, string) {
	// Ԥ�����ȥ�� ANSI ת���롢ͳһ���С�ȥβ���ո�
	s = normalizeText(s)

	lines := strings.Split(s, "\n")
	originalLines := len(lines)
	originalBytes := len(s)

	// δ���� �� ԭ������
	if originalBytes <= maxBytes && originalLines <= maxLines {
		return s, ""
	}

	// ѡ��Ҫ������У��ź��� + ��β
	selected := selectHygieneLines(lines, maxLines)

	// ��Ԥ��ض�
	var out strings.Builder
	used := 0
	kept := 0
	for _, line := range selected {
		lineBytes := len(line) + 1 // +1 for \n
		if used+lineBytes > maxBytes {
			if kept > 0 {
				break
			}
			// ���г��ޣ��ضϵ�Ԥ���ڣ�rune ��ȫ��
			if maxBytes < len(line) {
				line = snapToRuneBoundary(line, 0, maxBytes)
			}
		}
		if kept > 0 {
			out.WriteString("\n")
			used++
		}
		out.WriteString(line)
		used += len(line)
		kept++
	}

	omitted := originalBytes - used
	omittedLines := originalLines - kept
	notice := fmt.Sprintf("tool output truncated: %d of %d bytes, %d of %d lines elided",
		omitted, originalBytes, omittedLines, originalLines)

	out.WriteString(fmt.Sprintf(
		"\n\n[cache hygiene: omitted %d byte(s), %d line(s); use narrower read/grep/bash ranges for details]",
		omitted, omittedLines))
	return out.String(), notice
}

var signalKeywords = []string{
	"error", "Error", "ERROR",
	"fatal", "Fatal", "FATAL",
	"panic", "PANIC",
	"exception", "Exception",
	"failed", "Failed",
	"timeout", "Timeout",
	"denied", "Denied",
	"cannot", "invalid",
}

// normalizeText Ԥ�����ı���ȥ ANSI ת���롢ͳһ���С�ȥβ���ո�
func normalizeText(s string) string {
	// ȥ�� ANSI ת���� (ESC[...m)
	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' && s[j] != 'K' && s[j] != 'h' && s[j] != 'l' {
				if (s[j] >= '0' && s[j] <= '9') || s[j] == ';' || s[j] == '?' {
					j++
				} else {
					break
				}
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		out.WriteByte(s[i])
		i++
	}

	// ͳһ���С�ȥβ���ո�ѹ���������к��ظ���
	lines := strings.Split(strings.ReplaceAll(out.String(), "\r\n", "\n"), "\n")
	var result []string
	blankRun := 0
	prev := ""
	repeatCount := 0

	flushRepeat := func() {
		if repeatCount > 1 {
			result = append(result, fmt.Sprintf("[previous line repeated %d time(s)]", repeatCount-1))
		}
		repeatCount = 0
	}

	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			flushRepeat()
			blankRun++
			if blankRun <= 2 {
				result = append(result, "")
			}
			prev = ""
			continue
		}
		blankRun = 0
		if line == prev {
			repeatCount++
			continue
		}
		flushRepeat()
		result = append(result, line)
		prev = line
		repeatCount = 1
	}
	flushRepeat()
	return strings.Join(result, "\n")
}

// hasSignalKeyword ������Ƿ�����źŹؼ��ʡ�
func hasSignalKeyword(line string) bool {
	lower := strings.ToLower(line)
	for _, kw := range signalKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// selectHygieneLines ѡ��Ҫ������У��� + β + �ź��С�
func selectHygieneLines(lines []string, maxLines int) []string {
	if len(lines) <= maxLines {
		return lines
	}

	indexes := make(map[int]bool)
	headCount := min(80, maxLines/4)
	tailCount := min(120, maxLines/3)
	if headCount < 1 {
		headCount = 1
	}
	if tailCount < 1 {
		tailCount = 1
	}

	// �ײ�
	for i := 0; i < headCount && i < len(lines); i++ {
		indexes[i] = true
	}
	// β��
	for i := len(lines) - tailCount; i < len(lines); i++ {
		if i >= 0 {
			indexes[i] = true
		}
	}
	// �ź��У���� 48 ����
	signalCount := 0
	for i := 0; i < len(lines) && len(indexes) < maxLines; i++ {
		if hasSignalKeyword(lines[i]) && !indexes[i] {
			indexes[i] = true
			signalCount++
			if signalCount >= 48 {
				break
			}
		}
	}

	// ���к��������
	result := make([]string, 0, len(indexes))
	for i := 0; i < len(lines); i++ {
		if indexes[i] {
			result = append(result, lines[i])
		}
		if len(result) >= maxLines {
			break
		}
	}
	return result
}

// snapToRuneBoundary returns s[lo:hi] with the bounds nudged outward until
// both land on rune-start positions.
func snapToRuneBoundary(s string, lo, hi int) string {
	for lo > 0 && !utf8.RuneStart(s[lo]) {
		lo--
	}
	for hi < len(s) && !utf8.RuneStart(s[hi]) {
		hi++
	}
	return s[lo:hi]
}

// ������ V5.9: ������Ѽ�� ����������������������������������������������������������������������������������������

// ������ V5.10: ImmutablePrefix У�� ��������������������������������������������������������������������

// verifyPrefix ������ stream() ʱ���� L1+L2+tools �� SHA256 ָ�ƣ�
// ����ÿ��У��ָ���Ƿ�ƥ�䡣Ư�� �� panic�����⾲Ĭ�ƻ� DeepSeek ǰ׺���档
// ���� Kun immutable-prefix.ts �� Go ��ֲ��
func (a *AgentRunner) verifyPrefix(msgs []provider.Message, tools []provider.ToolSchema) {
	// ��ȡ L1 �� L2 ����
	var l1, l2 string
	if len(msgs) > 0 && msgs[0].Role == provider.RoleSystem {
		l1 = msgs[0].Content
	}
	if len(msgs) > 1 && msgs[1].Role == provider.RoleSystem {
		l2 = msgs[1].Content
	}

	// �淶�������б������������
	sorted := make([]provider.ToolSchema, len(tools))
	copy(sorted, tools)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	// ����ָ��
	h := sha256.New()
	h.Write([]byte(l1))
	h.Write([]byte(l2))
	h.Write([]byte{0}) // �ָ��
	for _, t := range sorted {
		h.Write([]byte(t.Name))
		h.Write([]byte(t.Description))
		h.Write(t.Parameters)
		h.Write([]byte{0})
	}
	fp := hex.EncodeToString(h.Sum(nil))[:16]

	if !a.prefixFingerprintSet {
		a.prefixFingerprint = fp
		a.prefixFingerprintSet = true
		return
	}
	if a.prefixFingerprint != fp {
		panic(fmt.Sprintf(
			"ImmutablePrefix DRIFT: L1/L2/tools fingerprint changed mid-session!\n"+
				"  expected: %s\n  actual:   %s\n"+
				"This will silently break DeepSeek prefix cache on every turn.\n"+
				"Check: is any code path mutating the system prompt, runtime prompt, or tool schemas after Lock()?",
			a.prefixFingerprint, fp))
	}
}

// cacheBreakDetector ��ÿ�� API ����ǰ��Ա� L1/L2/tools ��ϣ�� cache_read��
// �������������½� >5% �� >2000 tokens ʱ��϶���ԭ��
// ���������������޸��κλ���ǰ׺����Ӱ�� L1/L2/tools �ȶ��ԡ�
type cacheBreakDetector struct {
	prevL1    uint64 // 上上次 L1 哈希（用于 diagnose 比对）
	prevL2    uint64 // 上上次 L2 哈希
	prevTools uint64 // 上上次 tools 哈希
	lastL1    uint64 // �ϴ� L1 system ��Ϣ�� FNV-1a ��ϣ
	lastL2    uint64 // �ϴ� L2 runtime ��Ϣ�� FNV-1a ��ϣ
	lastTools uint64 // �ϴι��� schemas �� FNV-1a ��ϣ
	lastRead  int    // �ϴ� cache_read ֵ
	callCount int    // ���ü���
}

// record �� API ����ǰ��¼��ǰǰ׺��ϣ��
func (d *cacheBreakDetector) record(msgs []provider.Message, tools []provider.ToolSchema) {
	var l1, l2 string
	if len(msgs) > 0 && msgs[0].Role == provider.RoleSystem {
		l1 = msgs[0].Content
	}
	if len(msgs) > 1 && msgs[1].Role == provider.RoleSystem {
		l2 = msgs[1].Content
	}
	// 保存旧哈希用于 diagnose 比对
	d.prevL1 = d.lastL1
	d.prevL2 = d.lastL2
	d.prevTools = d.lastTools

	d.lastL1 = fnv1a(l1)
	d.lastL2 = fnv1a(l2)
	d.lastTools = hashTools(tools)
	d.callCount++
}

// check �� API ���ú��⻺����ѡ�
// ���ؿ��ַ�����ʾ�������ǿձ�ʾ����ԭ��
func (d *cacheBreakDetector) check(u *provider.Usage) string {
	if d.lastRead == 0 {
		d.lastRead = u.CacheHitTokens
		return "" // �״ε��ã��޻���
	}

	drop := d.lastRead - u.CacheHitTokens
	threshold := int(float64(d.lastRead) * 0.05)
	if threshold < 2000 {
		threshold = 2000
	}
	if drop < threshold {
		d.lastRead = u.CacheHitTokens
		return "" // ��������
	}

	// ��϶���ԭ��
	reason := d.diagnose()
	d.lastRead = u.CacheHitTokens
	return "[cache break #" + itoa(d.callCount) + ": " +
		itoa(d.lastRead+drop) + "��" + itoa(u.CacheHitTokens) +
		" tok (" + reason + ")]"
}

// diagnose ����������ѵĿ���ԭ��
func (d *cacheBreakDetector) diagnose() string {
	// 比对前后两次调用的哈希来区分 client-side 还是 server-side
	// 如果前后哈希变了，说明有代码路径修改了前缀（可能是 bug）
	// 如果没变，说明是服务端原因（TTL 过期、路由切换等）
	var parts []string
	if d.prevL1 != d.lastL1 {
		parts = append(parts, "L1 changed")
	}
	if d.prevL2 != d.lastL2 {
		parts = append(parts, "L2 changed")
	}
	if d.prevTools != d.lastTools {
		parts = append(parts, "tools changed")
	}
	if len(parts) > 0 {
		return "client-side prefix drift: " + strings.Join(parts, ", ")
	}
	return "server-side (L1/L2/tools unchanged)"
}

// fnv1a �����ַ����� 64-bit FNV-1a ��ϣ��
func fnv1a(s string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}

// hashTools ���㹤�� schemas ����Ϲ�ϣ��
func hashTools(tools []provider.ToolSchema) uint64 {
	h := fnv1a(itoa(len(tools)))
	for _, t := range tools {
		h ^= fnv1a(t.Name)
		h ^= fnv1a(t.Description)
	}
	return h
}

// finishReasonMessage maps an abnormal finish_reason to a one-line warning,
// returning ok=false for the normal terminations ("stop", "tool_calls") and a
// nil usage. The sink renders the message; the "! " prefix is presentation.
func finishReasonMessage(u *provider.Usage) (string, bool) {
	if u == nil {
		return "", false
	}
	switch u.FinishReason {
	case "length":
		return "response truncated: hit max output tokens", true
	case "content_filter":
		return "response blocked by content filter", true
	case "repetition_truncation":
		return "response truncated: model repetition detected", true
	default:
		return "", false
	}
}

// --- Adaptive token calibration (V7.0 DSR) ---

// fallbackTokPerChar is ~4 chars per token �� the middle-of-the-road estimate
// used before any provider usage data is available to calibrate.
const fallbackTokPerChar = 0.25

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
func msgChars(m provider.Message) int {
	n := len(m.Content)
	for _, tc := range m.ToolCalls {
		n += len(tc.Name) + len(tc.Arguments)
	}
	return n
}

// charsOfMessages returns the total character count across messages.
func charsOfMessages(msgs []provider.Message) int {
	n := 0
	for _, m := range msgs {
		n += msgChars(m)
	}
	return n
}
