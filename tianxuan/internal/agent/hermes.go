package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"tianxuan/internal/codegraph"
	"tianxuan/internal/event"
	"tianxuan/internal/planmode"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)


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

	// planner 是规划者的抽象。DeepSeek 使用 AgentRunner（TCCA 四域缓存），
	// XAI/Grok 等非 DeepSeek provider 使用 XAIPlanner（独立上下文管理）。
	// 根据 hermesProvider 的名称自动选择实现。
	planner Planner

	// V10.54: workspace root for project map injection.
	wsRoot          string
	lastProjectHash string // hash of last injected ProjectMap; "" means not injected yet or stale

	executorSinkWrapped bool // V10.87: guard against double-wrapping executor sink
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
	// V10.58: StrictEvidence is explicitly false — the planner uses read-only
	// tools and never calls complete_step, so strict verification is meaningless.
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
		h.planner = newPlanner(hermesProvider, readonlyTools, hermesSession,
			planMaxSteps, temperature, hermesPricing,
			contextWindow, archiveDir, plannerSink)
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
	if h.planner != nil {
		h.planner.SetSession(h.hermesSess)
	}
	// V10.87: reset project map hash so the new session gets a fresh injection.
	// Without this, injectProjectMap sees the stale hash and skips re-injection,
	// leaving the planner without a project map for the first turn.
	h.lastProjectHash = ""
}

// PlannerContext returns the planner agent's last usage and context window,
// for the status bar's per-model context gauge.
func (h *Hermes) PlannerContext() (used int, window int) {
	if h == nil || h.planner == nil {
		return 0, 0
	}
	u := h.planner.LastUsage()
	if u == nil {
		return 0, h.planner.ContextWindow()
	}
	return u.PromptTokens, h.planner.ContextWindow()
}

// SetAsker installs the interactive asker for plan confirmation (V10.34).
// nil means headless mode — plans auto-confirm without user approval.
// Also wires the asker into the plannerAgent so it can ask clarifying questions
// during planning (scope negotiation, detail gathering).
func (h *Hermes) SetAsker(a Asker) {
	h.asker = a
	if h.planner != nil {
		h.planner.SetAsker(a)
	}
}

// SetPlanMode propagates the read-only gate to both planner and executor
// agents. Ported from DeepSeek-Reasonix Coordinator.
func (h *Hermes) SetPlanMode(v bool) {
	if h == nil {
		return
	}
	if h.planner != nil {
		h.planner.SetPlanMode(v)
	}
	h.hephaestus.SetPlanMode(v)
}

// SetPlanModePolicy propagates the plan-mode tool safety policy to both agents.
func (h *Hermes) SetPlanModePolicy(p planmode.Policy) {
	if h == nil {
		return
	}
	if h.planner != nil {
		h.planner.SetPlanModePolicy(p)
	}
	h.hephaestus.SetPlanModePolicy(p)
}

// ── Run (top-level orchestration) ───────────────────────────────────────

// Run plans with the planner model, then hands the plan to the executor.
// Returns a merged TurnResult combining the planner's and executor's outcomes.
func (h *Hermes) Run(ctx context.Context, input string) (*TurnResult, error) {
	h.sink.Emit(event.Event{Kind: event.TurnStarted})

	// V10.31: fast path — skip planner for simple/quick tasks ("!" prefix).
	if result, err := h.runFastPath(ctx, input); result != nil || err != nil {
		return result, err
	}

	// Normal path: plan → confirm → execute.
	h.sink.Emit(event.Event{Kind: event.Phase, Text: h.hermesProvider.Name() + " · hermes"})
	prePlanLen := len(h.hermesSess.Messages)
	h.injectProjectMap()

	plan, err := h.planWithConfirmation(ctx, input, prePlanLen)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		// Hermes answered directly or user chose chat-only — the planner's
		// text has already been streamed; no executor dispatch needed.
		// Summary is empty because callers discard TurnResult for this path.
		return &TurnResult{Plan: input, Success: true}, nil
	}

	// Execute plan with verification loop (up to 3 rounds).
	// V10.87: after each execution, check StepResults; if any step failed,
	// generate a fix plan and re-execute automatically (no confirmation dialog).
	firstRound := h.executePlanWithRetry(ctx, input, *plan)
	return firstRound.result, firstRound.err
}

// fixAttempt records one round of auto-fix for reflection prompts in round 3.
type fixAttempt struct {
	round    int
	fixPlan  string
	feedback string
}

// executePlanWithRetry wraps executePlan in a verification loop.
// The first round executes the original plan; subsequent rounds create
// fix plans for failed steps and re-execute automatically.
type execRound struct {
	result *TurnResult
	err    error
}

func (h *Hermes) executePlanWithRetry(ctx context.Context, input string, initial planWithNote) execRound {
	var fixHistory []fixAttempt // V10.87: accumulate fix history for round-3 reflection

	// Round 1: execute the original plan (emit TurnResultEvent normally)
	result, err := h.executePlan(ctx, input, initial, false)

	for round := 2; round <= 3; round++ {
		if err != nil || result == nil {
			if result == nil && err == nil {
				// Both nil — construct a synthetic error so callers see the failure.
				result = &TurnResult{Plan: initial.text, Success: false, Errors: []string{"executePlan returned nil result without error"}}
				err = fmt.Errorf("hermes: executePlan returned nil, nil")
			}
			slog.Info("hermes: retry loop exited without resolution")
			break
		}
		if h.allStepsPassed(result) {
			if round > 2 {
				slog.Info("hermes: fix round succeeded", "round", round-1)
			}
			break
		}

		h.sink.Emit(event.Event{Kind: event.Phase, Text: "修正执行 (轮 " + strconv.Itoa(round) + "/3)"})

		// Generate a fix plan — round 2 is targeted, round 3 includes reflection
		fixPlan, fixErr := h.planFix(ctx, input, initial.text, result, round, fixHistory)
		if fixErr != nil {
			slog.Warn("hermes: fix plan generation failed", "error", fixErr)
			h.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
				Text: "自动修正计划生成失败: " + fixErr.Error()})
			break
		}

		// Execute the fix plan — suppress TurnResultEvent (emit once at end)
		result, err = h.executePlan(ctx, input, *fixPlan, true)

		// Record this fix attempt so round 3 can reflect on prior failures
		fixHistory = append(fixHistory, fixAttempt{
			round:    round,
			fixPlan:  fixPlan.text,
			feedback: formatExecutionFeedback(result),
		})
	}

	if result != nil && !h.allStepsPassed(result) && err == nil {
		slog.Info("hermes: 3 rounds exhausted with failures")
		h.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
			Text: "已尝试 3 轮自动修正，仍有失败步骤，请手动检查"})
	}

	// V10.87: if retries happened, emit a single final TurnResultEvent.
	// Rounds 2/3 suppressed theirs so the frontend sees exactly one result card.
	if len(fixHistory) > 0 {
		if result != nil {
			summary := h.formatSummary(result, err, !h.allStepsPassed(result))
			h.sink.Emit(event.Event{
				Kind: event.TurnResultEvent,
				PlanResult: &event.PlanResult{
					Plan:          result.Plan,
					FilesCreated:  result.FilesCreated,
					FilesModified: result.FilesModified,
					Success:       result.Success,
					Errors:        result.Errors,
					Summary:       summary,
				},
			})
		} else if err != nil {
			h.sink.Emit(event.Event{
				Kind: event.TurnResultEvent,
				PlanResult: &event.PlanResult{
					Success: false,
					Errors:  []string{err.Error()},
					Summary: "❌ 执行失败: " + err.Error(),
				},
			})
		}
	}

	return execRound{result: result, err: err}
}

// allStepsPassed checks whether every step in the execution result succeeded.
func (h *Hermes) allStepsPassed(r *TurnResult) bool {
	if r == nil {
		return false
	}
	if !r.Success {
		return false
	}
	for _, sr := range r.StepResults {
		if sr.Status != "success" {
			return false
		}
	}
	return true
}

// planFix asks the planner to produce a targeted fix for failed steps.
// Round 2 does a minimal fix; round 3 switches to reflection mode with full
// fix history, asking the planner to reconsider the overall approach.
func (h *Hermes) planFix(ctx context.Context, origInput string, originalPlan string, failed *TurnResult, round int, fixHistory []fixAttempt) (*planWithNote, error) {
	fixInput := buildFixPrompt(origInput, originalPlan, failed, round, fixHistory)

	// Use planWithTools (read-only investigation) for the fix plan
	planText, err := h.planWithTools(ctx, fixInput)
	if err != nil {
		return nil, fmt.Errorf("hermes: fix plan failed: %w", err)
	}

	return &planWithNote{text: planText}, nil
}

// buildFixPrompt constructs the prompt for a fix/replan round.
// Round 2 (or when there's no history) is a targeted fix for failed steps.
// Round 3+ is a reflection round asking the planner to reconsider the approach.
func buildFixPrompt(origInput, originalPlan string, failed *TurnResult, round int, fixHistory []fixAttempt) string {
	var failedSteps []string
	for _, sr := range failed.StepResults {
		if sr.Status != "success" {
			failedSteps = append(failedSteps, fmt.Sprintf("- ❌ %s: %s", sr.Step, sr.Result))
		}
	}

	errSummary := strings.Join(failed.Errors, "; ")
	execFeedback := formatExecutionFeedback(failed)

	if round == 2 || len(fixHistory) == 0 {
		// Round 2: targeted fix — only patch the broken steps
		return fmt.Sprintf(`以下步骤执行失败，请创建最小修正计划，仅修正失败的步骤：

原始任务:
%s

原计划:
%s

执行反馈:
%s

失败步骤:
%s

执行错误: %s

修正计划要求:
- 仅修复标记 ❌ 的步骤，不重做 ✅ 步骤
- 使用 <!--plan--> 标记
- 修正计划自动执行，不需要用户确认
`, origInput, originalPlan, execFeedback, strings.Join(failedSteps, "\n"), errSummary)
	}

	// Round 3+: reflection mode — prior rounds failed, reconsider the approach
	var historyLines []string
	for _, a := range fixHistory {
		historyLines = append(historyLines, fmt.Sprintf(
			"--- 第 %d 轮修正 ---\n修正计划:\n%s\n\n执行反馈:\n%s",
			a.round, a.fixPlan, a.feedback))
	}
	return fmt.Sprintf(`前两轮针对性修补均未完全解决，请重新审视整体方案：

原始任务:
%s

原计划:
%s

修正履历:
%s

当前仍失败的步骤:
%s

当前执行错误: %s

反思要求:
- 不要只修补细节——考虑根本方向是否合理
- 如果原计划的架构假设有误，请重新设计替代方案
- 仔细分析为什么前两轮修正没有解决问题
- 使用 <!--plan--> 标记
- 修正计划自动执行，不需要用户确认
`, origInput, originalPlan, strings.Join(historyLines, "\n\n"), strings.Join(failedSteps, "\n"), errSummary)
}

// ── Run sub-steps ───────────────────────────────────────────────────────

// wrapExecutorSink suppresses the executor's TurnStarted event and returns
// a restore function. Both runFastPath and executePlan use it — Hermes
// already emits TurnStarted at the top of Run(), and a duplicate from the
// executor would reset per-turn cost stats in the frontend.
func (h *Hermes) wrapExecutorSink() func() {
	if h.executorSinkWrapped {
		return func() {} // already wrapped — no-op restore
	}
	h.executorSinkWrapped = true
	origSink := h.hephaestus.Sink()
	h.hephaestus.SetSink(event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnStarted {
			return
		}
		origSink.Emit(e)
	}))
	return func() {
		h.hephaestus.SetSink(origSink)
		h.executorSinkWrapped = false
	}
}

// runFastPath handles the "!" prefix fast path that skips the planner entirely.
func (h *Hermes) runFastPath(ctx context.Context, input string) (*TurnResult, error) {
	task, ok := shouldSkipPlanner(input)
	if !ok {
		return nil, nil
	}
	// V10.87: bare "!" with no task after trimming — let planner handle it rather
	// than dispatching an empty task to the executor.
	if task == "" {
		return nil, nil
	}
	h.sink.Emit(event.Event{Kind: event.Phase, Text: h.hephaestus.ProvName() + " · 快速执行"})
	defer h.wrapExecutorSink()()
	// V10.87: pre-inject original input before handoff, matching executePlan's
	// injection order so the executor session's prefix is consistent across paths.
	h.hephaestus.Session().Add(provider.Message{Role: provider.RoleUser, Content: task})
	execResult, execErr := h.hephaestus.Run(ctx, formatHandoff(task, task, ""))

	// V10.87: emit TurnResultEvent so the frontend gets a structured result card,
	// matching executePlan's behaviour. Pre-injection stays as `task` — the "!"
	// marker is a Hermes-layer signal and must not leak to the executor.
	if execResult != nil {
		execResult.Plan = "" // fast path has no plan
	}
	hermesSummary := h.formatSummary(execResult, execErr, false)
	if hermesSummary != "" {
		h.sink.Emit(event.Event{Kind: event.Text, Text: hermesSummary})
	}
	if execResult != nil {
		h.sink.Emit(event.Event{
			Kind: event.TurnResultEvent,
			PlanResult: &event.PlanResult{
				Plan:          "",
				FilesCreated:  execResult.FilesCreated,
				FilesModified: execResult.FilesModified,
				Success:       execResult.Success,
				Errors:        execResult.Errors,
				Summary:       hermesSummary,
			},
		})
	} else if execErr != nil {
		h.sink.Emit(event.Event{
			Kind: event.TurnResultEvent,
			PlanResult: &event.PlanResult{
				Success: false,
				Errors:  []string{execErr.Error()},
				Summary: hermesSummary,
			},
		})
	}
	return execResult, execErr
}

func (h *Hermes) injectProjectMap() {
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

// planWithNote bundles a confirmed plan with any user note from confirmation.
type planWithNote struct {
	text     string
	userNote string
}

// planWithConfirmation runs the planner in a replan loop: plan → confirm →
// repeat on revise. Returns the confirmed plan, or nil when Hermes answered
// directly (no code changes needed) or the user chose chat-only.
func (h *Hermes) planWithConfirmation(ctx context.Context, input string, prePlanLen int) (*planWithNote, error) {
	var revisedNote string // V10.87: carry user feedback across revise→replan loop
	for {
		plan, err := h.plan(ctx, input)
		if err != nil {
			h.hermesSess.Truncate(prePlanLen)
			return nil, fmt.Errorf("hermes: %w", err)
		}
		if isAnswerNotAction(plan) {
			return nil, nil // Hermes answered directly
		}

		// Keep the full planner output — preamble (analysis/reasoning) is valuable
		// context for the executor. Previously only the <!--plan--> portion was kept.

		// V10.87: auto-confirm simple plans (≤3 steps, no new files) to save one
		// round-trip — unless the user explicitly asked for a revision, in which
		// case the replanned version must be shown.
		if h.asker != nil && shouldAutoConfirm(plan) && revisedNote == "" {
			return &planWithNote{text: plan, userNote: revisedNote}, nil
		}

		userNote, chatOnly, revise, err := h.confirmPlan(ctx, input, plan)
		if err != nil {
			h.hermesSess.Truncate(prePlanLen)
			return nil, err
		}
		if chatOnly {
			return nil, nil
		}
		if revise {
			if userNote != "" {
				// V10.58: append to current input (may already contain prior
				// feedback) instead of resetting to origInput — or the previous
				// round's feedback would be silently lost.
				input = input + "\n\n—— User feedback on previous plan ——\n" + userNote
				revisedNote = userNote
			}
			// V10.58: keep the original prePlanLen as the rollback baseline;
			// advancing it would leave abandoned plan messages in the session
			// if the re-plan also fails.
			continue
		}
		// Carry revised note through to final result (non-revise exits).
		note := userNote
		if revisedNote != "" {
			note = revisedNote
		}
		return &planWithNote{text: plan, userNote: note}, nil
	}
}

// executePlan dispatches the executor with the confirmed plan, feeds results
// back to the planner session, and emits TurnResultEvent for the frontend.
// When suppressResultEvent is true, TurnResultEvent is skipped — the caller
// (executePlanWithRetry) will emit a single final event after all retries.
func (h *Hermes) executePlan(ctx context.Context, origInput string, p planWithNote, suppressResultEvent bool) (*TurnResult, error) {
	plan := p.text
	userNote := p.userNote

	h.sink.Emit(event.Event{Kind: event.Phase, Text: h.hephaestus.ProvName() + " · Hephaestus"})
	defer h.wrapExecutorSink()()

	// V10.49: pre-inject the original Chinese input before the handoff prompt.
	h.hephaestus.Session().Add(provider.Message{Role: provider.RoleUser, Content: origInput})
	execResult, execErr := h.hephaestus.Run(ctx, formatHandoff(origInput, plan, userNote))

	// Attach plan to TurnResult so the returned value is self-contained.
	if execResult != nil {
		execResult.Plan = plan
	}

	// Preserve exec error when both result and error are non-nil (partial success).
	if execResult != nil && execErr != nil {
		execResult.Errors = append(execResult.Errors, execErr.Error())
	}

	// formatSummary: Hermes provides a concise completion message.
	hermesSummary := h.formatSummary(execResult, execErr, false)
	if hermesSummary != "" {
		h.sink.Emit(event.Event{Kind: event.Text, Text: hermesSummary})
	}

	// Feed results back to the planner and emit TurnResultEvent.
	if execResult != nil {
		h.feedResultToPlanner(execResult)
		if !suppressResultEvent {
			h.sink.Emit(event.Event{
				Kind: event.TurnResultEvent,
				PlanResult: &event.PlanResult{
					Plan:          execResult.Plan,
					FilesCreated:  execResult.FilesCreated,
					FilesModified: execResult.FilesModified,
					Success:       execResult.Success,
					Errors:        execResult.Errors,
					Summary:       hermesSummary,
				},
			})
		}
	} else if execErr != nil {
		synth := &TurnResult{
			Plan:    plan,
			Success: false,
			Errors:  []string{execErr.Error()},
			Summary: hermesSummary,
		}
		h.hermesSess.Add(provider.Message{
			Role:    provider.RoleUser,
			Content: formatExecutionFeedback(synth),
		})
		if !suppressResultEvent {
			h.sink.Emit(event.Event{
				Kind: event.TurnResultEvent,
				PlanResult: &event.PlanResult{
					Plan:    synth.Plan,
					Success: false,
					Errors:  synth.Errors,
					Summary: hermesSummary,
				},
			})
		}
	}
	return execResult, execErr
}

// feedResultToPlanner injects execution feedback into the planner's session
// and invalidates the project map cache on structural changes.
// V10.89: uses enhanced SDD feedback with Delta + Verify triad.
func (h *Hermes) feedResultToPlanner(r *TurnResult) {
	hasContent := r.Summary != "" || len(r.Errors) > 0 ||
		len(r.FilesCreated) > 0 || len(r.FilesModified) > 0
	if !hasContent {
		return
	}

	// XAI planner 用精简反馈（省 token），DeepSeek 用完整格式
	feedback := formatExecutionFeedbackEnhanced(r, r.Plan)
	if ff, ok := h.planner.(interface{ FormatFeedback(*TurnResult) string }); ok {
		feedback = ff.FormatFeedback(r)
	}

	h.hermesSess.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: feedback,
	})

	if h.wsRoot != "" && hasStructuralChange(r.FilesCreated, r.FilesModified) {
		h.lastProjectHash = ""
	}
}

// ── Formatting helpers ──────────────────────────────────────────────────

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
		summary = "(execution produced no summary — check Errors for details)"
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

// formatSummary produces a concise completion message from Hermes,
// replacing Hephaestus's verbose end-of-turn summary. It is a pure string
// formatter — no LLM call, no token cost. The output includes what was
// accomplished and any errors, without repeating the plan.
func (h *Hermes) formatSummary(r *TurnResult, execErr error, retriesExhausted bool) string {
	if r == nil {
		if execErr != nil {
			return "❌ 执行失败: " + execErr.Error()
		}
		return ""
	}

	// Build concise summary: what files changed, what errors occurred
	var parts []string
	if len(r.FilesCreated) > 0 {
		parts = append(parts, fmt.Sprintf("新建 %d 个文件", len(r.FilesCreated)))
	}
	if len(r.FilesModified) > 0 {
		parts = append(parts, fmt.Sprintf("修改 %d 个文件", len(r.FilesModified)))
	}
	if len(r.Errors) > 0 {
		parts = append(parts, fmt.Sprintf("%d 个错误", len(r.Errors)))
	}

	prefix := "✅ 任务完成"
	if !r.Success || len(r.Errors) > 0 {
		prefix = "⚠️ 任务部分完成"
	}

	msg := prefix
	if len(parts) > 0 {
		msg += " · " + strings.Join(parts, "，")
	}

	// Include step-level results when available
	if len(r.StepResults) > 0 {
		steps := make([]string, 0, len(r.StepResults))
		for _, sr := range r.StepResults {
			if sr.Status == "success" {
				steps = append(steps, "✅ "+sr.Step)
			} else {
				steps = append(steps, "❌ "+sr.Step)
			}
		}
		msg += "\n" + strings.Join(steps, "\n")
	} else if r.Success {
		// V10.87: model declared success but didn't record step details.
		msg += "\n（未记录步骤详情）"
	}

	if retriesExhausted {
		msg += "\n⚠️ 已尝试多轮自动修正"
	}

	return msg
}

// ── Plan implementation ──────────────────────────────────────────────────

// plan runs Hermes as an AgentRunner with read-only tools so it can investigate
// the codebase before proposing a plan. Falls back to planStream (zero-tool stream)
// when readonlyTools is nil — e.g. in tests or when no read-only registry is wired.
func (h *Hermes) plan(ctx context.Context, input string) (string, error) {
	if h.readonlyTools != nil {
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
	// Trigger compaction on the planner agent — planStream writes directly to
	// h.hermesSess without going through plannerAgent.Run(), so auto-compaction
	// never fires on this path. Without this, planStream sessions grow unbounded.
	if h.planner != nil {
		if err := h.planner.CompactNow(ctx, "planStream turn boundary"); err != nil {
			slog.Warn("hermes: planStream compaction failed", "error", err)
		}
	} else {
		// planStream without plannerAgent: no auto-compaction from AgentRunner.Run().
		// Manually trim to prevent unbounded memory growth in pure-stream sessions.
		msgs := h.hermesSess.Snapshot()
		if len(msgs) > 200 {
			var kept []provider.Message
			for _, m := range msgs {
				if m.Role == provider.RoleSystem {
					kept = append(kept, m)
				}
			}
			recent := msgs
			if len(recent) > 80 {
				recent = recent[len(recent)-80:]
			}
			for _, m := range recent {
				if m.Role != provider.RoleSystem {
					kept = append(kept, m)
				}
			}
			h.hermesSess.Replace(kept)
			h.lastProjectHash = "" // V10.87: compaction may have trimmed the project map; force re-inject next turn
		}
	}
	return plan, nil
}

// planWithTools runs the persistent planner Agent with read-only tools.
// V10.36: uses the persistent plannerAgent (created in NewHermes) instead of
// building a temporary AgentRunner each turn. The planner's session accumulates
// planning history + execution results across turns; compaction keeps it bounded.
func (h *Hermes) planWithTools(ctx context.Context, input string) (string, error) {
	if h.planner == nil {
		return "", fmt.Errorf("hermes: planner agent not initialized (no read-only tools)")
	}
	// Re-propagate asker to plannerAgent before each planning run,
	// ensuring the ask tool can interact with the user even if
	// SetAsker was called before plannerAgent was created.
	if h.asker != nil {
		h.planner.SetAsker(h.asker)
	}
	turnResult, err := h.planner.Run(ctx, input)
	if err != nil {
		return "", fmt.Errorf("hermes: %w", err)
	}
	if turnResult == nil || turnResult.Summary == "" {
		return "", fmt.Errorf("hermes: planner produced no output")
	}
	slog.Info("hermes: planner run summary", "summary", turnResult.Summary)
	return turnResult.Summary, nil
}

// autoGate approves every tool call — safe for read-only planners.
type autoGate struct{}

func (g *autoGate) Check(_ context.Context, _ string, _ json.RawMessage, _ bool) (bool, string, error) {
	return true, "", nil
}

// ── Plan helpers ─────────────────────────────────────────────────────

// shouldSkipPlanner detects tasks that are simple enough to execute directly,
// V10.34: only the explicit "!" marker skips the planner.
// Compose appends blocks after user input, so "!" at position 0 is reliable.
// Trailing "\n\n" Compose blocks are stripped from the returned task.
func shouldSkipPlanner(input string) (string, bool) {
	if stripped, ok := strings.CutPrefix(input, "!"); ok {
		task := strings.TrimSpace(stripped)
		if idx := strings.Index(task, "\n\n"); idx >= 0 {
			task = strings.TrimSpace(task[:idx])
		}
		return task, true
	}
	return "", false
}

// isAnswerNotAction checks whether the planner's output is a direct answer
// that needs no executor. The planner self-marks executable plans with
// <!--plan--> — if absent, Hermes answered directly.
func isAnswerNotAction(plan string) bool {
	trimmed := strings.TrimSpace(plan)
	return !strings.Contains(trimmed, "<!--plan-->")
}

func formatHandoff(task, plan, userNote string) string {
	note := ""
	if userNote != "" {
		note = fmt.Sprintf("\n\n📌 用户备注:\n%s\n", userNote)
	}
	return fmt.Sprintf(`# %s

任务:
%s

计划:
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
	const header = "任务:\n"
	i := strings.Index(trimmed, header)
	if i < 0 {
		return s
	}
	rest := trimmed[i+len(header):]
	if j := strings.Index(rest, "\n\n计划:"); j >= 0 {
		rest = rest[:j]
	}
	if task := strings.TrimSpace(rest); task != "" {
		return task
	}
	return s
}

// newPlanner 根据 provider 类型创建对应的规划者。
// DeepSeek → AgentRunner（TCCA 四域缓存 + 前缀守卫）
// 其他 → XAIPlanner（独立上下文管理）
func newPlanner(prov provider.Provider, tools *tool.Registry, session *Session,
	maxSteps int, temperature float64, pricing *provider.Pricing,
	ctxWindow int, archiveDir string, sink event.Sink) Planner {

	if !strings.Contains(strings.ToLower(prov.Name()), "deepseek") {
		return NewXAIPlanner(prov, tools, session, maxSteps, temperature,
			pricing, ctxWindow, archiveDir, sink)
	}
	return New(prov, tools, session, Options{
		MaxSteps:       maxSteps,
		Temperature:    temperature,
		Pricing:        pricing,
		Gate:           &autoGate{},
		DisableVerify:  true,
		PlannerMode:    true,
		StrictEvidence: false,
		ContextWindow:  ctxWindow,
		Compaction:     CompactionConfig{ArchiveDir: archiveDir, Window: ctxWindow},
	}, sink)
}
