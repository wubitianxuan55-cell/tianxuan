package agent

import (
	"strings"

	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// todoStepStuckNudge 是宿主对"当前 todo 步骤连续多轮工具失败"注入的
// Adaptive Execution 引导（编译期常量，缓存稳定）。
const todoStepStuckNudge = "[system] 当前步骤已连续多轮出现工具失败。停下来：\n" +
	"1. 诊断根因（reproduce → isolate），不要继续盲目尝试。\n" +
	"2. 若当前方案有问题，用 todo_write 调整步骤（拆分/缩小范围/换实现路径），" +
	"并在 complete_step 注明调整原因。\n" +
	"3. 若无法自行决定，用 ask 询问用户。"

// maybeNudgeStuckTodoStep 检测当前 in_progress todo 步骤的跨轮持续失败：
// 每轮存在（非权限阻断的）工具失败则累计；同一步骤连续失败达到阈值时注入
// Adaptive nudge 并重置计数（可再次累计）。todo 步骤变化或出现成功轮时
// 重置——与 detectRepeatedSteps（相同动作签名）互补，本检测覆盖"不同动作
// 但持续失败"的步骤。
func (a *AgentRunner) maybeNudgeStuckTodoStep(calls []provider.ToolCall, results []string) bool {
	if a == nil || a.plannerMode {
		return false
	}
	step := a.currentTodoStep()
	if step == "" {
		a.todoFailStep, a.todoFailCount = "", 0
		return false
	}
	if step != a.todoFailStep {
		a.todoFailStep, a.todoFailCount = step, 0
	}
	failed := 0
	for i := range calls {
		if i >= len(results) {
			break
		}
		if isApproachFailure(results[i]) {
			failed++
		}
	}
	if failed == 0 {
		a.todoFailCount = 0 // 成功轮中断连续失败
		return false
	}
	a.todoFailCount++
	if a.todoFailCount < TodoStepFailNudgeThreshold {
		return false
	}
	a.todoFailCount = 0 // 一次性 nudge，之后可重新累计
	a.session.Add(provider.Message{Role: provider.RoleUser, Content: todoStepStuckNudge})
	return true
}

// currentTodoStep 返回当前 in_progress todo 步骤的展示名（ActiveForm 优先）。
func (a *AgentRunner) currentTodoStep() string {
	a.todoMu.Lock()
	defer a.todoMu.Unlock()
	for _, t := range a.todoState {
		if canonicalTodoStatus(t.Status) == "in_progress" {
			if t.ActiveForm != "" {
				return t.ActiveForm
			}
			return t.Content
		}
	}
	return ""
}

// isApproachFailure 判定工具结果是否为"方案级失败"（非权限阻断）。
// blocked（权限拦截）不是方案问题，不应计入 Adaptive 失败累计。
func isApproachFailure(result string) bool {
	if env, ok := tool.ParseEnvelope(result); ok {
		return !env.OK && env.Code != tool.CodeBlocked
	}
	return strings.HasPrefix(result, "error:") ||
		strings.HasPrefix(result, "Error:") ||
		strings.HasPrefix(result, "[error")
}
