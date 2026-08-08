package cli

import (
	"os"
	"strings"
	"testing"

	"tianxuan/internal/tool"
)

// TestToolTraceCommandPrintsTail verifies `tools trace` reads the JSONL trace
// file in the workspace and prints the most recent entries, oldest first.
func TestToolTraceCommandPrintsTail(t *testing.T) {
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
	store.Record(tool.TraceEntry{Ts: "t1", Tool: "read_file", Outcome: "success", DurationMs: 5, Args: `{"path":"a.go"}`})
	store.Record(tool.TraceEntry{Ts: "t2", Tool: "edit_file", Outcome: "error", DurationMs: 9, Args: `{"path":"a.go","old_string":"x"}`})
	store.Record(tool.TraceEntry{Ts: "t3", Tool: "bash", Outcome: "success", DurationMs: 120, Args: `{"command":"go build ./..."}`})

	// capture stdout
	oldOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := toolTraceCommand([]string{"-n", "2"})
	w.Close()
	os.Stdout = oldOut
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])

	if code != 0 {
		t.Fatalf("toolTraceCommand exit = %d, want 0", code)
	}
	// tail 2 → last two entries (edit_file error + bash), not the first.
	if !strings.Contains(out, "edit_file") || !strings.Contains(out, "bash") {
		t.Fatalf("output = %q, want tail to include edit_file and bash", out)
	}
	if strings.Contains(out, "read_file") {
		t.Fatalf("output = %q, want read_file (first entry) excluded by tail 2", out)
	}
	if !strings.Contains(out, "2 of 3 dispatches") {
		t.Fatalf("output = %q, want summary line '2 of 3 dispatches'", out)
	}
}

// TestToolTraceCommandEmpty reports the empty state instead of failing.
func TestToolTraceCommandEmpty(t *testing.T) {
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(oldWD)

	code := toolTraceCommand(nil)
	if code != 0 {
		t.Fatalf("empty trace exit = %d, want 0 (reports empty state)", code)
	}
}
