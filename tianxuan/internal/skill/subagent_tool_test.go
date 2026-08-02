package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSubagentSkillToolDispatches verifies a dedicated subagent skill tool
// routes its task through the subagent runner with the built-in skill body.
func TestSubagentSkillToolDispatches(t *testing.T) {
	var ran string
	runner := func(_ context.Context, sk Skill, task string) (string, error) {
		ran = sk.Name + ":" + task
		return "distilled answer", nil
	}
	st := New(Options{HomeDir: t.TempDir()})
	tl := NewSubagentSkillTool(st, runner, "explore", "desc", "compact")
	if tl.Name() != "explore" || !tl.ReadOnly() {
		t.Errorf("name=%q readOnly=%v", tl.Name(), tl.ReadOnly())
	}
	out, err := tl.Execute(context.Background(), json.RawMessage(`{"task":"map the loop"}`))
	if err != nil {
		t.Fatal(err)
	}
	if ran != "explore:map the loop" {
		t.Errorf("runner got %q, want explore:map the loop", ran)
	}
	if !strings.Contains(out, "distilled") {
		t.Errorf("out = %q", out)
	}
}

// TestSubagentSkillToolRequiresTask verifies an empty task is rejected loudly.
func TestSubagentSkillToolRequiresTask(t *testing.T) {
	st := New(Options{HomeDir: t.TempDir()})
	tl := NewSubagentSkillTool(st, nil, "explore", "d", "c")
	if _, err := tl.Execute(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Error("subagent skill tool must require a task")
	}
}

// TestSubagentSkillToolUnknownSkill verifies a missing skill errors rather
// than silently passing a nil body to the runner.
func TestSubagentSkillToolUnknownSkill(t *testing.T) {
	st := New(Options{HomeDir: t.TempDir(), DisableBuiltins: true})
	tl := NewSubagentSkillTool(st, nil, "nope", "d", "c")
	if _, err := tl.Execute(context.Background(), json.RawMessage(`{"task":"x"}`)); err == nil {
		t.Error("unknown skill must error")
	}
}

// TestSubagentSkillToolNameNormalisation verifies hyphenated skill identifiers
// surface as underscore tool names (security-review → security_review) and
// still resolve the skill when executed.
func TestSubagentSkillToolNameNormalisation(t *testing.T) {
	var ran string
	runner := func(_ context.Context, sk Skill, task string) (string, error) {
		ran = sk.Name + ":" + task
		return "ok", nil
	}
	st := New(Options{HomeDir: t.TempDir()})
	tl := NewSubagentSkillTool(st, runner, "security-review", "d", "c")
	if got := tl.Name(); got != "security_review" {
		t.Fatalf("tool name = %q, want security_review", got)
	}
	if _, err := tl.Execute(context.Background(), json.RawMessage(`{"task":"check auth"}`)); err != nil {
		t.Fatal(err)
	}
	if ran != "security-review:check auth" {
		t.Errorf("runner got %q, want security-review:check auth", ran)
	}
}

// TestUiStylingToolGuide verifies the guide action returns the matching
// reference file from the ui-styling skill.
func TestUiStylingToolGuide(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".tianxuan/skills/ui-styling/SKILL.md", "---\ndescription: styling\n---\nbody")
	dir := filepath.Join(home, ".tianxuan", "skills", "ui-styling")
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "references", "shadcn-components.md"), []byte("## Dialog\n\nUse Radix dialog for overlays."), 0o644); err != nil {
		t.Fatal(err)
	}

	tl := NewUiStylingTool(New(Options{HomeDir: home}))
	out, err := tl.Execute(context.Background(), json.RawMessage(`{"action":"guide","args":"dialog"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Radix dialog") {
		t.Errorf("guide should return the dialog reference, got: %q", out)
	}
}

// TestUiStylingToolConfigMissingScript verifies config fails loudly when the
// skill ships no tailwind_config_gen.py.
func TestUiStylingToolConfigMissingScript(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".tianxuan/skills/ui-styling/SKILL.md", "---\ndescription: styling\n---\nbody")
	tl := NewUiStylingTool(New(Options{HomeDir: home}))
	if _, err := tl.Execute(context.Background(), json.RawMessage(`{"action":"config","args":"--colors brand:blue"}`)); err == nil {
		t.Error("config without script must error")
	}
}

// TestDesignRouter verifies task classification routes to the expected
// sub-agent skill.
func TestDesignRouter(t *testing.T) {
	st := New(Options{HomeDir: t.TempDir()})
	tl := NewDesignRouterTool(st)
	cases := []struct {
		task string
		want string
	}{
		{"设计一个 Instagram banner", "banner-design"},
		{"做一份产品发布幻灯片", "slides"},
		{"更新品牌声音指南", "brand"},
		{"生成 design token 规范", "design-system"},
		{"给后台页面配一套配色", "ui-ux-pro-max"},
		{"用 shadcn 实现一个 dialog 组件", "ui_styling"},
	}
	for _, c := range cases {
		out, err := tl.Execute(context.Background(), json.RawMessage(`{"task":"`+c.task+`"}`))
		if err != nil {
			t.Fatalf("%s: %v", c.task, err)
		}
		if !strings.Contains(out, `"subagent":"`+c.want+`"`) {
			t.Errorf("route(%q) = %s, want subagent %q", c.task, out, c.want)
		}
	}
}
