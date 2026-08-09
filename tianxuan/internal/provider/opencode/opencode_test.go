package opencode

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tianxuan/internal/provider"
)

// TestProtocolFor pins the model→protocol dispatch table against the Zen
// endpoint docs (2026-08): Claude/Qwen run on /v1/messages, GPT/Grok on
// /v1/responses, Gemini on the Google protocol, everything else on
// /v1/chat/completions.
func TestProtocolFor(t *testing.T) {
	cases := []struct {
		model string
		want  zenProtocol
	}{
		{"deepseek-v4-flash", protocolChat},
		{"deepseek-v4-flash-free", protocolChat},
		{"deepseek-v4-pro", protocolChat},
		{"kimi-k2.6", protocolChat},
		{"kimi-k3", protocolChat},
		{"glm-5.1", protocolChat},
		{"minimax-m2.7", protocolChat},
		{"big-pickle", protocolChat},
		{"north-mini-code-free", protocolChat},
		{"claude-haiku-4-5", protocolAnthropic},
		{"claude-sonnet-4-6", protocolAnthropic},
		{"claude-opus-4-8", protocolAnthropic},
		{"qwen3.7-plus", protocolAnthropic},
		{"gpt-5.4-nano", protocolResponses},
		{"gpt-5.6-sol", protocolResponses},
		{"grok-4.5", protocolResponses},
		{"gemini-3-flash", protocolGemini},
	}
	for _, tc := range cases {
		if got := protocolFor(tc.model); got != tc.want {
			t.Errorf("protocolFor(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

// TestNewDelegatesByProtocol verifies the factory routes each protocol family
// to the right backend and rejects protocols tianxuan does not speak yet.
func TestNewDelegatesByProtocol(t *testing.T) {
	// chat/completions family: usable with no key (Zen free tier is anonymous).
	p, err := New(provider.Config{Name: "zen", Model: "deepseek-v4-flash-free", BaseURL: "https://opencode.ai/zen/v1"})
	if err != nil {
		t.Fatalf("chat model: New: %v", err)
	}
	if p.Name() != "zen" {
		t.Errorf("chat model Name = %q, want %q", p.Name(), "zen")
	}

	// Anthropic family: delegates to the anthropic provider (x-api-key auth).
	if _, err := New(provider.Config{Name: "zen", Model: "claude-haiku-4-5", APIKey: "k"}); err != nil {
		t.Fatalf("anthropic model: New: %v", err)
	}

	// Responses family: the built-in responses client.
	p3, err := New(provider.Config{Name: "zen", Model: "gpt-5.4-nano", APIKey: "k"})
	if err != nil {
		t.Fatalf("responses model: New: %v", err)
	}
	if _, ok := p3.(*responsesClient); !ok {
		t.Errorf("responses model: got %T, want *responsesClient", p3)
	}

	// Gemini protocol is not implemented yet: loud, actionable error.
	_, err = New(provider.Config{Name: "zen", Model: "gemini-3-flash"})
	if err == nil {
		t.Fatal("gemini model: New should fail (unsupported protocol)")
	}
	if !strings.Contains(err.Error(), "gemini") {
		t.Errorf("gemini error should name the protocol: %v", err)
	}
}

// TestNewDefaultBaseURL verifies an empty base_url resolves to the Zen gateway
// (instead of openai.New's "base_url is required" / anthropic.New's first-party
// default, which would silently bypass Zen).
func TestNewDefaultBaseURL(t *testing.T) {
	for _, model := range []string{"deepseek-v4-flash-free", "claude-haiku-4-5", "gpt-5.4-nano"} {
		if _, err := New(provider.Config{Name: "zen", Model: model, APIKey: "k"}); err != nil {
			t.Errorf("model %q with empty base_url: %v", model, err)
		}
	}
}

// TestNewAnthropicStripsZenV1 guards the Zen endpoint layout: Zen's Anthropic
// endpoint is /zen/v1/messages, but the anthropic provider appends /v1/messages
// itself — so the Zen base URL must have its trailing /v1 stripped before
// delegation, or requests 404 on /zen/v1/v1/messages.
func TestNewAnthropicStripsZenV1(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusUnauthorized) // stop the provider's request early
	}))
	defer srv.Close()

	p, err := New(provider.Config{
		Name:    "zen",
		BaseURL: srv.URL + "/zen/v1",
		Model:   "claude-haiku-4-5",
		APIKey:  "k",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _ = p.Stream(ctx, provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if gotPath != "/zen/v1/messages" {
		t.Fatalf("request path = %q, want %q", gotPath, "/zen/v1/messages")
	}
}

// TestNewChatDeepSeekRoundTripsReasoning pins the Zen DeepSeek 400 regression:
// Zen serves deepseek-* models on /chat/completions but its base URL is
// opencode.ai, so the openai backend cannot detect DeepSeek by host. The
// opencode factory must declare the deepseek reasoning protocol explicitly —
// thinking.type=enabled in the request and reasoning_content round-tripped on
// the assistant tool_calls turn (the upstream API otherwise rejects the history
// with "The `reasoning_content` in the thinking mode must be passed back").
func TestNewChatDeepSeekRoundTripsReasoning(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":1,\"total_tokens\":6}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p, err := New(provider.Config{
		Name:    "zen",
		BaseURL: srv.URL,
		Model:   "deepseek-v4-flash-free",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, err := p.Stream(context.Background(), provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "read the config"},
			{Role: provider.RoleAssistant, Content: "", ReasoningContent: "REQUIRED-REASONING", ToolCalls: []provider.ToolCall{{ID: "call_1", Name: "read_file", Arguments: `{"path":"x"}`}}},
			{Role: provider.RoleTool, Content: "ok", ToolCallID: "call_1", Name: "read_file"},
			{Role: provider.RoleAssistant, Content: "plain answer", ReasoningContent: "PRIVATE-REASONING"},
			{Role: provider.RoleUser, Content: "thanks"},
		},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for range ch {
	}
	body := string(gotBody)
	if !strings.Contains(body, `"thinking":{"type":"enabled"}`) {
		t.Errorf("request must carry thinking.type=enabled for a Zen deepseek model:\n%s", body)
	}
	if !strings.Contains(body, "REQUIRED-REASONING") {
		t.Errorf("assistant tool_calls turn must round-trip reasoning_content:\n%s", body)
	}
	if strings.Contains(body, "PRIVATE-REASONING") {
		t.Errorf("plain assistant turn must not echo reasoning_content (billed prompt input):\n%s", body)
	}
}

// TestNewChatDeepSeekExplicitProtocolWins verifies an explicit
// reasoning_protocol from the provider config is respected instead of the
// deepseek default — disabling thinking must disable the reasoning round-trip.
func TestNewChatDeepSeekExplicitProtocolWins(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":1,\"total_tokens\":6}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p, err := New(provider.Config{
		Name:    "zen",
		BaseURL: srv.URL,
		Model:   "deepseek-v4-flash-free",
		Extra:   map[string]any{"reasoning_protocol": "none"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, err := p.Stream(context.Background(), provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "list files"},
			{Role: provider.RoleAssistant, Content: "", ReasoningContent: "SECRET-REASONING", ToolCalls: []provider.ToolCall{{ID: "call_1", Name: "ls", Arguments: `{}`}}},
			{Role: provider.RoleTool, Content: "ok", ToolCallID: "call_1", Name: "ls"},
			{Role: provider.RoleUser, Content: "thanks"},
		},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for range ch {
	}
	body := string(gotBody)
	if strings.Contains(body, "thinking") || strings.Contains(body, "SECRET-REASONING") {
		t.Errorf("explicit reasoning_protocol=none must disable thinking and the reasoning round-trip:\n%s", body)
	}
}
