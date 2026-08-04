package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyError(t *testing.T) {
	cases := []struct {
		tool    string
		code    string
		errMsg  string
		want    string
	}{
		{"edit_file", CodeValidationError, `field "path" must be a string`, "validation_error"},
		{"edit_file", CodeExecError, "old_string not found in x.go", "old_string_not_found"},
		{"edit_file", CodeExecError, "old_string is not unique in x.go (3 matches)", "old_string_not_unique"},
		{"delete_range", CodeExecError, "start_anchor not found", "anchor_not_found"},
		{"delete_symbol", CodeExecError, "symbol not found", "symbol_not_found"},
		{"bash", CodeTimeout, "command timed out", "timeout"},
		{"bash", CodeExecError, "command timed out after 120000 ms", "bash_timeout"},
		{"bash", CodeExecError, "command not found: foo", "command_not_found"},
		{"read_file", CodeNotFound, "no such file", "not_found"},
		{"write_file", CodeDenied, "blocked by policy", "denied"},
		{"git_commit", CodeExecError, "nothing to commit", "exec_error"},
	}
	for _, tc := range cases {
		if got := ClassifyError(tc.tool, tc.code, tc.errMsg); got != tc.want {
			t.Errorf("ClassifyError(%q, %q, %q) = %q, want %q", tc.tool, tc.code, tc.errMsg, got, tc.want)
		}
	}
}

func TestStatsRecordAggregates(t *testing.T) {
	s := NewMemStats()
	s.Record("edit_file", "old_string_not_found", "old_string not found in a.go")
	s.Record("edit_file", "old_string_not_found", "old_string not found in b.go")
	s.Record("edit_file", "validation_error", `field "path" must be a string`)
	s.Record("bash", "bash_timeout", "timed out")

	snap := s.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("snapshot has %d entries, want 3", len(snap))
	}
	var editNotFound, editValidation *ErrorStat
	for i := range snap {
		e := &snap[i]
		switch e.ErrorKind {
		case "old_string_not_found":
			editNotFound = e
		case "validation_error":
			editValidation = e
		}
	}
	if editNotFound == nil || editNotFound.Count != 2 {
		t.Fatalf("old_string_not_found count = %+v, want 2", editNotFound)
	}
	if editValidation == nil || editValidation.Count != 1 {
		t.Fatalf("validation_error entry = %+v, want count 1", editValidation)
	}
	if editNotFound.LastError == "" {
		t.Fatal("LastError should be populated")
	}
}

func TestStatsPersistsAndLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool-stats.json")
	s := NewStats(path)
	s.Record("edit_file", "old_string_not_found", "not found")

	loaded := NewStats(path)
	snap := loaded.Snapshot()
	if len(snap) != 1 || snap[0].Tool != "edit_file" || snap[0].Count != 1 {
		t.Fatalf("loaded snapshot = %+v, want edit_file count 1", snap)
	}
}

func TestStatsSnapshotSortedByCount(t *testing.T) {
	s := NewMemStats()
	s.Record("bash", "bash_timeout", "t")
	s.Record("edit_file", "old_string_not_found", "nf")
	s.Record("edit_file", "old_string_not_found", "nf")
	s.Record("edit_file", "old_string_not_found", "nf")

	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot has %d entries, want 2", len(snap))
	}
	if snap[0].Count < snap[1].Count {
		t.Fatalf("snapshot should be sorted by count desc, got %+v", snap)
	}
}

func TestStatsPathCreation(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, ".tianxuan")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	s := NewStats(filepath.Join(sub, "tool-stats.json"))
	s.Record("grep", "exec_error", "bad pattern")
	if _, err := os.Stat(filepath.Join(sub, "tool-stats.json")); err != nil {
		t.Fatalf("stats file not written: %v", err)
	}
}

func TestStatsEmptySnapshot(t *testing.T) {
	s := NewMemStats()
	snap := s.Snapshot()
	if len(snap) != 0 {
		t.Fatalf("fresh stats should be empty, got %+v", snap)
	}
	if strings.TrimSpace(s.Report()) == "" {
		t.Fatal("Report() of empty stats should still produce a header")
	}
}
