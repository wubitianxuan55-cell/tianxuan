package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/provider/openai"
	"tianxuan/internal/skill"
	"tianxuan/internal/tool"
)

// 复现：V10.122 之后高频内联技能（tdd / systematic-debugging 等）由
// withAutoSkill 自动注入 playbook，模型不再调用 run_skill —— 前端技能
// 统计只算 run_skill 调用，因此"技能使用"恒为 0，用户误以为技能没生效。

type toolEventSink struct {
	tools []event.Event
}

func (s *toolEventSink) Emit(e event.Event) {
	if e.Kind == event.ToolDispatch {
		s.tools = append(s.tools, e)
	}
}

func TestAutoSkillInjectionEmitsStatEvent(t *testing.T) {
	home := t.TempDir()
	if err := skill.EnsureBundled(home); err != nil {
		t.Fatal(err)
	}
	st := skill.New(skill.Options{HomeDir: home, DisableBuiltins: true})

	mock := &mockDeepSeek{t: t, reasoning: longReasoning}
	srv := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer srv.Close()

	sink := &toolEventSink{}
	prov, err := openai.New(provider.Config{Name: "deepseek", BaseURL: srv.URL, Model: "deepseek-reasoner", APIKey: "test"})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	a := New(prov, tool.NewRegistry(), NewSession(systemPrompt), Options{
		MaxSteps:    3,
		Temperature: 0,
	}, sink)
	a.SetAutoSkillStore(st)

	if _, err := a.Run(context.Background(), "用 TDD 实现这个功能"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// 自动注入必须发出可统计的 run_skill 事件，前端技能侧栏才能看到使用
	var found bool
	for _, e := range sink.tools {
		if e.Tool.Name == "run_skill" && strings.Contains(e.Tool.Args, "tdd") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("auto-injected skill must emit a run_skill stat event (skill usage was invisible)")
	}
}

func TestSoloSystemPrompt_GuidesSubagentRunSkill(t *testing.T) {
	p := SoloSystemPrompt
	for _, kw := range []string{"run_skill", "explore", "[🧬 subagent]", "review"} {
		if !strings.Contains(p, kw) {
			t.Errorf("SoloSystemPrompt should guide subagent skills via run_skill, missing %q", kw)
		}
	}
}
