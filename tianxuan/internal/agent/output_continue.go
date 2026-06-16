package agent

import (
	"strings"

	"tianxuan/internal/provider"
)

// outputLenNudgeCap prevents infinite continuation when the model keeps
// hitting the output length limit without producing tool calls.
const outputLenNudgeCap = 5

// outputLengthNudge is injected when finish_reason="length" and there are
// no tool calls — the model hit the output token limit mid-response.
const outputLengthNudge = "[system] " +
	"The previous response was cut off because it hit the output token limit. " +
	"Continue from where you left off — do NOT restart, recap, or repeat prior reasoning. " +
	"Keep reasoning concise, prefer concrete tool calls or final output."

// invalidOutputNudge is injected when the model produced only reasoning
// (no text, no tool calls) — a "think-only" turn that delivers nothing.
const invalidOutputNudge = "[system] " +
	"Your previous response contained no usable answer — it was empty " +
	"or had only thinking with no tool calls and no final text. " +
	"Provide a final answer or call a tool to make progress on the task."

// invalidOutputCap prevents infinite retry on invalid output.
const invalidOutputCap = 3

// maybeContinueOutputLength checks whether the model's output was truncated
// and injects a continuation nudge. Returns true when it did (caller should
// continue the loop).
func (a *AgentRunner) maybeContinueOutputLength(u *provider.Usage, calls []provider.ToolCall) bool {
	if u == nil || u.FinishReason != "length" {
		a.lenContCount = 0
		return false
	}
	// Only continue for pure-text truncation. If the model had tool calls,
	// tool results are the natural continuation — let the loop handle it.
	if len(calls) > 0 {
		return false
	}
	a.lenContCount++
	if a.lenContCount > outputLenNudgeCap {
		return false // safety valve
	}
	a.session.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: outputLengthNudge,
	})
	return true
}

// maybeRetryInvalidOutput checks whether the model produced a "think-only"
// or empty turn and injects a nudge. Returns true when it did.
func (a *AgentRunner) maybeRetryInvalidOutput(text, reasoning string, calls []provider.ToolCall) bool {
	// Only retry when there's genuinely nothing usable: no text, no tool calls.
	if strings.TrimSpace(text) != "" || len(calls) > 0 {
		a.invalidOutCount = 0
		return false
	}
	// If reasoning is also empty, it's a truly dead turn — no point retrying.
	if strings.TrimSpace(reasoning) == "" {
		return false
	}
	a.invalidOutCount++
	if a.invalidOutCount > invalidOutputCap {
		return false
	}
	a.session.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: invalidOutputNudge,
	})
	return true
}
