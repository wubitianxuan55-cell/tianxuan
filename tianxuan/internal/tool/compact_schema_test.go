package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakeCompactTool is a minimal CompactDescriptor whose compact schema must
// survive the provider-facing pipeline with its parameter descriptions intact.
type fakeCompactTool struct {
	name string
}

func (f fakeCompactTool) Name() string          { return f.name }
func (fakeCompactTool) Description() string     { return "fake tool" }
func (fakeCompactTool) ReadOnly() bool          { return true }
func (fakeCompactTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (fakeCompactTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return "", nil
}
func (fakeCompactTool) CompactDescription() string { return "fake" }
func (fakeCompactTool) CompactSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path"},"limit":{"type":"integer","minimum":1,"description":"Max lines (default 2000)"}},"required":["path"]}`)
}

// TestFilteredSchemasKeepsCompactDescriptions locks the model-facing contract:
// the compact schema the model sees must keep parameter descriptions after the
// canonicalization pipeline (V10.154). Without them the model guesses field
// semantics, which is the root cause of parameter-class tool errors.
func TestFilteredSchemasKeepsCompactDescriptions(t *testing.T) {
	reg := NewRegistry()
	reg.Add(fakeCompactTool{name: "fake"})

	var got []byte
	for _, ts := range reg.FilteredSchemas([]string{"fake"}) {
		got = ts.Parameters
	}
	if len(got) == 0 {
		t.Fatal("no schema produced")
	}
	s := string(got)
	if !strings.Contains(s, "File path") || !strings.Contains(s, "Max lines (default 2000)") {
		t.Fatalf("compact schema lost parameter descriptions through canonicalization: %s", s)
	}
	if !strings.Contains(s, `"minimum":1`) {
		t.Fatalf("compact schema lost constraints: %s", s)
	}
}
