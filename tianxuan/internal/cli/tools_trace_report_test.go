package cli

import (
	"os"
	"strings"
	"testing"

	"tianxuan/internal/tool"
)

// TestToolTraceReportCommandPrintsTable verifies `tools trace-report`
// aggregates the JSONL trace into a per-tool error-rate table.
func TestToolTraceReportCommandPrintsTable(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(oldWD)

	store, err := tool.NewTraceStore(tool.DefaultTracePath(dir))
	if err != nil {
		t.Fatalf("NewTraceStore: %v", err)
	}
	defer store.Close()
	store.Record(tool.TraceEntry{Ts: "t1", Tool: "read_file", Outcome: "success", DurationMs: 5})
	store.Record(tool.TraceEntry{Ts: "t2", Tool: "edit_file", Outcome: "error", DurationMs: 9, Error: "old_string not found"})
	store.Record(tool.TraceEntry{Ts: "t3", Tool: "edit_file", Outcome: "error", DurationMs: 11, Error: "old_string not found"})
	store.Record(tool.TraceEntry{Ts: "t4", Tool: "bash", Outcome: "success", DurationMs: 120})

	oldOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := toolTraceReportCommand()
	w.Close()
	os.Stdout = oldOut
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	out := string(buf[:n])

	if code != 0 {
		t.Fatalf("toolTraceReportCommand exit = %d, want 0", code)
	}
	for _, want := range []string{"tool", "edit_file", "100.0%", "read_file", "0.0%"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	// error-rate table is sorted by error count descending: edit_file first.
	efIdx := strings.Index(out, "edit_file")
	rfIdx := strings.Index(out, "read_file")
	if efIdx < 0 || rfIdx < 0 || efIdx > rfIdx {
		t.Fatalf("edit_file should precede read_file in sorted table:\n%s", out)
	}
}

// TestToolTraceReportCommandEmpty reports the empty state instead of failing.
func TestToolTraceReportCommandEmpty(t *testing.T) {
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(oldWD)

	code := toolTraceReportCommand()
	if code != 0 {
		t.Fatalf("empty trace-report exit = %d, want 0 (reports empty state)", code)
	}
}
