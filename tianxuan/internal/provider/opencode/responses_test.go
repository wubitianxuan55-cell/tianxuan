package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"tianxuan/internal/provider"
)

// TestResponsesBuildRequest verifies the chat-style provider.Request is
// translated into the OpenAI Responses wire format: assistant tool_calls
// expand into function_call items, tool results into function_call_output,
// tools become flat {"type":"function",...} entries, and MaxTokens maps to
// max_output_tokens.
func TestResponsesBuildRequest(t *testing.T) {
	c := &responsesClient{model: "gpt-5.4-nano"}
	req := c.buildRequest(provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "sys"},
			{Role: provider.RoleUser, Content: "hi"},
			{Role: provider.RoleAssistant, Content: "checking", ToolCalls: []provider.ToolCall{
				{ID: "call_1", Name: "bash", Arguments: `{"cmd":"ls"}`},
			}},
			{Role: provider.RoleTool, ToolCallID: "call_1", Name: "bash", Content: "out"},
		},
		Tools: []provider.ToolSchema{
			{Name: "bash", Description: "run a command", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
		MaxTokens: 4096,
	})
	if req.Model != "gpt-5.4-nano" || !req.Stream || req.MaxOutputTokens != 4096 {
		t.Errorf("request head wrong: %+v", req)
	}
	if len(req.Input) != 5 {
		t.Fatalf("input items = %d, want 5 (system,user,assistant,function_call,function_call_output)", len(req.Input))
	}
	want := []inputItem{
		{Role: "system", Content: []contentPart{{Type: "input_text", Text: "sys"}}},
		{Role: "user", Content: []contentPart{{Type: "input_text", Text: "hi"}}},
		{Role: "assistant", Content: []contentPart{{Type: "output_text", Text: "checking"}}},
		{Type: "function_call", CallID: "call_1", Name: "bash", Arguments: `{"cmd":"ls"}`},
		{Type: "function_call_output", CallID: "call_1", Output: "out"},
	}
	if !reflect.DeepEqual(req.Input, want) {
		t.Errorf("input mismatch:\n got %+v\nwant %+v", req.Input, want)
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "bash" || req.Tools[0].Description != "run a command" {
		t.Errorf("tools wrong: %+v", req.Tools)
	}
	if !json.Valid(req.Tools[0].Parameters) {
		t.Errorf("parameters not valid JSON: %s", req.Tools[0].Parameters)
	}
}

// TestResponsesBuildRequestToolPairingRepair makes sure an interrupted history
// (assistant tool_calls with no following result) is repaired before
// translation, mirroring the openai provider's guard.
func TestResponsesBuildRequestToolPairingRepair(t *testing.T) {
	c := &responsesClient{model: "gpt-5.4-nano"}
	req := c.buildRequest(provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "hi"},
			{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
				{ID: "call_1", Name: "bash", Arguments: `{}`},
			}},
		},
	})
	// assistant message + backfilled function_call_output for the missing result.
	if len(req.Input) != 3 {
		t.Fatalf("input items = %d, want 3 after pairing repair: %+v", len(req.Input), req.Input)
	}
	last := req.Input[len(req.Input)-1]
	if last.Type != "function_call_output" || last.CallID != "call_1" || last.Output == "" {
		t.Errorf("backfilled result wrong: %+v", last)
	}
}

// TestResponsesStreamText drives a full text lifecycle over a fake SSE server
// and checks the emitted Chunk sequence (text deltas, usage, done).
func TestResponsesStreamText(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"r1","status":"in_progress"}}`,
		``,
		`data: {"type":"response.in_progress","response":{"id":"r1","status":"in_progress"}}`,
		``,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"m1","status":"in_progress","role":"assistant","content":[]}}`,
		``,
		`data: {"type":"response.content_part.added","item_id":"m1","output_index":0,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
		``,
		`data: {"type":"response.output_text.delta","item_id":"m1","output_index":0,"content_index":0,"delta":"Hello"}`,
		``,
		`data: {"type":"response.output_text.delta","item_id":"m1","output_index":0,"content_index":0,"delta":" world"}`,
		``,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"m1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello world","annotations":[]}]}}`,
		``,
		`data: {"type":"response.completed","response":{"id":"r1","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":3},"output_tokens_details":{"reasoning_tokens":2}}}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	chunks := streamResponses(t, sse, provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	var kinds []provider.ChunkType
	var text strings.Builder
	for _, c := range chunks {
		kinds = append(kinds, c.Type)
		if c.Type == provider.ChunkText {
			text.WriteString(c.Text)
		}
	}
	if text.String() != "Hello world" {
		t.Errorf("text = %q, want %q", text.String(), "Hello world")
	}
	if !hasChunk(kinds, provider.ChunkUsage) || !hasChunk(kinds, provider.ChunkDone) {
		t.Errorf("missing usage/done; kinds = %v", kinds)
	}
	var u *provider.Usage
	for _, c := range chunks {
		if c.Type == provider.ChunkUsage {
			u = c.Usage
		}
	}
	if u == nil || u.TotalTokens != 15 || u.PromptTokens != 10 || u.CompletionTokens != 5 ||
		u.CacheHitTokens != 3 || u.ReasoningTokens != 2 {
		t.Errorf("usage wrong: %+v", u)
	}
}

// TestResponsesStreamToolCall verifies tool-call streaming: a start chunk as
// soon as the name is known, then a complete call with accumulated arguments.
func TestResponsesStreamToolCall(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"bash","arguments":"","status":"in_progress"}}`,
		``,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":0,"delta":"{\"cmd\":"}`,
		``,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":0,"delta":"\"ls\"}"}`,
		``,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"bash","arguments":"{\"cmd\":\"ls\"}","status":"completed"}}`,
		``,
		`data: {"type":"response.completed","response":{"id":"r1","status":"completed","output":[],"usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6}}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	chunks := streamResponses(t, sse, provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "list files"}},
	})
	var starts, completes []provider.ToolCall
	for _, c := range chunks {
		if c.Type == provider.ChunkToolCallStart {
			starts = append(starts, *c.ToolCall)
		}
		if c.Type == provider.ChunkToolCall {
			completes = append(completes, *c.ToolCall)
		}
	}
	if len(starts) != 1 || starts[0].ID != "call_1" || starts[0].Name != "bash" {
		t.Errorf("start chunks wrong: %+v", starts)
	}
	if len(completes) != 1 || completes[0].ID != "call_1" || completes[0].Name != "bash" ||
		completes[0].Arguments != `{"cmd":"ls"}` {
		t.Errorf("complete chunks wrong: %+v", completes)
	}
}

// TestResponsesStreamReasoning maps reasoning deltas (DeepSeek-native and the
// standard summary variant) to ChunkReasoning.
func TestResponsesStreamReasoning(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.reasoning_text.delta","item_id":"m1","output_index":0,"delta":"think"}`,
		``,
		`data: {"type":"response.reasoning_summary_text.delta","item_id":"m1","output_index":0,"summary_index":0,"delta":" harder"}`,
		``,
		`data: {"type":"response.completed","response":{"id":"r1","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	chunks := streamResponses(t, sse, provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	var reasoning strings.Builder
	for _, c := range chunks {
		if c.Type == provider.ChunkReasoning {
			reasoning.WriteString(c.Text)
		}
	}
	if reasoning.String() != "think harder" {
		t.Errorf("reasoning = %q, want %q", reasoning.String(), "think harder")
	}
}

// TestResponsesStreamError surfaces a streamed error event as ChunkError.
func TestResponsesStreamError(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"error","error":{"code":"invalid_request_error","message":"boom"}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	chunks := streamResponses(t, sse, provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	var err error
	for _, c := range chunks {
		if c.Type == provider.ChunkError {
			err = c.Err
		}
	}
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want error mentioning boom, got %v", err)
	}
}

// TestResponsesStreamAuthError verifies 401 surfaces as an actionable
// *provider.AuthError naming the key env var.
func TestResponsesStreamAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	c := &responsesClient{
		name:    "zen",
		apiKey:  "bad",
		keyEnv:  "OPENCODE_API_KEY",
		baseURL: srv.URL,
		model:   "gpt-5.4-nano",
		http:    srv.Client(),
	}
	_, err := c.Stream(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	var authErr *provider.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("want *provider.AuthError, got %T: %v", err, err)
	}
	if authErr.Provider != "zen" || authErr.KeyEnv != "OPENCODE_API_KEY" || authErr.Status != 401 {
		t.Errorf("AuthError fields wrong: %+v", authErr)
	}
}

// streamResponses spins up a fake SSE endpoint and drains the client stream.
func streamResponses(t *testing.T, sse string, req provider.Request) []provider.Chunk {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sse)
	}))
	defer srv.Close()

	c := &responsesClient{
		name:    "zen",
		apiKey:  "k",
		baseURL: srv.URL,
		model:   "gpt-5.4-nano",
		http:    srv.Client(),
	}
	ch, err := c.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var out []provider.Chunk
	for ck := range ch {
		out = append(out, ck)
	}
	return out
}

func hasChunk(kinds []provider.ChunkType, want provider.ChunkType) bool {
	for _, k := range kinds {
		if k == want {
			return true
		}
	}
	return false
}
