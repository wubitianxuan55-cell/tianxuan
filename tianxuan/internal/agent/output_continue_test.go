package agent

import (
	"testing"

	"tianxuan/internal/provider"
)

// TestOutputLengthContinueInjectsNudge when finish_reason="length" and no tool calls.
func TestOutputLengthContinueInjectsNudge(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}
	u := &provider.Usage{FinishReason: "length"}

	fired := a.maybeContinueOutputLength(u, nil)
	if !fired {
		t.Fatal("expected true for truncated output")
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Content != outputLengthNudge {
		t.Fatalf("nudge text mismatch: got %q", last.Content)
	}
}

// TestOutputLengthContinueSkipsWithToolCalls when the model had tool calls.
func TestOutputLengthContinueSkipsWithToolCalls(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}
	u := &provider.Usage{FinishReason: "length"}
	calls := []provider.ToolCall{{ID: "1", Name: "read_file"}}

	if a.maybeContinueOutputLength(u, calls) {
		t.Fatal("expected false when tool calls present")
	}
}

// TestOutputLengthContinueSkipsNormalFinish when finish_reason is "stop".
func TestOutputLengthContinueSkipsNormalFinish(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}
	u := &provider.Usage{FinishReason: "stop"}

	if a.maybeContinueOutputLength(u, nil) {
		t.Fatal("expected false for normal finish")
	}
}

// TestOutputLengthContinueSafetyValve caps retries.
func TestOutputLengthContinueSafetyValve(t *testing.T) {
	a := &AgentRunner{session: NewSession("")}
	u := &provider.Usage{FinishReason: "length"}

	for i := 0; i < OutputLenNudgeCap; i++ {
		if !a.maybeContinueOutputLength(u, nil) {
			t.Fatalf("retry %d: expected true", i+1)
		}
	}
	if a.maybeContinueOutputLength(u, nil) {
		t.Fatal("after cap: expected false")
	}
}

// TestInvalidOutputRetryInjectsNudge for think-only turn.
func TestInvalidOutputRetryInjectsNudge(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	fired := a.maybeRetryInvalidOutput("", "let me think about this...", nil)
	if !fired {
		t.Fatal("expected true for think-only output")
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Content != invalidOutputNudge {
		t.Fatalf("nudge text mismatch: got %q", last.Content)
	}
}

// TestInvalidOutputRetrySkipsWithText when there's actual content.
func TestInvalidOutputRetrySkipsWithText(t *testing.T) {
	a := &AgentRunner{session: NewSession("")}

	if a.maybeRetryInvalidOutput("here is the answer", "", nil) {
		t.Fatal("expected false when text present")
	}
}

// TestInvalidOutputRetrySkipsWithToolCalls when there are tool calls.
func TestInvalidOutputRetrySkipsWithToolCalls(t *testing.T) {
	a := &AgentRunner{session: NewSession("")}
	calls := []provider.ToolCall{{ID: "1", Name: "read_file"}}

	if a.maybeRetryInvalidOutput("", "thinking...", calls) {
		t.Fatal("expected false when tool calls present")
	}
}

// TestInvalidOutputRetrySkipsEmptyReasoning for truly dead turn.
func TestInvalidOutputRetrySkipsEmptyReasoning(t *testing.T) {
	a := &AgentRunner{session: NewSession("")}

	if a.maybeRetryInvalidOutput("", "", nil) {
		t.Fatal("expected false for empty turn")
	}
}

// TestInvalidOutputRetrySafetyValve caps retries.
func TestInvalidOutputRetrySafetyValve(t *testing.T) {
	a := &AgentRunner{session: NewSession("")}

	for i := 0; i < InvalidOutputCap; i++ {
		if !a.maybeRetryInvalidOutput("", "thinking...", nil) {
			t.Fatalf("retry %d: expected true", i+1)
		}
	}
	if a.maybeRetryInvalidOutput("", "thinking...", nil) {
		t.Fatal("after cap: expected false")
	}
}

// TestOutputLengthNudgeConstant verifies the nudge text.
func TestOutputLengthNudgeConstant(t *testing.T) {
	if outputLengthNudge == "" {
		t.Fatal("outputLengthNudge is empty")
	}
	verbs := []string{"%s", "%d", "%v", "%q", "%f", "%t", "%x", "%T"}
	for _, verb := range verbs {
		if containsStr(outputLengthNudge, verb) {
			t.Fatalf("outputLengthNudge contains format verb %q", verb)
		}
	}
}

// TestInvalidOutputNudgeConstant verifies the nudge text.
func TestInvalidOutputNudgeConstant(t *testing.T) {
	if invalidOutputNudge == "" {
		t.Fatal("invalidOutputNudge is empty")
	}
	verbs := []string{"%s", "%d", "%v", "%q", "%f", "%t", "%x", "%T"}
	for _, verb := range verbs {
		if containsStr(invalidOutputNudge, verb) {
			t.Fatalf("invalidOutputNudge contains format verb %q", verb)
		}
	}
}
