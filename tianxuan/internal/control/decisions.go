package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DecisionLogger records key routing and adaptation decisions for post-hoc
// debugging. Each entry is a self-contained JSON object appended to a JSONL
// file, following the same pattern as AuditLogger.
//
// V3.3: complements AuditLogger (tool-level) with decision-level traceability.
type DecisionLogger struct {
	mu   sync.Mutex
	file *os.File
}

// DecisionEntry is one row in the decision log.
type DecisionEntry struct {
	Timestamp  string `json:"timestamp"`
	TraceID    string `json:"trace_id,omitempty"`
	Decision   string `json:"decision"`    // e.g. "route", "promote", "demote", "lock", "compact", "storm_break"
	Kind       string `json:"kind"`         // task kind, e.g. "fix_bug"
	Version    int    `json:"version"`      // profile version at decision time
	Detail     string `json:"detail"`       // human-readable context
	AutoPlanScore float64 `json:"auto_plan_score,omitempty"` // classifier confidence
}

// NewDecisionLogger opens or creates the decisions JSONL file.
func NewDecisionLogger(dir string) (*DecisionLogger, error) {
	if dir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "decisions.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &DecisionLogger{file: f}, nil
}

// Log appends a decision entry. Thread-safe, best-effort (errors are silent).
func (l *DecisionLogger) Log(entry DecisionEntry) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().Format(time.RFC3339)
	}
	data, _ := json.Marshal(entry)
	data = append(data, '\n')
	l.file.Write(data)
}

// Close flushes and closes the decision log.
func (l *DecisionLogger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}
