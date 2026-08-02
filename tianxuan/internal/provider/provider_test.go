package provider

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// --- SanitizeToolPairing ---

// toolIDsAnswered reports whether every assistant tool_call id has a following
// tool message answering it — the contract the OpenAI/DeepSeek API enforces.
func toolIDsAnswered(msgs []Message) bool {
	answered := map[string]bool{}
	for _, m := range msgs {
		if m.Role == RoleTool {
			answered[m.ToolCallID] = true
		}
	}
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			if !answered[tc.ID] {
				return false
			}
		}
	}
	return true
}

func TestSanitizeToolPairingBackfillsDanglingCall(t *testing.T) {
	in := []Message{
		{Role: RoleUser, Content: "list files"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "ls"}}},
		{Role: RoleUser, Content: "never mind"},
	}
	out := SanitizeToolPairing(in)
	if !toolIDsAnswered(out) {
		t.Fatalf("dangling tool_call left unanswered: %+v", out)
	}
	// The backfilled result sits right after the assistant turn, keyed to its id.
	if out[2].Role != RoleTool || out[2].ToolCallID != "c1" {
		t.Fatalf("expected a backfilled tool result for c1 at index 2, got %+v", out[2])
	}
}

func TestSanitizeToolPairingKeepsCallOrderAndMultiple(t *testing.T) {
	in := []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "a"}, {ID: "b"}, {ID: "c"}}},
		{Role: RoleTool, ToolCallID: "b", Content: "B"}, // out of order, c missing
		{Role: RoleTool, ToolCallID: "a", Content: "A"},
	}
	out := SanitizeToolPairing(in)
	if !toolIDsAnswered(out) {
		t.Fatalf("not all calls answered: %+v", out)
	}
	gotOrder := []string{out[1].ToolCallID, out[2].ToolCallID, out[3].ToolCallID}
	want := []string{"a", "b", "c"}
	for i := range want {
		if gotOrder[i] != want[i] {
			t.Fatalf("tool results out of call order: got %v want %v", gotOrder, want)
		}
	}
}

func TestSanitizeToolPairingDropsOrphanToolMessage(t *testing.T) {
	in := []Message{
		{Role: RoleUser, Content: "hi"},
		{Role: RoleTool, ToolCallID: "ghost", Content: "leftover"}, // no preceding call
		{Role: RoleAssistant, Content: "hello"},
	}
	out := SanitizeToolPairing(in)
	for _, m := range out {
		if m.Role == RoleTool {
			t.Fatalf("orphan tool message survived: %+v", out)
		}
	}
	if len(out) != 2 {
		t.Fatalf("want 2 messages after dropping the orphan, got %d: %+v", len(out), out)
	}
}

func TestSanitizeToolPairingLeavesWellFormedUnchanged(t *testing.T) {
	in := []Message{
		{Role: RoleSystem, Content: "sys"},
		{Role: RoleUser, Content: "q"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "ls"}}},
		{Role: RoleTool, ToolCallID: "c1", Name: "ls", Content: "main.go"},
		{Role: RoleAssistant, Content: "done"},
	}
	out := SanitizeToolPairing(in)
	if len(out) != len(in) {
		t.Fatalf("well-formed history changed length: %d -> %d", len(in), len(out))
	}
	for i := range in {
		if out[i].Role != in[i].Role || out[i].Content != in[i].Content || out[i].ToolCallID != in[i].ToolCallID {
			t.Fatalf("well-formed message %d mutated: %+v -> %+v", i, in[i], out[i])
		}
	}
}

// --- Pricing.Cost ---

func TestPricingCostNil(t *testing.T) {
	var p *Pricing
	if got := p.Cost(&Usage{PromptTokens: 100}); got != 0 {
		t.Errorf("nil Pricing.Cost = %f, want 0", got)
	}
}

func TestPricingCostNilUsage(t *testing.T) {
	p := &Pricing{Input: 2.0, Output: 10.0}
	if got := p.Cost(nil); got != 0 {
		t.Errorf("nil Usage.Cost = %f, want 0", got)
	}
}

func TestPricingCostBothNil(t *testing.T) {
	var p *Pricing
	if got := p.Cost(nil); got != 0 {
		t.Errorf("both nil.Cost = %f, want 0", got)
	}
}

func TestPricingCostCalculation(t *testing.T) {
	p := &Pricing{
		CacheHit: 0.5,  // ¥0.5 per 1M cached tokens
		Input:    2.0,  // ¥2.0 per 1M uncached tokens
		Output:   10.0, // ¥10.0 per 1M completion tokens
	}
	u := &Usage{
		CacheHitTokens:   1_000_000,
		CacheMissTokens:  500_000,
		CompletionTokens: 200_000,
	}
	// Expected: (1M * 0.5 + 500K * 2.0 + 200K * 10.0) / 1M
	//         = (0.5 + 1.0 + 2.0) = 3.5
	got := p.Cost(u)
	if got != 3.5 {
		t.Errorf("Cost = %f, want 3.5", got)
	}
}

func TestPricingCostZeroTokens(t *testing.T) {
	p := &Pricing{Input: 2.0, Output: 10.0}
	u := &Usage{}
	if got := p.Cost(u); got != 0 {
		t.Errorf("zero tokens Cost = %f, want 0", got)
	}
}

// --- DeepSeek peak-hour billing (峰谷计价) ---

// bjHourUTC returns a time whose Beijing (UTC+8) wall clock is the given hour
// on 2026-08-02. Peak windows are Beijing 09:00–12:00 and 14:00–18:00.
func bjHourUTC(h int) time.Time {
	return time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC).Add(time.Duration(h-8) * time.Hour)
}

func TestIsDeepSeekPeak(t *testing.T) {
	for _, h := range []int{9, 10, 11, 14, 15, 16, 17} {
		if !IsDeepSeekPeak(bjHourUTC(h)) {
			t.Errorf("Beijing %02d:00 should be peak", h)
		}
	}
	for _, h := range []int{0, 8, 12, 13, 18, 23} {
		if IsDeepSeekPeak(bjHourUTC(h)) {
			t.Errorf("Beijing %02d:00 should be off-peak", h)
		}
	}
}

func TestIsDeepSeekPeakBoundaries(t *testing.T) {
	bj := time.FixedZone("Beijing", 8*3600)
	cases := []struct {
		wall string
		peak bool
	}{
		{"08:59", false},
		{"09:00", true},
		{"11:59", true},
		{"12:00", false},
		{"13:59", false},
		{"14:00", true},
		{"17:59", true},
		{"18:00", false},
	}
	for _, c := range cases {
		at, err := time.ParseInLocation("2006-01-02 15:04", "2026-08-02 "+c.wall, bj)
		if err != nil {
			t.Fatal(err)
		}
		if got := IsDeepSeekPeak(at); got != c.peak {
			t.Errorf("IsDeepSeekPeak(%s) = %v, want %v", c.wall, got, c.peak)
		}
	}
}

func TestPricingCostAtPeakDoubles(t *testing.T) {
	p := &Pricing{CacheHit: 0.02, Input: 1, Output: 2, PeakMultiplier: 2}
	u := &Usage{CacheHitTokens: 1_000_000, CacheMissTokens: 1_000_000, CompletionTokens: 1_000_000}
	base := 0.02 + 1 + 2
	if got := p.CostAt(u, bjHourUTC(10)); got != base*2 {
		t.Errorf("peak CostAt = %f, want %f", got, base*2)
	}
	if got := p.CostAt(u, bjHourUTC(20)); got != base {
		t.Errorf("off-peak CostAt = %f, want %f", got, base)
	}
}

func TestPricingCostAtNoPeakConfig(t *testing.T) {
	// PeakMultiplier unset (zero) keeps flat pricing in every window.
	p := &Pricing{CacheHit: 0.02, Input: 1, Output: 2}
	u := &Usage{CacheHitTokens: 1_000_000, CompletionTokens: 1_000_000}
	base := 0.02 + 2
	if got := p.CostAt(u, bjHourUTC(10)); got != base {
		t.Errorf("no-peak-config CostAt during peak = %f, want %f", got, base)
	}
}

func TestPricingCostAtNil(t *testing.T) {
	var p *Pricing
	if got := p.CostAt(&Usage{PromptTokens: 100}, time.Now()); got != 0 {
		t.Errorf("nil Pricing.CostAt = %f, want 0", got)
	}
	if got := (&Pricing{Input: 2.0}).CostAt(nil, time.Now()); got != 0 {
		t.Errorf("nil Usage.CostAt = %f, want 0", got)
	}
}

// --- Pricing.Symbol ---

func TestPricingSymbolDefault(t *testing.T) {
	p := &Pricing{}
	if got := p.Symbol(); got != "楼" {
		t.Errorf("empty Currency.Symbol() = %q, want 楼", got)
	}
}

func TestPricingSymbolNil(t *testing.T) {
	var p *Pricing
	if got := p.Symbol(); got != "楼" {
		t.Errorf("nil.Symbol() = %q, want 楼", got)
	}
}

func TestPricingSymbolCustom(t *testing.T) {
	p := &Pricing{Currency: "$"}
	if got := p.Symbol(); got != "$" {
		t.Errorf("Symbol() = %q, want $", got)
	}
}

// --- AuthError ---

func TestAuthErrorWithKeyEnv(t *testing.T) {
	e := &AuthError{Provider: "deepseek", KeyEnv: "DEEPSEEK_API_KEY", Status: 401}
	msg := e.Error()
	for _, want := range []string{"deepseek", "DEEPSEEK_API_KEY", "401", "invalid or expired"} {
		if !contains(msg, want) {
			t.Errorf("AuthError.Error() missing %q: %s", want, msg)
		}
	}
}

func TestAuthErrorWithoutKeyEnv(t *testing.T) {
	e := &AuthError{Provider: "openai", Status: 403}
	msg := e.Error()
	if !contains(msg, "the API key") {
		t.Errorf("AuthError without KeyEnv should say 'the API key': %s", msg)
	}
	if !contains(msg, "403") {
		t.Errorf("AuthError should include status code 403: %s", msg)
	}
}

func TestAuthErrorImplementsError(t *testing.T) {
	var err error = &AuthError{Provider: "test", Status: 401}
	if err.Error() == "" {
		t.Error("AuthError.Error() should not be empty")
	}
}

// --- Registry ---

func TestRegistryKindsSorted(t *testing.T) {
	// The openai package self-registers via init(); we can't control that here
	// but we can verify Kinds() returns a sorted list.
	kinds := Kinds()
	for i := 1; i < len(kinds); i++ {
		if kinds[i-1] >= kinds[i] {
			t.Errorf("Kinds() not sorted: %v", kinds)
			break
		}
	}
}

func TestNewUnknownKind(t *testing.T) {
	_, err := New("nonexistent-kind-xyzzy", Config{})
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if !contains(err.Error(), "unknown kind") {
		t.Errorf("error should mention 'unknown kind': %v", err)
	}
}

func TestNewWithRegisteredKind(t *testing.T) {
	// Register a mock factory.
	Register("test-mock-__"+t.Name(), func(cfg Config) (Provider, error) {
		return nil, nil
	})
	// We can't easily unregister, but we can test it doesn't panic.
}

func TestNewRejectsTypedNilProvider(t *testing.T) {
	kind := "test-typed-nil-__" + t.Name()
	Register(kind, func(cfg Config) (Provider, error) {
		var p *mockProvider
		return p, nil
	})

	_, err := New(kind, Config{})
	if err == nil {
		t.Fatal("New should reject typed nil provider")
	}
	if !contains(err.Error(), "returned nil provider") {
		t.Fatalf("New error = %v, want returned nil provider", err)
	}
}

// --- Role constants ---

func TestRoleConstants(t *testing.T) {
	if RoleSystem != "system" {
		t.Errorf("RoleSystem = %q", RoleSystem)
	}
	if RoleUser != "user" {
		t.Errorf("RoleUser = %q", RoleUser)
	}
	if RoleAssistant != "assistant" {
		t.Errorf("RoleAssistant = %q", RoleAssistant)
	}
	if RoleTool != "tool" {
		t.Errorf("RoleTool = %q", RoleTool)
	}
}

// --- ChunkType constants ---

func TestChunkTypeConstants(t *testing.T) {
	types := []ChunkType{ChunkText, ChunkReasoning, ChunkToolCallStart, ChunkToolCall, ChunkUsage, ChunkDone, ChunkError}
	for i, ct := range types {
		if int(ct) != i {
			t.Errorf("ChunkType %d: got %d", i, int(ct))
		}
	}
}

// --- ToolSchema ---

func TestToolSchemaJSON(t *testing.T) {
	ts := ToolSchema{
		Name:        "bash",
		Description: "Run a shell command",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	}
	b, err := json.Marshal(ts)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !contains(string(b), "bash") {
		t.Errorf("JSON missing name: %s", b)
	}
}

// helper
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Ensure the Provider interface is satisfied by a minimal mock (compile-time check).
var _ Provider = (*mockProvider)(nil)

type mockProvider struct{}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Stream(ctx context.Context, req Request) (<-chan Chunk, error) {
	ch := make(chan Chunk, 1)
	ch <- Chunk{Type: ChunkDone}
	close(ch)
	return ch, nil
}

func TestMockProviderImplementsInterface(t *testing.T) {
	p := &mockProvider{}
	if p.Name() != "mock" {
		t.Errorf("Name = %q", p.Name())
	}
	ch, err := p.Stream(context.Background(), Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	got := <-ch
	if got.Type != ChunkDone {
		t.Errorf("Chunk.Type = %d, want ChunkDone", got.Type)
	}
}
