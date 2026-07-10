package agent

import (
	"strings"

	"tianxuan/internal/provider"
)

// truncateStr returns s truncated to maxLen chars. Used for dedup key building.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// extractFilePath extracts a file path from tool call arguments for edit tools.
// Returns "" if no path can be extracted.
func extractFilePath(name string, args string) string {
	// Common keys for file paths in tool arguments.
	keys := []string{`"path"`, `"file_path"`, `"source"`, `"destination"`}
	lower := strings.ToLower(args)
	for _, key := range keys {
		idx := strings.Index(lower, key)
		if idx < 0 {
			continue
		}
		// Find the value after the key:  "path": "value"
		rest := args[idx+len(key):]
		colon := strings.Index(rest, ":")
		if colon < 0 {
			continue
		}
		val := strings.TrimSpace(rest[colon+1:])
		// Strip quotes
		val = strings.Trim(val, `"`)
		// Take until comma or closing brace
		if end := strings.IndexAny(val, `,}`); end >= 0 {
			val = val[:end]
		}
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"`)
		if val != "" {
			return val
		}
	}
	return ""
}

// msgChars counts the characters sent to the provider for one message —
// content plus tool-call names and arguments, but not reasoning (stripped on
// send).
func msgChars(m provider.Message) int {
	n := len(m.Content)
	for _, tc := range m.ToolCalls {
		n += len(tc.Name) + len(tc.Arguments)
	}
	return n
}

// charsOfMessages returns the total character count across messages.
func charsOfMessages(msgs []provider.Message) int {
	n := 0
	for _, m := range msgs {
		n += msgChars(m)
	}
	return n
}

// streamRecoveryMessage generates a recovery prompt for use when a stream
// is interrupted mid-response. The prompt varies based on whether partial
// text was already emitted.
// (Design adopted from DeepSeek-Reasonix-V1.12)
func streamRecoveryMessage(hasPartialText bool) string {
	if hasPartialText {
		return "The previous assistant response was interrupted during streaming. Continue the same task from immediately after the partial assistant message above. Do not repeat text that is already visible."
	}
	return "The previous assistant response was interrupted during streaming before visible answer text was completed. Continue the same task now and provide the next useful response."
}

// emptyFinalRetryMessage generates a retry prompt when the model returns
// no tool calls and no visible text. It nudges the model to produce a
// visible answer.
// (Design adopted from DeepSeek-Reasonix-V1.12)
func emptyFinalRetryMessage() string {
	return "The previous assistant response finished without any visible answer text. Continue the same task now and provide a concise visible answer to the user. Do not send reasoning only."
}

func midTurnSteerMessage(text string) string {
	return MidTurnSteerPrefix + "\n" + text
}

// quoteFilePaths wraps each path in backticks and joins with ", ".
// Returns "(none)" when the slice is empty.
func quoteFilePaths(paths []string) string {
	if len(paths) == 0 {
		return "(none)"
	}
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = "`" + p + "`"
	}
	return strings.Join(quoted, ", ")
}

// finalReadinessRetryMessage generates a retry prompt when the final-answer
// readiness check blocks completion.
// (Design adopted from DeepSeek-Reasonix-V1.12)
func finalReadinessRetryMessage(reason string) string {
	return "Host final-answer readiness check failed. Before giving a final answer, address the missing host-observable receipts: " + reason + ". Run the required tool calls, then answer when readiness is satisfied. If the blocked item needs user input, call the ask tool with concrete options and wait for its tool result; do not ask in prose."
}
