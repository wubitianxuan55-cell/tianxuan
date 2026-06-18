// Package control is the transport-agnostic session driver. A Controller owns
// the agent run loop and session lifecycle, takes commands (Send/Cancel/Approve/
// SetPlanMode/Compact/NewSession/…), and emits everything that happens —
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
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"tianxuan/internal/agent"
	"tianxuan/internal/billing"
	"tianxuan/internal/checkpoint"
	"tianxuan/internal/codegraph"
	"tianxuan/internal/command"
	"tianxuan/internal/config"
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
	autoPlan     string
	classifier   autoPlanClassifier
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

	ctxMgr            *tiancontext.ContextManager     // V3.0 Phase 5

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
	planMode    bool
	agentMode   string // V6.0 P3: "explore"|"develop"|"orchestrate"
	sessionPath string
	approvals   map[string]chan approvalReply
	asks        map[string]chan []event.AskAnswer
	granted     map[string]bool
	nextID      int
	// turn counts model turns this session, passed to hooks in their payload.
	turn int
	// autoApprove auto-allows writer tool calls without prompting. Set only while
	// executing a just-approved plan: approving the plan is the go-ahead, so the
	// model shouldn't re-prompt for every write of the work it just got cleared to
	// do. Deny rules still bite (those never reach the approver). Reset when the
	// execution turn returns.
	autoApprove bool

	// bypass is "YOLO" mode: while set, every approval prompt is auto-allowed for
	// the rest of the session (writers and bash run without asking). It is a
	// deliberate, session-scoped opt-in (the --dangerously-skip-permissions flag or
	// a runtime toggle), never persisted. Deny rules are unaffected — they're
	// resolved before the approver, so a denied tool is still blocked in YOLO mode.
	bypass bool

	// pendingMemory holds memory notes added mid-session (via "#" quick-add or a
	// memory edit) that haven't yet been folded into a turn. Compose drains it
	// onto the next outgoing turn — never into the cache-stable system prefix — so
	// a fresh memory takes effect this session without busting the prompt cache;
	// it joins the prefix naturally on the next session.
	pendingMemory []string
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
	CtxMgr         *tiancontext.ContextManager     // V3.0 Phase 5
	// WorkspaceRoot is the project root checkpoint restores are confined to ("" =
	// no confinement). Frontends pass the cwd they launched the session in.
	WorkspaceRoot string
	AutoPlan      string
	Classifier    autoPlanClassifier
}

// New builds a Controller. A nil Sink is replaced with event.Discard.
func New(opts Options) *Controller {
	sink := opts.Sink
	if nilutil.IsNil(sink) {
		sink = event.Discard
	}
	classifier := opts.Classifier
	if nilutil.IsNil(classifier) {
		classifier = nil
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
		autoPlan:     normalizeAutoPlan(opts.AutoPlan),
		classifier:   classifier,
		balanceURL:   opts.BalanceURL,
		balanceKey:   opts.BalanceKey,
		jobs:         opts.Jobs,
		reg:          opts.Registry,
		pluginCtx:    pluginCtx,
		ctxMgr:           opts.CtxMgr,
		cpRoot:           opts.WorkspaceRoot,
		approvals:    map[string]chan approvalReply{},
		asks:         map[string]chan []event.AskAnswer{},
		granted:      map[string]bool{},
	}
	// Checkpoints: bind a store to the session and route writer pre-edits into it.
	c.rebindCheckpoints(opts.SessionPath)
	if c.executor != nil {
		c.executor.SetPreEditHook(func(ch diff.Change) {
			if c.cp != nil {
				c.cp.Snapshot(ch)
			}
		})
		c.executor.SetMemoryQueue(c)
	}
	return c
}

// ckptDir derives a session's checkpoint directory from its file path
// (…/<id>.jsonl → …/<id>.ckpt). Empty path → empty (in-memory checkpoints).
func ckptDir(sessionPath string) string {
	if sessionPath == "" {
		return ""
	}
	return strings.TrimSuffix(sessionPath, ".jsonl") + ".ckpt"
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
		err := body(ctx)
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

// planApprovalTool is the Tool name on the ApprovalRequest the controller emits
// to gate a proposed plan. Frontends key their plan-approval UI on it (the
// desktop renders a plan card; the chat TUI a plan banner).
const planApprovalTool = "exit_plan_mode"

// planApprovedMessage is the follow-up turn sent once the user approves a plan —
// the in-context nudge to execute and keep the (already-seeded) task list honest.
const planApprovedMessage = "Plan approved — plan mode is off; you're cleared to make the changes without asking again. Implement the plan now. Keep the task list current with todo_write, preserving its two-level shape (phases at level 0, their sub-steps at level 1): mark the sub-step you start as in_progress, one in_progress at a time. Sign off each finished sub-step with complete_step, attaching the evidence it's done — the verification you ran, the diff/files you changed, or a manual check. Don't claim a step is done without evidence."

// orchestrateApprovedMessage is the post-plan-approval message for orchestrate mode
// (V6.0 P3). Stronger than planApprovedMessage: enforces structured phase-by-phase
// execution with explicit checkpoints between phases.
const orchestrateApprovedMessage = "Plan approved — orchestrate mode: execute phase by phase. " +
	"Use tree-structured task IDs in todo_write content: prefix phases with T1/T2/T3 and sub-steps with T1.1/T1.2 etc. " +
	"For each phase: (1) mark the first sub-step in_progress with todo_write, (2) execute it, " +
	"(3) sign off with complete_step attaching evidence, (4) repeat for remaining sub-steps. " +
	"After each phase completes, run tests to verify before moving to the next phase. " +
	"Keep the task list current at all times. Only stop when every phase is complete and verified."

// runTurn runs one model turn, then applies the plan-approval gate. This is the
// single, frontend-agnostic plan flow: in plan mode the model just researches
// (writers are blocked) and writes its plan as a normal answer — no special tool.
// When the turn ends with a text proposal, the controller asks the user to
// approve (reusing the ApprovalRequest channel both frontends already render);
// on approval it exits plan mode, seeds the task list from the plan, and
// continues straight into execution; on rejection it stays in plan mode so the
// next turn can revise. Plan mode is only ever set interactively, so the headless
// `Run` path (which doesn't call this) never blocks on a prompt.
func (c *Controller) runTurn(ctx context.Context, input string) error {
	return c.runTurnWithRaw(ctx, input, input)
}

func (c *Controller) runTurnWithRaw(ctx context.Context, input, raw string) error {
	c.maybeSessionStart(ctx)
	// V8.0 P1-4: plan mode smart clarification — prompt the user when input is too vague.
	if c.isPlanMode() && c.maybeClarifyVagueInput(raw) {
		return nil // question emitted, wait for user response
	}
	c.maybeAutoPlan(ctx, raw)

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
	c.beginCheckpoint(input)
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
	if err := c.runner.Run(ctx, input); err != nil {
		return err
	}
	// 每轮对话后自动快照保存，确保崩溃/重启不丢上下文
	if err := c.Snapshot(); err != nil {
		slog.Warn("controller: snapshot after turn", "err", err)
	}
	c.mu.Lock()
	plan := c.planMode
	mode := c.agentMode
	c.mu.Unlock()
	if !plan {
		return nil
	}

	// V6.0 P3: explore mode never gates — it just answers questions
	if mode == "explore" {
		return nil
	}

	proposal := lastAssistantText(c.History())
	if proposal == "" {
		return nil // no substantive proposal to gate
	}
	// The plan is already visible as the assistant's answer, so the request
	// carries no subject — it's purely the gate.
	allow, _, err := c.requestApproval(ctx, planApprovalTool, "")
	if err != nil {
		return err
	}
	if !allow {
		return nil // keep planning; plan mode stays on
	}
	c.SetPlanMode(false)
	c.seedPlanTodos(proposal)
	// The plan is the go-ahead: don't re-prompt for each write of the approved
	// work. Auto-approve writers for the duration of this execution turn only.
	c.mu.Lock()
	c.autoApprove = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.autoApprove = false
		c.mu.Unlock()
	}()

	// V6.0 P3: mode-specific post-approval message
	msg := planApprovedMessage
	if mode == "orchestrate" {
		msg = orchestrateApprovedMessage
	}
	return c.runner.Run(ctx, msg)
}

// lastAssistantText returns the content of the most recent assistant message with
// non-empty text — the model's final answer for the turn (its plan, in plan mode).
func lastAssistantText(msgs []provider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleAssistant && strings.TrimSpace(msgs[i].Content) != "" {
			return msgs[i].Content
		}
	}
	return ""
}

// Submit is the one-call entry for a simple frontend: it takes raw user input
// and does everything — slash-command dispatch, @-reference expansion, plan-mode
// composition — emitting all output as events. The HTTP/SSE server uses this so
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
	return c.runner.Run(ctx, input)
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

// Running reports whether a turn is currently in flight.
func (c *Controller) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// Approve answers a pending ApprovalRequest by ID: allow runs the call, session
// also remembers a grant for the rest of the session so the same tool+subject
// isn't re-prompted. Unknown/expired IDs are ignored.
func (c *Controller) Approve(id string, allow, session bool) {
	c.mu.Lock()
	reply := c.approvals[id]
	delete(c.approvals, id)
	c.mu.Unlock()
	if reply != nil {
		reply <- approvalReply{allow: allow, session: session} // buffered, never blocks
	}
}

// EnableInteractiveApproval swaps the executor's gate for one that routes "ask"
// decisions to the frontend via ApprovalRequest events, and wires the controller
// in as the executor's Asker so the `ask` tool can question the user. Interactive
// frontends (chat, desktop) call this; the headless run keeps the silent gate and
// a nil asker from setup.
func (c *Controller) EnableInteractiveApproval() {
	if c.executor != nil {
		c.executor.SetGate(permission.NewGate(c.policy, gateApprover{c}))
		c.executor.SetAsker(c)
	}
}

// Ask implements agent.Asker: it emits an AskRequest and blocks until
// AnswerQuestion(ID, …) answers or ctx is cancelled. promptMu serialises it
// against tool-approval prompts so at most one user prompt is outstanding.
func (c *Controller) Ask(ctx context.Context, questions []event.AskQuestion) ([]event.AskAnswer, error) {
	c.promptMu.Lock()
	defer c.promptMu.Unlock()

	c.mu.Lock()
	c.nextID++
	id := strconv.Itoa(c.nextID)
	reply := make(chan []event.AskAnswer, 1)
	c.asks[id] = reply
	c.mu.Unlock()

	c.sink.Emit(event.Event{Kind: event.AskRequest, Ask: event.Ask{ID: id, Questions: questions}})

	select {
	case ans := <-reply:
		return ans, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.asks, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

// AnswerQuestion resolves a pending AskRequest by ID with the user's selections.
// Unknown/expired IDs are ignored.
func (c *Controller) AnswerQuestion(id string, answers []event.AskAnswer) {
	c.mu.Lock()
	reply := c.asks[id]
	delete(c.asks, id)
	c.mu.Unlock()
	if reply != nil {
		reply <- answers // buffered, never blocks
	}
}

// SetPlanMode flips the executor's read-only gate without touching the
// cache-stable prompt prefix, and remembers the state so Compose can prepend the
// plan-mode marker to outgoing turns.
func (c *Controller) SetPlanMode(v bool) {
	c.mu.Lock()
	c.planMode = v
	c.mu.Unlock()
	if c.executor != nil {
		c.executor.SetPlanMode(v)
	}
}

// SetAgentMode switches the agent runtime mode and propagates to the executor.
func (c *Controller) SetAgentMode(mode string) {
	c.mu.Lock()
	c.agentMode = mode
	// V6.0 P3: sync controller's planMode flag for Compose
	switch mode {
	case "explore":
		c.planMode = true
	case "develop":
		c.planMode = false
	case "orchestrate":
		c.planMode = true
	}
	c.mu.Unlock()
	if c.executor != nil {
		c.executor.SetAgentMode(mode)
	}
}

// AgentMode returns the current mode.
func (c *Controller) AgentMode() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agentMode
}

// PlanMode reports whether outgoing turns currently receive the plan-mode
// marker. Frontends use it after Compose because auto-plan may flip the mode.
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

func (c *Controller) PlanMode() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.planMode
}

// Compact runs one compaction pass on the executor's session on demand.
// instructions is optional `/compact <focus>` guidance steering what to keep.
func (c *Controller) Compact(ctx context.Context, instructions string) error {
	if c.executor == nil {
		return nil
	}
	return c.executor.CompactNow(ctx, instructions)
}

// Dream extracts knowledge from the current session into project memory.
// Uses deterministic session summary (no LLM call). V6.0 Feature.
func (c *Controller) Dream(ctx context.Context) error {
	if c.executor == nil { return nil }
	msgs := c.executor.Session().Snapshot()
	if len(msgs) < 2 { return nil }
	summary := agent.BuildCompactSummary(msgs[1:])
	if summary == "" { return nil }
	date := time.Now().Format("2006-01-02")
	// Search past memories related to this session and merge insights
	related := ""
	if mem := c.Memory(); mem != nil && mem.Search != nil {
		matches := mem.Search.Search("project architecture code design")
		if len(matches) > 0 {
			var sb strings.Builder
			sb.WriteString("\nRelated past memories:\n")
			limit := 3
			if len(matches) < limit { limit = len(matches) }
			for i, m := range matches[:limit] {
				fmt.Fprintf(&sb, "  %d. %s\n", i+1, m.Name)
			}
			related = sb.String()
		}
	}
	entry := "/dream (" + date + "):\n" + summary + "\n" + related
	c.QueueMemory(entry)
	c.notice("dream: knowledge extracted (" + fmt.Sprintf("%d", len(summary)/4) + " tok" + related + ")")
	return nil
}

// Distill analyzes session patterns and suggests skills. V6.0 Feature.
func (c *Controller) Distill(ctx context.Context) error {
	if c.executor == nil { return nil }
	msgs := c.executor.Session().Snapshot()
	toolSeq := []string{}
	for _, m := range msgs {
		if m.Role == "assistant" {
			for _, tc := range m.ToolCalls {
				toolSeq = append(toolSeq, tc.Name)
			}
		}
	}
	if len(toolSeq) < 3 { return nil }
	patterns := findRepeatedPatterns(toolSeq, 2)
	if len(patterns) == 0 {
		c.notice("distill: no repeated patterns found")
		return nil
	}
	var sb strings.Builder
	sb.WriteString("/distill (" + time.Now().Format("2006-01-02") + "):\n")
	sb.WriteString("Detected repeated tool patterns:\n")
	for _, p := range patterns {
		sb.WriteString("  - " + p + "\n")
	}
	c.QueueMemory(sb.String())
	c.notice("distill: " + fmt.Sprintf("%d", len(patterns)) + " patterns found")
	return nil
}

func findRepeatedPatterns(seq []string, minLen int) []string {
	seen := map[string]int{}
	for i := 0; i <= len(seq)-minLen; i++ {
		for j := i + minLen; j <= len(seq); j++ {
			pat := ""
			for k := i; k < j; k++ {
				if k > i { pat += " -> " }
				pat += seq[k]
			}
			if len(strings.Fields(pat)) >= minLen { seen[pat]++ }
		}
	}
	var out []string
	for pat, count := range seen {
		if count >= 2 { out = append(out, pat+" (repeated "+fmt.Sprintf("%d", count)+"x)") }
	}
	return out
}

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

// NewSession snapshots the current conversation, rotates to a fresh file, and
// resets the executor to a clean session carrying the same system prompt. It
// ends the old session and starts the new one for lifecycle hooks.
func (c *Controller) NewSession() error {
	if c.executor == nil {
		return nil
	}
	if err := c.Snapshot(); err != nil {
		return err
	}
	c.hooks.SessionEnd(context.Background())
	if c.sessionDir != "" {
		c.mu.Lock()
		c.sessionPath = agent.NewSessionPath(c.sessionDir, c.label)
		c.mu.Unlock()
	}
	c.executor.SetSession(agent.NewSession(c.systemPrompt))
	c.rebindCheckpoints(c.SessionPath())
	// Reset V3.0 TCCA state so the new session starts clean.
	if c.ctxMgr != nil {
		c.ctxMgr.Flow().ReplaceMessages(nil)
	}
	c.mu.Lock()
	c.startedOnce = true // NewSession fires SessionStart itself; don't re-fire on the next turn
	c.mu.Unlock()
	c.hooks.SessionStart(context.Background())
	return nil
}

// RewindScope selects what a Rewind restores.
type RewindScope int

const (
	RewindCode         RewindScope = iota // files only
	RewindConversation                    // message log only
	RewindBoth                            // both
)

// Checkpoints lists the session's rewind points (one per user turn), oldest first.
func (c *Controller) Checkpoints() []checkpoint.Meta {
	if c.cp == nil {
		return nil
	}
	return c.cp.List()
}

// rewindFail emits the error as a Warn notice (so a frontend that swallows the
// returned error — e.g. the desktop bridge's .catch — still shows the user why
// the rewind did nothing) and returns it.
func (c *Controller) rewindFail(err error) error {
	c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: err.Error()})
	return err
}

// Rewind restores the session to the start of `turn`: Code reverts every file that
// turn (or a later one) changed to its pre-turn content; Conversation truncates the
// message log back to that turn; Both does both. Refused while a turn is running.
// Conversation rewind relies on the live boundary recorded at turn start, so it is
// unavailable for turns inherited from a resumed session (code rewind still works).
// Frontends re-render their transcript from History after the call.
func (c *Controller) Rewind(turn int, scope RewindScope) error {
	if c.cp == nil || c.executor == nil {
		return c.rewindFail(fmt.Errorf("checkpoints unavailable"))
	}
	c.mu.Lock()
	running := c.running
	boundary, hasBound := c.cpBound[turn]
	c.mu.Unlock()
	if running {
		return c.rewindFail(fmt.Errorf("cannot rewind while a turn is running"))
	}

	if scope == RewindCode || scope == RewindBoth {
		written, deleted, err := c.cp.RestoreCode(turn)
		if err != nil {
			return c.rewindFail(fmt.Errorf("rewind code: %w", err))
		}
		c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
			Text: fmt.Sprintf("rewound code to turn %d — %d file(s) restored, %d removed", turn, len(written), len(deleted))})
	}
	if scope == RewindConversation || scope == RewindBoth {
		if !hasBound {
			return c.rewindFail(fmt.Errorf("conversation rewind unavailable for turn %d (resumed session)", turn))
		}
		s := c.executor.Session()
		if boundary <= len(s.Messages) {
			s.Messages = s.Messages[:boundary]
			c.mu.Lock()
			c.cpTurn = turn // renumber future turns from here; later turns are gone
			for k := range c.cpBound {
				if k >= turn {
					delete(c.cpBound, k)
				}
			}
			c.mu.Unlock()
			if err := c.Snapshot(); err != nil {
				slog.Warn("controller: snapshot after rewind", "err", err)
			}
		}
		c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
			Text: fmt.Sprintf("rewound conversation to turn %d", turn)})
	}
	return nil
}

// Fork branches the conversation at the start of turn into a NEW session file,
// preserving the current one as the branch point, and switches to the branch. Code
// is untouched (it's a conversation operation). Like a conversation rewind it needs
// the live boundary, so it is unavailable for resumed-session turns and refused
// while a turn runs. Returns the new session path.
func (c *Controller) Fork(turn int) (string, error) {
	return c.ForkNamed(turn, "")
}

func (c *Controller) ForkNamed(turn int, name string) (string, error) {
	if c.executor == nil {
		return "", c.rewindFail(fmt.Errorf("checkpoints unavailable"))
	}
	if c.sessionDir == "" {
		return "", c.rewindFail(fmt.Errorf("fork needs session persistence, which is disabled"))
	}
	c.mu.Lock()
	running := c.running
	boundary, hasBound := c.cpBound[turn]
	c.mu.Unlock()
	if running {
		return "", c.rewindFail(fmt.Errorf("cannot fork while a turn is running"))
	}
	if !hasBound {
		return "", c.rewindFail(fmt.Errorf("fork unavailable for turn %d (resumed session)", turn))
	}

	// Persist the current conversation first so the branch point survives, then
	// seed a fresh session with the messages up to the fork and switch to it.
	if err := c.Snapshot(); err != nil {
		slog.Warn("controller: pre-fork snapshot", "err", err)
	}
	parentPath := c.SessionPath()
	parentID := agent.BranchID(parentPath)
	src := c.executor.Session().Snapshot()
	if boundary > len(src) {
		boundary = len(src)
	}
	forked := append([]provider.Message(nil), src[:boundary]...)
	sess := agent.NewSession("")
	sess.Messages = forked

	newPath := agent.NewSessionPath(c.sessionDir, c.label)
	if err := sess.Save(newPath); err != nil {
		return "", c.rewindFail(err)
	}
	if err := agent.SaveBranchMeta(newPath, agent.BranchMeta{
		Name:             strings.TrimSpace(name),
		ParentID:         parentID,
		ForkTurn:         turn,
		ForkMessageIndex: boundary,
	}); err != nil {
		return "", c.rewindFail(err)
	}
	c.executor.SetSession(sess)
	c.mu.Lock()
	c.sessionPath = newPath
	c.mu.Unlock()
	c.rebindCheckpoints(newPath)
	c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
		Text: fmt.Sprintf("forked conversation at turn %d into a new session", turn)})
	return newPath, nil
}

// Branch copies the current conversation into a child branch and switches to it.
// Unlike Fork, it branches at the current tip and does not require a checkpoint.
func (c *Controller) Branch(name string) (string, error) {
	if c.executor == nil {
		return "", c.rewindFail(fmt.Errorf("branch unavailable"))
	}
	if c.sessionDir == "" {
		return "", c.rewindFail(fmt.Errorf("branch needs session persistence, which is disabled"))
	}
	c.mu.Lock()
	running := c.running
	c.mu.Unlock()
	if running {
		return "", c.rewindFail(fmt.Errorf("cannot branch while a turn is running"))
	}
	if !c.executor.Session().HasContent() {
		return "", c.rewindFail(fmt.Errorf("nothing to branch yet"))
	}
	if err := c.Snapshot(); err != nil {
		return "", c.rewindFail(err)
	}
	parentPath := c.SessionPath()
	parentID := agent.BranchID(parentPath)
	src := c.executor.Session().Snapshot()
	branched := append([]provider.Message(nil), src...)
	sess := agent.NewSession("")
	sess.Messages = branched

	newPath := agent.NewSessionPath(c.sessionDir, c.label)
	if err := sess.Save(newPath); err != nil {
		return "", c.rewindFail(err)
	}
	if err := agent.SaveBranchMeta(newPath, agent.BranchMeta{
		Name:             strings.TrimSpace(name),
		ParentID:         parentID,
		ForkTurn:         -1,
		ForkMessageIndex: len(branched),
	}); err != nil {
		return "", c.rewindFail(err)
	}
	c.executor.SetSession(sess)
	c.mu.Lock()
	c.sessionPath = newPath
	c.mu.Unlock()
	c.rebindCheckpoints(newPath)
	c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
		Text: fmt.Sprintf("created branch %s", agent.BranchID(newPath))})
	return newPath, nil
}

// Branches lists saved conversation branches in this controller's session dir.
func (c *Controller) Branches() ([]agent.BranchInfo, error) {
	if c.sessionDir == "" {
		return nil, fmt.Errorf("session persistence is disabled")
	}
	if err := c.Snapshot(); err != nil {
		return nil, err
	}
	return agent.ListBranches(c.sessionDir)
}

func (c *Controller) SwitchBranch(ref string) (agent.BranchInfo, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return agent.BranchInfo{}, c.rewindFail(fmt.Errorf("usage: /switch <branch id|name>"))
	}
	c.mu.Lock()
	running := c.running
	c.mu.Unlock()
	if running {
		return agent.BranchInfo{}, c.rewindFail(fmt.Errorf("cannot switch branches while a turn is running"))
	}
	branches, err := c.Branches()
	if err != nil {
		return agent.BranchInfo{}, c.rewindFail(err)
	}
	match, err := resolveBranch(branches, ref)
	if err != nil {
		return agent.BranchInfo{}, c.rewindFail(err)
	}
	loaded, err := agent.LoadSession(match.Path)
	if err != nil {
		return agent.BranchInfo{}, c.rewindFail(err)
	}
	if c.executor != nil {
		c.executor.SetSession(loaded)
	}
	c.mu.Lock()
	c.sessionPath = match.Path
	c.mu.Unlock()
	c.rebindCheckpoints(match.Path)
	c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
		Text: fmt.Sprintf("switched to branch %s", branchDisplayName(match))})
	return match, nil
}

func resolveBranch(branches []agent.BranchInfo, ref string) (agent.BranchInfo, error) {
	refLower := strings.ToLower(ref)
	var matches []agent.BranchInfo
	for _, b := range branches {
		nameLower := strings.ToLower(strings.TrimSpace(b.Name))
		switch {
		case b.ID == ref || strings.EqualFold(b.ID, ref):
			return b, nil
		case b.Name != "" && nameLower == refLower:
			matches = append(matches, b)
		case strings.HasPrefix(strings.ToLower(b.ID), refLower):
			matches = append(matches, b)
		case b.Path == ref:
			return b, nil
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return agent.BranchInfo{}, fmt.Errorf("branch %q is ambiguous", ref)
	}
	return agent.BranchInfo{}, fmt.Errorf("branch %q not found", ref)
}

func branchDisplayName(b agent.BranchInfo) string {
	if strings.TrimSpace(b.Name) != "" {
		return fmt.Sprintf("%s (%s)", b.Name, b.ID)
	}
	return b.ID
}

// SummarizeFrom compresses the conversation from turn onward into one summary;
// SummarizeUpTo compresses everything before it. Both are Claude Code's "summarize
// from/up to here" — they restructure the message log (keeping code untouched), so
// afterwards the per-turn boundaries no longer map and conversation rewind/fork
// report "unavailable" until new turns rebuild them (code rewind, file-based, is
// unaffected). Refused while a turn runs; need the live boundary.
func (c *Controller) SummarizeFrom(ctx context.Context, turn int) error {
	return c.summarizeAt(ctx, turn, true)
}

func (c *Controller) SummarizeUpTo(ctx context.Context, turn int) error {
	return c.summarizeAt(ctx, turn, false)
}

func (c *Controller) summarizeAt(ctx context.Context, turn int, from bool) error {
	if c.executor == nil {
		return c.rewindFail(fmt.Errorf("checkpoints unavailable"))
	}
	c.mu.Lock()
	running := c.running
	boundary, hasBound := c.cpBound[turn]
	c.mu.Unlock()
	if running {
		return c.rewindFail(fmt.Errorf("cannot summarize while a turn is running"))
	}
	if !hasBound {
		return c.rewindFail(fmt.Errorf("summarize unavailable for turn %d (resumed session)", turn))
	}
	var err error
	if from {
		err = c.executor.SummarizeFrom(ctx, boundary)
	} else {
		err = c.executor.SummarizeUpTo(ctx, boundary)
	}
	if err != nil {
		return c.rewindFail(err)
	}
	// The log was restructured; existing boundaries no longer map. Drop them (keep
	// cpTurn monotonic so new turns don't collide with the store) — conversation
	// rewind degrades to "unavailable" until fresh turns rebuild boundaries.
	c.mu.Lock()
	c.cpBound = map[int]int{}
	c.mu.Unlock()
	if err := c.Snapshot(); err != nil {
		slog.Warn("controller: post-summarize snapshot", "err", err)
	}
	return nil
}

// Resume seeds the session from a loaded transcript and pins the active file to
// its path so auto-save keeps appending there.
func (c *Controller) Resume(s *agent.Session, path string) {
	if c.executor != nil {
		c.executor.SetSession(s)
	}
	c.mu.Lock()
	c.sessionPath = path
	c.mu.Unlock()
	c.rebindCheckpoints(path)
}

// Snapshot writes the executor's conversation to the active session file. No-op
// when persistence is unavailable or the session has never been used (no user
// interaction). Called after every turn so a crash loses at most one in-flight
// prompt.
func (c *Controller) Snapshot() error {
	c.mu.Lock()
	path := c.sessionPath
	c.mu.Unlock()
	if c.executor == nil || path == "" {
		return nil
	}
	s := c.executor.Session()
	if !s.HasContent() {
		return nil
	}
	if err := s.Save(path); err != nil {
		return err
	}
	return agent.TouchBranchMeta(path)
}

// SetSessionPath pins where auto-save lands (a fresh session file minted by the
// caller when no resume path applies).
func (c *Controller) SetSessionPath(p string) {
	c.mu.Lock()
	c.sessionPath = p
	c.mu.Unlock()
	c.rebindCheckpoints(p)
}

// SessionDir reports the directory new session files land in ("" disables
// persistence), so the caller can decide whether to mint a path.
func (c *Controller) SessionDir() string { return c.sessionDir }

// SessionPath reports the file the current conversation auto-saves to ("" when
// persistence is disabled), so a history view can mark the active session.
func (c *Controller) SessionPath() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionPath
}

// History returns the executor's current message log (for repopulating a
// resumed frontend's view).
func (c *Controller) History() []provider.Message {
	if c.executor == nil {
		return nil
	}
	return c.executor.Session().Snapshot() // copy — a turn may be appending concurrently
}

// ContextSnapshot returns (promptTokens, contextWindow) from the most recent
// turn. Both zero means no data yet — a gauge hides itself.
func (c *Controller) ContextSnapshot() (int, int) {
	if c.executor == nil {
		return 0, 0
	}
	u := c.executor.LastUsage()
	if u == nil {
		return 0, c.executor.ContextWindow()
	}
	return u.PromptTokens, c.executor.ContextWindow()
}

// CompactRatio returns the auto-compaction threshold as a fraction of the window
// (0 when the executor is unset). The status line shows headroom against it.
func (c *Controller) CompactRatio() float64 {
	if c.executor == nil {
		return 0
	}
	return c.executor.CompactRatio()
}

// LastUsage returns the most recent turn's token telemetry (nil before the first
// turn), so frontends can derive the prompt cache-hit rate for the status line.
func (c *Controller) LastUsage() *provider.Usage {
	if c.executor == nil {
		return nil
	}
	return c.executor.LastUsage()
}

// SessionCache returns cumulative cache hit/miss prompt tokens for the session,
// so a frontend can render the aggregate (session-wide) cache-hit rate — steadier
// than the single-turn rate and unaffected by compaction.
func (c *Controller) SessionCache() (hit, miss int) {
	if c.executor == nil {
		return 0, 0
	}
	return c.executor.SessionCache()
}

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
func (c *Controller) AddMCPServer(e config.PluginEntry) (int, error) {
	n, err := c.connectMCPServer(e)
	if err != nil {
		return 0, err
	}
	cfg, lerr := config.Load()
	if lerr != nil {
		return n, fmt.Errorf("connected, but reloading config to save failed: %w", lerr)
	}
	if err := cfg.UpsertPlugin(e); err != nil {
		return n, fmt.Errorf("connected, but config rejected the entry: %w", err)
	}
	if err := cfg.Save(); err != nil {
		return n, fmt.Errorf("connected, but saving config failed: %w", err)
	}
	return n, nil
}

func (c *Controller) connectMCPServer(e config.PluginEntry) (int, error) {
	exp := e.ExpandedPlugin()
	return c.connectMCPSpec(plugin.Spec{
		Name:    exp.Name,
		Type:    exp.Type,
		Command: exp.Command,
		Args:    exp.Args,
		Env:     exp.Env,
		URL:     exp.URL,
		Headers: exp.Headers,
	})
}

func (c *Controller) connectMCPSpec(s plugin.Spec) (int, error) {
	if c.host == nil {
		c.host = plugin.NewHost()
	}
	tools, err := c.host.Add(c.pluginCtx, s)
	if err != nil {
		return 0, err
	}
	if c.reg != nil {
		for _, t := range tools {
			c.reg.Add(t)
		}
	}
	return len(tools), nil
}

func (c *Controller) ConfiguredMCPNames() []string {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Plugins))
	for _, p := range cfg.Plugins {
		names = append(names, p.Name)
	}
	return names
}

func (c *Controller) DisconnectedMCPNames() []string {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	connected := map[string]bool{}
	if c.host != nil {
		for _, name := range c.host.ServerNames() {
			connected[name] = true
		}
	}
	var names []string
	for _, p := range cfg.Plugins {
		if !connected[p.Name] {
			names = append(names, p.Name)
		}
	}
	return names
}

func (c *Controller) ConnectConfiguredMCPServer(name string) (int, error) {
	cfg, err := config.Load()
	if err != nil {
		return 0, err
	}
	for _, p := range cfg.Plugins {
		if p.Name == name {
			return c.connectMCPServer(p)
		}
	}
	if name == "codegraph" {
		return c.connectCodegraphMCPServer(cfg)
	}
	return 0, fmt.Errorf("no configured MCP server named %q", name)
}

func (c *Controller) connectCodegraphMCPServer(cfg *config.Config) (int, error) {
	if !cfg.Codegraph.Enabled {
		return 0, fmt.Errorf("codegraph is disabled in config")
	}
	bin, ok := codegraph.Resolve(cfg.Codegraph.Path)
	if !ok {
		return 0, fmt.Errorf("codegraph is not installed")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	if err := codegraph.EnsureInit(c.pluginCtx, bin, cwd); err != nil {
		return 0, fmt.Errorf("codegraph init: %w", err)
	}
	return c.connectMCPSpec(plugin.Spec{Name: "codegraph", Command: bin, Args: []string{"serve", "--mcp"}, Dir: cwd})
}

// RemoveMCPServer disconnects a live MCP server — its tools vanish from the next
// turn — and removes it from the config file. It reports whether a live server was
// disconnected; an error only when the name is neither connected nor in config (or
// the config save fails). A server declared in .mcp.json disconnects for this
// session but returns on the next start, since that file isn't ours to edit.
func (c *Controller) RemoveMCPServer(name string) (disconnected bool, err error) {
	if c.host != nil {
		if prefix, ok := c.host.Remove(name); ok {
			disconnected = true
			if c.reg != nil {
				c.reg.RemovePrefix(prefix)
			}
		}
	}
	cfg, lerr := config.Load()
	if lerr != nil {
		return disconnected, lerr
	}
	inConfig := cfg.RemovePlugin(name)
	if inConfig {
		if serr := cfg.Save(); serr != nil {
			return disconnected, serr
		}
	}
	if !disconnected && !inConfig {
		return false, fmt.Errorf("no MCP server named %q", name)
	}
	return disconnected, nil
}

// DisconnectMCPServer disconnects a live server for this session without touching
// config — the connector toggle's "off". Its tools vanish next turn; it reconnects
// on the next session start, or now via ConnectConfiguredMCPServer (the "on").
// Reports whether a live server was actually disconnected.
func (c *Controller) DisconnectMCPServer(name string) bool {
	if c.host == nil {
		return false
	}
	prefix, ok := c.host.Remove(name)
	if ok && c.reg != nil {
		c.reg.RemovePrefix(prefix)
	}
	return ok
}

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

// SetBypass turns YOLO/bypass mode on or off for the session: while on, every
// approval prompt is auto-allowed (writers and bash run without asking). Deny
// rules still block. Runtime-only — never written to config.
func (c *Controller) SetBypass(on bool) {
	c.mu.Lock()
	c.bypass = on
	c.mu.Unlock()
}

// Bypass reports whether YOLO/bypass mode is on, for the status-bar indicator.
func (c *Controller) Bypass() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bypass
}
