package agent

import (
	"context"
	"strings"
	"testing"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// TestStaleWriteSoftWarns verifies a second write to the same file in one turn
// is no longer hard-blocked (V10.157): the write runs and a stale-content note
// is prepended, so the model can retry against the current disk content without
// a forced read_file round trip.
func TestStaleWriteSoftWarns(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "write_file", readOnly: false})
	a := New(nil, reg, NewSession(""), Options{}, event.Discard)
	a.staleWrittenFiles = map[string]bool{"D:/x.go": true}
	a.staleReadFiles = nil

	out := a.executeOne(context.Background(), provider.ToolCall{Name: "write_file", Arguments: `{"path":"D:/x.go"}`})
	if out.blocked {
		t.Fatalf("stale write should not be hard-blocked, got %+v", out)
	}
	if !strings.Contains(out.output, "stale content") {
		t.Fatalf("stale write should carry a warning, got %q", out.output)
	}
	if !strings.Contains(out.output, "write_file done") {
		t.Fatalf("stale write should still run, got %q", out.output)
	}
}

// TestStaleReadClearsWarning verifies a re-read in the same turn clears the
// stale warning entirely.
func TestStaleReadClearsWarning(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "write_file", readOnly: false})
	a := New(nil, reg, NewSession(""), Options{}, event.Discard)
	a.staleWrittenFiles = map[string]bool{"D:/x.go": true}
	a.staleReadFiles = map[string]bool{"D:/x.go": true}

	out := a.executeOne(context.Background(), provider.ToolCall{Name: "write_file", Arguments: `{"path":"D:/x.go"}`})
	if strings.Contains(out.output, "stale content") {
		t.Fatalf("re-read file should clear the warning, got %q", out.output)
	}
}
