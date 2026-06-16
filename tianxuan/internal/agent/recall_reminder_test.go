package agent

import (
	"testing"

	"tianxuan/internal/memory"
	"tianxuan/internal/provider"
)

// stubMemQueue is a minimal memory.Queue for testing.
type stubMemQueue struct{}

func (stubMemQueue) QueueMemory(_ string) {}

// TestRecallReminderInjectsWhenMemoryExists verifies the nudge is added.
func TestRecallReminderInjectsWhenMemoryExists(t *testing.T) {
	s := NewSession("")
	// Add a memory-update message so the reminder detects memory content
	s.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: "<memory-update>\n- Saved project memory\n</memory-update>",
	})
	a := &AgentRunner{session: s, memQueue: stubMemQueue{}}
	before := len(s.Messages)
	a.maybeRecallReminder()
	if len(s.Messages) != before+1 {
		t.Fatalf("expected +1 message, got +%d", len(s.Messages)-before)
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Role != provider.RoleUser {
		t.Fatalf("expected user role, got %s", last.Role)
	}
	if last.Content != recallReminderNudge {
		t.Fatalf("nudge text mismatch: got %q", last.Content)
	}
}

// TestRecallReminderSkipsWhenNoMemory verifies no nudge without memory.
func TestRecallReminderSkipsWhenNoMemory(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s} // memQueue is nil
	before := len(s.Messages)
	a.maybeRecallReminder()
	if len(s.Messages) != before {
		t.Fatal("expected no message when memQueue is nil")
	}
}

// TestRecallReminderNudgeConstant verifies the nudge text is safe.
func TestRecallReminderNudgeConstant(t *testing.T) {
	if recallReminderNudge == "" {
		t.Fatal("recallReminderNudge is empty")
	}
	verbs := []string{"%s", "%d", "%v", "%q", "%f", "%t", "%x", "%T"}
	for _, verb := range verbs {
		if containsStr(recallReminderNudge, verb) {
			t.Fatalf("recallReminderNudge contains format verb %q", verb)
		}
	}
}

// TestRecallReminderOneShot verifies the reminder fires only once per session.
func TestRecallReminderOneShot(t *testing.T) {
	s := NewSession("")
	s.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: "<memory-update>\n- Saved project memory\n</memory-update>",
	})
	a := &AgentRunner{session: s, memQueue: stubMemQueue{}}
	a.maybeRecallReminder()
	if len(s.Messages) != 2 { // memory-update + nudge
		t.Fatalf("first call: expected 2 messages, got %d", len(s.Messages))
	}
	a.maybeRecallReminder() // second call should be no-op
	if len(s.Messages) != 2 {
		t.Fatalf("second call: expected still 2 messages (one-shot), got %d", len(s.Messages))
	}
}

// compile-time check: stubMemQueue satisfies memory.Queue
var _ memory.Queue = stubMemQueue{}
