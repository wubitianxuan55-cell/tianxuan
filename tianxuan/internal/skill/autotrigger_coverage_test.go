package skill

import "testing"

// 复现：最常见的编程任务措辞无法触发技能自动注入——"修复缓存问题"不含
// systematic-debugging 现有关键词（崩溃/报错/调试/排查/修 bug），模型也
// 不会主动 run_skill，导致单模型下"完全不用技能"。
func TestAutoTrigger_CommonBugFixWording(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"修复缓存问题", "systematic-debugging"},
		{"修复这个 bug", "systematic-debugging"},
		{"修一下登录报错", "systematic-debugging"},
		{"测试失败了帮我看看", "systematic-debugging"},
		{"用 TDD 实现这个功能", "tdd"},
		{"先写测试再实现", "tdd"},
		{"补个测试用例", "tdd"},
		{"审查一下这次改动", "requesting-code-review"},
	}
	for _, c := range cases {
		if got := MatchSkill(c.input); got != c.want {
			t.Errorf("MatchSkill(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
