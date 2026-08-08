package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

var errBoom = errors.New("boom")

// TestExecuteOneRecordsTrace verifies the V10.167 defer records every dispatch
// path — success, execution error, and blocked — as one JSONL line each, with
// the outcome classified correctly.
func TestExecuteOneRecordsTrace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool-trace.jsonl")
	traceStore, err := tool.NewTraceStore(path)
	if err != nil {
		t.Fatalf("NewTraceStore: %v", err)
	}
	defer traceStore.Close()

	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_file", readOnly: true})
	reg.Add(fakeTool{name: "bash", readOnly: false, err: errBoom})
	reg.Add(fakeTool{name: "write_file", readOnly: false})

	g := &stubGate{deny: map[string]bool{"write_file": true}}
	a := New(nil, reg, NewSession(""), Options{Gate: g, ToolTrace: traceStore}, event.Discard)
	a.sessionID = "sess-trace"

	// success path (read-only fake tool)
	a.executeOne(context.Background(), provider.ToolCall{ID: "c1", Name: "read_file", Arguments: `{"path":"a.go"}`})
	// execution error path (bash fake errors)
	a.executeOne(context.Background(), provider.ToolCall{ID: "c2", Name: "bash", Arguments: `{"command":"boom"}`})
	// blocked path (denied by gate)
	a.executeOne(context.Background(), provider.ToolCall{ID: "c3", Name: "write_file", Arguments: `{"path":"x"}`})
	// unknown tool path — lookup fails before the defer registers, so it must
	// NOT emit a trace line (no tool to describe); assert absence by counting.
	a.executeOne(context.Background(), provider.ToolCall{ID: "c4", Name: "no_such_tool", Arguments: `{}`})

	lines := readTraceLines(t, path)
	if len(lines) != 3 {
		t.Fatalf("got %d trace lines, want 3 (unknown tool emits none)", len(lines))
	}

	var success, failed, blocked tool.TraceEntry
	for i := range lines {
		var e tool.TraceEntry
		if err := json.Unmarshal([]byte(lines[i]), &e); err != nil {
			t.Fatalf("line %d not valid JSON: %v\n%s", i, err, lines[i])
		}
		switch e.CallID {
		case "c1":
			success = e
		case "c2":
			failed = e
		case "c3":
			blocked = e
		}
	}

	if success.Outcome != "success" || success.Tool != "read_file" || success.SessionID != "sess-trace" {
		t.Fatalf("success entry = %+v, want outcome=success/tool=read_file/session=sess-trace", success)
	}
	if failed.Outcome != "error" || failed.Tool != "bash" || !strings.Contains(failed.Error, "boom") {
		t.Fatalf("error entry = %+v, want outcome=error/tool=bash/error mentions boom", failed)
	}
	if blocked.Outcome != "blocked" || blocked.Tool != "write_file" {
		t.Fatalf("blocked entry = %+v, want outcome=blocked/tool=write_file", blocked)
	}
}

// TestExecuteOneTraceCarriesTraceID verifies the per-turn trace ID propagates
// into the trace record, correlating a dispatch with its API turn.
func TestExecuteOneTraceCarriesTraceID(t *testing.T) {
	dir := t.TempDir()
	traceStore, err := tool.NewTraceStore(filepath.Join(dir, "trace.jsonl"))
	if err != nil {
		t.Fatalf("NewTraceStore: %v", err)
	}
	defer traceStore.Close()

	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_file", readOnly: true})
	a := New(nil, reg, NewSession(""), Options{ToolTrace: traceStore}, event.Discard)

	ctx := WithTraceID(context.Background(), "trace-abc")
	a.executeOne(ctx, provider.ToolCall{ID: "c1", Name: "read_file", Arguments: `{"path":"a.go"}`})

	lines := readTraceLines(t, filepath.Join(dir, "trace.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	var e tool.TraceEntry
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.TraceID != "trace-abc" {
		t.Fatalf("TraceID = %q, want trace-abc", e.TraceID)
	}
}

func readTraceLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open trace: %v", err)
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
	return lines
}
