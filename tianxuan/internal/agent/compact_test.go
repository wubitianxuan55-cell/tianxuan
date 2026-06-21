package agent

import (
	"context"
	"testing"

	"tianxuan/internal/strutil"
	"tianxuan/internal/event"
	"tianxuan/internal/provider"
)

// testAgent returns a minimal AgentRunner with a Discard sink so
// maybeCompact's sink.Emit doesn't panic in tests.
func testAgent(window int, ratio float64, keep int) *AgentRunner {
	return &AgentRunner{
		compaction: CompactionConfig{Window: window, Ratio: ratio, RecentKeep: keep},
		sink:       event.Discard,
	}
}

func TestMaybeCompact_NilUsage_FallbackToLastPrompt(t *testing.T) {
	a := testAgent(100000, 0.8, 5)
	// 构建有很多消息的会话，使截断能够触发
	s := NewSession("system")
	for i := 0; i < 100; i++ {
		s.Add(provider.Message{Role: provider.RoleUser, Content: "msg " + strutil.Itoa(i)})
		s.Add(provider.Message{Role: provider.RoleAssistant, Content: "reply " + strutil.Itoa(i)})
	}
	a.session = s
	a.compaction.LastPrompt = 90000 // 90% of window — 超过 80% 触发阈值

	original := len(s.Messages)

	// 传入 nil usage — 应该基于 LastPrompt fallback 仍然触发截断
	a.maybeCompact(context.Background(), nil)

	if len(a.session.Messages) >= original {
		t.Fatalf("期望截断触发 (usage=nil, LastPrompt=%d, window=%d, ratio=%.1f)，但消息数未减少: %d (原 %d)",
			a.compaction.LastPrompt, a.compaction.Window, a.compaction.Ratio,
			len(a.session.Messages), original)
	}
}

func TestMaybeCompact_NilUsage_LowLastPrompt_NoOp(t *testing.T) {
	a := testAgent(100000, 0.8, 5)
	s := NewSession("system")
	for i := 0; i < 50; i++ {
		s.Add(provider.Message{Role: provider.RoleUser, Content: "msg " + strutil.Itoa(i)})
	}
	a.session = s
	a.compaction.LastPrompt = 50000 // 50% of window — 低于 80% 触发阈值

	original := len(s.Messages)
	a.maybeCompact(context.Background(), nil)

	if len(a.session.Messages) != original {
		t.Fatalf("期望不触发截断 (usage=nil, LastPrompt=%d < %d*%.1f=%d)，但消息数变了: %d (原 %d)",
			a.compaction.LastPrompt, a.compaction.Window, a.compaction.Ratio,
			int(float64(a.compaction.Window)*a.compaction.Ratio),
			len(a.session.Messages), original)
	}
}

func TestMaybeCompact_WithUsage_StillWorks(t *testing.T) {
	// 回归测试：确保正常 usage 路径仍然正常
	a := testAgent(100000, 0.8, 5)
	s := NewSession("system")
	for i := 0; i < 200; i++ {
		s.Add(provider.Message{Role: provider.RoleUser, Content: "msg " + strutil.Itoa(i)})
		s.Add(provider.Message{Role: provider.RoleAssistant, Content: "reply " + strutil.Itoa(i)})
	}
	a.session = s
	original := len(s.Messages)

	// usage 高于阈值 → 应该截断
	a.maybeCompact(context.Background(), &provider.Usage{PromptTokens: 95000})

	if len(a.session.Messages) >= original {
		t.Fatalf("期望正常 usage 路径触发截断，但消息数未减少: %d (原 %d)",
			len(a.session.Messages), original)
	}
}

func TestMaybeCompact_WindowZero_NoOp(t *testing.T) {
	a := testAgent(0, 0.8, 5)
	s := NewSession("system")
	for i := 0; i < 100; i++ {
		s.Add(provider.Message{Role: provider.RoleUser, Content: "msg " + strutil.Itoa(i)})
	}
	a.session = s
	original := len(s.Messages)

	// Window=0 表示禁用截断
	a.maybeCompact(context.Background(), &provider.Usage{PromptTokens: 99999})
	a.maybeCompact(context.Background(), nil)

	if len(a.session.Messages) != original {
		t.Fatalf("Window=0 应禁用截断，但消息数变了: %d (原 %d)", len(a.session.Messages), original)
	}
}
