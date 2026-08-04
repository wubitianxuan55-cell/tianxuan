package agent

import (
	"context"
	"strings"
	"testing"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
	_ "tianxuan/internal/tool/builtin"
)

// TestSchemaValidationBlocksBadArgs verifies the codex-style argument
// validator runs in the production execution path: a type-invalid edit_file
// call is rejected with validation_error BEFORE the tool executes, and the
// tool result tells the model which field is wrong (so it can fix the args in
// the next turn instead of guessing again).
func TestSchemaValidationBlocksBadArgs(t *testing.T) {
	prov := &mockProvider{name: "p", chunks: []provider.Chunk{
		{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "c1", Name: "edit_file", Arguments: `{"path":123,"old_string":"x","new_string":"y"}`}},
		{Type: provider.ChunkDone},
	}}
	reg := tool.NewRegistry()
	for _, b := range tool.Builtins() {
		reg.Add(b)
	}
	a := New(prov, reg, NewSession(""), Options{MaxSteps: 1}, event.Discard)
	_, _ = a.Run(context.Background(), "edit test.go")

	var found string
	for _, m := range a.session.Messages {
		if m.Role == provider.RoleTool && m.Name == "edit_file" {
			found = m.Content
			break
		}
	}
	if found == "" {
		t.Fatal("no edit_file tool result in session")
	}
	if !strings.Contains(found, "validation_error") {
		t.Fatalf("tool result should be validation_error, got: %s", found)
	}
	if !strings.Contains(found, "path") {
		t.Fatalf("error should name field path, got: %s", found)
	}
}

// TestSchemaValidationAllowsAlias verifies deliberate aliases survive the
// validator: read_file({"file": ...}) is not a validation error even though
// "file" is absent from the schema (locked by args_alias_test.go).
func TestSchemaValidationAllowsAlias(t *testing.T) {
	prov := &mockProvider{name: "p", chunks: []provider.Chunk{
		{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "c1", Name: "read_file", Arguments: `{"file":"/x"}`}},
		{Type: provider.ChunkDone},
	}}
	reg := tool.NewRegistry()
	for _, b := range tool.Builtins() {
		reg.Add(b)
	}
	a := New(prov, reg, NewSession(""), Options{MaxSteps: 1}, event.Discard)
	_, _ = a.Run(context.Background(), "read a file")

	for _, m := range a.session.Messages {
		if m.Role == provider.RoleTool && m.Name == "read_file" {
			if strings.Contains(m.Content, "validation_error") {
				t.Fatalf("file alias must not be a validation error, got: %s", m.Content)
			}
			return
		}
	}
	t.Fatal("no read_file tool result in session")
}

// TestToolStatsRecordsFailures verifies the cross-session error stats wiring:
// a genuine tool failure (type-invalid edit_file) lands in the stats with its
// normalized kind, while the file alias call (success) does not.
func TestToolStatsRecordsFailures(t *testing.T) {
	stats := tool.NewMemStats()
	prov := &mockProvider{name: "p", chunks: []provider.Chunk{
		{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "c1", Name: "edit_file", Arguments: `{"path":123,"old_string":"x","new_string":"y"}`}},
		{Type: provider.ChunkDone},
	}}
	reg := tool.NewRegistry()
	for _, b := range tool.Builtins() {
		reg.Add(b)
	}
	a := New(prov, reg, NewSession(""), Options{MaxSteps: 1, ToolStats: stats}, event.Discard)
	_, _ = a.Run(context.Background(), "edit test.go")

	snap := stats.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("stats has %d entries, want 1 (validation_error): %+v", len(snap), snap)
	}
	e := snap[0]
	if e.Tool != "edit_file" || e.ErrorKind != "validation_error" || e.Count != 1 {
		t.Fatalf("stats entry = %+v, want edit_file/validation_error/1", e)
	}
}
