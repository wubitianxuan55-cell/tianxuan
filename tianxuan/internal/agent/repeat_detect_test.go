package agent

import (
	"encoding/json"
	"testing"

	"tianxuan/internal/provider"
)

// TestStepSignatureDeterministic verifies that the same tool calls in
// different key order produce the same signature.
func TestStepSignatureDeterministic(t *testing.T) {
	// Two semantically identical calls but with keys in different order
	args1 := `{"path":"a.txt","content":"hello"}`
	args2 := `{"content":"hello","path":"a.txt"}`

	calls1 := []provider.ToolCall{
		{ID: "1", Name: "write_file", Arguments: args1},
	}
	calls2 := []provider.ToolCall{
		{ID: "2", Name: "write_file", Arguments: args2},
	}

	sig1 := stepSignature(calls1)
	sig2 := stepSignature(calls2)
	if sig1 != sig2 {
		t.Fatalf("signatures differ for same call with different key order:\n  %q\n  %q", sig1, sig2)
	}
}

// TestStepSignatureEmpty returns "" for no tool calls.
func TestStepSignatureEmpty(t *testing.T) {
	if sig := stepSignature(nil); sig != "" {
		t.Fatalf("expected empty signature for nil calls, got %q", sig)
	}
	if sig := stepSignature([]provider.ToolCall{}); sig != "" {
		t.Fatalf("expected empty signature for empty calls, got %q", sig)
	}
}

// TestStepSignatureDifferentCalls produces different signatures.
func TestStepSignatureDifferentCalls(t *testing.T) {
	calls1 := []provider.ToolCall{
		{ID: "1", Name: "read_file", Arguments: `{"path":"a.txt"}`},
	}
	calls2 := []provider.ToolCall{
		{ID: "2", Name: "write_file", Arguments: `{"path":"a.txt"}`},
	}
	if stepSignature(calls1) == stepSignature(calls2) {
		t.Fatal("different tool names should produce different signatures")
	}
}

// TestStepSignatureMultipleCalls produces deterministic multi-call signature.
func TestStepSignatureMultipleCalls(t *testing.T) {
	// Multiple calls, sorted by name then id
	calls := []provider.ToolCall{
		{ID: "2", Name: "bash", Arguments: `{"cmd":"ls"}`},
		{ID: "1", Name: "read_file", Arguments: `{"path":"a.txt"}`},
	}
	sig := stepSignature(calls)
	// bash should come first (alphabetically)
	if len(sig) == 0 {
		t.Fatal("empty signature for multi-call step")
	}
	// Verify bash comes before read_file in the signature
	// Signature format: "toolName:canonicalArgs\ntoolName:canonicalArgs"
	bashIdx := stringsIndex(sig, "bash:")
	readIdx := stringsIndex(sig, "read_file:")
	if bashIdx < 0 || readIdx < 0 {
		t.Fatalf("signature missing expected tool names: %q", sig)
	}
	if bashIdx > readIdx {
		t.Fatalf("bash should come before read_file alphabetically: %q", sig)
	}
}

// TestDetectRepeatedStepsTriggersAfterThreshold injects a nudge after 3 identical steps.
func TestDetectRepeatedStepsTriggersAfterThreshold(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{
		{ID: "1", Name: "read_file", Arguments: `{"path":"a.txt"}`},
	}

	// First 2 times should NOT trigger
	if a.detectRepeatedSteps(calls) {
		t.Fatal("step 1: expected false")
	}
	if a.detectRepeatedSteps(calls) {
		t.Fatal("step 2: expected false")
	}
	// 3rd time should trigger
	if !a.detectRepeatedSteps(calls) {
		t.Fatal("step 3: expected true (nudge injected)")
	}
	// Verify nudge was added
	last := s.Messages[len(s.Messages)-1]
	if last.Role != provider.RoleUser {
		t.Fatalf("expected user-role nudge, got %s", last.Role)
	}
	if last.Content != repeatedStepNudge {
		t.Fatalf("nudge text mismatch: got %q", last.Content)
	}
}

// TestDetectRepeatedStepsResetsOnChange resets counter when step changes.
func TestDetectRepeatedStepsResetsOnChange(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls1 := []provider.ToolCall{
		{ID: "1", Name: "read_file", Arguments: `{"path":"a.txt"}`},
	}
	calls2 := []provider.ToolCall{
		{ID: "2", Name: "write_file", Arguments: `{"path":"b.txt"}`},
	}

	a.detectRepeatedSteps(calls1)
	a.detectRepeatedSteps(calls1)
	// Change step → counter resets
	if a.detectRepeatedSteps(calls2) {
		t.Fatal("step change should reset counter, not trigger")
	}
	if a.repeatCount != 1 {
		t.Fatalf("counter should be 1 after change, got %d", a.repeatCount)
	}
}

// TestDetectRepeatedStepsTextTurnResets resets when there are no tool calls.
func TestDetectRepeatedStepsTextTurnResets(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{
		{ID: "1", Name: "read_file", Arguments: `{"path":"a.txt"}`},
	}

	a.detectRepeatedSteps(calls)
	a.detectRepeatedSteps(calls)
	// Text-only turn (no calls) → resets
	if a.detectRepeatedSteps(nil) {
		t.Fatal("text-only turn should not trigger")
	}
	if a.repeatCount != 0 {
		t.Fatalf("counter should reset to 0 after text turn, got %d", a.repeatCount)
	}
}

// TestDetectRepeatedStepsDifferentArgs produces different signatures.
func TestDetectRepeatedStepsDifferentArgs(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls1 := []provider.ToolCall{
		{ID: "1", Name: "read_file", Arguments: `{"path":"a.txt"}`},
	}
	calls2 := []provider.ToolCall{
		{ID: "2", Name: "read_file", Arguments: `{"path":"b.txt"}`},
	}

	a.detectRepeatedSteps(calls1)
	a.detectRepeatedSteps(calls1)
	// Different args → different signature → reset
	if a.detectRepeatedSteps(calls2) {
		t.Fatal("different args should reset counter")
	}
	if a.repeatCount != 1 {
		t.Fatalf("counter should be 1, got %d", a.repeatCount)
	}
}

// TestRepeatedStepNudgeIsConstant verifies the nudge text is a compile-time constant.
func TestRepeatedStepNudgeIsConstant(t *testing.T) {
	if repeatedStepNudge == "" {
		t.Fatal("repeatedStepNudge is empty")
	}
	for i := 0; i < len(repeatedStepNudge); i++ {
		if repeatedStepNudge[i] == '%' {
			t.Fatal("repeatedStepNudge contains '%' — must have no format verbs")
		}
	}
}

// makeReadCall is a helper to create a read_file ToolCall with a given path.
func makeReadCall(id, path string) provider.ToolCall {
	args, _ := json.Marshal(map[string]string{"path": path})
	return provider.ToolCall{ID: id, Name: "read_file", Arguments: string(args)}
}

// TestStepSignatureCallsDifferentOrder produces same signature for
// the same set of calls in different order (model nondeterminism).
func TestStepSignatureCallsDifferentOrder(t *testing.T) {
	calls1 := []provider.ToolCall{
		makeReadCall("1", "a.txt"),
		makeReadCall("2", "b.txt"),
	}
	calls2 := []provider.ToolCall{
		makeReadCall("2", "b.txt"),
		makeReadCall("1", "a.txt"),
	}
	sig1 := stepSignature(calls1)
	sig2 := stepSignature(calls2)
	if sig1 != sig2 {
		t.Fatalf("same calls in different order should produce same signature:\n  %q\n  %q", sig1, sig2)
	}
}

// stringsIndex returns the index of substr in s, or -1 if not found.
func stringsIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
