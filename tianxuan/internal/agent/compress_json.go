package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ─── V9.1: JSON 结构掩码压缩（借鉴 Headroom json_handler.py）─────────

type jsonTokenType int

const (
	jtKey         jsonTokenType = iota
	jtStringValue
	jtNumber
	jtBool
	jtNull
	jtBracket
	jtColon
	jtComma
	jtWhitespace
)

type jsonTok struct {
	text  string
	start int
	end   int
	kind  jsonTokenType
}

type jsonSpan struct {
	start, end   int
	isStructural bool
}

// isJSON 快速检测字符串是否为有效 JSON 对象或数组。
func isJSON(s string) bool {
	t := strings.TrimSpace(s)
	if len(t) < 2 {
		return false
	}
	if t[0] != '{' && t[0] != '[' {
		return false
	}
	return json.Valid([]byte(t))
}

// compressJSON 对 JSON 输出做结构感知压缩。
func compressJSON(raw string) string {
	t := strings.TrimSpace(raw)
	if len(t) < 100 {
		return t
	}
	tokens := tokenizeJSON(t)
	if len(tokens) == 0 {
		return t
	}
	spans := buildJSONSpans(tokens)
	return compressJSONWithMask(t, spans)
}

// tokenizeJSON 逐字符扫描 JSON，标记每个 token 为结构或内容。
func tokenizeJSON(content string) []jsonTok {
	var tokens []jsonTok
	n := len(content)
	i := 0
	expectKey := false
	braceStack := make([]byte, 0, 16)

	for i < n {
		c := content[i]

		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			start := i
			for i < n && (content[i] == ' ' || content[i] == '\t' || content[i] == '\n' || content[i] == '\r') {
				i++
			}
			tokens = append(tokens, jsonTok{content[start:i], start, i, jtWhitespace})
			continue
		}

		if c == '{' || c == '}' || c == '[' || c == ']' {
			tokens = append(tokens, jsonTok{string(c), i, i + 1, jtBracket})
			switch c {
			case '{':
				braceStack = append(braceStack, '{')
				expectKey = true
			case '}':
				if len(braceStack) > 0 && braceStack[len(braceStack)-1] == '{' {
					braceStack = braceStack[:len(braceStack)-1]
				}
				expectKey = false
			case '[':
				braceStack = append(braceStack, '[')
				expectKey = false
			case ']':
				if len(braceStack) > 0 && braceStack[len(braceStack)-1] == '[' {
					braceStack = braceStack[:len(braceStack)-1]
				}
			}
			i++
			continue
		}

		if c == ':' {
			tokens = append(tokens, jsonTok{":", i, i + 1, jtColon})
			expectKey = false
			i++
			continue
		}

		if c == ',' {
			tokens = append(tokens, jsonTok{",", i, i + 1, jtComma})
			if len(braceStack) > 0 && braceStack[len(braceStack)-1] == '{' {
				expectKey = true
			}
			i++
			continue
		}

		if c == '"' {
			start := i
			i++
			for i < n && content[i] != '"' {
				if content[i] == '\\' {
					i += 2
				} else {
					i++
				}
			}
			if i < n {
				i++
			}
			text := content[start:i]
			j := i
			for j < n && (content[j] == ' ' || content[j] == '\t' || content[j] == '\n' || content[j] == '\r') {
				j++
			}
			isKey := j < n && content[j] == ':' && expectKey
			if isKey {
				tokens = append(tokens, jsonTok{text, start, i, jtKey})
				expectKey = false
			} else {
				tokens = append(tokens, jsonTok{text, start, i, jtStringValue})
			}
			continue
		}

		if (c >= '0' && c <= '9') || c == '-' {
			start := i
			if c == '-' {
				i++
			}
			for i < n && content[i] >= '0' && content[i] <= '9' {
				i++
			}
			if i < n && content[i] == '.' {
				i++
				for i < n && content[i] >= '0' && content[i] <= '9' {
					i++
				}
			}
			if i < n && (content[i] == 'e' || content[i] == 'E') {
				i++
				if i < n && (content[i] == '+' || content[i] == '-') {
					i++
				}
				for i < n && content[i] >= '0' && content[i] <= '9' {
					i++
				}
			}
			tokens = append(tokens, jsonTok{content[start:i], start, i, jtNumber})
			continue
		}

		if i+3 < n && content[i:i+4] == "true" {
			tokens = append(tokens, jsonTok{"true", i, i + 4, jtBool})
			i += 4
			continue
		}
		if i+4 < n && content[i:i+5] == "false" {
			tokens = append(tokens, jsonTok{"false", i, i + 5, jtBool})
			i += 5
			continue
		}
		if i+3 < n && content[i:i+4] == "null" {
			tokens = append(tokens, jsonTok{"null", i, i + 4, jtNull})
			i += 4
			continue
		}

		i++
	}
	return tokens
}

func buildJSONSpans(tokens []jsonTok) []jsonSpan {
	if len(tokens) == 0 {
		return nil
	}
	var spans []jsonSpan
	cur := jsonSpan{start: tokens[0].start, end: tokens[0].end, isStructural: isStructuralJSON(tokens[0])}
	for i := 1; i < len(tokens); i++ {
		t := tokens[i]
		s := isStructuralJSON(t)
		if s == cur.isStructural {
			cur.end = t.end
		} else {
			spans = append(spans, cur)
			cur = jsonSpan{start: t.start, end: t.end, isStructural: s}
		}
	}
	spans = append(spans, cur)
	return spans
}

func isStructuralJSON(t jsonTok) bool {
	switch t.kind {
	case jtKey, jtBracket, jtColon, jtComma, jtBool, jtNull:
		return true
	case jtNumber:
		return len(t.text) <= 10
	default:
		return false
	}
}

func compressJSONWithMask(content string, spans []jsonSpan) string {
	var b strings.Builder
	for _, sp := range spans {
		chunk := content[sp.start:sp.end]
		if sp.isStructural {
			b.WriteString(chunk)
		} else {
			b.WriteString(compressJSONContent(chunk))
		}
	}
	return b.String()
}

func compressJSONContent(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	const head = 80
	const tail = 40
	if len(r) <= head+tail+10 {
		return s
	}
	return string(r[:head]) +
		fmt.Sprintf("…[%d chars]…", len(r)-head-tail) +
		string(r[len(r)-tail:])
}
