package learning

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		// JSON-envelope (V8.9+): WrapError / WrapResult format
		{"json-envelope error", `{"ok":false,"success":false,"code":"exec_error","error":"file not found"}`, true},
		{"json-envelope blocked", `{"ok":false,"success":false,"code":"blocked","error":"blocked by storm guard"}`, true},
		{"json-envelope timeout", `{"ok":false,"success":false,"code":"timeout","error":"timed out"}`, true},
		{"json-envelope success", `{"ok":true,"success":true,"code":"ok","data":{"output":"done"}}`, false},
		{"json-envelope with message", `{"ok":true,"success":true,"code":"ok","message":"file written"}`, false},
		{"json-envelope not_found", `{"ok":false,"success":false,"code":"not_found","error":"path not found"}`, true},
		{"success output", "ok (cached)", false},
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

// TestObserveMergesAndPersists locks the learn-from-errors pipeline: each
// observed failure must be merged into the on-disk store (count grows per
// occurrence), different tools/kinds stay separate, and successes never create
// patterns. This regresses the bug where Extract() returned a pattern that was
// dropped before merge, so counts never advanced and the system-prompt guide
// stayed empty forever.
func TestObserveMergesAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learned-patterns.toml")
	e := NewExtractor(path)

	errEdit := `{"ok":false,"success":false,"code":"exec_error","error":"old_string not found in a.go"}`
	if p := e.Observe("edit_file", errEdit); p == nil || p.Count != 1 {
		t.Fatalf("first observe = %+v, want count 1", p)
	}
	if p := e.Observe("edit_file", errEdit); p == nil || p.Count != 2 {
		t.Fatalf("second observe = %+v, want count 2 (merge bug)", p)
	}
	if p := e.Observe("bash", `{"ok":false,"success":false,"code":"timeout","error":"timed out"}`); p == nil || p.Tool != "bash" {
		t.Fatalf("bash observe = %+v, want a bash pattern", p)
	}

	store, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}
	if len(store.Patterns) != 2 {
		t.Fatalf("store has %d patterns, want 2 (edit_file + bash)", len(store.Patterns))
	}
	if store.Patterns[0].Count != 2 {
		t.Fatalf("edit_file count = %d, want 2", store.Patterns[0].Count)
	}

	if p := e.Observe("edit_file", `{"ok":true,"success":true,"code":"ok"}`); p != nil {
		t.Fatalf("success should not create a pattern, got %+v", p)
	}
}

// TestActivePatternsThreshold verifies the injection threshold: patterns below
// the min-count are not surfaced into the system prompt.
func TestActivePatternsThreshold(t *testing.T) {
	store := &Store{Patterns: []Pattern{
		{Tool: "edit_file", ErrorKind: "old_string_not_found", Count: 2},
		{Tool: "bash", ErrorKind: "bash_timeout", Count: 5},
	}}
	active := ActivePatterns(store, 3)
	if len(active) != 1 || active[0].Tool != "bash" {
		t.Fatalf("active = %+v, want only bash (count 5 >= 3)", active)
	}
	guide := FormatGuide(active)
	if !strings.Contains(guide, "bash") || strings.Contains(guide, "edit_file") {
		t.Fatalf("guide should mention bash only, got: %s", guide)
	}
}

// TestObserveValidationError verifies the universal classifier: schema
// validation failures (V10.154 codex-style validator) are learnable for every
// tool, with a recovery hint telling the model to re-read the embedded schema.
func TestObserveValidationError(t *testing.T) {
	e := NewExtractor(filepath.Join(t.TempDir(), "learned-patterns.toml"))
	result := `{"ok":false,"success":false,"code":"validation_error","error":"field \"path\" must be a string, got float64","data":{"schema":"{\"type\":\"object\"}"}}`
	p := e.Observe("edit_file", result)
	if p == nil {
		t.Fatal("validation_error not learned")
	}
	if p.ErrorKind != "validation_error" {
		t.Fatalf("kind = %q, want validation_error", p.ErrorKind)
	}
	if !strings.Contains(p.RecoveryAction, "schema") {
		t.Fatalf("recovery should mention the embedded schema, got: %s", p.RecoveryAction)
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

	// maxAgeDays 过期删除：LastSeen 早于 N 天的 pattern 应被移除；空 LastSeen 保守保留。
	staleDate := time.Now().AddDate(0, 0, -60).Format("2006-01-02")
	recentDate := time.Now().Format("2006-01-02")
	sAge := &Store{Patterns: []Pattern{
		{Sig: "recent", Count: 5, LastSeen: recentDate},
		{Sig: "stale", Count: 5, LastSeen: staleDate},
		{Sig: "no-date", Count: 5},
	}}
	PruneOld(sAge, 30, 100)
	if len(sAge.Patterns) != 2 {
		t.Fatalf("expected 2 patterns after age prune (stale removed), got %d", len(sAge.Patterns))
	}
	for _, p := range sAge.Patterns {
		if p.Sig == "stale" {
			t.Errorf("expected stale pattern removed by maxAgeDays, got %+v", p)
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
