package config

import "testing"

// O6 契约：planner_max_steps 未配置（0）时应用保守默认上限，防止只读
// 规划者无限轮调查造成成本失控；显式配置值保持原样。
func TestPlannerMaxStepsVal_Default(t *testing.T) {
	if got := (AgentConfig{}).PlannerMaxStepsVal(); got != DefaultPlannerMaxSteps {
		t.Errorf("unset planner_max_steps should default to %d, got %d", DefaultPlannerMaxSteps, got)
	}
}

func TestPlannerMaxStepsVal_Explicit(t *testing.T) {
	if got := (AgentConfig{PlannerMaxSteps: 5}).PlannerMaxStepsVal(); got != 5 {
		t.Errorf("explicit 5 should stay 5, got %d", got)
	}
}
