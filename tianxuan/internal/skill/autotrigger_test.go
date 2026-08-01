package skill

import (
	"strings"
	"testing"
)

func TestMatchSkill(t *testing.T) {
	cases := []struct{ input, want string }{
		{"用 TDD 实现这个功能", "tdd"},
		{"tdd", "tdd"},
		{"先写测试再实现", "tdd"},
		{"程序崩溃了帮我看看", "systematic-debugging"},
		{"go test 失败了，报错信息如下", "systematic-debugging"},
		{"帮我排查这个定位问题", "systematic-debugging"},
		{"合并前帮我 review 一下代码", "requesting-code-review"},
		{"帮我做代码审查", "requesting-code-review"},
		{"按审查意见修改一下代码", "receiving-code-review"},
		{"根据 review 反馈调整实现", "receiving-code-review"},
		{"开发完成了，帮我收尾这个分支", "finish-development-branch"},
		{"帮我写一个网页", ""},
		{"今天天气怎么样", ""},
		{"帮我设计一个 logo", ""},
	}
	for _, c := range cases {
		if got := MatchSkill(c.input); got != c.want {
			t.Errorf("MatchSkill(%q) = %q, want %q", c.input, got, c.want)
		}
	}
	// 确定性：同输入 → 同结果（前缀缓存安全前提）
	if MatchSkill("程序崩溃了") != MatchSkill("程序崩溃了") {
		t.Fatal("MatchSkill must be deterministic")
	}
}

// TestInjectAutoSkillDeterministic locks the byte-level determinism the
// DeepSeek prefix cache needs: same input + same skill → identical bytes.
func TestInjectAutoSkillDeterministic(t *testing.T) {
	home := t.TempDir()
	if err := EnsureBundled(home); err != nil {
		t.Fatal(err)
	}
	st := New(Options{HomeDir: home, DisableBuiltins: true})
	in := "用 TDD 实现这个功能"
	out1 := InjectAutoSkill(in, st)
	out2 := InjectAutoSkill(in, st)
	if out1 != out2 {
		t.Fatalf("injection must be byte-deterministic (cache safety):\n%q\nvs\n%q", out1, out2)
	}
	if !strings.HasPrefix(out1, "<auto-skill>") {
		t.Errorf("should open with <auto-skill> block: %.80s", out1)
	}
	if !strings.Contains(out1, "</auto-skill>") {
		t.Errorf("missing </auto-skill> close tag")
	}
	if !strings.Contains(out1, "测试驱动开发") {
		t.Errorf("skill body should be injected")
	}
	if !strings.HasSuffix(out1, in) {
		t.Errorf("original user input must be preserved at the end: %.120s", out1)
	}
}

func TestInjectAutoSkillNoMatchPassthrough(t *testing.T) {
	home := t.TempDir()
	if err := EnsureBundled(home); err != nil {
		t.Fatal(err)
	}
	st := New(Options{HomeDir: home, DisableBuiltins: true})
	in := "帮我写一个网页"
	if got := InjectAutoSkill(in, st); got != in {
		t.Errorf("no-match input must pass through unchanged, got %q", got)
	}
}

// TestInjectAutoSkillSkipsSubagent: subagent skills are already surfaced as
// dedicated tools (explore/review/...) — auto-injecting their bodies would be
// redundant and would leak isolation.
func TestInjectAutoSkillSkipsSubagent(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".tianxuan/skills/sub.md", "---\ndescription: subagent skill\nrunas: subagent\n---\nsub body")
	st := New(Options{HomeDir: home, DisableBuiltins: true})
	rules := []AutoTriggerRule{{SkillName: "sub", Keywords: []string{"sub触发"}}}
	in := "sub触发场景"
	if got := injectAutoSkill(in, st, rules); got != in {
		t.Errorf("subagent skills must not be auto-injected, got %q", got)
	}
}

func TestInjectAutoSkillMissingSkillPassthrough(t *testing.T) {
	home := t.TempDir()
	if err := EnsureBundled(home); err != nil {
		t.Fatal(err)
	}
	st := New(Options{HomeDir: home, DisableBuiltins: true})
	rules := []AutoTriggerRule{{SkillName: "no-such-skill", Keywords: []string{"未知技能触发词"}}}
	in := "未知技能触发词场景"
	if got := injectAutoSkill(in, st, rules); got != in {
		t.Errorf("missing skill should pass through unchanged, got %q", got)
	}
}

// TestInjectAutoSkillTruncatesLongBody: 超长技能正文注入截断到上限，
// 控制每轮注入的缓存 miss token 成本；截断必须是确定性的。
func TestInjectAutoSkillTruncatesLongBody(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".tianxuan/skills/long.md",
		"---\ndescription: long skill\n---\n"+strings.Repeat("正文内容很长。", 2000))
	st := New(Options{HomeDir: home, DisableBuiltins: true})
	rules := []AutoTriggerRule{{SkillName: "long", Keywords: []string{"长技能"}}}
	in := "长技能场景"
	out1 := injectAutoSkill(in, st, rules)
	out2 := injectAutoSkill(in, st, rules)
	if out1 != out2 {
		t.Fatal("truncation must be byte-deterministic (cache safety)")
	}
	if !strings.Contains(out1, "已截断") {
		t.Errorf("should note truncation")
	}
	if got := len([]rune(out1)); got > maxAutoSkillBodyChars+len([]rune(in))+120 {
		t.Errorf("injected body too large: %d runes (cap %d)", got, maxAutoSkillBodyChars)
	}
}
