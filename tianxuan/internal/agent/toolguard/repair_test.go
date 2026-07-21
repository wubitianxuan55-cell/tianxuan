package toolguard

import (
	"strings"
	"testing"
)

// ── stripMarkdownFence ──────────────────────────────────────

func TestStripMarkdownFence_JSON(t *testing.T) {
	in := "```json\n{\"path\": \"x\"}\n```"
	out := stripMarkdownFence(in)
	if out != "{\"path\": \"x\"}" {
		t.Fatalf("expected unwrapped JSON, got %q", out)
	}
}

func TestStripMarkdownFence_Plain(t *testing.T) {
	in := "```\n{\"path\": \"x\"}\n```"
	out := stripMarkdownFence(in)
	if out != "{\"path\": \"x\"}" {
		t.Fatalf("expected unwrapped JSON, got %q", out)
	}
}

func TestStripMarkdownFence_NoFence(t *testing.T) {
	in := "{\"path\": \"x\"}"
	out := stripMarkdownFence(in)
	if out != in {
		t.Fatalf("no-fence should return unchanged, got %q", out)
	}
}

func TestStripMarkdownFence_Empty(t *testing.T) {
	if out := stripMarkdownFence(""); out != "" {
		t.Fatal("empty should remain empty")
	}
}

func TestStripMarkdownFence_WhitespaceAround(t *testing.T) {
	in := "  ```json\n{\"a\":1}\n```  "
	out := stripMarkdownFence(in)
	if out != "{\"a\":1}" {
		t.Fatalf("whitespace should be trimmed, got %q", out)
	}
}

// ── extractFirstJSONObject ──────────────────────────────────

func TestExtractFirstJSONObject_Simple(t *testing.T) {
	out := extractFirstJSONObject(`prefix {"key": "value"} suffix`)
	if out != `{"key": "value"}` {
		t.Fatalf("expected extracted object, got %q", out)
	}
}

func TestExtractFirstJSONObject_Nested(t *testing.T) {
	out := extractFirstJSONObject(`{"outer": {"inner": 1}}`)
	if out != `{"outer": {"inner": 1}}` {
		t.Fatalf("expected nested object, got %q", out)
	}
}

func TestExtractFirstJSONObject_StringWithBraces(t *testing.T) {
	out := extractFirstJSONObject(`{"key": "val{u}e"}`)
	if out != `{"key": "val{u}e"}` {
		t.Fatalf("braces inside string should not confuse parser, got %q", out)
	}
}

func TestExtractFirstJSONObject_EscapedQuote(t *testing.T) {
	out := extractFirstJSONObject(`{"k": "v\"al"}`)
	if out != `{"k": "v\"al"}` {
		t.Fatalf("escaped quote should not break parser, got %q", out)
	}
}

func TestExtractFirstJSONObject_Unclosed(t *testing.T) {
	out := extractFirstJSONObject(`{"key": "value"`)
	if out != "" {
		t.Fatalf("unclosed object should return empty, got %q", out)
	}
}

func TestExtractFirstJSONObject_NoBrace(t *testing.T) {
	out := extractFirstJSONObject(`no braces here`)
	if out != "" {
		t.Fatalf("no brace should return empty, got %q", out)
	}
}

func TestExtractFirstJSONObject_OnlyFirst(t *testing.T) {
	out := extractFirstJSONObject(`{"a":1} {"b":2}`)
	if out != `{"a":1}` {
		t.Fatalf("should extract only first object, got %q", out)
	}
}

// ── flattenWrapper ──────────────────────────────────────────

func TestFlattenWrapper_SingleKey(t *testing.T) {
	raw := map[string]any{"arguments": map[string]any{"path": "x.go"}}
	flattened, note := flattenWrapper(raw)
	if flattened == nil {
		t.Fatal("should flatten single-key wrapper")
	}
	if !strings.Contains(note, "flattened") {
		t.Fatalf("unexpected note: %s", note)
	}
	if flattened["path"] != "x.go" {
		t.Fatal("inner map not extracted")
	}
}

func TestFlattenWrapper_WithMetadata(t *testing.T) {
	raw := map[string]any{
		"arguments": map[string]any{"path": "x.go"},
		"tool":      "write_file",
	}
	flattened, _ := flattenWrapper(raw)
	if flattened == nil {
		t.Fatal("should flatten when non-wrapper keys are metadata only")
	}
}

func TestFlattenWrapper_NonWrapperKey(t *testing.T) {
	raw := map[string]any{
		"arguments": map[string]any{"path": "x.go"},
		"custom":    "value",
	}
	flattened, _ := flattenWrapper(raw)
	if flattened != nil {
		t.Fatal("should NOT flatten when unknown non-metadata key exists")
	}
}

func TestFlattenWrapper_NoWrapperKey(t *testing.T) {
	raw := map[string]any{"path": "x.go"}
	flattened, _ := flattenWrapper(raw)
	if flattened != nil {
		t.Fatal("should return nil when no wrapper key found")
	}
}

// ── scavengeSingleJSONString ────────────────────────────────

func TestScavengeSingleJSONString_Valid(t *testing.T) {
	raw := map[string]any{"args": `{"path": "x.go"}`}
	scavenged, note := scavengeSingleJSONString(raw)
	if scavenged == nil {
		t.Fatal("should scavenge JSON string")
	}
	if !strings.Contains(note, "scavenged") {
		t.Fatalf("unexpected note: %s", note)
	}
	if scavenged["path"] != "x.go" {
		t.Fatal("inner map not extracted")
	}
}

func TestScavengeSingleJSONString_MultiKey(t *testing.T) {
	raw := map[string]any{"a": "1", "b": "2"}
	scavenged, _ := scavengeSingleJSONString(raw)
	if scavenged != nil {
		t.Fatal("should not scavenge multi-key map")
	}
}

func TestScavengeSingleJSONString_NonString(t *testing.T) {
	raw := map[string]any{"args": 42}
	scavenged, _ := scavengeSingleJSONString(raw)
	if scavenged != nil {
		t.Fatal("should not scavenge non-string value")
	}
}

// ── truncateOversizedStrings ────────────────────────────────

func TestTruncateOversizedStrings_UnderLimit(t *testing.T) {
	raw := map[string]any{"key": "short"}
	fixed, changed := truncateOversizedStrings(raw, 100)
	if changed {
		t.Fatal("should not change strings under limit")
	}
	if fixed != nil {
		t.Fatal("should return nil when unchanged")
	}
}

func TestTruncateOversizedStrings_OverLimit(t *testing.T) {
	long := strings.Repeat("x", 200)
	raw := map[string]any{"key": long}
	fixed, changed := truncateOversizedStrings(raw, 100)
	if !changed {
		t.Fatal("should detect oversized string")
	}
	if fixed == nil {
		t.Fatal("should return fixed map")
	}
	v := fixed["key"].(string)
	if !strings.Contains(v, "[truncated") {
		t.Fatalf("truncated string missing marker: %q", v)
	}
	if len(v) > 100+50 { // 100 bytes + truncation marker
		t.Fatalf("truncated string too long: %d bytes", len(v))
	}
}

func TestTruncateOversizedStrings_Nested(t *testing.T) {
	raw := map[string]any{
		"outer": map[string]any{"inner": strings.Repeat("x", 200)},
	}
	fixed, changed := truncateOversizedStrings(raw, 100)
	if !changed {
		t.Fatal("should detect oversized string in nested map")
	}
	nested, _ := fixed["outer"].(map[string]any)
	if nested == nil {
		t.Fatal("nested map should be preserved")
	}
	v := nested["inner"].(string)
	if !strings.Contains(v, "[truncated") {
		t.Fatalf("nested truncated string missing marker: %q", v)
	}
}

// ── RepairDispatchToolArguments (integration) ──────────────

func TestRepairDispatch_NoRepairNeeded(t *testing.T) {
	raw := map[string]any{"path": "x.go"}
	result := RepairDispatchToolArguments(raw, ToolArgumentRepairOptions{MaxStringBytes: 512})
	if len(result.Notes) > 0 {
		t.Fatalf("no repair needed, got notes: %v", result.Notes)
	}
	if result.Arguments["path"] != "x.go" {
		t.Fatal("arguments should be unchanged")
	}
}

func TestRepairDispatch_FlattenWrapper(t *testing.T) {
	raw := map[string]any{"arguments": map[string]any{"path": "x.go"}}
	result := RepairDispatchToolArguments(raw, ToolArgumentRepairOptions{})
	if result.Arguments["path"] != "x.go" {
		t.Fatalf("should flatten wrapper, got %v", result.Arguments)
	}
	found := false
	for _, n := range result.Notes {
		if strings.Contains(n, "flattened") {
			found = true
		}
	}
	if !found {
		t.Fatal("notes should mention flattening")
	}
}

func TestRepairDispatch_ScavengeJSON(t *testing.T) {
	raw := map[string]any{"payload": `{"path": "x.go"}`}
	result := RepairDispatchToolArguments(raw, ToolArgumentRepairOptions{})
	if result.Arguments["path"] != "x.go" {
		t.Fatalf("should scavenge JSON string, got %v", result.Arguments)
	}
}

func TestRepairDispatch_PreserveLong(t *testing.T) {
	raw := map[string]any{"key": strings.Repeat("x", 200)}
	result := RepairDispatchToolArguments(raw, ToolArgumentRepairOptions{
		MaxStringBytes:      100,
		PreserveLongStrings: true,
	})
	v := result.Arguments["key"].(string)
	if strings.Contains(v, "[truncated") {
		t.Fatal("PreserveLongStrings should skip truncation")
	}
}

func TestRepairDispatch_ClonePreservesOriginal(t *testing.T) {
	raw := map[string]any{"path": "original.go"}
	result := RepairDispatchToolArguments(raw, ToolArgumentRepairOptions{})
	// Mutate result — should not affect original
	result.Arguments["path"] = "modified.go"
	if raw["path"] != "original.go" {
		t.Fatal("RepairDispatchToolArguments should clone, not mutate original")
	}
}
