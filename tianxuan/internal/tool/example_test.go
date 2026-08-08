package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestExampleFromSchema verifies a compact example JSON is generated from a
// tool schema's required fields and property types, so a validation error can
// point the model at the exact argument shape instead of the raw schema.
func TestExampleFromSchema(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"command":{"type":"string"},
			"cwd":{"type":"string"},
			"timeout_ms":{"type":"integer"}
		},
		"required":["command"]
	}`)
	got := ExampleFromSchema(schema)
	if !strings.Contains(got, `"command": "<command>"`) {
		t.Fatalf("example should include the required string field, got: %s", got)
	}
	if strings.Contains(got, "cwd") || strings.Contains(got, "timeout_ms") {
		t.Fatalf("example should only include required fields, got: %s", got)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("example must be valid JSON: %v (%s)", err, got)
	}
}

// TestExampleFromSchemaTypes verifies non-string property types get sensible
// example values.
func TestExampleFromSchemaTypes(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"limit":{"type":"integer"},
			"follow":{"type":"boolean"},
			"files":{"type":"array"},
			"opts":{"type":"object"}
		},
		"required":["limit","follow","files","opts"]
	}`)
	got := ExampleFromSchema(schema)
	for _, want := range []string{`"limit": 0`, `"follow": true`, `"files": []`, `"opts": {}`} {
		if !strings.Contains(got, want) {
			t.Fatalf("example missing %s, got: %s", want, got)
		}
	}
}

// TestExampleFromSchemaDegenerate verifies empty / unparseable schemas yield
// an empty example instead of panicking.
func TestExampleFromSchemaDegenerate(t *testing.T) {
	if got := ExampleFromSchema(nil); got != "" {
		t.Fatalf("nil schema should yield empty example, got %q", got)
	}
	if got := ExampleFromSchema(json.RawMessage(`{"type":"object"}`)); got != "" {
		t.Fatalf("schema without properties should yield empty example, got %q", got)
	}
}

// TestMisuseHintChatArgsOnBash verifies the known cross-tool mix-up detector
// flags chat-API style arguments passed to bash and stays quiet elsewhere.
func TestMisuseHintChatArgsOnBash(t *testing.T) {
	got := MisuseHint("bash", map[string]any{
		"model":      "deepseek-chat",
		"messages":   []any{map[string]any{"role": "user"}},
		"max_tokens": 100,
	})
	if got == "" || !strings.Contains(got, "chat") {
		t.Fatalf("bash with chat args should get a misuse hint, got %q", got)
	}
	if h := MisuseHint("bash", map[string]any{"command": "go test ./..."}); h != "" {
		t.Fatalf("bash with a valid command should have no hint, got %q", h)
	}
	if h := MisuseHint("read_file", map[string]any{"model": "x"}); h != "" {
		t.Fatalf("chat args on a non-bash tool should have no hint, got %q", h)
	}
}
