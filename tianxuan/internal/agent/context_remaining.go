package agent

import (
	"context"
	"encoding/json"
	"fmt"
)

// tokensLeftKey is the context key for the tokensLeft provider injected by
// executeOne before every tool call.
type tokensLeftKey struct{}

// withTokensLeft stamps ctx with a closure that reports the estimated number
// of tokens left in the context window. ok=false means the host has no
// configured window (compaction disabled), so no budget can be reported.
func withTokensLeft(ctx context.Context, fn func() (int, bool)) context.Context {
	return context.WithValue(ctx, tokensLeftKey{}, fn)
}

// tokensLeftOf returns the injected provider, if any. ok is false for a plain
// context (tool tests, calls outside the agent run loop).
func tokensLeftOf(ctx context.Context) (func() (int, bool), bool) {
	fn, ok := ctx.Value(tokensLeftKey{}).(func() (int, bool))
	return fn, ok
}

// ContextRemainingTool lets the model query how many tokens remain in the
// current context window before starting a large task (distilled from codex
// CLI's get_context_remaining). Long-running sessions otherwise surprise the
// model: it cannot see compaction looming and keeps stacking tool calls until
// the window overflows. A read-only query costs nothing and lets the model
// plan to converge (stop exploring, wrap up, or ask to continue later).
type ContextRemainingTool struct{}

func NewContextRemainingTool() *ContextRemainingTool { return &ContextRemainingTool{} }

func (*ContextRemainingTool) Name() string { return "get_context_remaining" }

func (*ContextRemainingTool) Description() string {
	return "Report the estimated number of tokens left in the current context window. " +
		"Call this before starting a large task (deep exploration, many file edits, a big " +
		"refactor) or when a long session feels crowded. If the remaining budget is low, " +
		"plan to converge: narrow the scope, stop exploring, and wrap up with a final answer " +
		"rather than starting new open-ended work."
}

func (*ContextRemainingTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

// ReadOnly is true: querying the budget has no side effects, so it needs no
// approval and stays available in plan mode.
func (*ContextRemainingTool) ReadOnly() bool { return true }

func (*ContextRemainingTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	fn, ok := tokensLeftOf(ctx)
	if !ok || fn == nil {
		return "", fmt.Errorf("get_context_remaining is only available inside an agent run (no token provider in context)")
	}
	left, ok := fn()
	if !ok {
		return "Context window is not configured (compaction disabled); no token budget to report. Treat the window as effectively unlimited.", nil
	}
	return fmt.Sprintf("Estimated tokens left in the context window: %d.", left), nil
}
