package openai

import "strings"

// sseFastPath extracts content and reasoning_content from a DeepSeek SSE data
// line using simple string scanning, avoiding the full json.Unmarshal cost for
// the ~90% of lines that only carry text deltas.
//
// Returns (content, reasoning, needsFullParse). When needsFullParse is false,
// the caller can emit ChunkText/ChunkReasoning directly. When true, the caller
// must fall back to json.Unmarshal — the line contains tool_calls, usage,
// finish_reason, or error fields.
func sseFastPath(data string) (content, reasoning string, needsFullParse bool) {
	// Quick gate: if the line contains any of these JSON keys, it needs full parse.
	if strings.Contains(data, `"tool_calls"`) ||
		strings.Contains(data, `"usage"`) ||
		strings.Contains(data, `"finish_reason"`) ||
		strings.Contains(data, `"error"`) {
		return "", "", true
	}

	// Extract "content":"..."  — the most common field.
	content = extractJSONString(data, `"content"`)
	// Extract "reasoning_content":"..."
	reasoning = extractJSONString(data, `"reasoning_content"`)

	// If we got neither, something unexpected — fall back to full parse.
	if content == "" && reasoning == "" {
		return "", "", true
	}
	return content, reasoning, false
}

// extractJSONString finds key:"value" in a JSON string and returns the unescaped
// value. Handles basic JSON escaping (\n, \", \\). Returns "" when the key is
// not found or the value is empty.
func extractJSONString(data, key string) string {
	// Find the key.
	idx := strings.Index(data, key)
	if idx < 0 {
		return ""
	}
	rest := data[idx+len(key):]

	// Skip : and optional whitespace.
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return ""
	}
	rest = rest[colon+1:]
	for len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
		rest = rest[1:]
	}

	// Expect opening quote.
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	rest = rest[1:]

	// Scan for closing quote, handling backslash escapes.
	var b strings.Builder
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if c == '\\' && i+1 < len(rest) {
			i++
			switch rest[i] {
			case 'n':
				b.WriteByte('\n')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte('\\')
				b.WriteByte(rest[i])
			}
			continue
		}
		if c == '"' {
			return b.String()
		}
		b.WriteByte(c)
	}
	return "" // unterminated string
}
