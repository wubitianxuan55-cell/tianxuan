package builtin

import (
	"strings"
	"testing"
	"unicode"

	"tianxuan/internal/tool"
)

// hasCJK reports whether s contains any Han characters.
func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// TestBuiltinFullDescriptionsEnglish locks the model-facing language contract:
// full tool descriptions and schema parameter descriptions must be English,
// matching the (English) system prompts. Mixed-language instructions degrade
// model adherence. Chinese compact descriptions are the deliberate exception
// (token-saving single-line summaries tuned for DeepSeek) and are not checked.
func TestBuiltinFullDescriptionsEnglish(t *testing.T) {
	for _, tl := range tool.Builtins() {
		if hasCJK(string(tl.Schema())) {
			t.Errorf("%s: Schema() contains CJK text", tl.Name())
		}
		if hasCJK(tl.Description()) {
			t.Errorf("%s: Description() contains CJK text", tl.Name())
		}
	}
}

// TestCompactDescriptionsStayChinese locks the compact-description language
// deliberately: compactDesc is Chinese on purpose (short single-line summaries
// that cut per-turn tokens for DeepSeek). If this ever flips to English, the
// per-turn schema prefix grows and the speed win disappears.
func TestCompactDescriptionsStayChinese(t *testing.T) {
	for _, tl := range tool.Builtins() {
		cd, ok := tl.(tool.CompactDescriptor)
		if !ok {
			continue
		}
		if strings.TrimSpace(cd.CompactDescription()) == "" {
			continue
		}
		if !hasCJK(cd.CompactDescription()) {
			t.Errorf("%s: CompactDescription() is not Chinese (deliberate token-saving design)", tl.Name())
		}
	}
}
