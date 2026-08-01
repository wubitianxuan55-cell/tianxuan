package agent

import (
	"strings"
	"testing"
	"time"

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

// TestAutoRecallInjectsBodyAndFreshness verifies the auto-recall block carries
// the memory body (truncated) and a staleness caveat for old memories.
func TestAutoRecallInjectsBodyAndFreshness(t *testing.T) {
	old := time.Now().Add(-72 * time.Hour)
	MemorySearchFunc = func(query string, limit int) []MemoryResult {
		return []MemoryResult{{
			Name:    "build-rule",
			Preview: "Build Rule — Always build with race detector",
			Body:    "Always run `go build -race` before submitting changes.",
			Mtime:   old,
		}}
	}
	t.Cleanup(func() { MemorySearchFunc = nil })

	s := NewSession("")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "怎么构建这个项目？"})
	a := &AgentRunner{session: s}
	before := len(s.Messages)
	a.maybeAutoRecall()
	if len(s.Messages) != before+1 {
		t.Fatalf("expected +1 injected message, got +%d", len(s.Messages)-before)
	}
	last := s.Messages[len(s.Messages)-1]
	if !strings.Contains(last.Content, "go build -race") {
		t.Fatalf("recall block must carry the memory body:\n%s", last.Content)
	}
	if !strings.Contains(last.Content, "3 days") && !strings.Contains(last.Content, "outdated") {
		t.Fatalf("recall block must note staleness for old memory:\n%s", last.Content)
	}
}

// TestAutoRecallRunsOncePerSession verifies auto-recall injects only on the
// first turn of a session — under the four-domain cache, per-turn injection
// would place dynamic bytes before the assistant reply and break the
// stable prefix; a single first-turn block becomes cached history for every
// later turn.
func TestAutoRecallRunsOncePerSession(t *testing.T) {
	MemorySearchFunc = func(query string, limit int) []MemoryResult {
		return []MemoryResult{{Name: "build-rule", Preview: "Build Rule"}}
	}
	t.Cleanup(func() { MemorySearchFunc = nil })

	s := NewSession("")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "怎么构建？"})
	a := &AgentRunner{session: s}
	a.maybeAutoRecall()
	if n := countAutoRecall(s.Snapshot()); n != 1 {
		t.Fatalf("first turn: want 1 injected block, got %d", n)
	}
	// A second user message must NOT trigger another recall injection.
	s.Add(provider.Message{Role: provider.RoleUser, Content: "构建命令是什么？"})
	a.maybeAutoRecall()
	if n := countAutoRecall(s.Snapshot()); n != 1 {
		t.Fatalf("second turn: want still 1 injected block (first-turn only), got %d", n)
	}
}

func countAutoRecall(msgs []provider.Message) int {
	n := 0
	for _, m := range msgs {
		if strings.Contains(m.Content, "[auto-recall]") {
			n++
		}
	}
	return n
}
