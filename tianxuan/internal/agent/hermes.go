package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"tianxuan/internal/codegraph"
	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// HermesPrompt steers the planner toward research-backed plans.
// V10.32: planner investigates code with read-only tools before planning.
// V10.33: planWithTools is now the sole plan path — planStream is the
// backward-compatible fallback when readonlyTools is nil (e.g. test harness).
const HermesPrompt = `You are Hermes — the planner in a two-model coding agent.
You investigate code with read-only tools, then write plans for Hephaestus to execute.

Your tools: read_file, grep, glob, lsp_*, codegraph, gitnexus — read-only.
You do NOT have bash, write, edit, or any side-effect tool. Never dwell on
this; it is by design. Hephaestus has those tools.

## Output

- Direct answer — no marker, no plan. User just needs information.
- Ask — use the ask tool when you need a decision you cannot make.
- Plan — open with <!--plan-->, then write steps. Code changes needed.
- No-op — investigation shows nothing to do: say so, stop, no marker.

3–8 steps. Format each step as:

  步骤 N：简短标题
  - **File(s)**：verified paths, or [NEW] for new files
  - **Change**：one sentence — what changes, on which symbol
  - **Depends on**：step number(s), or 无

Plan WHAT, not HOW. No code blocks, no function bodies.

## Hephaestus executes literally

Hephaestus has zero judgment. Wrong path → wrong file changed. Missing step →
step skipped. Vague instruction → random guess. Your plan is the only spec.

- Surgical: only touches the files you list. Directories as targets → nothing happens.
- Minimal: no interfaces, factories, base classes unless multiple callers exist.
- Errors surface (return err / panic), never silently swallowed.
- No TODO / placeholder. Every step must be runnable as written.
- Bug fix: reproduce step before any fix step.

After execution you receive [上一轮执行结果] with created/modified files,
per-step ✅/❌, and a summary. Trust the file list; re-read only when the
summary flags unresolved issues.

## UI design

When the task involves any visual output — pages, components, layout,
colors, typography — call read_skill(name="ui-ux-pro-max") and follow
its guidance. Never invent design parameters on your own.`

// HephaestusSystemPrompt is the executor's system prompt (L2 layer).
// Injected into the executor session at boot time so DeepSeek prefix cache
// treats the full L1+L2 as a stable prefix, instead of repeating the execution
// contract in every handoff user message.
const HephaestusSystemPrompt = `You are Hephaestus — the executor. Carry out Hermes' plan.

## Your partner Hermes

Hermes investigated the codebase. Its file paths and design decisions are
reliable. Do NOT redesign or question the approach unless reality contradicts
the plan (wrong path, missing function, incompatible API). Report any
deviation in complete_step evidence.

## 1. Think Before Coding

- Read the FULL plan before touching any file.
- Create todo items with todo_write: N steps → N items, first as in_progress.
- Scan dependencies. Never start before understanding what each step needs.

## 2. Simplicity First

- No features, abstractions, or error handling beyond the plan.
- No interfaces, base classes, or factories for single-use code.
- If 50 lines would do, don't write 200.
- Ask: would a senior engineer call this overcomplicated?

## 3. Surgical Changes

- Touch ONLY the files Hermes listed. Nothing else.
- Don't "improve" adjacent code, comments, or formatting.
- Match existing style. Remove only imports/variables YOUR changes orphaned.
- Test: every changed line traces to a plan step.

## 4. Goal-Driven Execution

TDD cycle per step: write failing test → confirm it fails → minimal code →
confirm it passes → complete_step with verifiable evidence (build output,
test results, diff). Never mark a step complete without evidence.

complete_step result field: one-line key output per step, so later steps
can reference it without re-reading files. Example:
"新增了 quoteFilePaths，位于 agent_helpers.go:95"

## Parallel first

Scan dependency graph before starting. Any 2+ steps with Depends on met
and disjoint file lists → dispatch via parallel_tasks, collect results,
complete_step with aggregates. Serial only when dependencies or shared
files force it.

## Failure handling

- Reproduce → isolate root cause → fix. Don't guess.
- 1 retry per failure. 3 failures on same step → STOP, report to Hermes.
- Never skip a failing step to hide it.

## End-of-turn report

After all steps: [步骤完成情况] — one line per step:
Step N — ✅/❌ — key output — file paths

- Use the ask tool when you need a real user decision (scope, approach, risk).
  Don't ask procedural questions — you're already executing.
- 📌 User note in handoff overrides Hermes' plan when they conflict.`

const hephaestusHandoffMarker = "tianxuan hephaestus handoff"

// Hermes runs two models in separate sessions to keep each one's prompt
// prefix cache-stable: a low-frequency planner proposes an approach, then the
// executor (a full tool-using AgentRunner) carries it out. The sessions never
// mix, so neither model's prefix is disturbed by the other's turns.
//
// V10.32: when readonlyTools is set, the planner uses AgentRunner with
// read-only tools (read_file/grep/glob/web_search/...) so it can investigate
// the codebase before proposing a plan. planMaxSteps caps planner turns.
type Hermes struct {
	hermesProvider provider.Provider
	hermesSess     *Session
	hermesSystem   string
	hermesPricing  *provider.Pricing
	hephaestus     *AgentRunner
	temperature    float64
	sink           event.Sink

	readonlyTools *tool.Registry // V10.32: if set, planner runs as AgentRunner
	planMaxSteps  int            // max planner tool-call turns (<=0 = unlimited)
	asker         Asker          // V10.34: interactive plan confirmation (nil = auto-confirm)

	// V10.36: persistent planner Agent with compaction — replaces per-turn temp AgentRunner.
	// The planner accumulates planning history + execution results across turns, with
	// compaction keeping the context bounded. This gives the planner a proper TCCA-like
	// architecture (L1 stable prefix + L4 growing flow + compaction).
	plannerAgent *AgentRunner

	// V10.54: workspace root for project map injection.
	wsRoot          string
	lastProjectHash string // hash of last injected ProjectMap; "" means not injected yet or stale
}

// NewHermes creates a Hermes orchestrator. hermesProvider is the planning model,
// hephaestus is the execution AgentRunner. sink receives events from both.
//
// V10.32: pass readonlyTools (nil for stream-only) and planMaxSteps to let
// Hermes use read-only tools for code investigation before proposing a plan.
// V10.36: contextWindow + archiveDir enable compaction on the planner's persistent session.
func NewHermes(hermesProvider provider.Provider, hermesSession *Session, hermesPricing *provider.Pricing, hephaestus *AgentRunner, temperature float64, sink event.Sink, readonlyTools *tool.Registry, planMaxSteps int, contextWindow int, archiveDir string, wsRoot string) *Hermes {
	if hermesSession == nil {
		hermesSession = NewSession("")
	}
	hermesSystem := sessionSystemPrompt(hermesSession)
	h := &Hermes{
		hermesProvider: hermesProvider,
		hermesSess:     hermesSession,
		hermesSystem:   hermesSystem,
		hermesPricing:  hermesPricing,
		hephaestus:     hephaestus,
		temperature:    temperature,
		sink:           sink,
		readonlyTools:  readonlyTools,
		planMaxSteps:   planMaxSteps,
		wsRoot:         wsRoot,
	}
	// V10.36: create persistent planner Agent with compaction so the planner
	// accumulates history across turns without unbounded growth.
	if readonlyTools != nil {
		plannerSink := event.FuncSink(func(e event.Event) {
			// Suppress TurnStarted from the planner agent — Hermes
			// already started the turn (line 150). A redundant
			// TurnStarted would reset perTurnPlannerUsage in the
			// frontend, zeroing out the planner's cost stats.
			if e.Kind == event.TurnStarted {
				return
			}
			if e.Kind == event.Usage {
				if e.UsageSource == "" || e.UsageSource == event.UsageSourceExecutor {
					e.UsageSource = event.UsageSourcePlanner
				}
			}
			sink.Emit(e)
		})
		h.plannerAgent = New(hermesProvider, readonlyTools, hermesSession, Options{
			MaxSteps:       planMaxSteps,
			Temperature:    temperature,
			Pricing:        hermesPricing,
			Gate:           &autoGate{},
			DisableVerify:  true,
			PlannerMode:    true,
			ContextWindow:  contextWindow,
			Compaction:     CompactionConfig{ArchiveDir: archiveDir, Window: contextWindow},
		}, plannerSink)
	}
	return h
}

func sessionSystemPrompt(s *Session) string {
	if s == nil {
		return ""
	}
	for _, m := range s.Snapshot() {
		if m.Role == provider.RoleSystem {
			return m.Content
		}
	}
	return ""
}

// ResetSession discards turn-local planner history when switching
// executor sessions. Carrying the old Hermes transcript across sessions
// can make the next plan reuse unrelated tasks.
func (h *Hermes) ResetSession() {
	if h == nil {
		return
	}
	system := h.hermesSystem
	if system == "" {
		system = sessionSystemPrompt(h.hermesSess)
	}
	h.hermesSess = NewSession(system)
	// Also update the plannerAgent's session pointer so it uses the fresh
	// session. Without this, plannerAgent still writes to the old session,
	// leaking stale history into future plans across session switches.
	if h.plannerAgent != nil {
		h.plannerAgent.SetSession(h.hermesSess)
	}
}

// PlannerContext returns the planner agent's last usage and context window,
// for the status bar's per-model context gauge.
func (h *Hermes) PlannerContext() (used int, window int) {
	if h == nil || h.plannerAgent == nil {
		return 0, 0
	}
	u := h.plannerAgent.LastUsage()
	if u == nil {
		return 0, h.plannerAgent.ContextWindow()
	}
	return u.PromptTokens, h.plannerAgent.ContextWindow()
}

// SetAsker installs the interactive asker for plan confirmation (V10.34).
// nil means headless mode — plans auto-confirm without user approval.
// Also wires the asker into the plannerAgent so it can ask clarifying questions
// during planning (scope negotiation, detail gathering).
func (h *Hermes) SetAsker(a Asker) {
	h.asker = a
	if h.plannerAgent != nil {
		h.plannerAgent.SetAsker(a)
	}
}

// Run plans with the planner model, then hands the plan to the executor.
// Returns a merged TurnResult combining the planner's and executor's outcomes.
func (h *Hermes) Run(ctx context.Context, input string) (*TurnResult, error) {
	h.sink.Emit(event.Event{Kind: event.TurnStarted})

	// V10.31: fast path — skip planner for simple/quick tasks
	if task, ok := shouldSkipPlanner(input); ok {
		h.sink.Emit(event.Event{Kind: event.Phase, Text: h.hephaestus.ProvName() + " · 快速执行"})
		return h.hephaestus.Run(ctx, task)
	}

	h.sink.Emit(event.Event{Kind: event.Phase, Text: h.hermesProvider.Name() + " · hermes"})
	prePlanLen := len(h.hermesSess.Messages)

	// V10.54: inject ProjectMap into planner session for structural context.
	if h.wsRoot != "" {
		pm := codegraph.Analyze(h.wsRoot)
		pmHash := pm.Hash()
		if pmHash != h.lastProjectHash {
			h.hermesSess.Add(provider.Message{
				Role:    provider.RoleUser,
				Content: "## 项目代码图谱\n\n" + pm.Format() + "\n\n以上是当前项目的代码结构概览，规划时可直接引用其中的路径和类型名。",
			})
			h.lastProjectHash = pmHash
		}
	}

	origInput := input // preserve original user input for handoff; replan loop may modify input
	var userNote, plan string
	var planErr error
	// V10.??: replan loop — user clicks "按用户意见修改计划" to revise the plan
	// with feedback, then the new plan goes through confirmation again.
	for {
		plan, planErr = h.plan(ctx, input)
		if planErr != nil {
			// Clean up any partial messages the planner may have left in the session.
			h.hermesSess.Truncate(prePlanLen)
			return nil, fmt.Errorf("hermes: %w", planErr)
		}
		if isAnswerNotAction(plan) {
			// Hermes answered directly — no Hephaestus needed.
			// Text has already been streamed by planWithTools/planStream; emitting
			// the full plan again here would duplicate the output.
		// direct answer already in hermesSess — no persistence needed
			return &TurnResult{Summary: plan, Success: true}, nil
		}

		// Strip preamble, keep <!--plan--> at the beginning
		if idx := strings.Index(plan, "<!--plan-->"); idx >= 0 {
			plan = "<!--plan-->\n" + strings.TrimSpace(plan[idx+len("<!--plan-->"):])
		}

		var chatOnly, revise bool
		userNote, chatOnly, revise, planErr = h.confirmPlan(ctx, input, plan)
		if planErr != nil {
			// User cancelled — roll back planner session to pre-plan state.
			h.hermesSess.Truncate(prePlanLen)
			return nil, planErr
		}
		if chatOnly {
			// User chose "仅聊天" — treat as direct answer, don't dispatch executor.
		// direct answer already in hermesSess — no persistence needed
			return &TurnResult{Summary: plan, Success: true}, nil
		}
		if revise {
			// User chose "按用户意见修改计划" — append feedback and re-plan.
			if userNote != "" {
				input = input + "\n\n—— User feedback on previous plan ——\n" + userNote
			}
			prePlanLen = len(h.hermesSess.Messages) // new baseline for next round
			continue
		}
		break // execute with Hephaestus
	}
	h.sink.Emit(event.Event{Kind: event.Phase, Text: h.hephaestus.ProvName() + " · Hephaestus"})
	// Suppress the executor's TurnStarted — Hermes already started the turn.
	// Without this, the redundant TurnStarted resets perTurnPlannerUsage in the
	// frontend, zeroing out the planner's cost stats.
	execSink := h.hephaestus.Sink()
	h.hephaestus.SetSink(event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnStarted {
			return
		}
		execSink.Emit(e)
	}))
	defer h.hephaestus.SetSink(execSink)
	// V10.49: pre-inject the original Chinese input before the handoff prompt
	// so it appears at the right position in the session. History() in
	// app_session.go skips the handoff message by prefix detection.
	h.hephaestus.Session().Add(provider.Message{Role: provider.RoleUser, Content: origInput})
	execResult, execErr := h.hephaestus.Run(ctx, formatHandoff(origInput, plan, userNote))

	// V10.37: executor returns structured TurnResult — no more post-hoc extraction.
	// Feed the result back into the planner's session so it has context for the next
	// turn. Include results even on error (e.g. model crashed mid-execution) —
	// Hermes needs to know that execution failed and what was attempted.
	if execResult != nil {
		hasContent := execResult.Summary != "" || len(execResult.Errors) > 0 ||
			len(execResult.FilesCreated) > 0 || len(execResult.FilesModified) > 0
		if hasContent {
			h.hermesSess.Add(provider.Message{
				Role:    provider.RoleUser,
				Content: formatExecutionFeedback(execResult),
			})
		}

		// V10.54: detect structural changes (go.mod, package.json, new files under internal/)
		// and invalidate the project map hash so the next planning turn re-scans.
		if h.wsRoot != "" && hasStructuralChange(execResult.FilesCreated, execResult.FilesModified) {
			h.lastProjectHash = ""
		}
	}
	return execResult, execErr
}

// formatExecutionFeedback converts a TurnResult into a structured summary
// for injection into the planner's session so the planner knows what happened.
func formatExecutionFeedback(r *TurnResult) string {
	status := "success"
	if !r.Success {
		status = "errors"
	}

	created := quoteFilePaths(r.FilesCreated)
	modified := quoteFilePaths(r.FilesModified)

	errors := "(none)"
	if len(r.Errors) > 0 {
		errors = strings.Join(r.Errors, "; ")
	}

	summary := r.Summary
	if summary == "" {
		summary = "(no summary)"
	}

	conclusion := ""
	if r.Success && len(r.Errors) == 0 {
		conclusion = "\n- ✅ 任务已完成（Success=true, Errors 为空）"
	} else {
		conclusion = "\n- ⚠️ 任务未完成，请检查 Errors 并修正"
	}

	return fmt.Sprintf("[上一轮执行结果] %s\n- Created: %s\n- Modified: %s\n- Errors: %s\n- Summary: %s%s\n", status, created, modified, errors, summary, conclusion)
}

// hasStructuralChange checks whether any file path indicates a structural change
// that would invalidate the cached ProjectMap (go.mod, package.json, or internal/ paths).
func hasStructuralChange(created, modified []string) bool {
	check := func(paths []string) bool {
		for _, p := range paths {
			if p == "go.mod" || p == "package.json" || p == "Cargo.toml" {
				return true
			}
			if strings.HasPrefix(p, "internal/") && strings.HasSuffix(p, ".go") {
				return true
			}
		}
		return false
	}
	return check(created) || check(modified)
}

// confirmPlan asks the user to approve the planner's output before handing off to
// the executor. Returns the user's free-typed note ("" when none), a chatOnly
// flag, and a revise flag (= user clicked "按用户意见修改计划"), and an error on
// cancellation. In headless mode (asker == nil) it auto-confirms.
//
// The confirmation dialog shows:
//   ○ 提交执行          — 同意计划，直接交由 Hephaestus 执行
//   ○ 仅聊天            — 计划误触发，仅作为普通对话回复，不派送执行者
//   ○ 按用户意见修改计划   — 将修改意见送回 Hermes 重新规划
//   ○ 取消              — 放弃本次任务
//   📝 文本框 — 输入修改意见
//
// For "按用户意见修改计划", the note text is extracted from Selected[1] (when
// available) and returned as the first string so the caller can feed it back
// to Hermes for re-planning.
func (h *Hermes) confirmPlan(ctx context.Context, task, plan string) (note string, chatOnly bool, revise bool, err error) {
	if h.asker == nil {
		return "", false, false, nil // headless: auto-confirm
	}
	answers, err := h.asker.Ask(ctx, []event.AskQuestion{{
		ID:     "confirm",
		Header: "计划确认",
		Prompt: fmt.Sprintf("任务：%s", truncateStr(task, 200)),
		Plan:   plan, // full plan rendered by PlanCard with Markdown
		Options: []event.AskOption{
			{Label: "提交执行", Description: "按计划交由 Hephaestus 立即执行"},
			{Label: "仅聊天", Description: "计划误触发，仅作为普通对话回复，不派送执行者"},
			{Label: "按用户意见修改计划", Description: "将修改意见送回 Hermes 重新规划"},
			{Label: "取消", Description: "放弃本次任务，不做任何更改"},
		},
	}})
	if err != nil {
		return "", false, false, fmt.Errorf("plan confirmation cancelled: %w", err)
	}
	if len(answers) == 0 || len(answers[0].Selected) == 0 {
		return "", false, false, fmt.Errorf("计划被取消（无回复）")
	}
	selected := answers[0].Selected[0]
	switch selected {
	case "提交执行":
		return "", false, false, nil // agree without notes
	case "仅聊天":
		return "", true, false, nil // chat-only: don't dispatch to executor
	case "按用户意见修改计划":
		feedback := ""
		if len(answers[0].Selected) > 1 {
			feedback = answers[0].Selected[1]
		}
		return feedback, false, true, nil // revise: re-plan with feedback
	case "取消":
		return "", false, false, fmt.Errorf("计划被用户取消")
	default:
		// User typed free-text in the input box without selecting a preset option.
		// Treat as "提交执行" with the typed text as execution notes.
		return selected, false, false, nil
	}
}


// ── Plan implementation ──────────────────────────────────────────────────

// plan runs Hermes as an AgentRunner with read-only tools so it can investigate
// the codebase before proposing a plan. Falls back to planStream (zero-tool stream)
// when readonlyTools is nil — e.g. in tests or when no read-only registry is wired.
func (h *Hermes) plan(ctx context.Context, input string) (string, error) {
	// V10.32+: AgentRunner mode — planner can call read-only tools.
	// planMaxSteps <= 0 means unlimited (rely on model to stop itself).
	if h.readonlyTools != nil && h.planMaxSteps >= 0 {
		return h.planWithTools(ctx, input)
	}
	return h.planStream(ctx, input)
}

// planStream is the backward-compatible zero-tool stream fallback, used when
// Hermes is constructed without a read-only tool registry (e.g. in tests).
func (h *Hermes) planStream(ctx context.Context, input string) (string, error) {
	msgs := make([]provider.Message, len(h.hermesSess.Messages)+1)
	copy(msgs, h.hermesSess.Messages)
	msgs[len(msgs)-1] = provider.Message{Role: provider.RoleUser, Content: input}

	ch, err := h.hermesProvider.Stream(ctx, provider.Request{
		Messages:    msgs,
		Temperature: h.temperature,
	})
	if err != nil {
		return "", err
	}

	var text strings.Builder
	var usage *provider.Usage
	for chunk := range ch {
		switch chunk.Type {
		case provider.ChunkText:
			text.WriteString(chunk.Text)
			h.sink.Emit(event.Event{Kind: event.Text, Text: chunk.Text})
		case provider.ChunkUsage:
			usage = chunk.Usage
		case provider.ChunkError:
			return "", chunk.Err
		}
	}
		h.sink.Emit(event.Event{Kind: event.Usage, Usage: usage, Pricing: h.hermesPricing, UsageSource: event.UsageSourcePlanner})

	plan := text.String()
	// Persist this turn into the session so the planner accumulates history
	// across turns, matching the planWithTools path.
	h.hermesSess.Add(provider.Message{Role: provider.RoleUser, Content: input})
	h.hermesSess.Add(provider.Message{Role: provider.RoleAssistant, Content: plan})
	return plan, nil
}

// planWithTools runs the persistent planner Agent with read-only tools.
// V10.36: uses the persistent plannerAgent (created in NewHermes) instead of
// building a temporary AgentRunner each turn. The planner's session accumulates
// planning history + execution results across turns; compaction keeps it bounded.
func (h *Hermes) planWithTools(ctx context.Context, input string) (string, error) {
	if h.plannerAgent == nil {
		return "", fmt.Errorf("hermes: planner agent not initialized (no read-only tools)")
	}
	// Re-propagate asker to plannerAgent before each planning run,
	// ensuring the ask tool can interact with the user even if
	// SetAsker was called before plannerAgent was created.
	if h.asker != nil {
		h.plannerAgent.SetAsker(h.asker)
	}
	turnResult, err := h.plannerAgent.Run(ctx, input)
	if err != nil {
		return "", fmt.Errorf("hermes: %w", err)
	}
	if turnResult != nil && turnResult.Summary != "" {
		slog.Info("hermes: planner run summary", "summary", turnResult.Summary)
	}

	// Extract the plan from the planner's persistent session (last assistant message).
	var plan string
	msgs := h.hermesSess.Messages
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleAssistant {
			plan = msgs[i].Content
			break
		}
	}
	if plan == "" {
		return "", fmt.Errorf("hermes: planner produced no output")
	}
	// NOTE: <!--plan--> marker is not stripped — it's an HTML comment, invisible
	// in rendered Markdown (PlanCard) and harmless in executor prompts.

	return plan, nil
}

// autoGate approves every tool call — safe for read-only planners.
type autoGate struct{}

func (g *autoGate) Check(_ context.Context, _ string, _ json.RawMessage, _ bool) (bool, string, error) {
	return true, "", nil
}
// ── Plan helpers ─────────────────────────────────────────────────────

// shouldSkipPlanner detects tasks that are simple enough to execute directly,
// V10.34: only the explicit "!" marker skips the planner — simple and read-only
// tasks now go through Hermes for direct answers instead of bypassing it.
// Heuristic keyword matching removed: Hermes is better at classifying tasks
// than a fixed keyword list, and the direct-answer path costs one planner call.
func shouldSkipPlanner(input string) (string, bool) {
	if stripped, ok := strings.CutPrefix(input, "!"); ok {
		return strings.TrimSpace(stripped), true
	}
	return "", false
}

// isAnswerNotAction checks whether the planner's output is a direct answer
// that needs no executor. The planner self-marks executable plans with
// <!--plan--> — if absent, Hermes answered directly. No length short-circuit:
// even short plans with the <!--plan--> marker trigger confirmation.
func isAnswerNotAction(plan string) bool {
	trimmed := strings.TrimSpace(plan)
	// <!--plan--> marks executable plans; absent means direct answer.
	return !strings.Contains(trimmed, "<!--plan-->")
}

func formatHandoff(task, plan, userNote string) string {
	note := ""
	if userNote != "" {
		note = fmt.Sprintf("\n\n📌 User note (written during plan confirmation):\n%s\n", userNote)
	}
	return fmt.Sprintf(`# %s

Original task:
%s

Hermes output:
%s%s`, hephaestusHandoffMarker, task, plan, note)
}

// HandoffTask returns the original user task embedded in an executor handoff
// message, or s unchanged when it is not one. Session previews and auto-titles
// use it so dual-model sessions surface the user's words, not the handoff
// boilerplate.
func HandoffTask(s string) string {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "# "+hephaestusHandoffMarker) {
		return s
	}
	const header = "Original task:\n"
	i := strings.Index(trimmed, header)
	if i < 0 {
		return s
	}
	rest := trimmed[i+len(header):]
	if j := strings.Index(rest, "\n\nHermes output:"); j >= 0 {
		rest = rest[:j]
	}
	if task := strings.TrimSpace(rest); task != "" {
		return task
	}
	return s
}
