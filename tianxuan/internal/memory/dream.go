package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// dreamMetaFile persists when the last consolidation ran and in which session,
// mirroring Qwen Code's dream metadata (lastDreamAt / lastDreamSessionId).
const dreamMetaFile = "dream-meta.json"

// defaultDreamMinHours / defaultDreamMinSessions match Qwen's scheduler.
const (
	defaultDreamMinHours    = 24
	defaultDreamMinSessions = 5
)

// DreamMetadata is the persisted auto-dream gate state for one store.
type DreamMetadata struct {
	LastDreamAt        time.Time `json:"lastDreamAt"`
	LastDreamSessionID string    `json:"lastDreamSessionId"`
}

// ReadDreamMetadata loads the persisted auto-dream gate state (zero value when
// none exists yet).
func ReadDreamMetadata(s Store) (DreamMetadata, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, dreamMetaFile))
	if err != nil {
		if os.IsNotExist(err) {
			return DreamMetadata{}, nil
		}
		return DreamMetadata{}, err
	}
	var m DreamMetadata
	if err := json.Unmarshal(b, &m); err != nil {
		return DreamMetadata{}, fmt.Errorf("parse %s: %w", dreamMetaFile, err)
	}
	return m, nil
}

// WriteDreamMetadata persists the auto-dream gate state.
func WriteDreamMetadata(s Store, m DreamMetadata) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir, dreamMetaFile), b, 0o644)
}

// DreamGateAllowed applies the scheduler gate: one dream per session, at most
// one per 24h, and only after at least 5 sessions have occurred since the last
// consolidation (Qwen's same_session / min_hours / min_sessions checks).
func DreamGateAllowed(s Store, meta DreamMetadata, sessionID string, newSessionCount int, now time.Time) (bool, string) {
	if meta.LastDreamSessionID == sessionID {
		return false, "same_session"
	}
	if !meta.LastDreamAt.IsZero() && now.Sub(meta.LastDreamAt) < defaultDreamMinHours*time.Hour {
		return false, "min_hours"
	}
	if newSessionCount < defaultDreamMinSessions {
		return false, "min_sessions"
	}
	return true, ""
}

// NewSessionFiles lists JSONL session files under dir whose mtime is strictly
// after since, excluding excludeID — the set of sessions to consolidate.
func NewSessionFiles(dir string, since time.Time, excludeID string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		if id == excludeID {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(since) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// readUserMessages extracts user message contents from a JSONL session file,
// skipping auto-injected control blocks. Best-effort: malformed lines are
// skipped.
func readUserMessages(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(line), &m); err != nil || m.Role != "user" {
			continue
		}
		if isTransientBlock(m.Content) {
			continue
		}
		if text := strings.TrimSpace(m.Content); text != "" {
			out = append(out, text)
		}
	}
	return out
}

// RunDream consolidates knowledge from sessions that finished since the last
// dream pass into one staged candidate (user confirmation required before it
// reaches active memory). It advances the dream metadata so the next gate
// check sees a fresh window. Returns the number of staged candidates.
func RunDream(s Store, sessionDir, sessionID string, now time.Time) (int, error) {
	if s.Dir == "" {
		return 0, nil
	}
	meta, err := ReadDreamMetadata(s)
	if err != nil {
		return 0, err
	}
	files := NewSessionFiles(sessionDir, meta.LastDreamAt, sessionID)
	if len(files) == 0 {
		return 0, nil
	}

	// Collect the first user request of each new session as the recurring
	// theme; deterministic, no LLM call.
	var themes []string
	for _, f := range files {
		msgs := readUserMessages(f)
		if len(msgs) == 0 {
			continue
		}
		themes = append(themes, truncateRunes(oneLine(msgs[0]), 80))
	}
	if len(themes) == 0 {
		// No scannable content: still advance the cursor so an empty dream
		// does not block on the same session set forever.
		return 0, WriteDreamMetadata(s, DreamMetadata{LastDreamAt: now, LastDreamSessionID: sessionID})
	}

	date := now.Format("2006-01-02")
	name := "dream-" + date
	if _, err := os.Stat(pendingPath(s, name)); err == nil {
		name = "dream-" + now.Format("20060102-150405")
	}
	desc := fmt.Sprintf("最近 %d 个会话的反复主题", len(themes))
	body := "## Recurring themes across sessions\n\n"
	for i, t := range themes {
		body += fmt.Sprintf("%d. %s\n", i+1, t)
	}
	cand := Candidate{
		Name:        name,
		Title:       "Dream 整合 (" + date + ")",
		Description: desc,
		Type:        TypeProject,
		Body:        body,
		Reason:      "auto-dream consolidation",
		Evidence:    strings.Join(themes, " | "),
	}
	if err := os.MkdirAll(filepath.Join(s.Dir, pendingDir), 0o755); err != nil {
		return 0, err
	}
	b, err := json.Marshal(cand)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(pendingPath(s, cand.Name), b, 0o644); err != nil {
		return 0, err
	}
	if err := WriteDreamMetadata(s, DreamMetadata{LastDreamAt: now, LastDreamSessionID: sessionID}); err != nil {
		return 0, err
	}
	return 1, nil
}
