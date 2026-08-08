package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// TraceEntry is one structured record of a tool dispatch (distilled from
// codex CLI's ToolDispatchTrace): who called which tool with what arguments,
// how it ended, and how long it took. Written as one JSON line per call so
// error-rate dashboards and regression analysis can stream the file.
type TraceEntry struct {
	Ts         string `json:"ts"`
	SessionID  string `json:"session_id,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
	CallID     string `json:"call_id,omitempty"`
	Tool       string `json:"tool"`
	ReadOnly   bool   `json:"read_only"`
	Args       string `json:"args,omitempty"`
	Outcome    string `json:"outcome"` // success | error | blocked
	Error      string `json:"error,omitempty"`
	OutputLen  int    `json:"output_len"`
	DurationMs int64  `json:"duration_ms"`
}

// Cap sizes so one pathological call (huge command, long error chain) cannot
// bloat the trace file; the model-facing output is already truncated upstream.
const (
	traceArgMax = 500
	traceErrMax = 300
)

// TraceStore appends one TraceEntry per tool dispatch as JSONL. It is
// thread-safe; each Record opens the file with O_APPEND, writes one line, and
// closes it. Lazy open/close (rather than a held handle) means the store never
// pins the file — hosts and tests can delete the trace file at any time, and
// a long-lived executor leaks no descriptor.
type TraceStore struct {
	mu   sync.Mutex
	path string
}

// NewTraceStore prepares a TraceStore writing to path. An empty path disables
// persistence (memory no-op), matching tool.Stats. The file itself is created
// lazily on the first Record.
func NewTraceStore(path string) (*TraceStore, error) {
	if path == "" {
		return &TraceStore{}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &TraceStore{path: path}, nil
}

// DefaultTracePath returns the canonical cross-session tool-trace file for a
// workspace root, next to tool-stats.json.
func DefaultTracePath(cwd string) string {
	return filepath.Join(cwd, ".tianxuan", "tool-trace.jsonl")
}

// Record appends one trace entry as a JSON line. A nil receiver is a no-op
// (the agent wires the store in optionally). Cap violations are truncated to
// keep the file bounded. Each write is its own open/append/close so the file
// is never pinned and concurrent dispatches never interleave lines.
func (s *TraceStore) Record(e TraceEntry) {
	if s == nil || s.path == "" {
		return
	}
	if len(e.Args) > traceArgMax {
		e.Args = e.Args[:traceArgMax]
	}
	if len(e.Error) > traceErrMax {
		e.Error = e.Error[:traceErrMax]
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(b, '\n'))
}

// Close is kept for interface parity with tool.Stats-style lifecycle; with
// lazy open/close per Record there is nothing to release, so it is a no-op.
func (s *TraceStore) Close() {}
