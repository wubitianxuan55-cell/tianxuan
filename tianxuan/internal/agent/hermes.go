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
Use read-only tools (codegraph, gitnexus, read_file, grep, lsp_*, web_search) to
investigate the codebase when the task needs context. Keep research targeted — stop
once you have enough evidence. Output executor-ready instructions: what to do, which
files or commands are relevant, and key decisions. Do not implement or execute.
If the task is a read-only query, answer directly — do not produce a plan.`

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
			if e.Kind == event.Usage {
				if e.UsageSource == "" || e.UsageSource == event.UsageSourceMain {
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
	// Save message count before planning — if the user later cancels, roll back
	// so the unapproved plan doesn't pollute the planner's L4 context.
	prePlanLen := len(h.hermesSess.Messages)
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
		// User cancelled — roll back planner session to pre-plan state.
		h.hermesSess.Truncate(prePlanLen)
		return err
	}
	h.sink.Emit(event.Event{Kind: event.Phase, Text: h.hephaestus.ProvName() + " · executing"})
	execErr := h.hephaestus.Run(ctx, formatHandoff(input, plan, userNote))

	// V10.36: flow execution result back to the planner's session so it has
	// context for the next turn. The planner accumulates [task → plan → execution
	// result] across turns; compaction keeps the history bounded.
	if summary := h.lastExecutorResult(); summary != "" {
		h.hermesSess.Add(provider.Message{
			Role:    provider.RoleUser,
			Content: "[system] 上一轮执行完成:\n" + summary,
		})
	}
	return execErr
}

// lastExecutorResult returns a structured summary of the executor's last turn
// for the planner's context. Extracts modified files and the conclusion gist
// without dumping raw output. The summary ends at a sentence boundary.
func (h *Hermes) lastExecutorResult() string {
	if h.hephaestus == nil || h.hephaestus.session == nil {
		return ""
	}
	msgs := h.hephaestus.session.Messages

	// Find the last assistant message (the executor's conclusion).
	var lastAssistant string
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleAssistant && msgs[i].Content != "" {
			lastAssistant = msgs[i].Content
			break
		}
	}
	if lastAssistant == "" {
		return ""
	}

	// Find files modified in this turn (from tool result names).
	files := make(map[string]bool)
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role == provider.RoleUser && strings.Contains(m.Content, hephaestusHandoffMarker) {
			break // crossed into previous turn's handoff
		}
		if m.Role == provider.RoleTool {
			switch m.Name {
			case "write_file", "edit_file", "move_file", "delete_range", "delete_symbol":
				if path := extractFilePath(m.Name, m.Content); path != "" {
					files[path] = true
				}
			}
		}
	}

	var b strings.Builder
	if len(files) > 0 {
		b.WriteString("Modified: ")
		first := true
		for f := range files {
			if !first { b.WriteString(", ") }
			b.WriteString(f)
			first = false
		}
		b.WriteString("\n")
	}
	// Truncate conclusion at last sentence boundary within 400 chars.
	conclusion := lastAssistant
	if len(conclusion) > 400 {
		conclusion = conclusion[:400]
		if idx := strings.LastIndexAny(conclusion, ".。!！?？\n"); idx > 200 {
			conclusion = conclusion[:idx+1]
		}
	}
	b.WriteString("Result: ")
	b.WriteString(conclusion)
	return b.String()
}

// confirmPlan asks the user to approve the planner's output before handing off to
// the executor. Returns the user's free-typed note ("" when none) and an error on
// cancellation. In headless mode (asker == nil) it auto-confirms.
//
// The confirmation dialog shows:
//   ○ 提交执行 — 同意计划，直接交由 Hephaestus 执行
//   ○ 取消 — 放弃本次任务
//   📝 文本框 — 输入修改意见（选填），提交后转发给 Hephaestus
//
// If the user types a note in the text box, that note becomes the return value
// and is injected into the handoff as "📌 User note". Clicking "取消" or
// dismissing returns an error.
func (h *Hermes) confirmPlan(ctx context.Context, task, plan string) (string, error) {
	if h.asker == nil {
		return "", nil // headless: auto-confirm
	}
	answers, err := h.asker.Ask(ctx, []event.AskQuestion{{
		ID:     "confirm",
		Header: "计划确认",
		Prompt: fmt.Sprintf("任务：%s", truncateStr(task, 200)),
		Plan:   plan, // full plan rendered by PlanCard with Markdown
		Options: []event.AskOption{
			{Label: "提交执行", Description: "按计划交由 Hephaestus 立即执行"},
			{Label: "取消", Description: "放弃本次任务，不做任何更改"},
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
	case "提交执行":
		return "", nil // agree without notes
	case "取消":
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
	if err := h.plannerAgent.Run(ctx, input); err != nil {
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
