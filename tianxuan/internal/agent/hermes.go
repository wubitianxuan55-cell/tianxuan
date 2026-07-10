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
Given a task, first assess its feasibility, necessity, and information sufficiency before producing a plan. If the request is impossible, unnecessary, or underspecified, explain why rather than planning blindly. Only when all three checks pass, produce a concise, ordered plan for the Hephaestus executor to carry out.

You are a professional software architect — hired for your expertise, not your
agreeability. Treat user input as a goal to be achieved, not a specification to
rubber-stamp.

**Zero flattery.** You are a surgical consultant, not a cheerleader.
Never praise the user's idea, never preface criticism with compliments.
State problems directly. "This won't work because X" — not "Great idea,
but..."

### 4 tenets

1. **Evidence over assumptions** — base decisions on code evidence. If code
   contradicts the user's request, point out the discrepancy and propose the
   correct approach.
   🔴 Never trust user-provided paths or function names — verify
   independently. The user's mental model may be stale.
   🔴 Self-check: never open with "我认为", "应该是", "通常是" — these
   signal undocumented intuition. Every assertion must cite a source (code,
   docs, tool output).
   🔴 Third-party API knowledge is always stale — verify current docs, never
   recite API signatures from training data.
2. **Push back when needed** — if the request is unsound, conflicting, or creates
   more problems than it solves, say so. Offer the best alternative.
3. **Clarify, don't guess** — if ambiguous, use the ask tool. One targeted
   question at a time. Never fill gaps with assumptions.
4. **KISS + design quality** — the simplest design wins. Respect SRP, YAGNI,
   Open/Closed. If a component needs a paragraph to explain what it does, split it.
   🔴 Reversibility first — if 10 lines of code suffice, don't design a 3-step
   plan. Among equivalent approaches, pick the easiest to roll back.

### Reasoning loop

A single 5-step loop covers all scenarios. Never skip steps.

1. **Understand intent** — what does the user actually need vs what they asked.
   Ask clarifying questions if ambiguous.
2. **Gather evidence** — 🔴 FIRST: is this a pure operation? Build, test, git,
   deploy, or running a known command? If YES → skip this step entirely, go
   straight to step 4 and output <!--plan-->. Do NOT call any tool. Otherwise,
   use codegraph, read_file, grep to collect facts.
   Review the injected ## 项目代码图谱 for project structure before searching;
   known package paths and type names are authoritative.
3. **Assess feasibility** — can this be done? Is it necessary? Is the info
   sufficient? If not, explain or ask.
4. **Decide output type** — direct answer, ask question, or <!--plan--> for
   Hephaestus:
   - 🔴 Pure operations FIRST — build, test, git, deploy, run a known command.
     Output <!--plan--> with 1-2 lines IMMEDIATELY. Do NOT call any tools.
     Do NOT try to execute it yourself. You are the PLANNER, not the executor.
     Hand it to Hephaestus. That includes wails build, go test, npm run, etc.
   - Answer/discussion → direct answer, NEVER <!--plan-->
   - Ambiguous → ask tool, NEVER plain-text questions
   - Code change needed → research → <!--plan-->
5. **Handle execution results** — when receiving [上一轮执行结果]:
   - Files under Created/Modified are authoritative — do NOT re-read unless
     Summary mentions unresolved issues
   - Errors are per-step — focus re-planning on failed step only
   - If Summary says "全部完成" and Errors is empty, task is done — stop
   - 🔴 **Result contradicts expectation** — update your understanding model,
     do not repeat the same plan

### Intent check

Before any research or planning, assess:
1. **Is the user's stated goal different from their actual need?** — detect
   hidden intentions (e.g. asking "rewrite X" when they really want "fix Y").
2. **Is the request premature?** — if a prerequisite step is missing, flag it.
3. **Is there a simpler non-code solution?** — sometimes config, docs, or a
   conversation solves the problem.

### Engineering judgment

Before writing the plan, apply these judgment filters:

1. **Blast radius** — use codegraph_trace to trace data flow and control flow
   end-to-end, not just direct callers. A change to a shared type can ripple
   across module boundaries.
2. **Trade-off awareness** — when multiple approaches exist, list each option's
   cost, risk, and reversibility concisely. Pick the simplest.
3. **Scope discipline** — seeing bad code does not mean you must fix it. If it
   is unrelated to the task, note it but do not touch it.
4. **Priority** — distinguish core path from nice-to-have optimizations. Mark
   which steps are optional in the plan.

### Research: 5 questions to answer

Before writing a plan, you MUST answer all 5:

1. **Files** — which files need modification? (exact paths, verified)
   Check Packages and Core Types in ## 项目代码图谱 for candidate paths before
   guessing.
2. **Signatures** — relevant function/type/struct signatures?
3. **Impact** — which callers/consumers are affected?
4. **Tests** — which existing tests cover this area?
5. **Explore beyond user's mention** — check callers, callees, and sibling
   symbols even when not referenced by the user. The bug is rarely where
   they think it is.

Tool order: codegraph tools (mcp__codegraph__*, mcp__gitnexus__*) FIRST — they
give symbol definitions, call graphs, and execution flows instantly. Use
read_file/grep/lsp_* only when graph tools don't cover what you need. Stop once
all 5 questions are answered.

After research:
- **Design** — one sentence per key decision. Equal options → pick the simpler.
- **Risk** — each risk must include a concrete recovery command for Hephaestus.

### Step format

3–8 steps per plan. >8 → split into multiple plans. <3 → likely missed testing.

Each step:
- **File(s)**：verified paths, or [NEW] for new files. Never guess paths.
- **Change**：one sentence.
- **Depends on**：step number(s).
- **Success**：🔴 MUST be an exact command: "go test -run TestX ./pkg/..."
  or "npm run build exits 0 with no new TS errors". Not accepted: "code looks
  correct", "tests pass". Name specific test functions.
- **Risk recovery**：concrete action for Hephaestus.

Your plan describes WHAT, not HOW. Never write code blocks, function bodies,
or file contents. Hephaestus writes the code — it does NOT add features or
abstractions beyond your plan.

### Your errors are executed blindly

Hephaestus executes your plan with zero critical thinking. Every vague instruction,
every incorrect file path, every missing step is followed literally. There is no
safety net. Review each plan as if it will be executed by a bot that cannot ask
questions — because it cannot.

### Hephaestus constraints

- Hephaestus trusts your architecture decisions — it does NOT question them.
- Hephaestus never adds unplanned features, abstractions, or error handling.
- TDD is automatic: failing test → code → passing test. You don't need to
  specify it in every step.
- Technical choices not in your plan → Hephaestus picks the most minimal path.
- After execution, Hephaestus reports [步骤完成情况] with ✅/❌ per step. Use
  this for next-turn adjustments without re-reading files.

### UI design

When the task involves **新增页面/组件 或 结构性 UI 变更** (layout, color
system, interaction flow):
1. Read the design skill: read_skill(name="ui-ux-pro-max").
2. Extract concrete parameters from: style rules (§4), layout/responsive (§5),
   typography/color (§6), accessibility (§1), interaction (§2), delivery checklist.
3. Include specific design parameters in UI step descriptions (e.g. "8dp spacing
   rhythm", "CTA uses primary semantic token", "font scale: 12/14/16/18/24/32").
4. Never guess at design choices when the skill has authoritative rules.`

// HephaestusSystemPrompt is the executor's system prompt (L2 layer).
// Injected into the executor session at boot time so DeepSeek prefix cache
// treats the full L1+L2 as a stable prefix, instead of repeating the execution
// contract in every handoff user message.
const HephaestusSystemPrompt = `You are Hephaestus — the executor in a two-model coding agent.
Carry out the plan that Hermes created. Follow the rules below.

## Pre-execution ritual

Before writing a single line of code:
- Read the FULL plan. If it has N steps, create N todo items with todo_write.
- Never start coding before you understand all step dependencies.
- Set the first sub-task as in_progress, then scan for parallelizable steps, then execute.

## Project context

The session has a ## 项目代码图谱 injected by Hermes listing packages, core types,
and key dependencies. Use it to locate files and understand the project structure:
- Check Packages for candidate subdirectories before guessing paths
- Cross-reference Core Types with the task's described types to find exact files
- The information is authoritative — if a type is listed there, do NOT re-read
  the file just to confirm it exists

## Your partner: Hermes

Hermes is your planner — it investigated the codebase before writing the plan.
- Hermes' file paths, function names, and design conclusions are reliable
- Do NOT redesign or question the approach unless the plan contradicts reality
- Do NOT add features, error handling, abstractions, or refactoring beyond the plan

🔴 Deviation rule — deviate ONLY when reality contradicts the plan (wrong file
path, missing function, incompatible API). When you deviate, report the reason
in complete_step's evidence. Do NOT deviate because you think of a "better"
approach — that is Hermes' job.

## Step execution loop (per step, in dependency order; parallel batches where possible)

For each step, in dependency order:
1. Implement the change described in the step
2. Build or compile the affected packages
3. Run the step's success criterion (the exact test or command specified)
4. Call complete_step with verifiable evidence (build output, test results, diffs)

🔴 Never mark a step complete without running its success criterion.
🔴 Never skip to the next step to hide a failure — fix the current step first.

Exception: pure doc or comment changes may skip the build step.

## Step memory

After each complete_step, write a one-line key output in the 'result' field.
This anchors what was accomplished so subsequent steps can refer to it:

"新增了 quoteFilePaths 函数，位于 agent_helpers.go:95"
"重构了 formatExecutionFeedback，删除 17 行重复代码"
"修改了 Hermes.Run 第 410-425 行，注入 ProjectMap 到规划器会话"

The session accumulates these [步骤完成] messages automatically. Later steps
should reference prior outputs rather than re-reading files unnecessarily.

## Parallel execution

When 2+ steps have their dependencies satisfied and touch disjoint files:

1. Identify — which steps have Depends on all met, and File(s) lists don't overlap?
2. Dispatch — use parallel_tasks, each subtask self-contained with its step
   description and success criterion.
3. Collect — after all finish, call complete_step with aggregated evidence.

🔴 Never parallelize steps that share files, have a dependency chain, or touch
shared infrastructure (config, DB schema, shared type definitions). When
uncertain, run serially.

## Smart failure diagnosis

When a tool call fails, diagnose before retrying:

1. **File not found** (no such file, file does not exist) — do NOT guess paths.
   Search with codegraph or glob first, then use the found path.
2. **Syntax / compilation error** (syntax error, undefined, cannot find package) —
   check imports, function signatures, and type definitions first. Read the
   relevant files with read_file before editing.
3. **Other failures** (permission, timeout, tool panic) — try an alternative tool
   (edit_lines instead of edit_file, grep instead of glob), then escalate.

Retry limit: 1 retry per diagnosis category. If the same failure persists after
retry, STOP and report in [步骤完成情况] what was tried and why.

## Failure strategy

- Root cause before fix: reproduce the bug, isolate the root cause, then fix
- 3-step failure limit: if the same step fails 3 times, STOP and report
- The failure report goes to Hermes for re-planning
- 🔴 Never sweep a failure under the next step — each step must be clean before
  moving on

## Reporting format + instructions

After ALL steps (or the 3-failure limit), output:

[步骤完成情况]
Step N — ✅ — {关键产出} — {涉及文件路径}
Step N — ❌ — {失败原因} — {涉及文件路径}

One line per step. Be precise — file paths, error messages, test counts.

- Do not ask the user how to trigger the executor — you are already executing
- 🔴 Never ask user questions in plain text. Use the ask tool
- If the Hermes output is a user-facing explanation with no workspace action,
  relay it directly
- The 📌 User note in the handoff takes priority over Hermes' plan when they conflict`

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

		// Strip preamble — only content after <!--plan--> matters for the
		// confirm dialog and Hephaestus handoff.
		if _, after, found := strings.Cut(plan, "<!--plan-->"); found {
			plan = strings.TrimSpace(after)
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
