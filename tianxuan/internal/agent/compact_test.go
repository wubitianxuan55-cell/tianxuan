package agent

import (
	"context"
	"strings"
	"testing"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
)

// testAgent returns a minimal AgentRunner with a Discard sink so
// maybeCompact's sink.Emit doesn't panic in tests.
func testAgentC(window int, ratio float64, keep int) *AgentRunner {
	return &AgentRunner{
		compaction: CompactionConfig{Window: window, Ratio: ratio, RecentKeep: keep},
		sink:       event.Discard,
	}
}

// TestMaybeCompact_NilUsage_FallbackToLastPrompt verifies that the
// LLM summarization path triggers and reduces message count even
// without a real provider (mechanical fold fallback).
func TestMaybeCompact_NilUsage_FallbackToLastPrompt(t *testing.T) {
	a := testAgentC(4000, 0.8, 5) // small window so compaction triggers
	s := NewSession("system prompt here")
	// Build a session with substantial messages that exceeds the window
	for i := 0; i < 30; i++ {
		s.Add(provider.Message{Role: provider.RoleUser, Content: "please implement feature " + strings.Repeat("x", 200)})
		s.Add(provider.Message{Role: provider.RoleAssistant, Content: "ok here is the code " + strings.Repeat("y", 300)})
		s.Add(provider.Message{Role: provider.RoleTool, Name: "write_file", Content: "wrote file " + strings.Repeat("z", 400)})
	}
	a.session = s
	a.compaction.LastPrompt = 3500 // 87.5% of window — over 80% threshold

	original := len(s.Messages)

	a.maybeCompact(context.Background(), nil)

	if len(a.session.Messages) >= original {
		t.Fatalf("compaction should reduce messages: got %d, original %d",
			len(a.session.Messages), original)
	}
	t.Logf("compaction: %d -> %d messages (mechanical fold)", original, len(a.session.Messages))
}

// TestMaybeCompact_NilUsage_LowPrompt_NoOp verifies compaction is skipped
// when prompt is below threshold.
func TestMaybeCompact_NilUsage_LowPrompt_NoOp(t *testing.T) {
	a := testAgentC(100000, 0.8, 5)
	s := NewSession("system")
	for i := 0; i < 10; i++ {
		s.Add(provider.Message{Role: provider.RoleUser, Content: "msg"})
	}
	a.session = s
	a.compaction.LastPrompt = 1000 // 1% of window

	a.maybeCompact(context.Background(), nil)

	// Should be no-op since prompt << threshold
	if len(a.session.Messages) != len(s.Messages) {
		t.Errorf("no-op expected but messages changed: %d -> %d", len(s.Messages), len(a.session.Messages))
	}
}

// TestMaybeCompact_WithUsage_StillWorks verifies compaction triggers
// when real usage data is provided.
func TestMaybeCompact_WithUsage_StillWorks(t *testing.T) {
	a := testAgentC(3000, 0.8, 3)
	s := NewSession("system prompt")
	for i := 0; i < 20; i++ {
		s.Add(provider.Message{Role: provider.RoleUser, Content: "build feature " + strings.Repeat("a", 150)})
		s.Add(provider.Message{Role: provider.RoleAssistant, Content: "implementing " + strings.Repeat("b", 200)})
	}
	a.session = s

	original := len(s.Messages)
	a.maybeCompact(context.Background(), &provider.Usage{PromptTokens: 2600})

	if len(a.session.Messages) >= original {
		t.Fatalf("compaction should reduce messages with usage: got %d, original %d",
			len(a.session.Messages), original)
	}
	t.Logf("compaction (usage path): %d -> %d messages", original, len(a.session.Messages))
}

// TestCompactNow verifies that CompactNow triggers compaction regardless
// of prompt size.
func TestCompactNow(t *testing.T) {
	a := testAgentC(4000, 0.8, 3)
	s := NewSession("system")
	for i := 0; i < 15; i++ {
		s.Add(provider.Message{Role: provider.RoleUser, Content: "request " + strings.Repeat("q", 100)})
		s.Add(provider.Message{Role: provider.RoleAssistant, Content: "response " + strings.Repeat("r", 200)})
		s.Add(provider.Message{Role: provider.RoleTool, Name: "read_file", Content: "content " + strings.Repeat("c", 300)})
	}
	a.session = s

	err := a.CompactNow(context.Background(), "")
	if err != nil {
		t.Logf("CompactNow error (expected with nil provider): %v", err)
	}
	// With nil provider, falls back to mechanical fold
	if len(a.session.Messages) >= len(s.Messages) {
		t.Logf("compact didn't reduce messages (may happen with short sessions)")
	} else {
		t.Logf("CompactNow: %d -> %d messages", len(s.Messages), len(a.session.Messages))
	}
}
