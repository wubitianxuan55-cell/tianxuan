package agent

import (
	"strings"
	"testing"
)

// ── shouldSkipPlanner ──────────────────────────────────────

func TestShouldSkipPlanner_BangPrefix(t *testing.T) {
	s, ok := shouldSkipPlanner("!build desktop")
	if !ok {
		t.Fatal("expected true for ! prefix")
	}
	if s != "build desktop" {
		t.Fatalf("got %q, want %q", s, "build desktop")
	}
}

func TestShouldSkipPlanner_NoBang(t *testing.T) {
	_, ok := shouldSkipPlanner("build desktop")
	if ok {
		t.Fatal("expected false without ! prefix")
	}
}

func TestShouldSkipPlanner_Empty(t *testing.T) {
	_, ok := shouldSkipPlanner("")
	if ok {
		t.Fatal("expected false for empty input")
	}
}

func TestShouldSkipPlanner_BangOnly(t *testing.T) {
	s, ok := shouldSkipPlanner("!")
	if !ok {
		t.Fatal("expected true for bare !")
	}
	if s != "" {
		t.Fatalf("got %q, want empty", s)
	}
}

func TestShouldSkipPlanner_BangWithSpaces(t *testing.T) {
	s, ok := shouldSkipPlanner("!  run tests  ")
	if !ok {
		t.Fatal("expected true for ! with spaces")
	}
	if s != "run tests" {
		t.Fatalf("got %q, want %q", s, "run tests")
	}
}

// ── isAnswerNotAction ──────────────────────────────────────

func TestIsAnswerNotAction_Short(t *testing.T) {
	if !isAnswerNotAction("ok") {
		t.Fatal("short text should be treated as answer")
	}
}

func TestIsAnswerNotAction_Empty(t *testing.T) {
	if !isAnswerNotAction("") {
		t.Fatal("empty text should be treated as answer")
	}
}

func TestIsAnswerNotAction_WhitespaceOnly(t *testing.T) {
	if !isAnswerNotAction("   ") {
		t.Fatal("whitespace-only should be treated as answer")
	}
}

func TestIsAnswerNotAction_WithPlan(t *testing.T) {
	// Need > 100 chars to avoid the short-circuit for short texts
	prefix := strings.Repeat("x", 120)
	plan := "<!--plan-->\n" + prefix
	if isAnswerNotAction(plan) {
		t.Fatal("text containing <!--plan--> should NOT be treated as answer")
	}
}

func TestIsAnswerNotAction_LongWithoutPlan(t *testing.T) {
	long := strings.Repeat("x", 150)
	// Long text without <!--plan--> IS a direct answer (Hermes answered directly)
	if !isAnswerNotAction(long) {
		t.Fatal("long text without <!--plan--> should be treated as a direct answer")
	}
}

// ── formatHandoff ─────────────────────────────────────────

func TestFormatHandoff_Normal(t *testing.T) {
	out := formatHandoff("build", "run wails build", "")
	if !strings.Contains(out, hephaestusHandoffMarker) {
		t.Fatal("handoff missing marker")
	}
	if !strings.Contains(out, "Original task:\nbuild") {
		t.Fatal("handoff missing original task")
	}
	if !strings.Contains(out, "Hermes output:\nrun wails build") {
		t.Fatal("handoff missing Hermes output")
	}
	if strings.Contains(out, "📌 User note (written during plan confirmation)") {
		t.Fatal("should not contain user note section when empty")
	}
}

func TestFormatHandoff_WithUserNote(t *testing.T) {
	out := formatHandoff("build", "run wails build", "also run tests first")
	if !strings.Contains(out, "📌 User note (written during plan confirmation):\nalso run tests first") {
		t.Fatal("handoff missing user note")
	}
}

func TestFormatHandoff_EmptyPlan(t *testing.T) {
	out := formatHandoff("build", "", "")
	if !strings.Contains(out, "Hermes output:\n") {
		t.Fatal("handoff should still have Hermes output section")
	}
}

func TestFormatHandoff_SpecialChars(t *testing.T) {
	out := formatHandoff(`test "quotes"`, `plan with <angle>`, "")
	if !strings.Contains(out, `test "quotes"`) {
		t.Fatal("handoff should preserve special chars in task")
	}
	if !strings.Contains(out, `plan with <angle>`) {
		t.Fatal("handoff should preserve special chars in plan")
	}
}

// ── HandoffTask ───────────────────────────────────────────

func TestHandoffTask_ExtractsTask(t *testing.T) {
	handoff := formatHandoff("build the app", "run wails build", "")
	extracted := HandoffTask(handoff)
	if extracted != "build the app" {
		t.Fatalf("got %q, want %q", extracted, "build the app")
	}
}

func TestHandoffTask_NonHandoffPassthrough(t *testing.T) {
	plain := "just a regular message"
	if HandoffTask(plain) != plain {
		t.Fatal("non-handoff should pass through unchanged")
	}
}

func TestHandoffTask_Empty(t *testing.T) {
	if HandoffTask("") != "" {
		t.Fatal("empty should return empty")
	}
}

func TestHandoffTask_ShortMessage(t *testing.T) {
	s := "hi"
	if HandoffTask(s) != s {
		t.Fatal("short message should pass through")
	}
}

// ── persistAnswer ─────────────────────────────────────────

func TestPersistAnswer_NilReceiver(t *testing.T) {
	var h *Hermes
	// should not panic
	h.persistAnswer("query", "answer")
}

func TestPersistAnswer_NilHephaestus(t *testing.T) {
	h := &Hermes{}
	// should not panic
	h.persistAnswer("query", "answer")
}
