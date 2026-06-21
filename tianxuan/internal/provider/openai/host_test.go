package openai

import (
	"testing"

	"tianxuan/internal/provider"
)

func TestIsDeepSeek(t *testing.T) {
	tests := []struct {
		baseURL string
		want    bool
	}{
		{"https://api.deepseek.com/v1", true},
		{"https://api.deepseek.com", true},
		{"https://eu.deepseek.com/v1", true},
		{"https://us.deepseek.com", true},
		{"https://deepseek.com/v1", false},       // bare apex
		{"https://deepseek.com", false},           // bare apex
		{"https://api.openai.com/v1", false},
		{"https://api.minimaxi.com/v1", false},
	}
	for _, tc := range tests {
		got := IsDeepSeek(tc.baseURL)
		if got != tc.want {
			t.Errorf("IsDeepSeek(%q) = %v, want %v", tc.baseURL, got, tc.want)
		}
	}
}

func TestIsMiniMax(t *testing.T) {
	tests := []struct {
		baseURL string
		want    bool
	}{
		{"https://api.minimaxi.com/v1", true},
		{"https://api.minimaxi.com", true},
		{"https://eu.minimaxi.com/v1", true},
		{"https://minimaxi.com/v1", false},       // bare apex
		{"https://api.deepseek.com/v1", false},
		{"https://api.openai.com/v1", false},
	}
	for _, tc := range tests {
		got := IsMiniMax(tc.baseURL)
		if got != tc.want {
			t.Errorf("IsMiniMax(%q) = %v, want %v", tc.baseURL, got, tc.want)
		}
	}
}

func TestBuildRequestDeepSeekThinking(t *testing.T) {
	// DeepSeek client should inject thinking.type=enabled
	c := &client{model: "deepseek-v4", deepseek: true}
	req := c.buildRequest(provider.Request{})
	if req.Thinking == nil {
		t.Fatal("DeepSeek request.Thinking must not be nil")
	}
	if req.Thinking.Type != "enabled" {
		t.Errorf("DeepSeek thinking.type = %q, want enabled", req.Thinking.Type)
	}
}

func TestBuildRequestDeepSeekRoundTripsReasoning(t *testing.T) {
	// DeepSeek thinking mode: assistant tool_calls turn MUST carry reasoning_content
	c := &client{model: "deepseek-v4", deepseek: true}
	req := c.buildRequest(provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "do it"},
			{
				Role:             provider.RoleAssistant,
				Content:          "",
				ReasoningContent: "let me think about this...",
				ToolCalls:        []provider.ToolCall{{ID: "call_1", Name: "read_file", Arguments: `{"path":"x"}`}},
			},
		},
	})
	// The assistant message with tool_calls should preserve reasoning_content
	found := false
	for _, m := range req.Messages {
		if m.ReasoningContent == "let me think about this..." {
			found = true
			break
		}
	}
	if !found {
		t.Error("DeepSeek tool_calls turn must round-trip reasoning_content")
	}
}

func TestBuildRequestNonDeepSeekStripsReasoning(t *testing.T) {
	// Non-DeepSeek clients should NOT inject thinking or round-trip reasoning
	c := &client{model: "mimo-v2"}
	req := c.buildRequest(provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleAssistant, Content: "answer", ReasoningContent: "private"},
		},
	})
	if req.Thinking != nil {
		t.Error("non-DeepSeek request.Thinking must be nil")
	}
	for _, m := range req.Messages {
		if m.ReasoningContent != "" {
			t.Error("non-DeepSeek must not echo reasoning_content")
		}
	}
}

func TestNewAutoDetectsDeepSeek(t *testing.T) {
	p, err := New(provider.Config{
		Name:    "ds",
		BaseURL: "https://api.deepseek.com",
		Model:   "deepseek-v4",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c := p.(*client)
	if !c.deepseek {
		t.Error("api.deepseek.com should auto-detect deepseek=true")
	}
	if c.effort != "high" {
		t.Errorf("DeepSeek default effort = %q, want high", c.effort)
	}
}

func TestNewDeepSeekEffortValidation(t *testing.T) {
	_, err := New(provider.Config{
		Name:    "ds",
		BaseURL: "https://api.deepseek.com",
		Model:   "deepseek-v4",
		Extra:   map[string]any{"effort": "low"},
	})
	if err == nil {
		t.Error("DeepSeek with effort=low should fail validation")
	}
}
