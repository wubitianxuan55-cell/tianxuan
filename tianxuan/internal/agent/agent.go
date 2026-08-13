package agent

import (
	"context"
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
	"tianxuan/internal/skill"
	"tianxuan/internal/tool"
)

// Asker puts structured multiple-choice questions to the user and blocks for the
// answers. The agent consults it for the `ask` tool. It is interface-shaped so
// the agent stays independent of the frontend; a nil asker means no interactive
// user (headless runs), where `ask` returns a "decide for yourself" result. The
// interactive frontends wire the controller in as the Asker.

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
	dedupHashes map[string]bool // V8.0 P0-2: deterministic pruning (tool+args+result → seen)

	// V10.27: 后台任务启停循环检测 — 防止模型反复 start-bash → kill_shell
	// 而不读取输出。跨轮次累计，仅 bash_output/wait/前台 bash 重置计数。
	bgStartKillStreak    int  // 连续启停循环计数（跨轮次累计）
	bgJobStartedThisTurn bool // 本轮启动了后台 bash 任务
	bgOutputReadThisTurn bool // 本轮读取了后台任务输出（bash_output / wait）
	bgJobKilledThisTurn  bool // 本轮杀掉了后台任务（kill_shell）

	// V10.28: stale anchor 编辑守卫 — 同一轮内编辑文件后必须重新 read_file，
	// 防止 old_string 锚点过时。追踪每轮写入和读取的文件路径。
	staleMu           sync.Mutex      // 保护 staleWrittenFiles / staleReadFiles 的并发访问
	staleWrittenFiles map[string]bool // 本轮已写入的文件路径
	staleReadFiles    map[string]bool // 本轮已读取的文件路径

	// V10.13: 成功循环检测 — 移植自 Reasonix repeatedSuccessBlock。
	// 检测写工具在同一用户轮次中重复成功调用，阈值 2 次后阻止。
	repeatMu            sync.Mutex
	repeatSuccessCounts map[string]int

	// V6.0: 回忆提醒开关（recall_reminder.go）
	recallReminderFired bool
	// V10.101: maybeAutoRecall 会话级一次性 — 首轮注入后不再注入，
	// 避免每轮动态块落在缓存未命中区破坏前缀稳定。
	autoRecallFired bool

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
		Observe(toolName, result string) *learning.Pattern
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
	// V10.173: 会话级 token 预算渐进提醒（蒸馏 codex rollout_budget）。
	// 软信号：跨阈值注入 user 消息提醒模型规划收敛；与成本 budgetGate 硬门共存。
	tokenBudget *TokenBudget
	// lspManager runs LSP diagnostics on files modified by writer tools
	// and injects results so the model can fix compilation errors.
	lspManager interface {
		Diagnostics(ctx context.Context, file string) (string, error)
	}

	// auditFunc, when non-nil, is called after each tool execution for
	// audit trail logging (V3.2).
	auditFunc func(tool string, taskKind string, readOnly bool, outcome string, errMsg string, outputLen int, durationMs int64)
	// toolStats, when non-nil, aggregates per-tool failure modes across
	// sessions (V10.154, distilled from codex ToolDispatchTrace).
	toolStats *tool.Stats
	// toolTrace, when non-nil, appends one structured JSONL record per tool
	// dispatch for offline error-rate analysis (V10.167).
	toolTrace *tool.TraceStore

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
	// offloadDir is the configured offload base directory. Store creation is
	// deferred until sessionID is set (see SetArchive), because the store's
	// per-session subdirectory is derived from it.
	offloadDir string

	// autoSkill is the skill store consulted by withAutoSkill (V10.122).
	// nil disables automatic skill injection. Injection is a deterministic
	// pure function of (input, stored inline-skill body), so it never churns
	// the DeepSeek prefix cache beyond the natural input bytes.
	autoSkill *skill.Store
	// autoInjected tracks skills already auto-injected this session (V10.123),
	// so a matching later turn does not re-inject the same body — repeated
	// injection would re-bill the body as cache-miss tokens every turn.
	// Reset on compaction, because the summary may drop the injected block.
	autoInjected map[string]bool
	// autoSkillSeq guarantees unique synthetic tool IDs for auto-skill stat
	// events across turns.
	autoSkillSeq atomic.Int64

	// failureGuard is the host-side failure-escalation state machine
	// (V10.148, distilled from Reasonix internal/recovery). Reset at each
	// Run; checked before mutation execution and observed after each result.
	failureGuard *FailureGuard
	// lastGuardOutcomes carries per-call escalation outcomes from the most
	// recent executeBatch so the serial run loop can inject guidance messages
	// in deterministic order (parallel tool calls cannot inject directly).
	lastGuardOutcomes []GuardOutcome
}

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
		toolStats:     opts.ToolStats,
		toolTrace:     opts.ToolTrace,
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
	// V10.173: 会话级 token 预算渐进提醒（蒸馏 codex rollout_budget）
	if opts.TokenBudgetLimit > 0 {
		r.tokenBudget = NewTokenBudget(opts.TokenBudgetLimit, opts.TokenBudgetReminders)
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
	// Context offloading: enable when a base directory is configured. Store
	// creation is deferred until SetArchive sets sessionID (per-session
	// subdirectory). Only the executor (not planner) offloads — the planner
	// runs with a read-only toolset and its own session.
	r.SetOffload(opts.OffloadDir, opts.OffloadThresholdChars)
	// V10.148: Auto Failure Guard — host-side failure escalation.
	r.failureGuard = NewFailureGuard()
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

// tokensLeft estimates the tokens remaining in the context window: window
// minus the chars of the live session scaled by the observed chars/token
// ratio. ok=false when no window is configured (compaction disabled), so no
// budget can be reported. Backs the get_context_remaining tool.
func (a *AgentRunner) tokensLeft() (int, bool) {
	win := a.compaction.Window
	if win <= 0 {
		return 0, false
	}
	used := int(float64(charsOfMessages(a.session.Messages)) * a.tokPerChar())
	if used >= win {
		return 0, true
	}
	return win - used, true
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
// V10.101: now accepts the current canonical todo list so unverified completed
// todos are actually detected — the old nil argument caused the check to be a no-op.
func (a *AgentRunner) finalReadinessCheck(currentTodos []evidence.TodoItem) (blocked bool, reason string) {
	if a.evidence == nil {
		return false, ""
	}
	// Single-model mode (strictVerify=false): complete_step is optional — a
	// todo_write "completed" is a sufficient host-observable state, and forcing
	// a sign-off ceremony adds rounds with no partner to receive it. Dual-model
	// mode keeps the check: the planner host depends on complete_step evidence to replan.
	if !a.evidence.StrictVerification() {
		return false, ""
	}
	// Check for unverified completed todos: the model marked a todo as
	// "completed" (via todo_write) but never ran complete_step for it.
	unverified, hasBaseline := a.evidence.UnverifiedCompletedTodos(currentTodos)
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
