package agent

import "testing"

// V10.134 契约：单模型交互模式的 auto_plan 决策——复用 DecidePlannerRoute，
// 只有复杂/多步骤任务（RoutePlanAndExec）才进入计划模式。

func TestShouldAutoPlan_Disabled(t *testing.T) {
	for _, mode := range []string{"", "off"} {
		if ShouldAutoPlan("修复并发竞态问题", mode) {
			t.Errorf("ShouldAutoPlan(complex, %q) should be false", mode)
		}
	}
	if ShouldAutoPlan("修复并发竞态问题", "invalid") {
		t.Error("unknown mode should disable auto-plan")
	}
}

func TestShouldAutoPlan_EnabledComplex(t *testing.T) {
	for _, mode := range []string{"ask", "on"} {
		for _, input := range []string{
			"修复并发竞态问题",   // high_risk
			"重构认证模块",       // complex
			"前后端联调",        // cross_surface
			"fix the cache bug in user.go and auth.go", // multi-file
		} {
			if !ShouldAutoPlan(input, mode) {
				t.Errorf("ShouldAutoPlan(%q, %q) should be true (complex task)", input, mode)
			}
		}
	}
}

func TestShouldAutoPlan_SimpleStaysDirect(t *testing.T) {
	for _, mode := range []string{"ask", "on"} {
		for _, input := range []string{
			"你好",
			"fix typo in readme", // atomic
			"构建",               // directive
			"运行测试",            // read_only
		} {
			if ShouldAutoPlan(input, mode) {
				t.Errorf("ShouldAutoPlan(%q, %q) should be false (simple task)", input, mode)
			}
		}
	}
}
