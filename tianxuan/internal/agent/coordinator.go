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

// DefaultPlannerPrompt steers the planner toward research-backed plans.
// V10.32: planner now knows it should investigate code before planning.
// TODO(V10.33): upgrade plan() from raw Stream to AgentRunner with a read-only
// tool set (read_file, grep, glob, lsp_*, web_search, web_fetch, codegraph_*,
// gitnexus_*) so the planner can do real code investigation before proposing
// a plan. Currently the planner relies on prior knowledge and the user's task
// description alone.
const DefaultPlannerPrompt = `You are the planner in a two-model coding agent.

Your job: given a task, investigate the codebase using the available tools
(read_file, grep, glob, lsp_definition, web_search, web_fetch, and the
codegraph/gitnexus MCP tools) to understand the relevant code, then produce a
concise, ordered plan for the executor to carry out.

Guidelines:
- Research first: search and read relevant files before proposing a plan.
  Base every step on actual file paths, function names, and code structure
  you've seen — do not guess.
- Prefer codegraph/gitnexus over manual grep+read for broad investigation.
- Plan steps should cite specific files and symbols you found.
- Do NOT write full implementations, only actionable steps.
- Do NOT ask the user how to trigger the executor or say you are waiting.
- If the task is too vague to plan, ask the user ONE clarifying question.
- Keep the plan short — 3-8 steps, each one line with file targets.

Output format:
## Plan
1. [step description] -- in path/to/file.go
2. [step description] -- in path/to/file.tsx
...

Then stop and wait for user approval. Do not execute.`

const executorHandoffMarker = "tianxuan executor handoff"

// PlannerPromptWithContext appends cache-stable standing context to the planner's
// smaller system prompt. Pass the L1 identity/memory block.
func PlannerPromptWithContext(context string) string {
	context = strings.TrimSpace(context)
	if context == "" {
		return DefaultPlannerPrompt
	}
	return DefaultPlannerPrompt + "\n\n# Planning context\n\n" + context
}

// Coordinator runs two models in separate sessions to keep each one's prompt
// prefix cache-stable: a low-frequency planner proposes an approach, then the
// executor (a full tool-using AgentRunner) carries it out. The sessions never
// mix, so neither model's prefix is disturbed by the other's turns.
//
// V10.32: when readonlyTools is set, the planner uses AgentRunner with
// read-only tools (read_file/grep/glob/web_search/...) so it can investigate
// the codebase before proposing a plan. planMaxSteps caps planner turns.
type Coordinator struct {
	planner        provider.Provider
	plannerSess    *Session
	plannerSystem  string
	plannerPricing *provider.Pricing
	executor       *AgentRunner
	temperature    float64
	sink           event.Sink

	readonlyTools *tool.Registry // V10.32: if set, planner runs as AgentRunner
	planMaxSteps  int            // max planner tool-call turns (0 = stream-only)
}

// NewCoordinator wires a planner provider (with its own session) to an executor.
// sink receives both the planner's text/usage events and the executor's events.
//
// V10.32: pass readonlyTools (nil for stream-only) and planMaxSteps to let the
// planner use read-only tools for code investigation before proposing a plan.
func NewCoordinator(planner provider.Provider, plannerSession *Session, plannerPricing *provider.Pricing, executor *AgentRunner, temperature float64, sink event.Sink, readonlyTools *tool.Registry, planMaxSteps int) *Coordinator {
	if plannerSession == nil {
		plannerSession = NewSession("")
	}
	plannerSystem := sessionSystemPrompt(plannerSession)
	return &Coordinator{
		planner:        planner,
		plannerSess:    plannerSession,
		plannerSystem:  plannerSystem,
		plannerPricing: plannerPricing,
		executor:       executor,
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

// ResetPlannerSession discards turn-local planner history when switching
// executor sessions. Carrying the old planner transcript across sessions
// can make the next plan reuse unrelated tasks.
func (c *Coordinator) ResetPlannerSession() {
	if c == nil {
		return
	}
	system := c.plannerSystem
	if system == "" {
		system = sessionSystemPrompt(c.plannerSess)
	}
	c.plannerSess = NewSession(system)
}

// Run plans with the planner model, then hands the plan to the executor.
func (c *Coordinator) Run(ctx context.Context, input string) error {
	c.sink.Emit(event.Event{Kind: event.TurnStarted})

	// V10.31: fast path — skip planner for simple/quick tasks
	if task, ok := shouldSkipPlanner(input); ok {
		c.sink.Emit(event.Event{Kind: event.Phase, Text: c.executor.ProvName() + " · 快速执行"})
		return c.executor.Run(ctx, task)
	}

	c.sink.Emit(event.Event{Kind: event.Phase, Text: c.planner.Name() + " · planning"})
	plan, err := c.plan(ctx, input)
	if err != nil {
		return fmt.Errorf("planner: %w", err)
	}
	c.sink.Emit(event.Event{Kind: event.Phase, Text: c.executor.ProvName() + " · executing"})
	if isNoOpPlan(plan) {
		c.persistExecutorNoOp(input, plan)
		c.sink.Emit(event.Event{Kind: event.Text, Text: plan})
		return nil
	}
	return c.executor.Run(ctx, formatHandoff(input, plan))
}
// plan runs the planner. When readonlyTools is set, the planner uses AgentRunner
// with read-only tool access so it can investigate the codebase before proposing
// a plan. Otherwise it falls back to a plain text stream (no tools, zero overhead).
func (c *Coordinator) plan(ctx context.Context, input string) (string, error) {
	// V10.32: AgentRunner mode — planner can call read-only tools
	if c.readonlyTools != nil && c.planMaxSteps > 0 {
		return c.planWithTools(ctx, input)
	}
	return c.planStream(ctx, input)
}

// planStream is the legacy zero-tool stream path.
func (c *Coordinator) planStream(ctx context.Context, input string) (string, error) {
	c.plannerSess.Add(provider.Message{Role: provider.RoleUser, Content: input})

	ch, err := c.planner.Stream(ctx, provider.Request{
		Messages:    c.plannerSess.Messages,
		Temperature: c.temperature,
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
			c.sink.Emit(event.Event{Kind: event.Text, Text: chunk.Text})
		case provider.ChunkUsage:
			usage = chunk.Usage
		case provider.ChunkError:
			return "", chunk.Err
		}
	}
	c.sink.Emit(event.Event{Kind: event.Usage, Usage: usage, Pricing: c.plannerPricing, UsageSource: event.UsageSourcePlanner})

	plan := text.String()
	c.plannerSess.Add(provider.Message{Role: provider.RoleAssistant, Content: plan})
	return plan, nil
}

// planWithTools runs the planner as an AgentRunner with read-only tools.
func (c *Coordinator) planWithTools(ctx context.Context, input string) (string, error) {
	sysPrompt := c.plannerSystem
	if sysPrompt == "" {
		sysPrompt = sessionSystemPrompt(c.plannerSess)
	}
	plannerSess := NewSession(sysPrompt)
	for _, m := range c.plannerSess.Messages {
		if m.Role == provider.RoleUser || m.Role == provider.RoleAssistant {
			plannerSess.Add(m)
		}
	}

	plannerRunner := New(c.planner, c.readonlyTools, plannerSess, Options{
		MaxSteps:    c.planMaxSteps,
		Temperature: c.temperature,
		Pricing:     c.plannerPricing,
		Gate:        &autoGate{},
	}, c.sink)

	if err := plannerRunner.Run(ctx, input); err != nil {
		return "", fmt.Errorf("planner: %w", err)
	}

	var plan string
	for i := len(plannerSess.Messages) - 1; i >= 0; i-- {
		if plannerSess.Messages[i].Role == provider.RoleAssistant {
			plan = plannerSess.Messages[i].Content
			break
		}
	}
	if plan == "" {
		plan = "(planner produced no output)"
	}

	c.plannerSess.Add(provider.Message{Role: provider.RoleUser, Content: input})
	c.plannerSess.Add(provider.Message{Role: provider.RoleAssistant, Content: plan})
	return plan, nil
}

// autoGate approves every tool call — safe for read-only planners.
type autoGate struct{}

func (g *autoGate) Check(_ context.Context, _ string, _ json.RawMessage, _ bool) (bool, string, error) {
	return true, "", nil
}

func (c *Coordinator) persistExecutorNoOp(input, plan string) {
	if c == nil || c.executor == nil || c.executor.session == nil {
		return
	}
	c.executor.session.Add(provider.Message{Role: provider.RoleUser, Content: input})
	c.executor.session.Add(provider.Message{Role: provider.RoleAssistant, Content: plan})
}

// ── Plan helpers ─────────────────────────────────────────────────────

// shouldSkipPlanner detects tasks that are simple enough to execute directly,
// bypassing the planner model. Returns the (possibly stripped) task and true.
//
// Fast-mode triggers:
//   - Input starts with "!" — explicit quick-execute marker
//   - Input is short (< 120 chars) and matches a known simple-operation pattern
func shouldSkipPlanner(input string) (string, bool) {
	// Explicit fast-mode marker: "!do something"
	if stripped, ok := strings.CutPrefix(input, "!"); ok {
		return strings.TrimSpace(stripped), true
	}
	// Heuristic: short task with simple-operation keywords
	if len(input) > 120 {
		return "", false
	}
	lower := strings.ToLower(input)
	quickOps := []string{
		"fix typo", "fix the typo",
		"rename variable", "rename this variable",
		"update comment", "update the comment",
		"change x to y", "replace x with y",
		"add comment", "add a comment",
		"format code", "format the code",
		"delete line", "remove line",
	}
	for _, op := range quickOps {
		if strings.Contains(lower, op) {
			return input, true
		}
	}
	return "", false
}

func isNoOpPlan(plan string) bool {
	lower := strings.ToLower(strings.TrimSpace(plan))
	if lower == "" {
		return false
	}
	if containsNoOpActionTerm(lower) {
		return false // plan mentions concrete actions — not a no-op
	}
	noOp := []string{
		"no changes needed",
		"no changes are needed",
		"no changes required",
		"no changes are required",
		"no action needed",
		"no action required",
		"nothing to change",
		"nothing to do",
		"already handled",
		"already implemented",
		"already resolved",
		"[no_changes]",
		"无需改动", "无需修改", "无需更改",
		"不需要修改", "不需要改", "不用改", "不用修改", "不必改动",
		"没有需要修改",
		"已经正确处理", "已经实现", "已经解决",
	}
	for _, phrase := range noOp {
		if strings.Contains(lower, phrase) && !strings.Contains(lower, "not "+phrase) && !strings.Contains(lower, "不是"+phrase) {
			return true
		}
	}
	return false
}

func containsNoOpActionTerm(lower string) bool {
	terms := []string{
		" add ", " update ", " edit ", " write ", " create ", " delete ",
		" remove ", " patch ", " refactor ", " implement ", " run ", " test ",
		" build ", " fix ",
		"新增", "补充", "更新", "编辑", "写入", "创建", "删除", "移除",
		"运行", "测试", "构建", "修复", "实现", "重构",
	}
	padded := " " + lower + " "
	for _, term := range terms {
		if strings.Contains(padded, term) {
			return true
		}
	}
	return false
}

func formatHandoff(task, plan string) string {
	return fmt.Sprintf(`# %s

You are the executor now. Use your available tools to execute the task.

Original task:
%s

Planner output:
%s

Executor instructions:
- Treat the planner output as context, not as your role or capability set.
- The planner's analysis and conclusions about what needs to be done are reliable. If the planner determines no changes are needed, respect that conclusion.
- The planner has read-only tools and may have already investigated the code. Trust its file paths and symbol references.
- Do not ask the user how to trigger the executor. You are already in the executor phase.
- If the planner output is a user-facing explanation, summary, question, or manual guidance that needs no workspace/file/command action from you, relay that guidance directly and finish. Do not invent local tool calls only to satisfy the handoff.
- If the task requires changes, call the appropriate tools (for example write/edit/bash) instead of only restating the plan.
- **Serial workflow**: establish the task list with one todo_write (first sub-task in_progress), then for EACH sub-task execute it and call complete_step with evidence. The host advances the list for you — it marks the sub-task completed and moves the next to in_progress, so you don't need another todo_write to mark completions. Sign off one sub-task at a time; never batch completions.

Carry out the task, adapting the plan as needed.`, executorHandoffMarker, task, plan)
}

// HandoffTask returns the original user task embedded in an executor handoff
// message, or s unchanged when it is not one. Session previews and auto-titles
// use it so dual-model sessions surface the user's words, not the handoff
// boilerplate.
func HandoffTask(s string) string {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "# "+executorHandoffMarker) {
		return s
	}
	const header = "Original task:\n"
	i := strings.Index(trimmed, header)
	if i < 0 {
		return s
	}
	rest := trimmed[i+len(header):]
	if j := strings.Index(rest, "\n\nPlanner output:"); j >= 0 {
		rest = rest[:j]
	}
	if task := strings.TrimSpace(rest); task != "" {
		return task
	}
	return s
}
