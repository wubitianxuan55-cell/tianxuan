package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSummarizeTraceEmpty verifies an empty trace file yields an empty report
// (no panic, no error) so the CLI can print the friendly empty state.
func TestSummarizeTraceEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool-trace.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := SummarizeTrace(path)
	if err != nil {
		t.Fatalf("SummarizeTrace: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d tools, want 0", len(got))
	}
}

// TestSummarizeTraceAggregates verifies per-tool counts, error rate and
// average duration are computed from the JSONL stream.
func TestSummarizeTraceAggregates(t *testing.T) {
	dir := t.TempDir()
	store, err := NewTraceStore(filepath.Join(dir, "tool-trace.jsonl"))
	if err != nil {
		t.Fatalf("NewTraceStore: %v", err)
	}
	defer store.Close()

	store.Record(TraceEntry{Ts: "t1", Tool: "read_file", Outcome: "success", DurationMs: 5})
	store.Record(TraceEntry{Ts: "t2", Tool: "read_file", Outcome: "success", DurationMs: 6})
	store.Record(TraceEntry{Ts: "t3", Tool: "read_file", Outcome: "error", DurationMs: 7, Error: "file not found"})
	store.Record(TraceEntry{Ts: "t4", Tool: "bash", Outcome: "success", DurationMs: 120})
	store.Record(TraceEntry{Ts: "t5", Tool: "bash", Outcome: "blocked", DurationMs: 0})
	store.Record(TraceEntry{Ts: "t6", Tool: "edit_file", Outcome: "error", DurationMs: 30, Error: "old_string not found"})
	store.Record(TraceEntry{Ts: "t7", Tool: "edit_file", Outcome: "error", DurationMs: 40, Error: "old_string not unique"})

	got, err := SummarizeTrace(filepath.Join(dir, "tool-trace.jsonl"))
	if err != nil {
		t.Fatalf("SummarizeTrace: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d tools, want 3", len(got))
	}

	byTool := map[string]ToolTraceStat{}
	for _, s := range got {
		byTool[s.Tool] = s
	}

	rf := byTool["read_file"]
	if rf.Calls != 3 || rf.Success != 2 || rf.Errors != 1 || rf.Blocked != 0 {
		t.Fatalf("read_file = %+v, want calls 3 / success 2 / errors 1 / blocked 0", rf)
	}
	if rf.AvgMs != 6 {
		t.Fatalf("read_file AvgMs = %d, want 6", rf.AvgMs)
	}
	if rf.ErrorRate != float64(1)/3 {
		t.Fatalf("read_file ErrorRate = %v, want 0.333…", rf.ErrorRate)
	}

	bs := byTool["bash"]
	if bs.Calls != 2 || bs.Success != 1 || bs.Errors != 0 || bs.Blocked != 1 {
		t.Fatalf("bash = %+v, want calls 2 / success 1 / errors 0 / blocked 1", bs)
	}
	if bs.ErrorRate != 0.5 {
		t.Fatalf("bash ErrorRate = %v, want 0.5", bs.ErrorRate)
	}

	ef := byTool["edit_file"]
	if ef.Calls != 2 || ef.Errors != 2 || ef.ErrorRate != 1.0 {
		t.Fatalf("edit_file = %+v, want calls 2 / errors 2 / rate 1.0", ef)
	}
}

// TestSummarizeTraceTopErrors verifies the most frequent error text per tool
// is ranked first and capped at 3 distinct entries.
func TestSummarizeTraceTopErrors(t *testing.T) {
	dir := t.TempDir()
	store, err := NewTraceStore(filepath.Join(dir, "tool-trace.jsonl"))
	if err != nil {
		t.Fatalf("NewTraceStore: %v", err)
	}
	defer store.Close()

	for i := 0; i < 2; i++ {
		store.Record(TraceEntry{Ts: "t", Tool: "edit_file", Outcome: "error", Error: "old_string not found"})
	}
	store.Record(TraceEntry{Ts: "t", Tool: "edit_file", Outcome: "error", Error: "old_string not unique"})
	store.Record(TraceEntry{Ts: "t", Tool: "edit_file", Outcome: "error", Error: "old_string not found"})
	store.Record(TraceEntry{Ts: "t", Tool: "bash", Outcome: "error", Error: "command not found"})
	for i := 0; i < 5; i++ {
		store.Record(TraceEntry{Ts: "t", Tool: "bash", Outcome: "error", Error: "exit status 1"})
	}

	got, err := SummarizeTrace(filepath.Join(dir, "tool-trace.jsonl"))
	if err != nil {
		t.Fatalf("SummarizeTrace: %v", err)
	}
	byTool := map[string]ToolTraceStat{}
	for _, s := range got {
		byTool[s.Tool] = s
	}

	ef := byTool["edit_file"]
	if len(ef.TopErrors) != 2 {
		t.Fatalf("edit_file TopErrors = %v, want 2 entries", ef.TopErrors)
	}
	if ef.TopErrors[0] != "old_string not found" {
		t.Fatalf("edit_file TopErrors[0] = %q, want %q", ef.TopErrors[0], "old_string not found")
	}

	bs := byTool["bash"]
	if len(bs.TopErrors) != 2 || bs.TopErrors[0] != "exit status 1" {
		t.Fatalf("bash TopErrors = %v, want ['exit status 1', 'command not found']", bs.TopErrors)
	}
}

// TestSummarizeTraceSkipsBadLines verifies a corrupt JSONL line is skipped
// without aborting the whole report.
func TestSummarizeTraceSkipsBadLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool-trace.jsonl")
	content := "this is not json\n" +
		`{"ts":"t1","tool":"read_file","outcome":"success","duration_ms":4}` + "\n" +
		"{\n" // truncated line at EOF
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := SummarizeTrace(path)
	if err != nil {
		t.Fatalf("SummarizeTrace: %v", err)
	}
	if len(got) != 1 || got[0].Tool != "read_file" || got[0].Calls != 1 {
		t.Fatalf("got %+v, want read_file calls 1", got)
	}
}

// TestSummarizeTraceSorting verifies tools are ordered by error count
// (descending), then call count (descending), then name.
func TestSummarizeTraceSorting(t *testing.T) {
	dir := t.TempDir()
	store, err := NewTraceStore(filepath.Join(dir, "tool-trace.jsonl"))
	if err != nil {
		t.Fatalf("NewTraceStore: %v", err)
	}
	defer store.Close()

	store.Record(TraceEntry{Ts: "t", Tool: "grep", Outcome: "error"})
	store.Record(TraceEntry{Ts: "t", Tool: "edit_file", Outcome: "error"})
	store.Record(TraceEntry{Ts: "t", Tool: "edit_file", Outcome: "error"})
	store.Record(TraceEntry{Ts: "t", Tool: "bash", Outcome: "error"})
	store.Record(TraceEntry{Ts: "t", Tool: "bash", Outcome: "error"})
	store.Record(TraceEntry{Ts: "t", Tool: "bash", Outcome: "success"})

	got, err := SummarizeTrace(filepath.Join(dir, "tool-trace.jsonl"))
	if err != nil {
		t.Fatalf("SummarizeTrace: %v", err)
	}
	wantOrder := []string{"bash", "edit_file", "grep"}
	if len(got) != len(wantOrder) {
		t.Fatalf("got %d tools, want %d", len(got), len(wantOrder))
	}
	for i, want := range wantOrder {
		if got[i].Tool != want {
			t.Fatalf("order[%d] = %q, want %q (got %+v)", i, got[i].Tool, want, got)
		}
	}
	// tie-break on calls: edit_file and bash both have 2 errors, but bash has
	// 3 calls so it must come first; grep has 1 error and is last.
	if strings.Join([]string{got[0].Tool, got[1].Tool, got[2].Tool}, ",") != "bash,edit_file,grep" {
		t.Fatalf("unexpected order: %s", strings.Join([]string{got[0].Tool, got[1].Tool, got[2].Tool}, ","))
	}
}
