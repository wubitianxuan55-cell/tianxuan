package main

import (
	"strings"
	"testing"
)

func TestExtractMemoryStatementEmpty(t *testing.T) {
	tests := []string{
		"",
		"短",
		"太短了",
		"just a short one",
	}
	for _, tc := range tests {
		statement, reason := extractMemoryStatement(tc)
		if statement != "" || reason != "" {
			t.Errorf("extractMemoryStatement(%q) = (%q, %q), want (\"\", \"\")", tc, statement, reason)
		}
	}
}

func TestExtractMemoryStatementChineseMarker(t *testing.T) {
	// 包含"记住"的文本应该被检测到
	content := "请记住用户偏好使用 tabs 而非 spaces 缩进"
	statement, reason := extractMemoryStatement(content)
	if statement == "" {
		t.Errorf("extractMemoryStatement should detect '记住' marker in: %q", content)
	}
	if reason == "" {
		t.Errorf("extractMemoryStatement should provide a reason for: %q", content)
	}
}

func TestExtractMemoryStatementDontMarker(t *testing.T) {
	// 包含"不要"的约束性语句应该被检测
	content := "不要在 main 分支上直接提交代码，始终使用 feature 分支"
	statement, reason := extractMemoryStatement(content)
	if statement == "" {
		t.Errorf("extractMemoryStatement should detect '不要' marker in: %q", content)
	}
	if reason == "" {
		t.Errorf("extractMemoryStatement should provide a reason for: %q", content)
	}
}

func TestExtractMemoryStatementTooLong(t *testing.T) {
	// 超过 420 个字符的文本应该被跳过
	long := strings.Repeat("这是一个很长的记忆内容。", 50) // ~750 runes
	statement, _ := extractMemoryStatement(long)
	if statement != "" {
		t.Errorf("extractMemoryStatement should reject very long content, got: %q", statement)
	}
}

func TestExtractMemoryStatementNoMarker(t *testing.T) {
	// 不包含任何标记的普通对话不应被检测
	content := "请帮我写一个函数来计算斐波那契数列"
	statement, _ := extractMemoryStatement(content)
	if statement != "" {
		t.Errorf("extractMemoryStatement should not detect memory in plain request: %q", content)
	}
}

func TestNormalizeSuggestionKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello world"},
		{"  Extra   Spaces  ", "extra spaces"},
		{"UPPERCASE", "uppercase"},
		{"MixedCase Text", "mixedcase text"},
	}
	for _, tc := range tests {
		got := normalizeSuggestionKey(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeSuggestionKey(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestExistingCovers(t *testing.T) {
	existing := []string{"hello world", "foo bar"}
	if !existingCovers(existing, "hello") {
		t.Error("existingCovers should find substring match")
	}
	if !existingCovers(existing, "hello world extra") {
		t.Error("existingCovers should find superset match")
	}
	if existingCovers(existing, "baz") {
		t.Error("existingCovers should not match unrelated text")
	}
	if !existingCovers(existing, "") {
		t.Error("existingCovers should return true for empty key")
	}
}

// TestIsTransientBlockEmbedded verifies an embedded <memory-update> control
// block in the middle of a user message is rejected — the old prefix-only
// check let "默认是不是中文 <memory-update> ..." leak into suggestions.
func TestIsTransientBlockEmbedded(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{"默认是不是中文 <memory-update> The following project-memory changes were just made </memory-update>", true},
		{"<memory-update>\nSome memory changed\n</memory-update>", true},
		{"[system] This session has saved memory", true},
		{"[auto-recall] 相关记忆自动检索结果\n- foo: bar", true},
		{"以后统一用 tabs 缩进", false},
		{"我在看 [system] 是什么", false},
	}
	for _, tc := range cases {
		if got := isTransientBlock(tc.content); got != tc.want {
			t.Errorf("isTransientBlock(%q) = %v, want %v", tc.content, got, tc.want)
		}
	}
}

// TestSharesCorePhrase verifies reworded duplicate rules share a core phrase
// even though neither string fully contains the other.
func TestSharesCorePhrase(t *testing.T) {
	if !sharesCorePhrase("规则：Go 代码改动需 go build + 受影响包测试", "规则（Go 代码改动 = go build + 受影响包测试），已满足") {
		t.Error("reworded duplicate rules must share a core phrase")
	}
	if sharesCorePhrase("规则：使用 tabs 缩进", "偏好：使用中文注释") {
		t.Error("unrelated statements must not share a core phrase")
	}
	if sharesCorePhrase("短", "短句") {
		t.Error("statements below the minimum length must not match")
	}
}

func TestOneLine(t *testing.T) {
	// oneLine collapses all whitespace (including newlines) into single spaces.
	tests := []struct {
		input    string
		expected string
	}{
		{"line one\nline two\nline three", "line one line two line three"},
		{"single line", "single line"},
		{"", ""},
		{"\nstarts with newline", "starts with newline"},
		{"  spaces  \nmore", "spaces more"},
	}
	for _, tc := range tests {
		got := oneLine(tc.input)
		if got != tc.expected {
			t.Errorf("oneLine(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestSuggestionName(t *testing.T) {
	// suggestionName should produce a kebab-case slug
	name := suggestionName("", "用户偏好使用 tabs 缩进", "fallback")
	if name == "" || name == "fallback" {
		t.Errorf("suggestionName should produce a meaningful name, got: %q", name)
	}
}

func TestSuggestionTitle(t *testing.T) {
	title := suggestionTitle("请记住用户偏好使用 tabs 缩进", "Default")
	if title == "Default" {
		t.Errorf("suggestionTitle should derive title from statement, got: %q", title)
	}
}
