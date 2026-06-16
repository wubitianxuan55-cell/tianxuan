// Package archive provides JSONL-backed session archive storage for
// cross-session analysis (Dream/Distill). Messages are recorded after each
// turn — never in the hot path of prompt assembly, so cache stability is
// unaffected.
//
// V7.0: zero external dependencies, uses JSONL files under .tianxuan/archive/.
package archive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Record is a single message stored in the archive.
type Record struct {
	Role      string `json:"role"`
	Content   string `json:"content,omitempty"`
	ToolCalls string `json:"tool_calls,omitempty"` // JSON array string
	Turn      int    `json:"turn"`
	TimeUS    int64  `json:"time_us"`
}

// Store records session messages as JSONL files.
// Thread-safe; writes are best-effort.
type Store struct {
	mu  sync.Mutex
	dir string
}

// Open creates (or reuses) the archive directory.
// Pass "" to disable archiving.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// RecordMessage appends a message record to the session's JSONL file.
// Best-effort — errors are silently ignored.
func (s *Store) RecordMessage(sessionID, role, content, toolCallsJSON string, turn int) {
	if s == nil || sessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	rec := Record{
		Role:      role,
		Content:   truncate(content, 2000),
		ToolCalls: toolCallsJSON,
		Turn:      turn,
		TimeUS:    time.Now().UnixMicro(),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}

	path := filepath.Join(s.dir, sessionID+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(data)
	f.Write([]byte("\n"))
}

// QueryResult holds a match from the archive.
type QueryResult struct {
	SessionID string
	Role      string
	Content   string
	ToolCalls string
	Turn      int
}

// SearchMessages scans recent session files for keyword matches.
func (s *Store) SearchMessages(keywords []string, limit int) ([]QueryResult, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	// Sort by modification time (newest first)
	sort.Slice(entries, func(i, j int) bool {
		ii, _ := entries[i].Info()
		jj, _ := entries[j].Info()
		if ii == nil || jj == nil {
			return false
		}
		return ii.ModTime().After(jj.ModTime())
	})

	var results []QueryResult
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(e.Name(), ".jsonl")
		path := filepath.Join(s.dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		// Scan from end (most recent first)
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			var rec Record
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue
			}
			// Check keyword match
			matched := false
			lower := strings.ToLower(rec.Content)
			for _, kw := range keywords {
				if strings.Contains(lower, strings.ToLower(kw)) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			results = append(results, QueryResult{
				SessionID: sessionID,
				Role:      rec.Role,
				Content:   rec.Content,
				ToolCalls: rec.ToolCalls,
				Turn:      rec.Turn,
			})
			if len(results) >= limit {
				return results, nil
			}
		}
	}
	return results, nil
}

// SessionSummary holds aggregated stats for a session.
type SessionSummary struct {
	ID         string
	ToolCounts map[string]int
}

// ListRecentSessions returns session summaries from recent archive files.
func (s *Store) ListRecentSessions(limit int) ([]SessionSummary, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		ii, _ := entries[i].Info()
		jj, _ := entries[j].Info()
		if ii == nil || jj == nil {
			return false
		}
		return ii.ModTime().After(jj.ModTime())
	})

	var sessions []SessionSummary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(e.Name(), ".jsonl")
		ss := SessionSummary{
			ID:         sessionID,
			ToolCounts: make(map[string]int),
		}
		path := filepath.Join(s.dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var rec Record
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue
			}
			if rec.ToolCalls == "" || rec.ToolCalls == "[]" {
				continue
			}
			var calls []struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal([]byte(rec.ToolCalls), &calls); err != nil {
				continue
			}
			for _, c := range calls {
				ss.ToolCounts[c.Name]++
			}
		}
		sessions = append(sessions, ss)
		if len(sessions) >= limit {
			break
		}
	}
	return sessions, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
