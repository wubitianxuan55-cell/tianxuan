package control

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AuditLogger records tool call events to a JSONL file for post-hoc analysis.
// Each line is a self-contained JSON object so the file is append-only and
// crash-safe (no partial writes corrupt earlier entries).
//
// V3.2: foundational audit trail — what was called, when, by which kind of
// task, and what the outcome was. Does NOT yet support playback/replay.
type AuditLogger struct {
	mu   sync.Mutex
	file *os.File
	path string
}

// AuditEntry is one row in the audit log.
type AuditEntry struct {
	Timestamp  string `json:"timestamp"`   // ISO-8601
	Tool       string `json:"tool"`        // tool name, e.g. "edit_file"
	TaskKind   string `json:"task_kind"`   // classifyIntent result, e.g. "fix_bug"
	ReadOnly   bool   `json:"read_only"`   // was this a read-only tool?
	Outcome    string `json:"outcome"`      // "success" | "error" | "blocked"
	Error      string `json:"error,omitempty"` // error message, if any
	OutputLen  int    `json:"output_len"`    // length of tool output in chars
	DurationMs int64  `json:"duration_ms"`  // wall-clock execution time
}

// NewAuditLogger opens or creates the JSONL audit file under the given directory.
// The directory is created if it doesn't exist. Returns nil when dir is empty
// (audit disabled). Errors on file creation are returned but non-fatal — the
// caller may ignore them and continue without audit.
func NewAuditLogger(dir string) (*AuditLogger, error) {
	if dir == "" {
		return nil, nil // audit disabled
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("audit: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "audit.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}
	return &AuditLogger{file: f, path: path}, nil
}

// Log appends one audit entry to the JSONL file. Thread-safe. Errors are
// silently ignored — audit is best-effort and must never block the agent.
func (l *AuditLogger) Log(entry AuditEntry) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')
	l.file.Write(data)
}

// Close flushes and closes the audit file.
func (l *AuditLogger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// Path returns the full path to the audit JSONL file.
func (l *AuditLogger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// LogToolCall is a convenience wrapper that creates an AuditEntry and logs it.
// taskKind may be empty (will be logged as "unknown").
func (l *AuditLogger) LogToolCall(tool string, taskKind string, readOnly bool, outcome string, errMsg string, outputLen int, durationMs int64) {
	l.Log(AuditEntry{
		Timestamp:  time.Now().Format(time.RFC3339),
		Tool:       tool,
		TaskKind:   taskKind,
		ReadOnly:   readOnly,
		Outcome:    outcome,
		Error:      errMsg,
		OutputLen:  outputLen,
		DurationMs: durationMs,
	})
}

// --- V3.4: audit replay / session summary ---

// AuditSummary aggregates statistics from the audit log.
type AuditSummary struct {
	TotalCalls    int
	SuccessCount  int
	ErrorCount    int
	BlockedCount  int
	TotalDuration int64 // milliseconds
	TopTools      map[string]int
	ByKind        map[string]int
}

// Summarize reads the audit JSONL file and returns aggregate statistics.
// Returns nil if the file doesn't exist or can't be read.
func SummarizeAuditLog(path string) (*AuditSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := &AuditSummary{
		TopTools: make(map[string]int),
		ByKind:   make(map[string]int),
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry AuditEntry
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		s.TotalCalls++
		s.TotalDuration += entry.DurationMs
		s.TopTools[entry.Tool]++
		s.ByKind[entry.TaskKind]++

		switch entry.Outcome {
		case "success":
			s.SuccessCount++
		case "error":
			s.ErrorCount++
		case "blocked":
			s.BlockedCount++
		}
	}
	return s, nil
}

// Format returns a human-readable summary string.
func (s *AuditSummary) Format() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Audit: %d calls (%d ok, %d err, %d blocked) | %dms total",
		s.TotalCalls, s.SuccessCount, s.ErrorCount, s.BlockedCount, s.TotalDuration)
	return b.String()
}
