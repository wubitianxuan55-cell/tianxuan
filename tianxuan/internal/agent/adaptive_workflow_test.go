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
