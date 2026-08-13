// Package control is the transport-agnostic session driver. A Controller owns
// the agent run loop and session lifecycle, takes commands (Send/Cancel/Approve/
// Compact/NewSession/…), and emits everything that happens —
// reasoning, tool calls, approvals, turn completion — as a typed event stream to
// a single event.Sink.
//
// The point is one orchestration layer behind every frontend: a terminal TUI, a
// desktop webview, or an HTTP/SSE server each drive the Controller identically
// (issue commands, render events) and none of them re-implement turn lifecycle,
// cancellation, or approval. The Controller depends on no frontend.
package control

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"tianxuan/internal/agent"
	"tianxuan/internal/billing"
	"tianxuan/internal/checkpoint"
	"tianxuan/internal/command"
	tiancontext "tianxuan/internal/context"
	"tianxuan/internal/diff"
	"tianxuan/internal/event"
	"tianxuan/internal/hook"
	"tianxuan/internal/jobs"
	"tianxuan/internal/memory"
	"tianxuan/internal/nilutil"
	"tianxuan/internal/permission"
	"tianxuan/internal/plugin"
	"tianxuan/internal/provider"
	"tianxuan/internal/skill"
	"tianxuan/internal/tool"
)

// Controller drives one chat session. Construct with New; drive with the command
// methods; observe through the Sink passed in Options.
type Controller struct {
	runner   agent.Runner
	executor *agent.Agent
	sink     event.Sink
	policy   permission.Policy

	label        string
	systemPrompt string
	sessionDir   string
	host         *plugin.Host
	commands     []command.Command
	skills       []skill.Skill
	hooks        *hook.Runner // session hook runner; nil-safe (no hooks configured)
	mem          *memory.Set
	cleanup      func()
	startedOnce  bool // guards the one-shot SessionStart hook on first turn

	// balanceURL/balanceKey target the active provider's optional wallet-balance
	// endpoint (empty when the provider declares none). Captured at build so a
	// model/key switch — which rebuilds the controller — refreshes them.
	balanceURL string
	balanceKey string

	// jobs is the session-scoped background-job manager. The agent's background
	// tools spawn into it; Compose drains its completion notes into the next turn;
	// Close cancels its still-running jobs.
	jobs *jobs.Manager

	// reg is the live tool registry the executor reads each turn; pluginCtx is the
	// session-scoped context a hot-added stdio server binds its subprocess to.
	// Together they let AddMCPServer connect a server mid-session and have its tools
	// available on the next turn (see AddMCPServer / RemoveMCPServer).
	reg       *tool.Registry
	pluginCtx context.Context

	// bgCtx / bgCancel governs background operations started by Submit
	// (/compact, /dream, /distill, /new). Calling Close cancels them so
	// there are no orphaned goroutines during shutdown.
	bgCtx    context.Context
	bgCancel context.CancelFunc

	ctxMgr *tiancontext.ContextManager // V3.0 Phase 5

	// Checkpoints (snapshot-based rewind). cp is the per-session store rebound when
	// the session path changes; cpRoot is the workspace root used to guard restore
	// writes. cpTurn is the monotonic turn counter (decoupled from the store so it
	// never collides after a restructure); cpBound[turn] records len(Session.Messages)
	// at that turn's start — the truncation boundary for a conversation rewind/fork.
	// Boundaries are persisted in each checkpoint and rebuilt from the store on
	// resume (so a reopened session can still rewind conversation / fork), but
	// dropped after a summarize restructures the log so those operations report
	// "unavailable" rather than mis-truncating; code rewind (file-based) is unaffected.
	cp      *checkpoint.Store
	cpRoot  string
	cpTurn  int
	cpBound map[int]int

	// promptMu serialises approval prompts so at most one is outstanding at a
	// time (parallel read-only tool calls don't normally gate, writers run
	// serially — but this keeps the contract explicit). Held across the blocking
	// wait, so it must never be taken by the Approve command path.
	promptMu sync.Mutex

	// mu guards the run state and approval bookkeeping; every critical section
	// under it is short and non-blocking.
	mu          sync.Mutex
	cancel      context.CancelFunc
	running     bool
	sessionPath string
	approvals   map[string]chan approvalReply
	asks        map[string]chan []event.AskAnswer
	granted     map[string]bool
	nextID      int
	turn        int
	autoApprove bool

	// permLevel controls permission strictness: "ask" (prompt before writes, default),
	// "auto" (allow writes without asking), or "yolo" (skip all prompts).
	permLevel string

	// pendingMemory holds memory notes added mid-session (via "#" quick-add or a
	// memory edit) that haven't yet been folded into a turn. Compose drains it
	// onto the next outgoing turn — never into the cache-stable system prefix — so
	// a fresh memory takes effect this session without busting the prompt cache;
	// it joins the prefix naturally on the next session.
	pendingMemory []string
	// sessionFacts holds temporary memories the model saved with session=true.
	// They persist across turns and can be promoted via PromoteSessionFacts().
	sessionFacts []memory.Memory
	// goal is set via /goal — the stopping condition for the session.
	goal string
}

type approvalReply struct {
	allow   bool
	session bool
}

// Options carries the already-built pieces setup assembles. Lifecycle metadata
// lets the controller mint and rotate session files; Host/Commands are surfaced
// to frontends that resolve MCP prompts and slash commands.
type Options struct {
	Runner       agent.Runner
	Executor     *agent.Agent
	Sink         event.Sink
	Policy       permission.Policy
	Label        string
	SystemPrompt string
	SessionDir   string
	SessionPath  string
	Host         *plugin.Host
	Commands     []command.Command
	Skills       []skill.Skill
	Hooks        *hook.Runner
	Memory       *memory.Set
	Cleanup      func()
	// BalanceURL/BalanceKey wire the active provider's optional wallet-balance
	// endpoint and bearer key; empty when the provider declares no balance_url.
	BalanceURL string
	BalanceKey string
	// Jobs is the session-scoped background-job manager (nil disables background jobs).
	Jobs *jobs.Manager
	// Registry is the executor's live tool set, and PluginCtx the session-scoped
	// context; both are needed for hot-adding MCP servers via AddMCPServer.
	Registry  *tool.Registry
	PluginCtx context.Context
	CtxMgr    *tiancontext.ContextManager // V3.0 Phase 5
	// WorkspaceRoot is the project root checkpoint restores are confined to ("" =
	// no confinement). Frontends pass the cwd they launched the session in.
	WorkspaceRoot string
}

// New builds a Controller. A nil Sink is replaced with event.Discard.
func New(opts Options) *Controller {
	sink := opts.Sink
	if nilutil.IsNil(sink) {
		sink = event.Discard
	}
	pluginCtx := opts.PluginCtx
	if pluginCtx == nil {
		pluginCtx = context.Background()
	}
	c := &Controller{
		runner:       opts.Runner,
		executor:     opts.Executor,
		sink:         sink,
		policy:       opts.Policy,
		label:        opts.Label,
		systemPrompt: opts.SystemPrompt,
		sessionDir:   opts.SessionDir,
		sessionPath:  opts.SessionPath,
		host:         opts.Host,
		commands:     opts.Commands,
		skills:       opts.Skills,
		hooks:        opts.Hooks,
		mem:          opts.Memory,
		cleanup:      opts.Cleanup,
		balanceURL:   opts.BalanceURL,
		balanceKey:   opts.BalanceKey,
		jobs:         opts.Jobs,
		reg:          opts.Registry,
		pluginCtx:    pluginCtx,
		ctxMgr:       opts.CtxMgr,
		cpRoot:       opts.WorkspaceRoot,
		permLevel:    "ask",
		approvals:    map[string]chan approvalReply{},
		asks:         map[string]chan []event.AskAnswer{},
		granted:      map[string]bool{},
	}
	c.bgCtx, c.bgCancel = context.WithCancel(context.Background())

	// Checkpoints: bind a store to the session and route writer pre-edits into it.
	c.rebindCheckpoints(opts.SessionPath)
	if c.executor != nil {
		c.executor.SetPreEditHook(func(ch diff.Change) {
			if c.cp != nil {
				c.cp.Snapshot(ch)
			}
		})
		c.executor.SetMemoryQueue(c)
		c.executor.SetSessionSaver(c)
		c.executor.SetPromoter(c)
	}
	return c
}

// rebindCheckpoints points the store at the (possibly new) session, loading any
// checkpoints already on disk, and resets the turn boundaries. Called on
// construction and whenever the session path changes (NewSession/Resume/SetSessionPath).
func (c *Controller) rebindCheckpoints(sessionPath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cp = checkpoint.New(ckptDir(sessionPath), c.cpRoot)
	c.cpTurn = c.cp.NextTurn() // continue numbering past any checkpoints on disk
	c.cpBound = c.cp.Bounds()  // rebuilt from persisted checkpoints so a resumed
	if c.cpBound == nil {      // session can still rewind conversation / fork
		c.cpBound = map[int]int{}
	}
}

// beginCheckpoint opens a checkpoint for the turn about to run, recording the
// current message count as the conversation-rewind boundary. Called at the top of
// runTurn, before the user message is appended.
func (c *Controller) beginCheckpoint(input string) {
	if c.cp == nil || c.executor == nil {
		return
	}
	c.mu.Lock()
	turn := c.cpTurn
	c.cpTurn++
	msgIndex := len(c.executor.Session().Messages)
	c.cpBound[turn] = msgIndex
	c.mu.Unlock()
	c.cp.Begin(turn, input, msgIndex)
}

// --- commands (frontend → controller) ---

// runGuarded runs body on a background goroutine under a fresh cancellable
// context, guarding against concurrent turns and emitting a TurnDone event when
// it finishes (Err set on failure; nil also for a user Cancel). A no-op if a
// turn is already in flight.
func (c *Controller) runGuarded(body func(ctx context.Context) error) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
			Text: "a turn is already running — this request was ignored"})
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.running = true
	c.mu.Unlock()

	go func() {
		panicked := false
		defer func() {
			if r := recover(); r != nil {
				slog.Error("controller: runGuarded panic", "panic", r)
				panicked = true
				c.mu.Lock()
				c.running = false
				c.cancel = nil
				c.mu.Unlock()
				c.sink.Emit(event.Event{Kind: event.TurnDone, Err: fmt.Errorf("panic: %v", r)})
			}
		}()
		err := body(ctx)
		if panicked {
			return // TurnDone already emitted in recover path
		}
		c.mu.Lock()
		c.running = false
		c.cancel = nil
		c.mu.Unlock()
		c.sink.Emit(event.Event{Kind: event.TurnDone, Err: err})
	}()
}

// Send starts a turn with an uncomposed message. The controller applies
// auto-plan, plan-mode, memory, and background-job framing inside the async turn
// path so frontends do not block on classifier I/O.
func (c *Controller) Send(input string) {
	c.SendWithRaw(input, input)
}

// SendWithRaw starts a turn with separate model input and raw prompt text. The
// raw prompt is used only for auto-plan scoring; it deliberately excludes
// resolved @-reference payloads so referenced file contents cannot inflate the
// complexity score.
func (c *Controller) SendWithRaw(input, raw string) {
	c.runGuarded(func(ctx context.Context) error { return c.runTurnWithRaw(ctx, input, raw) })
}

// Steer injects a mid-turn correction from the user while a turn is running:
// the message is queued into the executor's steer buffer and consumed as
// guidance at the next model step, without cancelling or restarting the turn.
// When idle it falls back to Send (starts a normal turn). This is the
// single-model Adaptive Execution counterpart to CancelAndSubmit — steer for
// course corrections, cancel+resubmit for a full restart.
func (c *Controller) Steer(input string) {
	if c.Running() && c.executor != nil {
		c.executor.Steer(input)
		c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
			Text: "纠偏指令已注入当前任务（下一步生效）"})
		return
	}
	c.Send(input)
}

// runTurn runs one model turn.

func (c *Controller) runTurn(ctx context.Context, input string) error {
	return c.runTurnWithRaw(ctx, input, input)
}

func (c *Controller) runTurnWithRaw(ctx context.Context, input, raw string) error {
	c.maybeSessionStart(ctx)

	// V3.0 Phase 5: ContextManager handles first-turn orchestration.

	// V3.0 Phase 5: ContextManager handles first-turn orchestration.
	// ProcessFirstTurn locks the runtime (idempotent). On the first turn,
	// also push the L2 system prompt into the agent so the model gets
	// project/task context. Subsequent turns reuse the cached L2 bytes.
	if c.ctxMgr != nil {
		wasLocked := c.ctxMgr.Runtime().IsLocked()
		c.ctxMgr.ProcessFirstTurn(input)
		if !wasLocked {
			// V7.5: 将运行时上下文合并到 L1 系统提示词末尾，
			// 取代原 L2 注入 + WarmupCache 方案，前缀永不改变。
			c.executor.MergeRuntimePrompt(c.ctxMgr.Runtime().SystemPrompt())
		}
	}

	input = c.Compose(input)
	// Open a checkpoint for this turn before the user message is appended, so the
	// recorded message boundary precedes it and pre-edit snapshots land here.
	c.beginCheckpoint(agent.StripTransientBlocks(input))
	// UserPromptSubmit / Stop hooks bracket the whole turn (incl. the plan
	// research + approved-execution sub-turns below): a gating UserPromptSubmit
	// aborts before any model call; Stop fires once when the turn returns.
	if c.hooks.Enabled() {
		c.mu.Lock()
		c.turn++
		turn := c.turn
		c.mu.Unlock()
		if block, _ := c.hooks.PromptSubmit(ctx, input, turn); block {
			return nil // the hook's notify callback already surfaced the reason
		}
		defer func() { c.hooks.Stop(ctx, lastAssistantText(c.History()), turn) }()
	}
	if _, err := c.runner.Run(ctx, input); err != nil {
		return err
	}
	// 每轮对话后自动快照保存，确保崩溃/重启不丢上下文
	if err := c.Snapshot(); err != nil {
		slog.Warn("controller: snapshot after turn", "err", err)
	}
	// 每轮对话后自动提取记忆候选（仅暂存 pending，用户确认后才落盘）
	c.autoExtract()
	// 待确认候选超过 30 条时提炼为 1 条，避免 pending 无限堆积
	c.autoCondense()
	// 每轮对话后检查 Dream 调度门控，通过则整合跨会话知识为候选
	c.autoDream()
	return nil
}

// lastAssistantText returns the content of the most recent assistant message with
// non-empty text.
func lastAssistantText(msgs []provider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleAssistant && strings.TrimSpace(msgs[i].Content) != "" {
			return msgs[i].Content
		}
	}
	return ""
}

// Submit is the one-call entry for a simple frontend: it takes raw user input
// and does everything — slash-command dispatch, @-reference expansion —
// emitting all output as events. The HTTP/SSE server uses this so
// a browser client only POSTs the typed line.
//
// Slash commands route to the matching primitive: /compact and /new run their
// session op and emit a Notice; /mcp__server__prompt and custom /commands
// resolve to a turn; an unknown slash emits a Notice. Anything else is a normal
// turn with its @-references resolved first.
func (c *Controller) notice(text string) {
	c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Text: text})
}

// Run executes a turn synchronously, returning the agent's error. Used by the
// headless `tianxuan run` path, where the Sink renders to stdout and the caller
// just needs the exit status — no TurnDone event, no cancel bookkeeping.
func (c *Controller) Run(ctx context.Context, input string) error {
	c.maybeSessionStart(ctx)

	// V3.0 Phase 5: ContextManager takes over first-turn orchestration.
	if c.ctxMgr != nil {
		wasLocked := c.ctxMgr.Runtime().IsLocked()
		c.ctxMgr.ProcessFirstTurn(input)
		if !wasLocked {
			// V7.5: 将运行时上下文合并到 L1
			c.executor.MergeRuntimePrompt(c.ctxMgr.Runtime().SystemPrompt())
		}
	}

	if c.hooks.Enabled() {
		c.mu.Lock()
		c.turn++
		turn := c.turn
		c.mu.Unlock()
		if block, _ := c.hooks.PromptSubmit(ctx, input, turn); block {
			return nil
		}
		defer func() { c.hooks.Stop(ctx, lastAssistantText(c.History()), turn) }()
	}
	_, err := c.runner.Run(ctx, input)
	return err
}

// Cancel aborts the in-flight turn. A goroutine blocked awaiting approval
// unblocks via the cancelled context.
func (c *Controller) Cancel() {
	c.mu.Lock()
	cancel := c.cancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// CancelAndSubmit cancels the in-flight turn and immediately submits a new one
// — all within a single atomic operation. This avoids the race between frontend
// Cancel → wait-for-running-false → Submit by spinning until the cancelled turn
// finishes, then starting the new turn. Used by the correction feature (Shift+Enter
// during a running turn).
func (c *Controller) CancelAndSubmit(input string) {
	c.Cancel()
	// Wait for the cancelled turn to fully exit — the goroutine in runGuarded
	// sets c.running = false in its defer.
	for c.Running() {
		// Brief sleep to avoid busy-wait. The cancelled goroutine exits within
		// milliseconds once ctx.Done() propagates through the agent loop.
		select {
		case <-time.After(5 * time.Millisecond):
		}
	}
	c.Submit(input)
}

// Running reports whether a turn is currently in flight.
func (c *Controller) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// SetGoal sets the session goal (set via /goal) and propagates it to the
// SetGoal sets the session goal (set via /goal) and propagates it to the
// executor so the stop gate can enforce it.
func (c *Controller) SetGoal(g string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.goal = g
	if c.executor != nil {
		c.executor.SetGoal(g)
	}
}

// Goal returns the current session goal.
func (c *Controller) Goal() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.goal
}

// Compact runs one compaction pass on the executor's session on demand.
// instructions is optional `/compact <focus>` guidance steering what to keep.
// Since V5.0, explicit compaction has been replaced by automatic truncation in
// the run loop (≥500K tokens → three-tier compression). This method exists for
// API compatibility — the built-in truncation handles context pressure.
func (c *Controller) Compact(ctx context.Context, instructions string) error {
	if c.executor == nil {
		return nil
	}
	c.notice("compact: automatic truncation handles context compression — no manual /compact needed")
	return nil
}

// Dream extracts knowledge from the current session into project memory.
// Uses deterministic session summary (no LLM call). V6.0 Feature.
// maybeSessionStart fires the SessionStart hook exactly once per session, lazily
// on the first turn — by then the sink/notify is wired, and a resumed session
// fires it too (its first post-resume turn).
func (c *Controller) maybeSessionStart(ctx context.Context) {
	c.mu.Lock()
	if c.startedOnce {
		c.mu.Unlock()
		return
	}
	c.startedOnce = true
	c.mu.Unlock()
	c.hooks.SessionStart(ctx)
}

// TCCAStats returns a formatted cache metrics report (V3.0).
// Returns empty string when ctxMgr is not wired.
func (c *Controller) TCCAStats() string {
	if c.ctxMgr == nil {
		return "TCCA not available (ContextManager not wired)"
	}
	r := c.ctxMgr.Metrics()
	return fmt.Sprintf(
		"TCCA Session Cache Report\n"+
			"========================\n"+
			"Layers:\n"+
			"  L1 Identity:  %d bytes\n"+
			"  L2 Runtime:   %d bytes\n"+
			"  L3 Skill:     v%d\n"+
			"  L4 Flow:      %d messages\n"+
			"\n"+
			"Savings (session):\n"+
			"  Compaction:   %d tokens saved (%d passes)\n"+
			"  Fork reuse:   %d tokens saved (%d forks)\n"+
			"  节省:         ¥%.4f\n"+
			"  Latency:      %d ms\n",
		r.L1Size, r.L2Size, r.L3Version, r.L4Messages,
		r.SavedByCompact, r.CompactionCount,
		r.SavedByFork, r.ForkCount,
		r.SavedUSD*7.25, r.SavedLatencyMs,
	)
}

// TCCAReport returns the structured cache metrics report (V3.0).
// Returns zero-value CacheReport when ctxMgr is not wired.
func (c *Controller) TCCAReport() tiancontext.CacheReport {
	if c.ctxMgr == nil {
		return tiancontext.CacheReport{}
	}
	return c.ctxMgr.Metrics()
}

// SystemPrompt returns the L1 system prompt.
func (c *Controller) SystemPrompt() string {
	if c.ctxMgr != nil {
		return c.ctxMgr.Identity().SystemPrompt()
	}
	return c.systemPrompt
}

// Executor returns the raw executor Agent for scheduler use.
func (c *Controller) Executor() *agent.Agent { return c.executor }

// Balance queries the active provider's wallet balance, or (nil, nil) when the
// provider declares no balance_url — so a caller treats "not configured" and
// "fetched" the same and just omits the readout when nil.
func (c *Controller) Balance(ctx context.Context) (*billing.Balance, error) {
	if strings.TrimSpace(c.balanceURL) == "" {
		return nil, nil
	}
	return billing.Fetch(ctx, c.balanceURL, c.balanceKey)
}

// Host returns the running MCP host (nil when no plugins), for frontends that
// list servers / resolve MCP prompts.
func (c *Controller) Host() *plugin.Host { return c.host }

// Commands returns the loaded custom slash commands.
func (c *Controller) Commands() []command.Command { return c.commands }

// Skills returns the discoverable skills (for the slash menu and `/skill`).
func (c *Controller) Skills() []skill.Skill { return c.skills }

// HookRunner returns the session's hook runner (nil-safe; may hold zero hooks),
// so a frontend can list the active hooks via `/hooks`.
func (c *Controller) HookRunner() *hook.Runner { return c.hooks }

// AddMCPServer connects an MCP server live and persists it to the config file. Its
// tools are registered immediately and become available on the next turn (the
// agent reads the registry per turn). The raw entry — ${VARS} intact — is what's
// written to disk; the live connection uses the expanded form. Returns the number
// of tools the server exposed. A save failure after a successful connect is
// reported but non-fatal: the server still works this session.

// Label returns the human-readable model label, e.g. "deepseek-flash".
func (c *Controller) Label() string { return c.label }

// Close stops plugin subprocesses and releases resources. A session that ever
// started fires SessionEnd so a teardown hook runs.
func (c *Controller) Close() {
	c.mu.Lock()
	started := c.startedOnce
	c.mu.Unlock()
	if started {
		c.hooks.SessionEnd(context.Background())
	}
	if c.bgCancel != nil {
		c.bgCancel()
	}
	if c.jobs != nil {
		c.jobs.Close() // cancel any still-running background jobs
	}
	if c.cleanup != nil {
		c.cleanup()
	}
}

// Jobs returns the still-running background jobs for the status bar (nil when
// background jobs are disabled).
func (c *Controller) Jobs() []jobs.View {
	if c.jobs == nil {
		return nil
	}
	return c.jobs.Running()
}
