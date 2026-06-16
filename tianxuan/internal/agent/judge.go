package agent

import (
	"context"
	"encoding/json"
	"strings"

	"tianxuan/internal/provider"
)

// judgeSystemPrompt is the system prompt for the independent judge model.
// It evaluates whether a session goal has been truly satisfied.
// Compile-time constant — not part of the main conversation's prefix cache.
const judgeSystemPrompt = `You are a stop-condition evaluator for an AI coding agent. 
Read the conversation transcript carefully, then judge whether the user's stopping condition has been satisfied.

Return a JSON object with exactly one of these shapes:
- {"ok": true, "reason": "<quote specific evidence from the transcript>"}
- {"ok": false, "reason": "<what is missing or blocking>"}
- {"ok": false, "impossible": true, "reason": "<why the condition cannot be satisfied>"}

Rules:
- Only return ok=true when the transcript contains clear, specific evidence that the condition is met.
- The assistant's own claims of completion are NOT sufficient evidence — look for actual tool outputs, test results, file changes.
- Use impossible=true only when the condition is genuinely unachievable (contradictory, missing dependencies, explicitly tried and failed).
- Always quote specific transcript evidence in the reason field.`

// judgeVerdict is the structured output from the judge model.
type judgeVerdict struct {
	OK         bool   `json:"ok"`
	Impossible bool   `json:"impossible"`
	Reason     string `json:"reason"`
}

// judgeGoal evaluates whether the session goal has been satisfied by sending
// an independent API call to a judge model (typically flash/cheap). Returns
// the verdict and any error.
//
// Cache safety: this is a separate API call with its own messages — it does
// NOT modify the main conversation's prefix-cache-stable system prompt or
// tool list. The judge prompt is a compile-time constant.
func (a *AgentRunner) judgeGoal(ctx context.Context, goal string) (judgeVerdict, error) {
	// Use flash provider if available, fall back to main provider
	prov := a.flashProv
	if prov == nil {
		prov = a.prov
	}
	if prov == nil {
		return judgeVerdict{OK: false, Reason: "no provider available for judge call"}, nil
	}

	// Build judge messages: system + conversation summary + question
	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: judgeSystemPrompt},
	}

	// Include last N messages as evidence (not the full history — keep judge cost low)
	allMsgs := a.session.Messages
	start := 0
	if len(allMsgs) > 30 {
		start = len(allMsgs) - 30
	}
	var sb strings.Builder
	sb.WriteString("Conversation transcript (most recent messages):\n\n")
	for i := start; i < len(allMsgs); i++ {
		m := allMsgs[i]
		switch m.Role {
		case provider.RoleUser:
			sb.WriteString("USER: ")
			sb.WriteString(truncateText(m.Content, 300))
			sb.WriteString("\n")
		case provider.RoleAssistant:
			if m.Content != "" {
				sb.WriteString("ASSISTANT: ")
				sb.WriteString(truncateText(m.Content, 300))
				sb.WriteString("\n")
			}
			for _, tc := range m.ToolCalls {
				sb.WriteString("TOOL: ")
				sb.WriteString(tc.Name)
				sb.WriteString("(")
				sb.WriteString(truncateText(tc.Arguments, 150))
				sb.WriteString(")\n")
			}
		case provider.RoleTool:
			sb.WriteString("TOOL RESULT: ")
			sb.WriteString(truncateText(m.Content, 200))
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n---\n")
	sb.WriteString("Stopping condition: ")
	sb.WriteString(goal)
	sb.WriteString("\n\nHas this condition been satisfied? Reply with JSON only.")

	msgs = append(msgs, provider.Message{Role: provider.RoleUser, Content: sb.String()})

	// Stream a response from the judge model (flash/cheap)
	chunks, err := prov.Stream(ctx, provider.Request{
		Messages:    msgs,
		Temperature: 0,
		MaxTokens:   200,
	})
	if err != nil {
		return judgeVerdict{OK: false, Reason: "judge call failed: " + err.Error()}, nil
	}

	// Collect all text chunks
	var text strings.Builder
	for chunk := range chunks {
		if chunk.Err != nil {
			return judgeVerdict{OK: false, Reason: "judge stream error: " + chunk.Err.Error()}, nil
		}
		text.WriteString(chunk.Text)
	}

	result := strings.TrimSpace(text.String())
	// Extract JSON from markdown code blocks if present
	if strings.HasPrefix(result, "```") {
		if idx := strings.Index(result, "{");
			idx >= 0 {
			end := strings.LastIndex(result, "}")
			if end > idx {
				result = result[idx : end+1]
			}
		}
	}
	var v judgeVerdict
	if err := json.Unmarshal([]byte(result), &v); err != nil {
		return judgeVerdict{OK: false, Reason: "judge response unparseable: " + result}, nil
	}
	return v, nil
}
