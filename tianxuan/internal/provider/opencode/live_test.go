package opencode

import (
	"context"
	"os"
	"testing"
	"time"

	"tianxuan/internal/provider"
)

// TestLiveChatFree exercises the Zen free tier end-to-end with no API key
// (deepseek-v4-flash-free is anonymous). Run explicitly:
//
//	TIANXUAN_LIVE_OPENCODE=1 go test ./internal/provider/opencode/ -run TestLiveChatFree -v
func TestLiveChatFree(t *testing.T) {
	if os.Getenv("TIANXUAN_LIVE_OPENCODE") == "" {
		t.Skip("TIANXUAN_LIVE_OPENCODE not set — skipping live test")
	}
	prov, err := New(provider.Config{Name: "zen", Model: "deepseek-v4-flash-free"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	chunks := streamLive(t, prov, "What is 1+1? Answer in one word.")
	if got := countKind(chunks, provider.ChunkText) + countKind(chunks, provider.ChunkReasoning); got == 0 {
		t.Fatalf("no text/reasoning chunks: %v", kindsOf(chunks))
	}
	if !hasKind(chunks, provider.ChunkDone) {
		t.Fatalf("stream never completed: %v", kindsOf(chunks))
	}
}

// TestLiveResponses exercises the Responses protocol (GPT class) against the
// real Zen gateway. Needs OPENCODE_API_KEY.
func TestLiveResponses(t *testing.T) {
	apiKey := os.Getenv("OPENCODE_API_KEY")
	if apiKey == "" {
		t.Skip("OPENCODE_API_KEY not set — skipping live test")
	}
	prov, err := New(provider.Config{
		Name:   "zen",
		Model:  "gpt-5.4-nano",
		APIKey: apiKey,
		Extra:  map[string]any{"api_key_env": "OPENCODE_API_KEY"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	chunks := streamLive(t, prov, "What is 2+2? Answer in one word.")
	if got := countKind(chunks, provider.ChunkText) + countKind(chunks, provider.ChunkReasoning); got == 0 {
		t.Fatalf("no text/reasoning chunks: %v", kindsOf(chunks))
	}
	if !hasKind(chunks, provider.ChunkDone) {
		t.Fatalf("stream never completed: %v", kindsOf(chunks))
	}
}

// TestLiveMessages exercises the Anthropic protocol (Claude class) against the
// real Zen gateway. Needs OPENCODE_API_KEY.
func TestLiveMessages(t *testing.T) {
	apiKey := os.Getenv("OPENCODE_API_KEY")
	if apiKey == "" {
		t.Skip("OPENCODE_API_KEY not set — skipping live test")
	}
	prov, err := New(provider.Config{
		Name:   "zen",
		Model:  "claude-haiku-4-5",
		APIKey: apiKey,
		Extra:  map[string]any{"api_key_env": "OPENCODE_API_KEY"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	chunks := streamLive(t, prov, "What is 3+3? Answer in one word.")
	if got := countKind(chunks, provider.ChunkText) + countKind(chunks, provider.ChunkReasoning); got == 0 {
		t.Fatalf("no text/reasoning chunks: %v", kindsOf(chunks))
	}
	if !hasKind(chunks, provider.ChunkDone) {
		t.Fatalf("stream never completed: %v", kindsOf(chunks))
	}
}

func streamLive(t *testing.T, prov provider.Provider, prompt string) []provider.Chunk {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	ch, err := prov.Stream(ctx, provider.Request{
		Messages:  []provider.Message{{Role: provider.RoleUser, Content: prompt}},
		MaxTokens: 128,
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var out []provider.Chunk
	for c := range ch {
		out = append(out, c)
	}
	return out
}

func countKind(chunks []provider.Chunk, kind provider.ChunkType) int {
	n := 0
	for _, c := range chunks {
		if c.Type == kind {
			n++
		}
	}
	return n
}

func hasKind(chunks []provider.Chunk, kind provider.ChunkType) bool {
	return countKind(chunks, kind) > 0
}

func kindsOf(chunks []provider.Chunk) []provider.ChunkType {
	out := make([]provider.ChunkType, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, c.Type)
	}
	return out
}
