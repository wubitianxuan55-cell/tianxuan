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

You have two modes:

1. DIRECT ANSWER: when the task is a read-only query ("how does X work?",
   "where is Y?", "what does Z do?"), investigate with your read-only tools
   and answer directly. Do NOT produce a plan — just give the answer.
   The Hephaestus executor will NOT be invoked for pure answers.

2. PLAN: when the task requires code changes (writing, editing, deleting,
   refactoring, building, testing, etc.), investigate the codebase with your
   read-only tools, then output an ordered plan for Hephaestus to carry out.

Available tools: read_file, grep, glob, lsp_definition, web_search, web_fetch,
codegraph_*, gitnexus_*, memory_search, read_skill, git_status/diff/log.

Guidelines:
- Research before answering or planning. Base everything on actual code.
- Prefer codegraph/gitnexus over manual grep+read for broad investigation.
- If the task is too vague, ask ONE clarifying question before proceeding.
- Plans: 3-8 steps, each one line with file targets.

Output for PLAN mode:
## Plan
1. [step description] -- path/to/file.go
...

Output for DIRECT ANSWER mode: just the answer, no "## Plan" header.

Then stop and wait for user approval. Do not execute.`

const hephaestusHandoffMarker = "tianxuan hephaestus handoff"

// HermesPromptWithContext appends cache-stable standing context to the planner's
// smaller system prompt. Pass the L1 identity/memory block.
func HermesPromptWithContext(context string) string {
	context = strings.TrimSpace(context)
	if context == "" {
		return HermesPrompt
	}
	return HermesPrompt + "\n\n# Planning context\n\n" + context
}

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
}

// NewHermes creates a Hermes orchestrator. hermesProvider is the planning model,
// hephaestus is the execution AgentRunner. sink receives events from both.
//
// V10.32: pass readonlyTools (nil for stream-only) and planMaxSteps to let
// Hermes use read-only tools for code investigation before proposing a plan.
func NewHermes(hermesProvider provider.Provider, hermesSession *Session, hermesPricing *provider.Pricing, hephaestus *AgentRunner, temperature float64, sink event.Sink, readonlyTools *tool.Registry, planMaxSteps int) *Hermes {
	if hermesSession == nil {
		hermesSession = NewSession("")
	}
	hermesSystem := sessionSystemPrompt(hermesSession)
	return &Hermes{
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

// SetAsker installs the interactive asker for plan confirmation (V10.34).
// nil means headless mode — plans auto-confirm without user approval.
func (h *Hermes) SetAsker(a Asker) { h.asker = a }

// Run plans with the planner model, then hands the plan to the executor.
func (h *Hermes) Run(ctx context.Context, input string) error {
	h.sink.Emit(event.Event{Kind: event.TurnStarted})

	// V10.31: fast path — skip planner for simple/quick tasks
	if task, ok := shouldSkipPlanner(input); ok {
		h.sink.Emit(event.Event{Kind: event.Phase, Text: h.hephaestus.ProvName() + " · 快速执行"})
		return h.hephaestus.Run(ctx, task)
	}

	h.sink.Emit(event.Event{Kind: event.Phase, Text: h.hermesProvider.Name() + " · planning"})
	plan, err := h.plan(ctx, input)
	if err != nil {
		return fmt.Errorf("hermes: %w", err)
	}
	if isAnswerNotAction(plan) {
		// Hermes answered directly — no Hephaestus needed.
		h.persistAnswer(input, plan)
		h.sink.Emit(event.Event{Kind: event.Text, Text: plan})
		return nil
	}
	// V10.34: 交互式计划确认 — Hermes 展示计划，等待用户同意后再执行。
	// Headless 模式（无 asker）自动通过；用户可在确认框下方输入框填写修改意见。
	userNote, err := h.confirmPlan(ctx, input, plan)
	if err != nil {
		return err
	}
	h.sink.Emit(event.Event{Kind: event.Phase, Text: h.hephaestus.ProvName() + " · executing"})
	return h.hephaestus.Run(ctx, formatHandoff(input, plan, userNote))
}

// confirmPlan asks the user to approve the planner's output before handing off to
// the executor. Returns the user's free-typed note ("" when none) and an error on
// cancellation. In headless mode (asker == nil) it auto-confirms.
func (h *Hermes) confirmPlan(ctx context.Context, task, plan string) (string, error) {
	if h.asker == nil {
		return "", nil // headless: auto-confirm
	}
	answers, err := h.asker.Ask(ctx, []event.AskQuestion{{
		ID:     "confirm",
		Header: "计划确认",
		Prompt: fmt.Sprintf("同意此计划吗？有修改意见可在下方文本框中输入（可选）。\n\n--- 任务 ---\n%s\n\n--- 计划 ---\n%s",
			task, plan),
		Options: []event.AskOption{
			{Label: "同意，开始执行", Description: "按计划立即交由 Hephaestus 执行"},
			{Label: "取消，不执行", Description: "放弃本次任务，不做任何更改"},
		},
	}})
	if err != nil {
		return "", fmt.Errorf("plan confirmation cancelled: %w", err)
	}
	if len(answers) == 0 || len(answers[0].Selected) == 0 {
		return "", fmt.Errorf("计划被取消（无回复）")
	}
	selected := answers[0].Selected[0]
	switch selected {
	case "同意，开始执行":
		return "", nil // agree without notes
	case "取消，不执行":
		return "", fmt.Errorf("计划被用户取消")
	default:
		// Free-typed text in the input box: agree with user notes
		return selected, nil
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
	h.hermesSess.Add(provider.Message{Role: provider.RoleUser, Content: input})

	ch, err := h.hermesProvider.Stream(ctx, provider.Request{
		Messages:    h.hermesSess.Messages,
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
	h.hermesSess.Add(provider.Message{Role: provider.RoleAssistant, Content: plan})
	return plan, nil
}

// planWithTools runs Hermes as an AgentRunner with read-only tools.
func (h *Hermes) planWithTools(ctx context.Context, input string) (string, error) {
	sysPrompt := h.hermesSystem
	if sysPrompt == "" {
		sysPrompt = sessionSystemPrompt(h.hermesSess)
	}
	hermesSess := NewSession(sysPrompt)
	for _, m := range h.hermesSess.Messages {
		if m.Role == provider.RoleUser || m.Role == provider.RoleAssistant {
			hermesSess.Add(m)
		}
	}

	hermesRunner := New(h.hermesProvider, h.readonlyTools, hermesSess, Options{
		MaxSteps:       h.planMaxSteps,
		Temperature:    h.temperature,
		Pricing:        h.hermesPricing,
		Gate:           &autoGate{},
		DisableVerify:  true, // planner only investigates, never executes
	}, h.sink)

	if err := hermesRunner.Run(ctx, input); err != nil {
		return "", fmt.Errorf("hermes: %w", err)
	}

	var plan string
	for i := len(hermesSess.Messages) - 1; i >= 0; i-- {
		if hermesSess.Messages[i].Role == provider.RoleAssistant {
			plan = hermesSess.Messages[i].Content
			break
		}
	}
	if plan == "" {
		plan = "(hermes produced no output)"
	}

	h.hermesSess.Add(provider.Message{Role: provider.RoleUser, Content: input})
	h.hermesSess.Add(provider.Message{Role: provider.RoleAssistant, Content: plan})
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

// isAnswerNotAction checks whether the planner's output is a self-contained
// answer that needs no executor (Hermes answered directly — no Hephaestus).
//
// True when:
//   - Plan contains explicit no-op markers ("no changes needed", "无需改动", etc.)
//   - Plan contains NO write/action terms at all (read-only query answered by Hermes)
//
// False when: plan mentions concrete write operations needing an executor.
func isAnswerNotAction(plan string) bool {
	lower := strings.ToLower(strings.TrimSpace(plan))
	if lower == "" {
		return false // empty plan → let executor ask for clarification
	}
	// If the plan contains concrete action terms, executor is needed.
	if containsActionTerm(lower) {
		return false
	}
	// No action terms found → Hermes answered directly. No Hephaestus needed.
	return true
}

// containsActionTerm reports whether s mentions a write/destructive operation.
func containsActionTerm(lower string) bool {
	terms := []string{
		" add ", " update ", " edit ", " write ", " create ", " delete ",
		" remove ", " patch ", " refactor ", " implement ", " run ", " test ",
		" build ", " fix ", " modify ", " change ", " replace ", " rename ",
		" commit ", " merge ", " rebase ",
		"新增", "补充", "更新", "编辑", "写入", "创建", "删除", "移除",
		"运行", "测试", "构建", "修复", "实现", "重构", "修改", "替换",
	}
	padded := " " + lower + " "
	for _, term := range terms {
		if strings.Contains(padded, term) {
			return true
		}
	}
	return false
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
- Treat the Hermes output as context, not as your role or capability set.
- Hermes' analysis and conclusions about what needs to be done are reliable. If Hermes determines no changes are needed, respect that conclusion.
- Hermes has read-only tools and may have already investigated the code. Trust its file paths and symbol references.
- Do not ask the user how to trigger the executor. You are already in the executor phase.
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
