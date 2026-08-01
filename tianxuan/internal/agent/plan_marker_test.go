package agent

import "testing"

// O4 契约：<!--plan--> 标记缺失但输出呈计划结构时，不能静默当作聊天
// 回复——否则规划者漏写标记会导致任务不执行且无任何提示。

func TestIsAnswerNotAction_PlanStructureWithoutMarker(t *testing.T) {
	cases := []struct {
		text    string
		isAnswer bool // true = 当作直接回答（不执行）
	}{
		// 计划结构但缺标记 → 视为计划（走确认，用户可"仅聊天"取消）
		{"步骤 1：写失败测试\n步骤 2：最小实现", false},
		{"Step 1: write failing test\nStep 2: implement", false},
		{"步骤 1：写失败测试\n- **Verify**：go test ./...", false},
		{"步骤 1：重构\n- **File(s)**：internal/agent/hermes.go", false},
		{"步骤 1：加字段\n- **Delta**：MODIFIED", false},
		// 无计划结构 → 直接回答
		{"步骤 1：单步无字段", true},
		{"这是一个直接回答", true},
		{"[no_changes]", true},
		{"", true},
		// 有标记 → 计划（原有行为）
		{"<!--plan-->\n步骤 1：x", false},
	}
	for _, c := range cases {
		if got := isAnswerNotAction(c.text); got != c.isAnswer {
			t.Errorf("isAnswerNotAction(%q) = %v, want %v", c.text, got, c.isAnswer)
		}
	}
}

func TestLooksLikePlan(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"步骤 1：a\n步骤 2：b", true},
		{"步骤 1：a\n- **Verify**：go test", true},
		{"步骤 1：a", false},
		{"普通聊天", false},
		{"", false},
	}
	for _, c := range cases {
		if got := looksLikePlan(c.text); got != c.want {
			t.Errorf("looksLikePlan(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}
