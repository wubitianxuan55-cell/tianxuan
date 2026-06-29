package openai

import (
	"strings"
	"testing"
)

func TestSSEFastPathContentOnly(t *testing.T) {
	data := `{"choices":[{"delta":{"content":"hello world"},"index":0}]}`
	content, reasoning, needsFull := sseFastPath(data)
	if needsFull {
		t.Fatal("content-only line should not need full parse")
	}
	if content != "hello world" {
		t.Fatalf("content = %q, want %q", content, "hello world")
	}
	if reasoning != "" {
		t.Fatalf("reasoning should be empty, got %q", reasoning)
	}
}

func TestSSEFastPathReasoningOnly(t *testing.T) {
	data := `{"choices":[{"delta":{"reasoning_content":"let me think..."},"index":0}]}`
	content, reasoning, needsFull := sseFastPath(data)
	if needsFull {
		t.Fatal("reasoning-only line should not need full parse")
	}
	if reasoning != "let me think..." {
		t.Fatalf("reasoning = %s, want %s", reasoning, "let me think...")
	}
	if content != "" {
		t.Fatalf("content should be empty, got %q", content)
	}
}

func TestSSEFastPathBothFields(t *testing.T) {
	data := `{"choices":[{"delta":{"content":"answer","reasoning_content":"thinking"},"index":0}]}`
	content, reasoning, needsFull := sseFastPath(data)
	if needsFull {
		t.Fatal("should not need full parse")
	}
	if content != "answer" {
		t.Fatalf("content = %q", content)
	}
	if reasoning != "thinking" {
		t.Fatalf("reasoning = %q", reasoning)
	}
}

func TestSSEFastPathToolCallsNeedsFullParse(t *testing.T) {
	data := `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"read_file"}}]},"index":0}]}`
	_, _, needsFull := sseFastPath(data)
	if !needsFull {
		t.Fatal("tool_calls should trigger full parse")
	}
}

func TestSSEFastPathUsageNeedsFullParse(t *testing.T) {
	data := `{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`
	_, _, needsFull := sseFastPath(data)
	if !needsFull {
		t.Fatal("usage should trigger full parse")
	}
}

func TestSSEFastPathFinishReasonNeedsFullParse(t *testing.T) {
	data := `{"choices":[{"delta":{},"finish_reason":"stop","index":0}]}`
	_, _, needsFull := sseFastPath(data)
	if !needsFull {
		t.Fatal("finish_reason should trigger full parse")
	}
}

func TestSSEFastPathErrorNeedsFullParse(t *testing.T) {
	data := `{"error":{"message":"something went wrong"}}`
	_, _, needsFull := sseFastPath(data)
	if !needsFull {
		t.Fatal("error should trigger full parse")
	}
}

func TestSSEFastPathEmptyDelta(t *testing.T) {
	data := `{"choices":[{"delta":{},"index":0}]}`
	_, _, needsFull := sseFastPath(data)
	if !needsFull {
		t.Fatal("empty delta (no content, no reasoning) should trigger full parse")
	}
}

func TestSSEFastPathJSONEscaping(t *testing.T) {
	data := `{"choices":[{"delta":{"content":"line1\\nline2 with \\\"quote\\\""},"index":0}]}`
	content, _, needsFull := sseFastPath(data)
	if needsFull {
		t.Fatal("should not need full parse")
	}
	if !strings.Contains(content, "line2") || !strings.Contains(content, "quote") {
		t.Fatalf("content missing expected substrings: %s", content)
	}
}

func TestSSEFastPathChineseContent(t *testing.T) {
	data := `{"choices":[{"delta":{"content":"你好世界"},"index":0}]}`
	content, _, needsFull := sseFastPath(data)
	if needsFull {
		t.Fatal("Chinese content should not need full parse")
	}
	if content != "你好世界" {
		t.Fatalf("content = %q", content)
	}
}

func TestSSEFastPathEmptyString(t *testing.T) {
	_, _, needsFull := sseFastPath("")
	if !needsFull {
		t.Fatal("empty string should trigger full parse")
	}
}
