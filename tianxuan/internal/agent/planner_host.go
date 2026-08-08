package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"tianxuan/internal/event"
	"tianxuan/internal/planmode"
)

// PlannerHost 是单模型规划模式宿主：同一个 AgentRunner（同一个 provider、
// 同一个 session）先以只读门控跑一轮规划（planmode.Marker + planModeGate），
// 经用户确认后在同一 session 执行计划，失败步骤由同一模型自动修正（≤3 轮）。
//
// 与已删除的双模型 Hermes 的本质区别：
//   - 单一 provider / 单一 session / 单一 L1 前缀，规划推理对执行直接可见；
//   - 规划与执行切换只动运行时 gate（SetPlanMode），工具 schema 全程不变，
//     遵守前缀缓存铁律（V8.0.2 教训：session 中途增删工具 = 缓存归零）。
//
// autoPlan 取值 "off"（默认，直接执行，等价于 Codex 自适应方式）或
// "ask"（复杂任务先规划、确认后执行）。桌面端按钮通过 SetEnabled 热切换。
type PlannerHost struct {
	executor *AgentRunner
	enabled  atomic.Bool
	asker    Asker
	sink     event.Sink

	executorSinkWrapped bool // 防止 wrapExecutorSink 重复包装
}

// NewPlannerHost 创建单模型规划模式宿主。autoPlan 为 "off" 时规划关闭，
// 其余非空值（"ask"/"on"）开启。
func NewPlannerHost(executor *AgentRunner, autoPlan string, sink event.Sink) *PlannerHost {
	h := &PlannerHost{executor: executor, sink: sink}
	h.enabled.Store(autoPlan != "" && autoPlan != "off")
	return h
}

// Enabled 报告规划模式当前是否开启。
func (h *PlannerHost) Enabled() bool {
	if h == nil {
		return false
	}
	return h.enabled.Load()
}

// SetEnabled 热切换规划模式（桌面端按钮）。
func (h *PlannerHost) SetEnabled(v bool) {
	if h == nil {
		return
	}
	h.enabled.Store(v)
}

// SetAsker 安装交互确认器；nil 表示 headless（计划自动确认）。
// 同时接线到 executor，使规划阶段也能用 ask 工具澄清需求。
func (h *PlannerHost) SetAsker(a Asker) {
	h.asker = a
	if h.executor != nil {
		h.executor.SetAsker(a)
	}
}

// Run 执行一轮。规划模式关闭或任务不需要规划时直接执行；
// 否则走 规划 → 确认 → 执行 → 自动修正 流程。
func (h *PlannerHost) Run(ctx context.Context, input string) (*TurnResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	h.sink.Emit(event.Event{Kind: event.TurnStarted})

	// "!" 前缀显式跳过规划，直接执行。
	if task, ok := shouldSkipPlanner(input); ok {
		if task == "" {
			// 裸 "!" 没有任务——按正常路径处理，避免空派发。
			return h.runExecutor(ctx, input, false)
		}
		return h.runExecutor(ctx, task, true)
	}

	// 路由：只有复杂/多文件/高风险任务（RoutePlanAndExec）且规划模式
	// 开启时才进入规划流程；其余直接执行（保持默认 codex 方式的效率）。
	if !h.Enabled() || DecidePlannerRoute(input).Route != RoutePlanAndExec {
		return h.runExecutor(ctx, input, false)
	}
	return h.runPlanned(ctx, input)
}

// runExecutor 直接派发执行轮，抑制重复的 TurnStarted 并发出结果事件。
func (h *PlannerHost) runExecutor(ctx context.Context, input string, clearPlan bool) (*TurnResult, error) {
	defer h.wrapExecutorSink()()
	execResult, execErr := h.executor.Run(ctx, input)
	h.emitExecutorResult(execResult, execErr, false, clearPlan)
	return execResult, execErr
}

// runPlanned 执行 规划 → 确认 → 执行 → 自动修正 的完整流程。
func (h *PlannerHost) runPlanned(ctx context.Context, input string) (*TurnResult, error) {
	h.sink.Emit(event.Event{Kind: event.Phase, Text: h.executor.ProvName() + " · 规划"})
	defer h.wrapExecutorSink()()

	confirmed, err := h.planWithConfirmation(ctx, input)
	if err != nil {
		return nil, err
	}
	if confirmed == nil {
		// 规划者直接回答或用户选择仅聊天——无执行轮。
		return &TurnResult{Plan: input, Success: true}, nil
	}

	// 执行已确认的计划。
	h.sink.Emit(event.Event{Kind: event.Phase, Text: h.executor.ProvName() + " · 执行"})
	execResult, execErr := h.executor.Run(ctx, confirmedExecutionInput(confirmed.text))
	if execResult != nil {
		execResult.Plan = confirmed.text
	}
	if execResult != nil && execErr != nil {
		execResult.Errors = append(execResult.Errors, execErr.Error())
	}

	// 自动修正循环：失败步骤由同一模型生成修正计划并立即执行（≤3 轮）。
	// 单模型下修正轮 = 一条提示（buildFixPrompt）→ 模型先输出 <!--plan-->
	// 修正计划再直接执行，不再经过独立的规划确认。
	var fixHistory []fixAttempt
	for round := 2; round <= 3; round++ {
		if execErr != nil || execResult == nil {
			break
		}
		if h.allStepsPassed(execResult, confirmed.text) {
			break
		}
		h.sink.Emit(event.Event{Kind: event.Phase, Text: "修正执行 (轮 " + strconv.Itoa(round) + "/3)"})
		fixInput := buildFixPrompt(input, confirmed.text, execResult, round, fixHistory)
		execResult, execErr = h.executor.Run(ctx, fixInput)
		if execResult != nil {
			execResult.Plan = confirmed.text
		}
		feedback := ""
		if execResult != nil {
			feedback = formatExecutionFeedbackEnhanced(execResult, confirmed.text)
			if len([]rune(feedback)) > 4096 {
				feedback = truncateString(feedback, 4096) + "\n...(truncated)"
			}
		}
		fixHistory = append(fixHistory, fixAttempt{
			round:    round,
			fixPlan:  fixInput,
			feedback: feedback,
		})
	}

	if execResult != nil && !h.allStepsPassed(execResult, execResult.Plan) && execErr == nil {
		h.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
			Text: "已尝试 3 轮自动修正，仍有失败步骤，请手动检查"})
	}
	retriesExhausted := len(fixHistory) > 0 && execResult != nil && !h.allStepsPassed(execResult, execResult.Plan)
	h.emitExecutorResult(execResult, execErr, retriesExhausted, false)
	return execResult, execErr
}

// planWithNote 捆绑已确认的计划与用户备注。
type planWithNote struct {
	text     string
	userNote string
}

// planWithConfirmation 运行规划轮并在交互模式下循环确认：
// 用户"按用户意见修改计划"时把反馈追加进输入重新规划。
// 返回 nil 表示规划者直接回答或用户选择仅聊天。
func (h *PlannerHost) planWithConfirmation(ctx context.Context, input string) (*planWithNote, error) {
	plan, err := h.planOnce(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("planner: %w", err)
	}
	if isAnswerNotAction(plan) {
		return nil, nil // 规划者直接回答，无需执行
	}
	// headless（asker=nil）或简单计划（≤3 步、无新文件、可验证）自动确认。
	if h.asker == nil || shouldAutoConfirm(plan) {
		return &planWithNote{text: plan}, nil
	}
	for {
		userNote, chatOnly, revise, err := h.confirmPlan(ctx, input, plan)
		if err != nil {
			return nil, err
		}
		if chatOnly {
			return nil, nil
		}
		if !revise {
			return &planWithNote{text: plan, userNote: userNote}, nil
		}
		if userNote != "" {
			input = input + "\n\n—— User feedback on previous plan ——\n" + userNote
		}
		plan, err = h.planOnce(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("planner: %w", err)
		}
		if isAnswerNotAction(plan) {
			return nil, nil
		}
	}
}

// planOnce 以只读门控跑一轮规划。规划指令（planmode.Marker + 用户输入）
// 与产出的计划都会留在同一 session，执行轮直接可见。
func (h *PlannerHost) planOnce(ctx context.Context, input string) (string, error) {
	// 规划轮临时进入 plannerMode：跳过 executor 专属逻辑与三闸门，
	// 使模型输出计划后自然停止。turn 内单 goroutine 串行，切换安全。
	h.executor.setPlannerMode(true)
	h.executor.SetPlanMode(true)
	planResult, planErr := h.executor.Run(ctx, planmode.Marker+"\n\n"+input)
	h.executor.SetPlanMode(false)
	h.executor.setPlannerMode(false)
	if planErr != nil {
		return "", planErr
	}
	plan := strings.TrimSpace(planResult.Summary)
	if plan == "" {
		return "", fmt.Errorf("planner produced no output")
	}
	return plan, nil
}

// confirmedExecutionInput 构造执行轮输入：计划文本已在同一 session，
// 这里显式引用已确认的计划，防止确认环节修订后产生歧义。
func confirmedExecutionInput(plan string) string {
	return "已确认执行以下计划：\n\n" + displayPlan(plan)
}

// allStepsPassed 检查执行结果中是否所有步骤成功。
// 判定顺序（最可靠信号优先）：
//  1. StepResults 非空且全部 success → 通过；
//  2. StepResults 非空且含非 success → 失败；
//  3. 无 StepResults 但计划有步骤 → 失败（执行者未签收）；
//  4. 无 StepResults 且计划无步骤 → 回退 Success/Errors（只读任务）。
func (h *PlannerHost) allStepsPassed(r *TurnResult, plan string) bool {
	if r == nil {
		return false
	}
	if len(r.StepResults) > 0 {
		for _, sr := range r.StepResults {
			if sr.Status != "success" {
				return false
			}
		}
		return true
	}
	if countPlanSteps(plan) > 0 {
		return false
	}
	if len(r.Errors) > 0 || !r.Success {
		return false
	}
	return true
}

// buildFixPrompt 构造修正计划的提示。
// round 2（或没有历史时）是定向修复；round 3+ 是反思轮。
func buildFixPrompt(origInput, originalPlan string, failed *TurnResult, round int, fixHistory []fixAttempt) string {
	var failedSteps []string
	for _, sr := range failed.StepResults {
		if sr.Status != "success" {
			failedSteps = append(failedSteps, fmt.Sprintf("- ❌ %s: %s", sr.Step, sr.Result))
		}
	}
	errSummary := strings.Join(failed.Errors, "; ")
	execFeedback := formatExecutionFeedbackEnhanced(failed, originalPlan)
	if len([]rune(execFeedback)) > 4096 {
		execFeedback = truncateString(execFeedback, 4096) + "\n...(truncated)"
	}

	var fixTarget string
	if len(failedSteps) > 0 {
		fixTarget = fmt.Sprintf("失败步骤:\n%s", strings.Join(failedSteps, "\n"))
	} else if errSummary != "" {
		fixTarget = fmt.Sprintf("执行错误（无步骤级别失败，以下错误需要修复）:\n%s", errSummary)
	} else {
		fixTarget = "执行反馈中未收集到步骤级别的失败信息——检查是否所有 complete_step 调用都成功了。确认任务实际完成状态，若已完成则直接结束，否则补充缺失的修复。"
	}
	if uncovered := checkStepCoverage(originalPlan, failed); len(uncovered) > 0 {
		fixTarget += fmt.Sprintf("\n\n计划中存在但执行未签收的步骤（可能被跳过或改名，修正计划必须覆盖）:\n%s",
			strings.Join(uncovered, "\n"))
	}

	if round == 2 || len(fixHistory) == 0 {
		return fmt.Sprintf(`以下步骤执行失败，请创建最小修正计划，仅修正失败的步骤，然后立即执行：

原始任务:
%s

原计划:
%s

执行反馈:
%s

%s

修正计划要求:
- 仅修复失败的部分，不重做成功步骤
- 先输出 <!--plan--> 修正计划，然后直接执行，不再询问用户
`, origInput, originalPlan, execFeedback, fixTarget)
	}

	var historyLines []string
	for _, a := range fixHistory {
		historyLines = append(historyLines, fmt.Sprintf(
			"--- 第 %d 轮修正 ---\n修正计划:\n%s\n\n执行反馈:\n%s",
			a.round, a.fixPlan, a.feedback))
	}
	return fmt.Sprintf(`前两轮针对性修补均未完全解决，请重新审视整体方案，然后立即执行：

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
- 先输出 <!--plan--> 修正计划，然后直接执行，不再询问用户
`, origInput, originalPlan, strings.Join(historyLines, "\n\n"), strings.Join(failedSteps, "\n"), errSummary)
}

// fixAttempt 记录一轮修正的历史，供 round 3 反思使用。
type fixAttempt struct {
	round    int
	fixPlan  string
	feedback string
}

// emitExecutorResult 发出摘要文本与 TurnResultEvent（前端结果卡）。
func (h *PlannerHost) emitExecutorResult(r *TurnResult, execErr error, retriesExhausted, clearPlan bool) {
	if r != nil && clearPlan {
		r.Plan = ""
	}
	summary := h.formatSummary(r, execErr, retriesExhausted)
	if summary != "" {
		h.sink.Emit(event.Event{Kind: event.Text, Text: summary})
	}
	if r != nil {
		h.sink.Emit(event.Event{
			Kind: event.TurnResultEvent,
			PlanResult: &event.PlanResult{
				Plan:          r.Plan,
				FilesCreated:  r.FilesCreated,
				FilesModified: r.FilesModified,
				Success:       r.Success,
				Errors:        r.Errors,
				Summary:       summary,
			},
		})
	} else if execErr != nil {
		h.sink.Emit(event.Event{
			Kind: event.TurnResultEvent,
			PlanResult: &event.PlanResult{
				Success: false,
				Errors:  []string{execErr.Error()},
				Summary: summary,
			},
		})
	}
}

// formatSummary 生成简洁的完成摘要，纯字符串格式化，无额外模型调用。
func (h *PlannerHost) formatSummary(r *TurnResult, execErr error, retriesExhausted bool) string {
	if r == nil {
		if execErr != nil {
			return "❌ 执行失败: " + execErr.Error()
		}
		return ""
	}
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
	} else if r.Success && (len(r.FilesCreated) > 0 || len(r.FilesModified) > 0) {
		msg += "\n（未记录步骤详情）"
	}
	if retriesExhausted {
		msg += "\n⚠️ 已尝试多轮自动修正"
	}
	return msg
}

// wrapExecutorSink 抑制执行轮的 TurnStarted 事件（PlannerHost 已在 Run
// 顶部发过一次），防止前端每轮用量统计被重置。返回恢复函数。
func (h *PlannerHost) wrapExecutorSink() func() {
	if h.executorSinkWrapped {
		return func() {}
	}
	h.executorSinkWrapped = true
	origSink := h.executor.Sink()
	h.executor.SetSink(event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnStarted {
			return
		}
		origSink.Emit(e)
	}))
	return func() {
		h.executor.SetSink(origSink)
		h.executorSinkWrapped = false
	}
}

// shouldSkipPlanner 检测 "!" 前缀（显式跳过规划直接执行）。
// 尾部 "\n\n" 的 Compose 附加块会被剥离。
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

// isAnswerNotAction 判断规划轮输出是否为直接回答（无执行需要）。
// 带 <!--plan--> 或计划结构（步骤行/字段）视为计划；否则是回答。
func isAnswerNotAction(plan string) bool {
	trimmed := strings.TrimSpace(plan)
	if strings.Contains(trimmed, "<!--plan-->") {
		return false
	}
	return !looksLikePlan(trimmed)
}

// truncateString 按 rune 安全截断，避免多字节字符（中文/emoji）被切坏。
func truncateString(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}

// formatExecutionFeedback 将 TurnResult 转为结构化执行反馈。
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
		conclusion = "\n- ✅ 任务已完成"
	} else if r.Success {
		conclusion = "\n- ⚠️ 步骤已完成但存在非致命警告（见 Errors）"
	} else {
		conclusion = "\n- ❌ 任务未完成，请检查 Errors 并修正"
	}
	return fmt.Sprintf("[上一轮执行结果] %s\n- Created: %s\n- Modified: %s\n- Errors: %s\n- Summary: %s%s\n", status, created, modified, errors, summary, conclusion)
}
