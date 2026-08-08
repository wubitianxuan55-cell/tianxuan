package agent

import (
	"context"
	"strings"
	"testing"
)

// fakeTokensLeft injects a fixed tokensLeft closure for tool tests.
func withFakeTokensLeft(ctx context.Context, left int, ok bool) context.Context {
	return withTokensLeft(ctx, func() (int, bool) { return left, ok })
}

func TestContextRemainingToolBasicContract(t *testing.T) {
	tr := &ContextRemainingTool{}
	if tr.Name() != "get_context_remaining" {
		t.Fatalf("Name() = %q, want get_context_remaining", tr.Name())
	}
	if !tr.ReadOnly() {
		t.Fatal("ReadOnly() = false, want true (querying remaining budget has no side effects)")
	}
	if !strings.Contains(tr.Description(), "token") {
		t.Fatalf("Description() = %q, want mention of tokens", tr.Description())
	}
	if !strings.Contains(string(tr.Schema()), `"type":"object"`) {
		t.Fatalf("Schema() = %s, want empty object schema", tr.Schema())
	}
}

func TestContextRemainingToolReportsTokensLeft(t *testing.T) {
	ctx := withFakeTokensLeft(context.Background(), 4321, true)
	out, err := (&ContextRemainingTool{}).Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "4321") {
		t.Fatalf("output = %q, want it to contain the remaining count 4321", out)
	}
}

func TestContextRemainingToolWindowUnavailable(t *testing.T) {
	// ok=false means the host has no context window configured; the tool should
	// say so plainly rather than erroring (a query is not a failure).
	ctx := withFakeTokensLeft(context.Background(), 0, false)
	out, err := (&ContextRemainingTool{}).Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "not configured") && !strings.Contains(out, "unavailable") {
		t.Fatalf("output = %q, want it to explain the window is unavailable", out)
	}
}

func TestContextRemainingToolNoInjection(t *testing.T) {
	// A plain context (outside an agent run, e.g. headless tool test) has no
	// tokensLeft provider — the tool must fail loudly, never guess.
	_, err := (&ContextRemainingTool{}).Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("Execute with plain context: want error (no provider injected)")
	}
}
