package agent

import (
	"strings"
	"testing"

	"tianxuan/internal/provider"
)

func TestPostBatchCoherenceCheck_ReadFailBlocksWrite(t *testing.T) {
	a := &AgentRunner{}

	calls := []provider.ToolCall{
		{ID: "c1", Name: "read_file", Arguments: `{"path":"foo.go"}`},
		{ID: "c2", Name: "edit_file", Arguments: `{"path":"foo.go","old_string":"x","new_string":"y"}`},
	}
	results := []string{
		"error: file not found",  // read_file failed
		"",                        // edit_file hasn't run yet (would fail anyway)
	}

	a.postBatchCoherenceCheck(calls, results)

	if !strings.HasPrefix(results[1], "blocked: cannot edit") {
		t.Errorf("edit_file after failed read should be blocked, got: %q", results[1])
	}
	if !strings.Contains(results[1], "foo.go") {
		t.Errorf("block message should mention file path, got: %q", results[1])
	}
}

func TestPostBatchCoherenceCheck_WriteFailWarnsRead(t *testing.T) {
	a := &AgentRunner{}

	calls := []provider.ToolCall{
		{ID: "c1", Name: "write_file", Arguments: `{"path":"bar.go","content":"x"}`},
		{ID: "c2", Name: "read_file", Arguments: `{"path":"bar.go"}`},
	}
	results := []string{
		"blocked: permission denied", // write_file failed
		"package bar\n...",            // read_file succeeded (but data may be stale)
	}

	a.postBatchCoherenceCheck(calls, results)

	if !strings.Contains(results[1], "[coherence warning") {
		t.Errorf("read after failed write should have coherence warning, got: %q", results[1])
	}
}

func TestPostBatchCoherenceCheck_NoFailuresNoChanges(t *testing.T) {
	a := &AgentRunner{}

	calls := []provider.ToolCall{
		{ID: "c1", Name: "read_file", Arguments: `{"path":"a.go"}`},
		{ID: "c2", Name: "edit_file", Arguments: `{"path":"a.go","old_string":"x","new_string":"y"}`},
	}
	results := []string{
		"package a\nfunc main() {}",
		"File edited successfully",
	}

	orig := make([]string, len(results))
	copy(orig, results)
	a.postBatchCoherenceCheck(calls, results)

	for i := range results {
		if results[i] != orig[i] {
			t.Errorf("result[%d] changed when no failures: %q -> %q", i, orig[i], results[i])
		}
	}
}

func TestPostBatchCoherenceCheck_SingleCallNoop(t *testing.T) {
	a := &AgentRunner{}

	calls := []provider.ToolCall{
		{ID: "c1", Name: "edit_file", Arguments: `{"path":"x.go","old_string":"a","new_string":"b"}`},
	}
	results := []string{"File edited successfully"}

	orig := make([]string, len(results))
	copy(orig, results)
	a.postBatchCoherenceCheck(calls, results)

	for i := range results {
		if results[i] != orig[i] {
			t.Errorf("single-call batch should be noop, result[%d] changed", i)
		}
	}
}

func TestPostBatchCoherenceCheck_DifferentFilesUnaffected(t *testing.T) {
	a := &AgentRunner{}

	calls := []provider.ToolCall{
		{ID: "c1", Name: "read_file", Arguments: `{"path":"foo.go"}`},
		{ID: "c2", Name: "edit_file", Arguments: `{"path":"bar.go","old_string":"x","new_string":"y"}`},
	}
	results := []string{
		"error: foo.go not found",
		"", // different file, should NOT be blocked
	}

	a.postBatchCoherenceCheck(calls, results)

	if strings.HasPrefix(results[1], "blocked:") {
		t.Errorf("edit of different file should not be blocked, got: %q", results[1])
	}
}

func TestPostBatchCoherenceCheck_AlreadyErrorNotOverwritten(t *testing.T) {
	a := &AgentRunner{}

	// If edit_file already has an error (ran and failed), don't overwrite it.
	calls := []provider.ToolCall{
		{ID: "c1", Name: "read_file", Arguments: `{"path":"foo.go"}`},
		{ID: "c2", Name: "edit_file", Arguments: `{"path":"foo.go","old_string":"x","new_string":"y"}`},
	}
	results := []string{
		"error: foo.go not found",
		"error: old_string not found", // edit_file already failed on its own
	}

	a.postBatchCoherenceCheck(calls, results)

	if strings.HasPrefix(results[1], "blocked:") {
		t.Errorf("existing error should not be overwritten, got: %q", results[1])
	}
}

func TestPostBatchCoherenceCheck_PrecheckBlockedRecognized(t *testing.T) {
	a := &AgentRunner{}

	calls := []provider.ToolCall{
		{ID: "c1", Name: "read_file", Arguments: `{"path":"foo.go"}`},
		{ID: "c2", Name: "edit_file", Arguments: `{"path":"foo.go","old_string":"x","new_string":"y"}`},
	}
	results := []string{
		"precheck blocked: foo.go not readable", // precheck blocks the read
		"",
	}

	a.postBatchCoherenceCheck(calls, results)

	if !strings.HasPrefix(results[1], "blocked: cannot edit") {
		t.Errorf("edit after precheck-blocked read should be blocked, got: %q", results[1])
	}
}

func TestPostBatchCoherenceCheck_EmptyPathToolsUnaffected(t *testing.T) {
	a := &AgentRunner{}

	// Tools with no file path in arguments (like bash) should be unaffected.
	calls := []provider.ToolCall{
		{ID: "c1", Name: "read_file", Arguments: `{"path":"foo.go"}`},
		{ID: "c2", Name: "bash", Arguments: `{"command":"echo hello"}`},
	}
	results := []string{
		"error: foo.go not found",
		"hello",
	}

	origResults2 := results[1]
	a.postBatchCoherenceCheck(calls, results)

	if results[1] != origResults2 {
		t.Errorf("non-file tool should be unaffected, got: %q", results[1])
	}
}

func TestIsReadTool(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"read_file", true},
		{"grep", true},
		{"ls", true},
		{"glob", true},
		{"lsp_definition", true},
		{"lsp_references", true},
		{"lsp_hover", true},
		{"edit_file", false},
		{"bash", false},
		{"task", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isReadTool(tt.name); got != tt.expected {
				t.Errorf("isReadTool(%q) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestIsWriteTool(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"edit_file", true},
		{"write_file", true},
		{"multi_edit", true},
		{"edit_lines", true},
		{"delete_range", true},
		{"delete_symbol", true},
		{"read_file", false},
		{"grep", false},
		{"bash", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWriteTool(tt.name); got != tt.expected {
				t.Errorf("isWriteTool(%q) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}
