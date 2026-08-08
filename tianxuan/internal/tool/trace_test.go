package tool

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTraceStoreAppendsJSONL verifies each Record appends one self-contained
// JSON line (JSONL format) so the file can be streamed/parsed line by line.
func TestTraceStoreAppendsJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool-trace.jsonl")
	store, err := NewTraceStore(path)
	if err != nil {
		t.Fatalf("NewTraceStore: %v", err)
	}
	defer store.Close()

	store.Record(TraceEntry{
		Ts:         "2026-08-08T15:00:00+08:00",
		SessionID:  "sess-1",
		TraceID:    "trace-1",
		CallID:     "call-1",
		Tool:       "edit_file",
		ReadOnly:   false,
		Args:       `{"path":"a.go","old_string":"x","new_string":"y"}`,
		Outcome:    "error",
		Error:      "old_string not found",
		OutputLen:  12,
		DurationMs: 34,
	})
	store.Record(TraceEntry{
		Ts:         "2026-08-08T15:00:01+08:00",
		SessionID:  "sess-1",
		TraceID:    "trace-1",
		CallID:     "call-2",
		Tool:       "bash",
		ReadOnly:   false,
		Args:       `{"command":"go build ./..."}`,
		Outcome:    "success",
		OutputLen:  99,
		DurationMs: 1200,
	})

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open trace file: %v", err)
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}

	var first TraceEntry
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line 1 not valid JSON: %v\n%s", err, lines[0])
	}
	if first.Tool != "edit_file" || first.Outcome != "error" || first.DurationMs != 34 {
		t.Fatalf("line 1 = %+v, want edit_file/error/34ms", first)
	}
	if first.Error != "old_string not found" {
		t.Fatalf("line 1 Error = %q", first.Error)
	}
}

// TestTraceStoreTruncatesLongFields caps Args and Error so a single bad call
// (e.g. a huge bash command or a long error chain) cannot bloat the trace file.
func TestTraceStoreTruncatesLongFields(t *testing.T) {
	dir := t.TempDir()
	store, err := NewTraceStore(filepath.Join(dir, "trace.jsonl"))
	if err != nil {
		t.Fatalf("NewTraceStore: %v", err)
	}
	defer store.Close()

	long := strings.Repeat("a", traceArgMax*2)
	store.Record(TraceEntry{
		Ts:      "t",
		Tool:    "bash",
		Args:    long,
		Outcome: "error",
		Error:   long,
	})

	// The store should persist the capped values, not the originals.
	raw, err := os.ReadFile(filepath.Join(dir, "trace.jsonl"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got TraceEntry
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Args) != traceArgMax {
		t.Fatalf("Args length = %d, want capped at %d", len(got.Args), traceArgMax)
	}
	if len(got.Error) != traceErrMax {
		t.Fatalf("Error length = %d, want capped at %d", len(got.Error), traceErrMax)
	}
}

// TestTraceStoreNilSafe verifies a nil *TraceStore never panics — the agent
// wires one in optionally, so zero-value safety is a hard contract.
func TestTraceStoreNilSafe(t *testing.T) {
	var s *TraceStore
	s.Record(TraceEntry{Tool: "x"}) // must not panic
	s.Close()                        // must not panic
}

// TestDefaultTracePath returns the canonical per-workspace trace file path
// next to tool-stats.json.
func TestDefaultTracePath(t *testing.T) {
	got := DefaultTracePath(`C:\work\proj`)
	want := filepath.Join(`C:\work\proj`, ".tianxuan", "tool-trace.jsonl")
	if got != want {
		t.Fatalf("DefaultTracePath = %q, want %q", got, want)
	}
}
