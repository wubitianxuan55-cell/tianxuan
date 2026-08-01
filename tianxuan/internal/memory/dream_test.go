package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeSessionFile writes a minimal JSONL session file with the given user
// messages so dream scanning has history to read.
func writeSessionFile(t *testing.T, dir, id string, userMessages ...string) string {
	t.Helper()
	path := filepath.Join(dir, id+".jsonl")
	var b strings.Builder
	for _, u := range userMessages {
		b.WriteString(`{"role":"user","content":"` + u + `"}` + "\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestDreamGateBlocksWithinWindow verifies the 24h / 5-session gate skips when
// the previous dream is too recent.
func TestDreamGateBlocksWithinWindow(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	meta := DreamMetadata{LastDreamAt: time.Now().Add(-2 * time.Hour)}
	allowed, reason := DreamGateAllowed(s, meta, "sess-9", 10, time.Now())
	if allowed {
		t.Fatalf("dream should be blocked within 24h window, reason=%q", reason)
	}
}

// TestDreamGateRequiresFiveSessions verifies the gate skips when fewer than
// five sessions have occurred since the last dream.
func TestDreamGateRequiresFiveSessions(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	meta := DreamMetadata{LastDreamAt: time.Now().Add(-48 * time.Hour)}
	allowed, reason := DreamGateAllowed(s, meta, "sess-1", 3, time.Now())
	if allowed {
		t.Fatalf("dream should require >=5 sessions, reason=%q", reason)
	}
}

// TestDreamGateAllowsAfterThreshold verifies the gate opens once both the time
// and session-count thresholds are met.
func TestDreamGateAllowsAfterThreshold(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	meta := DreamMetadata{LastDreamAt: time.Now().Add(-48 * time.Hour)}
	allowed, reason := DreamGateAllowed(s, meta, "sess-1", 6, time.Now())
	if !allowed {
		t.Fatalf("dream should be allowed past thresholds, reason=%q", reason)
	}
}

// TestDreamGateBlocksSameSession verifies one dream per session (dedupe).
func TestDreamGateBlocksSameSession(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	meta := DreamMetadata{LastDreamAt: time.Now().Add(-48 * time.Hour), LastDreamSessionID: "sess-1"}
	allowed, _ := DreamGateAllowed(s, meta, "sess-1", 6, time.Now())
	if allowed {
		t.Fatal("dream must not run twice in the same session")
	}
}

// TestRunDreamStagesConsolidatedCandidate verifies a dream pass scans new
// sessions and stages one consolidated candidate for user confirmation.
func TestRunDreamStagesConsolidatedCandidate(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}
	sessionDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		writeSessionFile(t, sessionDir, "sess-"+string(rune('0'+i)), "修复数据库连接超时问题")
	}

	n, err := RunDream(s, sessionDir, "sess-9", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("dream should stage 1 consolidated candidate, got %d", n)
	}
	pending := PendingCandidates(s)
	if len(pending) != 1 {
		t.Fatalf("want 1 pending candidate, got %d", len(pending))
	}
	if pending[0].Type != TypeProject {
		t.Fatalf("dream candidate should be project, got %q", pending[0].Type)
	}
	if !strings.Contains(pending[0].Description, "5") {
		t.Fatalf("dream candidate must mention session count: %+v", pending[0])
	}
}

// TestRunDreamNoNewSessions verifies a dream pass with no sessions since the
// last dream stages nothing.
func TestRunDreamNoNewSessions(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}
	sessionDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := DreamMetadata{LastDreamAt: time.Now()}
	if err := WriteDreamMetadata(s, meta); err != nil {
		t.Fatal(err)
	}
	n, err := RunDream(s, sessionDir, "sess-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("no new sessions should stage nothing, got %d", n)
	}
}

// TestRunDreamAdvancesMetadata verifies the dream run records lastDreamAt and
// the session id so the next gate check sees a fresh window.
func TestRunDreamAdvancesMetadata(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}
	sessionDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		writeSessionFile(t, sessionDir, "sess-"+string(rune('0'+i)), "重构用户认证模块")
	}
	if _, err := RunDream(s, sessionDir, "sess-9", time.Now()); err != nil {
		t.Fatal(err)
	}
	meta, err := ReadDreamMetadata(s)
	if err != nil {
		t.Fatal(err)
	}
	if meta.LastDreamSessionID != "sess-9" {
		t.Fatalf("metadata session id = %q, want sess-9", meta.LastDreamSessionID)
	}
	if meta.LastDreamAt.IsZero() {
		t.Fatal("lastDreamAt must be set")
	}
}
