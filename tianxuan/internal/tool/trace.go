package tool

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// ToolTraceStat aggregates one tool's dispatch outcomes across a trace file
// (V10.171, distilled from codex's ToolDispatchTrace consumption model): the
// per-tool error rate is the metric that decides where the next optimization
// pass should invest.
type ToolTraceStat struct {
	Tool      string   `json:"tool"`
	Calls     int      `json:"calls"`
	Success   int      `json:"success"`
	Errors    int      `json:"errors"`
	Blocked   int      `json:"blocked"`
	ErrorRate float64  `json:"error_rate"` // (errors+blocked)/calls
	AvgMs     int64    `json:"avg_ms"`
	TotalMs   int64    `json:"total_ms"`
	TopErrors []string `json:"top_errors,omitempty"`
}

const topErrorsMax = 3

// traceAgg is the mutable per-tool accumulator behind SummarizeTrace.
type traceAgg struct {
	tool      string
	calls     int
	success   int
	errors    int
	blocked   int
	totalMs   int64
	errCounts map[string]int
}

// SummarizeTrace streams a JSONL trace file and aggregates per-tool dispatch
// outcomes. Corrupt lines are skipped: a trace file is append-only diagnostic
// data, so one bad line must not hide the rest. Tools are ordered by error
// count (descending), then call count (descending), then name.
func SummarizeTrace(path string) ([]ToolTraceStat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	aggs := map[string]*traceAgg{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e TraceEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		a, ok := aggs[e.Tool]
		if !ok {
			a = &traceAgg{tool: e.Tool, errCounts: map[string]int{}}
			aggs[e.Tool] = a
		}
		a.calls++
		a.totalMs += e.DurationMs
		switch e.Outcome {
		case "error":
			a.errors++
			if e.Error != "" {
				a.errCounts[e.Error]++
			}
		case "blocked":
			a.blocked++
		default:
			a.success++
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	out := make([]ToolTraceStat, 0, len(aggs))
	for _, a := range aggs {
		rate := 0.0
		if a.calls > 0 {
			rate = float64(a.errors+a.blocked) / float64(a.calls)
		}
		out = append(out, ToolTraceStat{
			Tool:      a.tool,
			Calls:     a.calls,
			Success:   a.success,
			Errors:    a.errors,
			Blocked:   a.blocked,
			ErrorRate: rate,
			AvgMs:     a.totalMs / int64(a.calls),
			TotalMs:   a.totalMs,
			TopErrors: topErrors(a.errCounts),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Errors != out[j].Errors {
			return out[i].Errors > out[j].Errors
		}
		if out[i].Calls != out[j].Calls {
			return out[i].Calls > out[j].Calls
		}
		return out[i].Tool < out[j].Tool
	})
	return out, nil
}

// topErrors returns the most frequent error texts, capped at topErrorsMax;
// ties break by text so the output is deterministic.
func topErrors(counts map[string]int) []string {
	if len(counts) == 0 {
		return nil
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > topErrorsMax {
		keys = keys[:topErrorsMax]
	}
	return keys
}

// TraceReportTable renders the per-tool error-rate table for the CLI.
func TraceReportTable(stats []ToolTraceStat) string {
	var b strings.Builder
	b.WriteString("tool                    calls  success  error  blocked  error%  avg_ms  top_error\n")
	for _, s := range stats {
		top := ""
		if len(s.TopErrors) > 0 {
			top = s.TopErrors[0]
		}
		fmt.Fprintf(&b, "%-22s %6d %8d %6d %8d %6.1f%% %7d  %s\n",
			s.Tool, s.Calls, s.Success, s.Errors, s.Blocked, s.ErrorRate*100, s.AvgMs, top)
	}
	return b.String()
}
