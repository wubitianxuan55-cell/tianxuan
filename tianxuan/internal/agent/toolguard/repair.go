package toolguard

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// ─── V5.12: Tool-Call-Repair (Kun tool-call-repair.ts 移植) ──────────────

// ToolArgumentRepairOptions 控制工具参数修复行为。
type ToolArgumentRepairOptions struct {
	// MaxStringBytes 是单个字符串参数的最大字节数（UTF-8）。
	// 超过此限制的字符串会被截断。默认 512KB。
	MaxStringBytes int
	// PreserveLongStrings 为 true 时跳过截断（文件变更类工具示例）。
	PreserveLongStrings bool
}

// ToolArgumentRepairResult 包含修复后的参数和修复说明。
type ToolArgumentRepairResult struct {
	Arguments map[string]any
	Notes     []string
}

const defaultMaxStringBytes = 512 * 1024

// wrapperKeys — 可能包含实际参数的包装键名。
var wrapperKeys = []string{"arguments", "args", "input", "parameters", "params", "payload", "__raw"}

// toolMetadataKeys — 工具元数据键（不应触发展平）。
var toolMetadataKeys = map[string]bool{
	"tool": true, "toolName": true, "tool_name": true,
	"name": true, "id": true, "callId": true, "call_id": true, "type": true,
}

// RepairDispatchToolArguments 对已解析的工具参数执行与提供者无关的修复。
//
// 三阶段修复：
//  1. 展平包装器 — 如果参数被 {"arguments": {...}} 包裹，提取内层
//  2. 捞取 JSON — 如果参数是 {"key": "{\"path\":\"x\"}"} 这种 JSON 字符串
//  3. 截断超大字符串 — 超过 MaxStringBytes 的字符串截断
//
// 这是 Kun tool-call-repair.ts 的 Go 移植。
func RepairDispatchToolArguments(raw map[string]any, opts ToolArgumentRepairOptions) ToolArgumentRepairResult {
	notes := make([]string, 0, 2)
	current := cloneMap(raw)

	// Phase 1: 展平包装器
	if flattened, note := flattenWrapper(current); flattened != nil {
		current = flattened
		notes = append(notes, note)
	} else if scavenged, note := scavengeSingleJSONString(current); scavenged != nil {
		// Phase 2: 捞取 JSON 字符串
		current = scavenged
		notes = append(notes, note)
	}

	// Phase 3: 截断超大字符串
	maxBytes := opts.MaxStringBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxStringBytes
	}
	if !opts.PreserveLongStrings {
		if fixed, ok := truncateOversizedStrings(current, maxBytes); ok {
			current = fixed
			notes = append(notes, "truncated oversized argument string(s)")
		}
	}

	return ToolArgumentRepairResult{Arguments: current, Notes: notes}
}

// ─── Phase 1: 展平包装器 ──────────────────────────────────────────────

func flattenWrapper(raw map[string]any) (map[string]any, string) {
	for _, key := range wrapperKeys {
		val, ok := raw[key]
		if !ok {
			continue
		}
		if !canFlattenWrapper(raw, key) {
			continue
		}
		parsed := valueToObject(val)
		if parsed == nil {
			continue
		}
		return parsed, "flattened " + key + " wrapper"
	}
	return nil, ""
}

func canFlattenWrapper(raw map[string]any, wrapperKey string) bool {
	if len(raw) == 1 {
		return true
	}
	for k := range raw {
		if k != wrapperKey && !toolMetadataKeys[k] {
			return false
		}
	}
	return true
}

// ─── Phase 2: 捞取 JSON 字符串 ────────────────────────────────────────

func scavengeSingleJSONString(raw map[string]any) (map[string]any, string) {
	if len(raw) != 1 {
		return nil, ""
	}
	for key, val := range raw {
		s, ok := val.(string)
		if !ok {
			return nil, ""
		}
		parsed := parseJSONishObject(s)
		if parsed == nil {
			return nil, ""
		}
		return parsed, "scavenged JSON object from " + key
	}
	return nil, ""
}

func valueToObject(val any) map[string]any {
	if m, ok := val.(map[string]any); ok {
		return cloneMap(m)
	}
	if s, ok := val.(string); ok {
		return parseJSONishObject(s)
	}
	return nil
}

func parseJSONishObject(text string) map[string]any {
	candidates := []string{text}
	if stripped := stripMarkdownFence(text); stripped != text {
		candidates = append(candidates, stripped)
	}
	if extracted := extractFirstJSONObject(text); extracted != "" && extracted != text {
		candidates = append(candidates, extracted)
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(candidate), &result); err == nil {
			return result
		}
	}
	return nil
}

func stripMarkdownFence(text string) string {
	t := strings.TrimSpace(text)
	// ```json ... ``` or ``` ... ```
	for _, prefix := range []string{"```json", "```javascript", "```js", "```"} {
		if strings.HasPrefix(t, prefix) && strings.HasSuffix(t, "```") {
			inner := t[len(prefix):]
			inner = inner[:len(inner)-3]
			return strings.TrimSpace(inner)
		}
	}
	return text
}

func extractFirstJSONObject(text string) string {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		ch := text[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == '{' {
			depth++
		}
		if ch == '}' {
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

// ─── Phase 3: 截断超大字符串 ──────────────────────────────────────────

func truncateOversizedStrings(value map[string]any, maxBytes int) (map[string]any, bool) {
	changed := false
	out := make(map[string]any, len(value))
	for k, v := range value {
		if s, ok := v.(string); ok {
			if utf8ByteLen(s) > maxBytes {
				changed = true
				out[k] = sliceUTF8(s, maxBytes) + "\n...[truncated by tianxuan tool argument repair]"
			} else {
				out[k] = v
			}
		} else if nested, ok := v.(map[string]any); ok {
			if fixed, c := truncateOversizedStrings(nested, maxBytes); c {
				changed = true
				out[k] = fixed
			} else {
				out[k] = v
			}
		} else {
			out[k] = v
		}
	}
	if !changed {
		return nil, false
	}
	return out, true
}

func utf8ByteLen(s string) int {
	return len(s) // Go strings are byte sequences; for ASCII this is exact; for multi-byte it's the UTF-8 byte count
}

func sliceUTF8(s string, maxBytes int) string {
	used := 0
	for i, r := range s {
		next := utf8.RuneLen(r)
		if used+next > maxBytes {
			return s[:i]
		}
		used += next
	}
	return s
}

func cloneMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
