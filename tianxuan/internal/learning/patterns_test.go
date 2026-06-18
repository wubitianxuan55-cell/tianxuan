package learning

import (
	"strings"
	"testing"
)

func TestIsErrorResult(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{"plain error", "error: file not found", true},
		{"failed prefix", "failed: connection refused", true},
		{"not found", "old_string not found in file", true},
		{"not unique", "pattern is not unique", true},
		{"timeout", "error: operation timeout", true},
		{"denied", "permission denied", true},
		{"refused", "connection refused", true},
		{"conflict", "merge conflict detected", true},
		{"mixed case", "Error: something went wrong", true},
		{"success output", "ok (cached)", false},
		{"empty string", "", false},
		{"normal text", "file content here\nline 2", false},
		{"git diff output", "diff --git a/file.go b/file.go\n+ok", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isErrorResult(tt.input)
			if got != tt.expect {
				t.Errorf("isErrorResult(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

func TestPatternSig(t *testing.T) {
	sig1 := patternSig("edit_file", "old_string_not_found")
	sig2 := patternSig("edit_file", "old_string_not_found")
	sig3 := patternSig("edit_file", "old_string_not_unique")

	if sig1 != sig2 {
		t.Errorf("same inputs should produce same sig: %s != %s", sig1, sig2)
	}
	if sig1 == sig3 {
		t.Errorf("different kinds should produce different sigs: %s", sig1)
	}
	if len(sig1) != 16 {
		t.Errorf("sig should be 16 hex chars, got %d", len(sig1))
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		n      int
		expect string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 10, ""},
		{"abc", 2, "ab..."},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.n)
		if got != tt.expect {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.expect)
		}
	}
}

func TestMergePattern(t *testing.T) {
	s := &Store{}
	p1 := &Pattern{Sig: "abc", Tool: "edit_file", ErrorKind: "e1", Count: 1, LastSeen: "2026-01-01"}
	p2 := &Pattern{Sig: "abc", Tool: "edit_file", ErrorKind: "e1", Count: 1, LastSeen: "2026-01-02"}
	p3 := &Pattern{Sig: "def", Tool: "grep", ErrorKind: "e2", Count: 1, LastSeen: "2026-01-03"}

	MergePattern(s, p1)
	if len(s.Patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(s.Patterns))
	}

	MergePattern(s, p2)
	if len(s.Patterns) != 1 {
		t.Fatalf("expected still 1 pattern after merge, got %d", len(s.Patterns))
	}
	if s.Patterns[0].Count != 2 {
		t.Errorf("expected count 2, got %d", s.Patterns[0].Count)
	}
	if s.Patterns[0].LastSeen != "2026-01-02" {
		t.Errorf("expected LastSeen updated to 2026-01-02, got %s", s.Patterns[0].LastSeen)
	}

	MergePattern(s, p3)
	if len(s.Patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(s.Patterns))
	}
}

func TestActivePatterns(t *testing.T) {
	s := &Store{Patterns: []Pattern{
		{Sig: "a", Count: 5, Skipped: false},
		{Sig: "b", Count: 2, Skipped: false},
		{Sig: "c", Count: 10, Skipped: true},
		{Sig: "d", Count: 8, Skipped: false},
	}}

	active := ActivePatterns(s, 3)
	if len(active) != 2 {
		t.Fatalf("expected 2 active patterns (minCount=3), got %d", len(active))
	}
	// Sorted by count desc
	if active[0].Count != 8 {
		t.Errorf("expected first pattern count 8, got %d", active[0].Count)
	}
	if active[1].Count != 5 {
		t.Errorf("expected second pattern count 5, got %d", active[1].Count)
	}

	// Empty store
	empty := ActivePatterns(&Store{}, 3)
	if len(empty) != 0 {
		t.Errorf("expected 0 patterns from empty store, got %d", len(empty))
	}
}

func TestFormatGuide(t *testing.T) {
	patterns := []Pattern{
		{Tool: "edit_file", ErrorKind: "old_string_not_found", RecoveryAction: "read file first"},
		{Tool: "bash", ErrorKind: "bash_timeout", RecoveryAction: "reduce scope"},
	}
	guide := FormatGuide(patterns)

	if !strings.Contains(guide, "Learned Patterns") {
		t.Errorf("expected 'Learned Patterns' in guide, got: %s", guide)
	}
	if !strings.Contains(guide, "edit_file") {
		t.Errorf("expected 'edit_file' in guide, got: %s", guide)
	}
	if !strings.Contains(guide, "read file first") {
		t.Errorf("expected recovery action in guide, got: %s", guide)
	}

	// Empty patterns
	empty := FormatGuide(nil)
	if empty != "" {
		t.Errorf("expected empty guide for nil patterns, got: %s", empty)
	}
	empty2 := FormatGuide([]Pattern{})
	if empty2 != "" {
		t.Errorf("expected empty guide for empty patterns, got: %s", empty2)
	}
}

func TestPruneOld(t *testing.T) {
	s := &Store{Patterns: []Pattern{
		{Sig: "a", Count: 10, Skipped: false},
		{Sig: "b", Count: 5, Skipped: true},
		{Sig: "c", Count: 3, Skipped: false},
		{Sig: "d", Count: 8, Skipped: false},
	}}

	PruneOld(s, 30, 2)
	if len(s.Patterns) != 2 {
		t.Fatalf("expected 2 patterns after prune (maxPatterns=2), got %d", len(s.Patterns))
	}
	// Skipped pattern "b" should be removed
	for _, p := range s.Patterns {
		if p.Skipped {
			t.Errorf("expected no skipped patterns, got: %+v", p)
		}
	}

	// Nil store
	PruneOld(nil, 30, 10) // should not panic
}

// Test errorClassifiers for each tool
func TestErrorClassifiers(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		result   string
		wantKind string
	}{
		// edit_file
		{"edit_file: old_string not found", "edit_file", "error: old_string not found in file", "old_string_not_found"},
		{"edit_file: not found variant", "edit_file", "not found", "old_string_not_found"},
		{"edit_file: not unique", "edit_file", "old_string is not unique", "old_string_not_unique"},
		{"edit_file: success", "edit_file", "file edited successfully", ""},

		// delete_range
		{"delete_range: not found", "delete_range", "start_anchor not found", "anchor_not_found"},
		{"delete_range: success", "delete_range", "range deleted", ""},

		// delete_symbol
		{"delete_symbol: not found", "delete_symbol", "symbol not found", "symbol_not_found"},
		{"delete_symbol: success", "delete_symbol", "symbol deleted", ""},

		// grep
		{"grep: no results", "grep", "error: no results found", "grep_no_results"},
		{"grep: 0 results", "grep", "error: 0 results", "grep_no_results"},
		{"grep: empty result not an error", "grep", "", ""},
		{"grep: success", "grep", "file.go:10: match", ""},

		// glob
		{"glob: no matches", "glob", "error: no matches found", "glob_no_matches"},
		{"glob: no files", "glob", "error: no files found", "glob_no_matches"},
		{"glob: success", "glob", "file.go\nother.go", ""},

		// bash
		{"bash: command not found", "bash", "bash: command not found", "command_not_found"},
		{"bash: merge conflict", "bash", "CONFLICT in file.go", "git_merge_conflict"},
		{"bash: timeout", "bash", "error: command timed out after 120s", "bash_timeout"},
		{"bash: permission denied", "bash", "permission denied", "permission_denied"},
		{"bash: file not found", "bash", "error: no such file or directory", "file_not_found"},
		{"bash: success", "bash", "ok", ""},

		// web_fetch
		{"web_fetch: timeout", "web_fetch", "error: request timed out", "fetch_timeout"},
		{"web_fetch: connection error", "web_fetch", "connection refused", "fetch_connection_error"},
		{"web_fetch: success", "web_fetch", "200 OK", ""},

		// write_file
		{"write_file: outside workspace", "write_file", "error: path outside workspace confine", "write_outside_workspace"},
		{"write_file: success", "write_file", "file written", ""},
	}

	extractor := NewExtractor("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := extractor.Extract(tt.tool, tt.result)
			if tt.wantKind == "" {
				if p != nil {
					t.Errorf("expected nil pattern for success result, got kind=%q", p.ErrorKind)
				}
				return
			}
			if p == nil {
				t.Fatalf("expected pattern for %s/%s, got nil", tt.tool, tt.wantKind)
			}
			if p.ErrorKind != tt.wantKind {
				t.Errorf("expected kind %q, got %q", tt.wantKind, p.ErrorKind)
			}
			if p.Tool != tt.tool {
				t.Errorf("expected tool %q, got %q", tt.tool, p.Tool)
			}
			if p.RecoveryAction == "" {
				t.Errorf("expected non-empty recovery action")
			}
			if p.Count != 1 {
				t.Errorf("expected count 1, got %d", p.Count)
			}
			if p.Sig == "" {
				t.Errorf("expected non-empty sig")
			}
		})
	}
}

func TestExtractEmptyResult(t *testing.T) {
	extractor := NewExtractor("")
	p := extractor.Extract("edit_file", "")
	if p != nil {
		t.Errorf("expected nil for empty result, got %+v", p)
	}
}

func TestExtractUnknownTool(t *testing.T) {
	extractor := NewExtractor("")
	p := extractor.Extract("unknown_tool", "error: something failed")
	if p != nil {
		t.Errorf("expected nil for unknown tool, got %+v", p)
	}
}
