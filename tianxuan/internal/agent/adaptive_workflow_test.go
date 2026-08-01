package agent

import (
	"strings"
	"testing"
)

// V10.135 契约：单模型工作流是 Adaptive Execution——todo 是活文档而非
// 合同、无计划确认往返、失败按级升级（重试→根因诊断→换方案→收敛）。
// 与双模型（完整计划→确认→执行，执行者禁改计划）形成明确差异。
func TestSoloSystemPrompt_AdaptiveExecution(t *testing.T) {
	p := SoloSystemPrompt
	required := []string{
		"Adaptive Execution",
		"living document",            // todo 活文档
		"not a contract",             // 计划非合同
		"No plan-approval round-trip", // 无计划确认
		"**Adapt**",                  // 执行中调整计划
		"switch approach",            // 3 次同方案失败换方案
		"diagnose the root cause",    // 根因诊断
		"deliverable subset",         // 收敛策略
	}
	for _, kw := range required {
		if !strings.Contains(p, kw) {
			t.Errorf("SoloSystemPrompt missing Adaptive-Execution keyword %q", kw)
		}
	}
}

// TestSoloSystemPrompt_NoDualModelPlanContract：单模型提示词不得复制双模型
// 的"计划是合同、执行者不得修改"语义。
func TestSoloSystemPrompt_NoDualModelPlanContract(t *testing.T) {
	p := SoloSystemPrompt
	for _, forbidden := range []string{"NEVER write a new plan", "don't question or deviate"} {
		if strings.Contains(p, forbidden) {
			t.Errorf("SoloSystemPrompt must not carry the dual-model plan-contract wording %q", forbidden)
		}
	}
}

// V10.136 契约：单模型必须有进度保护——连续无进展轮次后重新评估 todo /
// 收敛（ask 或缩小范围），不能无限循环。收敛对象是自己（Adaptive），
// 不是"交还 Hermes"（那是双模型执行者的语义）。
func TestSoloSystemPrompt_ProgressGuard(t *testing.T) {
	p := SoloSystemPrompt
	for _, kw := range []string{"Progress guard", "连续 8 轮", "重新评估当前 todo", "连续 16 轮"} {
		if !strings.Contains(p, kw) {
			t.Errorf("SoloSystemPrompt missing progress-guard keyword %q", kw)
		}
	}
	if strings.Contains(p, "交还 Hermes") {
		t.Error("Solo progress guard must converge itself, not hand back to Hermes")
	}
}

// V10.139 契约：子代理并行优先——调查默认走子代理，只有决策/实现/验证
// 所需信息才进主上下文；批量调查中间信息由子代理隔离消化。
func TestSoloSystemPrompt_SubagentDefaultInvestigation(t *testing.T) {
	p := SoloSystemPrompt
	required := []string{
		"default for investigation",
		"隔离上下文",
		"explore", "research",
		"parallel_skills",
		"file:line",
	}
	for _, kw := range required {
		if !strings.Contains(p, kw) {
			t.Errorf("SoloSystemPrompt missing subagent-priority keyword %q", kw)
		}
	}
}
