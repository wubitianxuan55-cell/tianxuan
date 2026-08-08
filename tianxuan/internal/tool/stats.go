package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrorStat aggregates one tool's failure mode observed across executions.
// Distilled from codex CLI's ToolDispatchTrace: before you can lower a tool
// error rate, you must be able to measure which tool and which failure mode
// dominate.
type ErrorStat struct {
	Tool      string `json:"tool"`
	ErrorKind string `json:"error_kind"`
	Count     int    `json:"count"`
	LastSeen  string `json:"last_seen"`
	LastError string `json:"last_error,omitempty"`
}

// Stats tracks per-tool error counts in memory and persists them as JSON so
// cross-session error rates can be queried (CLI `tools stats`). Writes are
// atomic (temp file + rename) so a crash never corrupts the aggregate.
type Stats struct {
	mu      sync.Mutex
	path    string
	entries map[string]*ErrorStat // key: tool + "\x00" + errorKind
}

// NewStats returns a Stats that loads any existing file at path and persists
// every Record to it. An empty path disables persistence (memory only).
func NewStats(path string) *Stats {
	s := &Stats{path: path, entries: map[string]*ErrorStat{}}
	if path != "" {
		s.load()
	}
	return s
}

// NewMemStats returns a Stats with persistence disabled (used by tests and
// hosts that only want the live aggregate).
func NewMemStats() *Stats {
	return NewStats("")
}

// DefaultStatsPath returns the canonical cross-session stats file for a
// workspace root.
func DefaultStatsPath(cwd string) string {
	return filepath.Join(cwd, ".tianxuan", "tool-stats.json")
}

// Record counts one tool failure of the given kind.
func (s *Stats) Record(toolName, kind, errMsg string) {
	if s == nil {
		return
	}
	key := toolName + "\x00" + kind
	now := time.Now().Format(time.RFC3339)
	s.mu.Lock()
	e, ok := s.entries[key]
	if !ok {
		e = &ErrorStat{Tool: toolName, ErrorKind: kind}
		s.entries[key] = e
	}
	e.Count++
	e.LastSeen = now
	if errMsg != "" {
		e.LastError = truncateErr(errMsg)
	}
	s.mu.Unlock()
	s.save()
}

// Snapshot returns the aggregated entries sorted by count (descending), then
// tool name.
func (s *Stats) Snapshot() []ErrorStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ErrorStat, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Tool != out[j].Tool {
			return out[i].Tool < out[j].Tool
		}
		return out[i].ErrorKind < out[j].ErrorKind
	})
	return out
}

// Report renders a human-readable table for the CLI.
func (s *Stats) Report() string {
	var b strings.Builder
	b.WriteString("tool                    error_kind                    count  last_seen\n")
	for _, e := range s.Snapshot() {
		fmt.Fprintf(&b, "%-22s %-30s %5d  %s\n", e.Tool, e.ErrorKind, e.Count, e.LastSeen)
	}
	return b.String()
}

func (s *Stats) load() {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var persisted []ErrorStat
	if err := json.Unmarshal(raw, &persisted); err != nil {
		return
	}
	for _, e := range persisted {
		key := e.Tool + "\x00" + e.ErrorKind
		s.entries[key] = &ErrorStat{
			Tool:      e.Tool,
			ErrorKind: e.ErrorKind,
			Count:     e.Count,
			LastSeen:  e.LastSeen,
			LastError: e.LastError,
		}
	}
}

func (s *Stats) save() {
	if s.path == "" {
		return
	}
	s.mu.Lock()
	entries := make([]ErrorStat, 0, len(s.entries))
	for _, e := range s.entries {
		entries = append(entries, *e)
	}
	s.mu.Unlock()

	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return
	}
	if dir := filepath.Dir(s.path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
}

// ClassifyError maps a tool failure to a normalized error kind so stats can be
// aggregated across sessions. It mirrors the failure classes already taught by
// internal/learning/patterns.go.
func ClassifyError(toolName, code, errMsg string) string {
	switch code {
	case CodeValidationError:
		return "validation_error"
	case CodeTimeout:
		return "timeout"
	case CodeDenied:
		return "denied"
	case CodeNotFound:
		return "not_found"
	case CodeBlocked:
		return "blocked"
	case CodeUnknownTool:
		return "unknown_tool"
	}
	msg := strings.ToLower(errMsg)
	switch toolName {
	case "edit_file", "multi_edit":
		if strings.Contains(msg, "not found") {
			return "old_string_not_found"
		}
		if strings.Contains(msg, "not unique") {
			return "old_string_not_unique"
		}
	case "delete_range":
		if strings.Contains(msg, "not found") {
			return "anchor_not_found"
		}
	case "delete_symbol":
		if strings.Contains(msg, "not found") {
			return "symbol_not_found"
		}
	case "apply_patch":
		if strings.Contains(msg, "invalid patch") {
			return "patch_parse_error"
		}
		if strings.Contains(msg, "not unique") {
			return "block_not_unique"
		}
		if strings.Contains(msg, "not found") {
			return "block_not_found"
		}
	case "bash":
		if strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out") {
			return "bash_timeout"
		}
		if strings.Contains(msg, "command not found") || strings.Contains(msg, "not recognized") {
			return "command_not_found"
		}
	}
	return "exec_error"
}

func truncateErr(msg string) string {
	const max = 160
	if len(msg) <= max {
		return msg
	}
	return msg[:max] + "..."
}
