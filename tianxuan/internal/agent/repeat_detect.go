package agent

import (
	"sort"
	"strings"

	"tianxuan/internal/provider"
)

// repeatedStepThreshold is how many consecutive identical assistant steps
// trigger the nudge. Mirrors MiMo-Code's REPEATED_STEP_THRESHOLD.
const repeatedStepThreshold = 3

// repeatedStepNudge is the turn-tail synthetic user message injected when the
// model has taken the same action 3 times in a row. Compile-time constant
// for DeepSeek prefix-cache stability.
const repeatedStepNudge = "[system] Your last 3 steps have been identical — " +
	"you appear to be repeating the same action without making progress. " +
	"Stop and reconsider: the current approach is not working. " +
	"Try a different strategy, use a different tool, or if you are blocked, " +
	"explain the situation instead of repeating the same step."

// stepSignature computes a stable signature for an assistant step's action —
// the tool calls it made (name + key-order-independent canonical args).
// Returns "" when the step has no tool calls (pure text turn), since there
// is no repeated *action* to detect.
//
// Mirrors MiMo-Code's stepSignature() in session/prompt.ts:140-148.
func stepSignature(calls []provider.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	// Sort by (name, id) for deterministic ordering — models may emit
	// the same set of calls in different order.
	sorted := make([]provider.ToolCall, len(calls))
	copy(sorted, calls)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].ID < sorted[j].ID
	})

	var sb strings.Builder
	for i, tc := range sorted {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(tc.Name)
		sb.WriteByte(':')
		sb.WriteString(canonicalizeArgs(tc.Arguments))
	}
	return sb.String()
}

// detectRepeatedSteps checks whether the model has taken the same action
// repeatedStepThreshold times in a row. Returns true when it injected a nudge
// (caller should continue the loop).
func (a *AgentRunner) detectRepeatedSteps(calls []provider.ToolCall) bool {
	sig := stepSignature(calls)
	if sig == "" {
		a.repeatSig, a.repeatCount = "", 0
		return false
	}

	if sig != a.repeatSig {
		a.repeatSig, a.repeatCount = sig, 1
		return false
	}

	a.repeatCount++
	if a.repeatCount < repeatedStepThreshold {
		return false
	}

	// Inject deterministic nudge as turn-tail synthetic user message.
	a.session.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: repeatedStepNudge,
	})
	return true
}
