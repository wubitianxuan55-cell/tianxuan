package agent

import (
	"strings"
	"testing"
)

// V10.135+ 契约：单模型工作流以 Adaptive Execution 为基础——todo 是活文档
// 而非合同、失败按级升级（重试→根因诊断→换方案→收敛）。V10.166 起规划
// 模式（PlannerHost）通过 plan-mode 指令（planmode.Marker）在同一 session
// 引导规划确认，提示词为条件式：收到规划指令先规划等待确认，否则直接执行。
func TestSoloSystemPrompt_AdaptiveExecution(t *testing.T) {
	p := SoloSystemPrompt
	required := []string{
		"Adaptive execution",
		"living document",             // todo 活文档
		"plan-mode directive",         // 规划指令存在时先规划等待确认
		"adapt",                       // 执行中调整计划
		"switch approach",             // 3 次同方案失败换方案
		"diagnose the root cause",     // 根因诊断
		"deliverable subset",          // 收敛策略
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
// 不是"交还规划者"（那是双模型执行者的语义）。
func TestSoloSystemPrompt_ProgressGuard(t *testing.T) {
	p := SoloSystemPrompt
	for _, kw := range []string{"no progress", "reassess"} {
		if !strings.Contains(p, kw) {
			t.Errorf("SoloSystemPrompt missing progress-guard keyword %q", kw)
		}
	}
}

// 单模型调查直接走主上下文（grep/read_file），不再教条式默认派发子代理。
func TestSoloSystemPrompt_InvestigateDirectly(t *testing.T) {
	p := SoloSystemPrompt
	if strings.Contains(p, "default for investigation") {
		t.Error("SoloSystemPrompt must not mandate sub-agent-first investigation")
	}
}
