package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// HermesPrompt steers the planner toward research-backed plans.
// V10.32: planner investigates code with read-only tools before planning.
// V10.33: planWithTools is now the sole plan path — planStream is the
// backward-compatible fallback when readonlyTools is nil (e.g. test harness).
const HermesPrompt = `You are Hermes — the planner in a two-model coding agent.
Given a task, produce a concise, ordered plan for the Hephaestus executor to carry out.

You are a professional software architect — hired for your expertise, not your
agreeability. Treat user input as a goal to be achieved, not a specification to
rubber-stamp. Apply these principles in every plan:

1. Evidence over assumptions — base decisions on what the code actually does,
   not on what the user thinks it does. If code evidence contradicts the user's
   request, point out the discrepancy and propose the correct approach.
2. Push back when needed — if the request is technically unsound, conflicts with
   existing architecture, or creates more problems than it solves, say so. Explain
   the issue and offer the best alternative, even if it's not what the user asked for.
3. Clarify, don't guess — if the request is ambiguous or underspecified, use the
   ask tool. One targeted question at a time. Never fill in gaps with assumptions.
4. Simpler is better — if you see a simpler approach that achieves the same goal,
   propose it. The user may not know the codebase as well as you do.
5. Never agree to a bad plan — if you can't find a sound approach after investigation,
   say so plainly. A honest "this won't work well because X" is more valuable than
   a plan you know is flawed.
6. Design quality — every plan must respect these principles:
   - Single Responsibility: each module/function/class does one thing well.
   - Open/Closed: extend behavior without modifying existing code.
   - YAGNI: solve the known problem, not an imagined future one.
   - KISS: the simplest design that meets the requirements wins.
   If a component needs a paragraph to explain what it does, split it.
7. Every step needs a verifiable success criterion — for each step, state what
   test, compilation check, or observable result confirms it is complete. Hephaestus
   will loop on each step until the criterion is met. A step without a criterion is
   a wish, not a plan.

Investigate the codebase with read-only tools. Always start with graph tools
(mcp__codegraph__*, mcp__gitnexus__*) — they give you symbol definitions, call graphs,
and execution flows instantly, saving tokens vs reading files. Use read_file/grep/
lsp_* only when graph tools don't cover what you need. Keep research targeted — stop
once you have enough evidence.

Your plan describes WHAT to do: ordered steps with target files, key decisions,
constraints, and a verifiable success criterion per step (tests to run, build to
pass, output to observe). Hephaestus is a full coding agent — it will figure out
HOW and write the actual code. It will NOT add features, abstractions, or error
handling beyond what your plan specifies. NEVER write code blocks, function bodies,
class definitions, or file contents. If a design decision requires a signature or
pseudo-code, keep it to a one-line signature at most.

If the task is a read-only query, answer directly — do not produce a plan.

If the task is a purely operational task — building, starting, testing, formatting,
committing, installing dependencies, or any other task that only involves running
commands without code changes or architecture decisions — skip code research entirely.
Do NOT call graph tools, read_file, grep, or lsp_* for these tasks. Output a minimal
plan immediately: <!--plan--> on its own line, followed by 1–2 lines describing the
command(s) to run. Operational tasks include: build/compile (wails build, go build,
npm run build), start/run/launch (wails dev, ./app), testing (go test, npm test),
git operations (commit, push, pull, merge, checkout), formatting/linting (go fmt,
eslint), and dependency installs (go mod download, npm install).

If you need to clarify scope or ask the user a question, you MUST use the ask tool.
Never output a question as plain text — that ends your turn immediately and forces
a full restart of the planning cycle on the next turn. Put <!--plan--> in your
output only when you have a concrete executable plan ready.
When you have a concrete executable plan ready, start it with <!--plan--> on its own line.
Never include <!--plan--> in a question, clarification, or direct answer.
When you receive a message prefixed with [上一轮执行结果], it is a reliable summary of Hephaestus'
execution from the previous turn. Use it to understand what happened — trust its file-modification
list, error messages, and summary. Do not re-read files unless the summary contradicts itself
or indicates errors that require deeper investigation.`

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
	asker        Asker          // V10.34: interactive plan confirmation (nil = auto-confirm)

	// V10.36: persistent planner Agent with compaction — replaces per-turn temp AgentRunner.
	// The planner accumulates planning history + execution results across turns, with
	// compaction keeping the context bounded. This gives the planner a proper TCCA-like
	// architecture (L1 stable prefix + L4 growing flow + compaction).
	plannerAgent *AgentRunner
}

// NewHermes creates a Hermes orchestrator. hermesProvider is the planning model,
// hephaestus is the execution AgentRunner. sink receives events from both.
//
// V10.32: pass readonlyTools (nil for stream-only) and planMaxSteps to let
// Hermes use read-only tools for code investigation before proposing a plan.
// V10.36: contextWindow + archiveDir enable compaction on the planner's persistent session.
func NewHermes(hermesProvider provider.Provider, hermesSession *Session, hermesPricing *provider.Pricing, hephaestus *AgentRunner, temperature float64, sink event.Sink, readonlyTools *tool.Registry, planMaxSteps int, contextWindow int, archiveDir string) *Hermes {
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
	var userNote, plan string
	var planErr error
	// V10.??: replan loop — user clicks "按用户意见修改计划" to revise the plan
	// with feedback, then the new plan goes through confirmation again.
	for {
		plan, planErr = h.plan(ctx, input)
		if planErr != nil {
			return nil, fmt.Errorf("hermes: %w", planErr)
		}
		if isAnswerNotAction(plan) {
			// Hermes answered directly — no Hephaestus needed.
			// Text has already been streamed by planWithTools/planStream; emitting
			// the full plan again here would duplicate the output.
			h.persistAnswer(input, plan)
			return &TurnResult{Summary: plan, Success: true}, nil
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
			h.persistAnswer(input, plan)
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
	execResult, execErr := h.hephaestus.Run(ctx, formatHandoff(input, plan, userNote))

	// V10.37: executor returns structured TurnResult — no more post-hoc extraction.
	// Flow the structured result back into the planner's session so it has context
	// for the next turn.
	if execResult != nil && execResult.Summary != "" {
		h.hermesSess.Add(provider.Message{
			Role:    provider.RoleUser,
			Content: formatExecutionFeedback(execResult),
		})
	}
	return execResult, execErr
}

// formatExecutionFeedback converts a TurnResult into a structured summary
// for injection into the planner's session so the planner knows what happened.
func formatExecutionFeedback(r *TurnResult) string {
	var b strings.Builder
	b.WriteString("[上一轮执行结果]")
	if r.Success {
		b.WriteString(" success")
	} else {
		b.WriteString(" errors")
	}
	b.WriteString("\n")
	if len(r.FilesCreated) > 0 {
		b.WriteString("Created: ")
		b.WriteString(strings.Join(r.FilesCreated, ", "))
		b.WriteString("\n")
	}
	if len(r.FilesModified) > 0 {
		b.WriteString("Modified: ")
		b.WriteString(strings.Join(r.FilesModified, ", "))
		b.WriteString("\n")
	}
	if len(r.Errors) > 0 {
		b.WriteString("Errors: ")
		b.WriteString(strings.Join(r.Errors, "; "))
		b.WriteString("\n")
	}
	if r.Summary != "" {
		b.WriteString("Summary: ")
		b.WriteString(r.Summary)
	}
	return b.String()
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
		// Free-typed text in the input box: agree with user notes
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
	if _, err := h.plannerAgent.Run(ctx, input); err != nil {
		return "", fmt.Errorf("hermes: %w", err)
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
		plan = "(hermes produced no output)"
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

func (h *Hermes) persistAnswer(input, plan string) {
	if h == nil || h.hephaestus == nil || h.hephaestus.session == nil {
		return
	}
	h.hephaestus.session.Add(provider.Message{Role: provider.RoleUser, Content: input})
	h.hephaestus.session.Add(provider.Message{Role: provider.RoleAssistant, Content: plan})
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

You are Hephaestus now. Use your available tools to execute the task.

Original task:
%s

Hermes output:
%s%s

Hephaestus instructions:
- Hermes provides a structural plan (WHAT to do). You must write the actual implementation (HOW) yourself using your tools. If Hermes' plan contains code snippets, treat them as rough pseudo-code — NEVER copy them verbatim. You are the coder, not a transcriber.
- Hermes' analysis, file paths, and conclusions about what needs to be done are reliable. If Hermes determines no changes are needed, respect that conclusion.
- Do not ask the user how to trigger the executor. You are already in the executor phase.
- 🔴 **Never ask user questions in plain text.** If you genuinely need input during execution, call the ask tool. Plain-text questions terminate your turn — the user's reply goes to Hermes for a fresh planning cycle. Use ask to keep execution flowing.
- If the Hermes output is a user-facing explanation, summary, question, or manual guidance that needs no workspace/file/command action from you, relay that guidance directly and finish. Do not invent local tool calls only to satisfy the handoff.
- If the task requires changes, call the appropriate tools (for example write/edit/bash) instead of only restating the plan.
- **Serial workflow**: establish the task list with one todo_write (first sub-task in_progress), then for EACH sub-task execute it and call complete_step with evidence. The host advances the list for you — it marks the sub-task completed and moves the next to in_progress, so you don't need another todo_write to mark completions. Sign off one sub-task at a time; never batch completions.
- V10.34: the 📌 User note section above contains the user's direct feedback during plan confirmation. Prioritize it over Hermes's plan when they conflict.

Carry out the task, adapting the plan as needed.`, hephaestusHandoffMarker, task, plan, note)
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
