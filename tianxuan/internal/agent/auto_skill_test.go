package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tianxuan/internal/skill"
)

// TestWithAutoSkillDedup: 同一技能会话内只注入一次——后续轮次消息保持
// 接近原始输入（少塞重复正文 = 少付 miss token），compaction 重置后允许
// 重新注入（历史被摘要、技能正文可能丢失）。
func TestWithAutoSkillDedup(t *testing.T) {
	home := t.TempDir()
	if err := skill.EnsureBundled(home); err != nil {
		t.Fatal(err)
	}
	st := skill.New(skill.Options{HomeDir: home, DisableBuiltins: true})
	a := &AgentRunner{autoSkill: st}

	in := "用 TDD 实现这个功能"
	first := a.withAutoSkill(in)
	if !strings.Contains(first, "<auto-skill>") {
		t.Fatalf("first call should inject the skill body: %.100s", first)
	}
	second := a.withAutoSkill(in)
	if second != in {
		t.Fatalf("same skill on a later turn must not re-inject (dedup): %q", second)
	}
	// 不同技能仍正常注入
	third := a.withAutoSkill("程序崩溃了帮我排查一下")
	if !strings.Contains(third, "<auto-skill>") {
		t.Fatalf("a different skill should still inject: %.100s", third)
	}
	// compaction 重置（autoInjected 清空）后允许重新注入
	a.autoInjected = nil
	fourth := a.withAutoSkill(in)
	if !strings.Contains(fourth, "<auto-skill>") {
		t.Fatalf("after compaction reset the skill should inject again: %.100s", fourth)
	}
}

// TestCacheHitPrefixStableWithAutoSkill locks the DeepSeek full-message prefix
// invariant under auto-skill injection: every request re-sends the full prior
// history byte-identical, so the cached prefix equals the entire previous
// request even though turn 1 carried an injected skill block.
func TestCacheHitPrefixStableWithAutoSkill(t *testing.T) {
	home := t.TempDir()
	if err := skill.EnsureBundled(home); err != nil {
		t.Fatal(err)
	}
	st := skill.New(skill.Options{HomeDir: home, DisableBuiltins: true})

	mock := &mockDeepSeek{t: t, withTools: true, reasoning: longReasoning, toolRounds: 2}
	srv := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer srv.Close()

	a, _ := newAgent(t, srv.URL, mock.tools(), 0 /*no compaction*/, 0)
	a.SetAutoSkillStore(st)

	if _, err := a.Run(context.Background(), "用 TDD 实现这个功能并完成"); err != nil {
		t.Fatalf("Run 1: %v", err)
	}
	if _, err := a.Run(context.Background(), "继续"); err != nil {
		t.Fatalf("Run 2: %v", err)
	}

	for i := 1; i < len(mock.reqChars); i++ {
		if mock.hitChars[i] != mock.reqChars[i-1] {
			t.Errorf("PREFIX BROKEN at req %d: cached %d chars but the full prior request was %d chars (auto-skill injection must keep history byte-stable)",
				i, mock.hitChars[i], mock.reqChars[i-1])
		}
	}
}
