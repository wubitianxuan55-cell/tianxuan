package control

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tianxuan/internal/agent"
	"tianxuan/internal/event"
	"tianxuan/internal/memory"
	"tianxuan/internal/provider"
)

// newExtractController builds a Controller whose session carries the given
// messages and whose auto-memory store lives under dir.
func newExtractController(t *testing.T, dir string, msgs []provider.Message) *Controller {
	t.Helper()
	sess := agent.NewSession("sys")
	for _, m := range msgs {
		sess.Add(m)
	}
	exec := agent.New(nil, nil, sess, agent.Options{}, event.Discard)
	userDir := filepath.Join(dir, "cfg")
	store := memory.StoreFor(userDir, dir)
	ctrl := New(Options{
		Executor:   exec,
		Memory:     &memory.Set{Store: store, CWD: dir, UserDir: userDir},
		SessionDir: filepath.Join(dir, "sessions"),
		Label:      "test",
	})
	ctrl.SetSessionPath(filepath.Join(dir, "sessions", "sess-1.jsonl"))
	return ctrl
}

// TestAutoExtractStagesPendingAfterTurn verifies a finished turn's durable
// statements land in pending — not active memory — and the confirmation path
// writes them into the store.
func TestAutoExtractStagesPendingAfterTurn(t *testing.T) {
	dir := t.TempDir()
	ctrl := newExtractController(t, dir, []provider.Message{
		{Role: provider.RoleUser, Content: "请记住：本项目统一使用 tabs 缩进"},
	})

	ctrl.autoExtract()
	pending := ctrl.PendingMemories()
	if len(pending) != 1 {
		t.Fatalf("want 1 staged candidate, got %d: %+v", len(pending), pending)
	}
	if got := ctrl.Memory().Store.List(); len(got) != 0 {
		t.Fatalf("auto-extract must not write active memory before confirmation: %+v", got)
	}

	if _, err := ctrl.AcceptPendingMemory(pending[0].Name); err != nil {
		t.Fatal(err)
	}
	if got := ctrl.Memory().Store.List(); len(got) != 1 {
		t.Fatalf("confirmed memory missing from store: %+v", got)
	}
	if len(ctrl.PendingMemories()) != 0 {
		t.Fatal("confirmed candidate must leave pending")
	}
}

// TestAutoExtractSkipsRepeatRuns verifies the cursor prevents re-staging the
// same messages on a second pass (Qwen extract-cursor semantics).
func TestAutoExtractSkipsRepeatRuns(t *testing.T) {
	dir := t.TempDir()
	ctrl := newExtractController(t, dir, []provider.Message{
		{Role: provider.RoleUser, Content: "以后统一用 tabs 缩进"},
	})
	ctrl.autoExtract()
	ctrl.autoExtract()
	if len(ctrl.PendingMemories()) != 1 {
		t.Fatalf("second pass must not duplicate candidates, got %d", len(ctrl.PendingMemories()))
	}
}

// TestRejectPendingMemory verifies declining a candidate leaves the store
// untouched.
func TestRejectPendingMemory(t *testing.T) {
	dir := t.TempDir()
	ctrl := newExtractController(t, dir, []provider.Message{
		{Role: provider.RoleUser, Content: "始终先写测试再动手改代码"},
	})
	ctrl.autoExtract()
	pending := ctrl.PendingMemories()
	if len(pending) != 1 {
		t.Fatalf("want 1 staged candidate, got %d", len(pending))
	}
	if err := ctrl.RejectPendingMemory(pending[0].Name); err != nil {
		t.Fatal(err)
	}
	if len(ctrl.PendingMemories()) != 0 {
		t.Fatal("rejected candidate must leave pending")
	}
	if got := ctrl.Memory().Store.List(); len(got) != 0 {
		t.Fatalf("reject must not write active memory: %+v", got)
	}
}

// TestAutoExtractSkipsTransientBlocks verifies control blocks injected by the
// controller itself never become candidates.
func TestAutoExtractSkipsTransientBlocks(t *testing.T) {
	dir := t.TempDir()
	ctrl := newExtractController(t, dir, []provider.Message{
		{Role: provider.RoleUser, Content: "<memory-update>\nA change\n</memory-update>"},
		{Role: provider.RoleUser, Content: "[auto-recall] 相关记忆自动检索结果:\n- foo: bar"},
		{Role: provider.RoleUser, Content: "记住：构建时始终使用 --race 标志"},
	})
	ctrl.autoExtract()
	pending := ctrl.PendingMemories()
	if len(pending) != 1 {
		t.Fatalf("want 1 candidate (only the real request), got %d", len(pending))
	}
	if !strings.Contains(pending[0].Description, "--race") {
		t.Fatalf("wrong candidate survived: %+v", pending)
	}
}

// TestAutoDreamStagesCandidateAfterGate verifies the scheduler path: with
// enough new sessions and no recent dream, one consolidated candidate is
// staged; a second pass in the same session stages nothing new.
func TestAutoDreamStagesCandidateAfterGate(t *testing.T) {
	dir := t.TempDir()
	ctrl := newExtractController(t, dir, []provider.Message{
		{Role: provider.RoleUser, Content: "怎么修复这个构建？"},
	})
	sessionDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 6; i++ {
		writeCtrlSession(t, sessionDir, "sess-"+string(rune('0'+i)), "重构用户认证模块")
	}

	ctrl.autoDream()
	pending := ctrl.PendingMemories()
	if len(pending) != 1 {
		t.Fatalf("want 1 dream candidate, got %d: %+v", len(pending), pending)
	}
	if !strings.Contains(pending[0].Description, "5") {
		t.Fatalf("dream candidate must mention the consolidated session count: %+v", pending[0])
	}
	// Same session: gate blocks a second run.
	ctrl.autoDream()
	if len(ctrl.PendingMemories()) != 1 {
		t.Fatal("same-session dream must not stage a second candidate")
	}
}

// writeCtrlSession writes a JSONL session file with one user message.
func writeCtrlSession(t *testing.T, dir, id, userMsg string) {
	t.Helper()
	content := `{"role":"user","content":"` + userMsg + `"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestAutoDreamGateBlocksWithinWindow verifies a recent dream suppresses the
// scheduler even when enough sessions exist.
func TestAutoDreamGateBlocksWithinWindow(t *testing.T) {
	dir := t.TempDir()
	ctrl := newExtractController(t, dir, []provider.Message{
		{Role: provider.RoleUser, Content: "继续"},
	})
	// Seed metadata with a dream 2 hours ago.
	meta := memory.DreamMetadata{LastDreamAt: time.Now().Add(-2 * time.Hour)}
	if err := memory.WriteDreamMetadata(ctrl.Memory().Store, meta); err != nil {
		t.Fatal(err)
	}
	sessionDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 6; i++ {
		writeCtrlSession(t, sessionDir, "sess-"+string(rune('0'+i)), "继续工作")
	}
	ctrl.autoDream()
	if len(ctrl.PendingMemories()) != 0 {
		t.Fatal("dream must be suppressed within the 24h window")
	}
}

// TestAutoDreamMaintainsProfileAndEvictsStale verifies the dream pass also
// rebuilds the project profile and archives cold, weakly-reinforced memories.
func TestAutoDreamMaintainsProfileAndEvictsStale(t *testing.T) {
	dir := t.TempDir()
	ctrl := newExtractController(t, dir, []provider.Message{
		{Role: provider.RoleUser, Content: "继续"},
	})
	store := ctrl.Memory().Store
	if _, err := store.Save(memory.Memory{Name: "cold-rule", Description: "d", Type: memory.TypeProject, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := memory.WriteStrength(store, "cold-rule", memory.Strength{Count: 1, LastAccessAt: time.Now().Add(-120 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	sessionDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 6; i++ {
		writeCtrlSession(t, sessionDir, "sess-"+string(rune('0'+i)), "重构用户认证模块")
	}

	ctrl.autoDream()
	// The cold memory must have been archived by the maintenance pass.
	if got := store.List(); len(got) != 0 {
		t.Fatalf("stale memory should be archived by dream maintenance: %+v", got)
	}
	// Profile file must exist.
	profilePath := filepath.Join(store.Dir, "profile.json")
	if _, err := os.Stat(profilePath); err != nil {
		t.Fatalf("dream pass must rebuild the project profile: %v", err)
	}
}
