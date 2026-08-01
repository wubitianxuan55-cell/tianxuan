package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tianxuan/internal/provider"
)

func msgsUser(content ...string) []provider.Message {
	var out []provider.Message
	for _, c := range content {
		out = append(out, provider.Message{Role: provider.RoleUser, Content: c})
	}
	return out
}

func msgAssistant(content string) provider.Message {
	return provider.Message{Role: provider.RoleAssistant, Content: content}
}

// TestExtractWritesPendingNotStore verifies auto-extract stages candidates in
// the pending directory and never writes active memory before user confirmation.
func TestExtractWritesPendingNotStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	s := Store{Dir: dir}
	msgs := msgsUser("请记住：这个项目使用 tabs 缩进", "普通问题，无需记忆")

	n, err := ExtractCandidates(s, "sess-1", msgs)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want 1 candidate, got %d", n)
	}
	// Not in the active store yet.
	if len(s.List()) != 0 {
		t.Fatalf("auto-extract must not write active memory: %+v", s.List())
	}
	// Staged under pending/.
	pending := PendingCandidates(s)
	if len(pending) != 1 {
		t.Fatalf("want 1 pending candidate, got %d: %+v", len(pending), pending)
	}
	if pending[0].Description == "" {
		t.Fatal("candidate must carry a description")
	}
}

// TestExtractCursorAdvance verifies the processed-offset cursor prevents
// re-extracting the same messages within one session, and resets on a new
// session id (Qwen extract-cursor semantics).
func TestExtractCursorAdvance(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	s := Store{Dir: dir}

	first := msgsUser("以后统一用 tabs 缩进", "普通问题")
	if n, err := ExtractCandidates(s, "sess-1", first); err != nil || n != 1 {
		t.Fatalf("first extract: n=%d err=%v", n, err)
	}
	// Same session, same history: nothing new to extract.
	if n, err := ExtractCandidates(s, "sess-1", first); err != nil || n != 0 {
		t.Fatalf("same-session repeat must extract nothing: n=%d err=%v", n, err)
	}
	// Same session, appended history: only the new message is scanned.
	appended := append(append([]provider.Message(nil), first...), msgsUser("始终在提交前运行测试")...)
	if n, err := ExtractCandidates(s, "sess-1", appended); err != nil || n != 1 {
		t.Fatalf("appended-session extract: n=%d err=%v", n, err)
	}
	// New session id restarts scanning (history differs after resume). A new
	// candidate message is extracted even though earlier messages were already
	// staged for the previous session.
	fresh := msgsUser("以后统一用 tabs 缩进", "普通问题", "偏好：优先使用中文注释")
	if n, err := ExtractCandidates(s, "sess-2", fresh); err != nil || n != 1 {
		t.Fatalf("new-session extract: n=%d err=%v", n, err)
	}
	// The already-staged candidate is not duplicated into pending by the new
	// session — dedup against pending prevents stacked duplicates.
	if n, err := ExtractCandidates(s, "sess-2", fresh); err != nil || n != 0 {
		t.Fatalf("new-session repeat must extract nothing: n=%d err=%v", n, err)
	}
}

// TestExtractSkipsTransientBlocks verifies auto-injected control blocks
// (<memory-update>, [auto-recall], [system]) never become memory candidates.
func TestExtractSkipsTransientBlocks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	s := Store{Dir: dir}
	msgs := msgsUser(
		"<memory-update>\nSome memory changed\n</memory-update>",
		"[auto-recall] 相关记忆自动检索结果:\n- foo: bar",
		"[system] This session has saved memory",
		"记住：项目使用 Go 1.26",
	)
	n, err := ExtractCandidates(s, "sess-1", msgs)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want 1 candidate (only the real request), got %d", n)
	}
	if got := PendingCandidates(s); len(got) != 1 || !strings.Contains(got[0].Description, "Go 1.26") {
		t.Fatalf("wrong candidate survived: %+v", got)
	}
}

// TestExtractScansAssistantSummaries verifies assistant messages are scanned
// too, so validated decisions and conclusions can be staged for confirmation.
func TestExtractScansAssistantSummaries(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	s := Store{Dir: dir}
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "帮我看下构建脚本"},
		msgAssistant("记住：该项目的发布流程始终通过 CI 触发，不要本地手动构建"),
	}
	n, err := ExtractCandidates(s, "sess-1", msgs)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want 1 candidate from assistant summary, got %d", n)
	}
}

// TestAcceptPendingSavesToStore verifies confirming a staged candidate writes
// it into active memory and removes it from pending.
func TestAcceptPendingSavesToStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	s := Store{Dir: dir}
	if n, err := ExtractCandidates(s, "sess-1", msgsUser("偏好：优先使用中文注释")); err != nil || n != 1 {
		t.Fatalf("extract: n=%d err=%v", n, err)
	}
	pending := PendingCandidates(s)
	if len(pending) != 1 {
		t.Fatalf("want 1 pending, got %d", len(pending))
	}
	path, err := AcceptCandidate(s, pending[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("accepted memory missing on disk: %v", err)
	}
	if len(PendingCandidates(s)) != 0 {
		t.Fatal("accepted candidate must leave pending")
	}
	list := s.List()
	if len(list) != 1 || !strings.Contains(list[0].Description, "中文注释") {
		t.Fatalf("accepted memory not in active store: %+v", list)
	}
}

// TestRejectPendingRemovesCandidate verifies declining a candidate discards it
// without touching active memory.
func TestRejectPendingRemovesCandidate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	s := Store{Dir: dir}
	if n, err := ExtractCandidates(s, "sess-1", msgsUser("始终先写测试再动手改代码")); err != nil || n != 1 {
		t.Fatalf("extract: n=%d err=%v", n, err)
	}
	pending := PendingCandidates(s)
	if err := RejectCandidate(s, pending[0].Name); err != nil {
		t.Fatal(err)
	}
	if len(PendingCandidates(s)) != 0 {
		t.Fatal("rejected candidate must leave pending")
	}
	if len(s.List()) != 0 {
		t.Fatal("reject must not write active memory")
	}
}

// TestExtractDedupAgainstExisting verifies a candidate already covered by an
// active memory is not staged again.
func TestExtractDedupAgainstExisting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	s := Store{Dir: dir}
	if _, err := s.Save(Memory{Name: "tabs-rule", Title: "Prefers tabs", Description: "项目统一使用 tabs 缩进", Type: TypeProject, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	n, err := ExtractCandidates(s, "sess-1", msgsUser("记住：项目统一使用 tabs 缩进"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("already-covered candidate must be deduped, got %d", n)
	}
}

// TestExtractSkipsEmbeddedTransientBlocks verifies a control block embedded in
// the middle of a user message (e.g. the host appends <memory-update> after
// the user's own text) never leaks into memory candidates. The old check only
// matched message *prefixes*, so "默认是不是中文 <memory-update> ..." slipped
// through and staged the control block as a candidate.
func TestExtractSkipsEmbeddedTransientBlocks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	s := Store{Dir: dir}
	msgs := msgsUser(
		"默认是不是中文 <memory-update> The following project-memory changes were just made and apply from now on: - Saved memory \"codex\": Codex 中文化部署 </memory-update>",
		"以后统一用 tabs 缩进",
	)
	n, err := ExtractCandidates(s, "sess-1", msgs)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want 1 candidate (only the real request), got %d", n)
	}
	pending := PendingCandidates(s)
	if len(pending) != 1 {
		t.Fatalf("want 1 pending, got %d: %+v", len(pending), pending)
	}
	if strings.Contains(pending[0].Description, "memory-update") || strings.Contains(pending[0].Description, "project-memory") {
		t.Fatalf("control block leaked into candidate: %+v", pending[0])
	}
	if !strings.Contains(pending[0].Description, "tabs") {
		t.Fatalf("real request missing from candidate: %+v", pending[0])
	}
}

// TestExtractDedupAgainstPending verifies a candidate already staged in
// pending/ is not staged again by a later scan of a different session. The old
// dedup only compared against active memory, so repeated rules (e.g. "go build
// + affected package tests") accumulated duplicate pending files.
func TestExtractDedupAgainstPending(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	s := Store{Dir: dir}
	first := msgsUser("规则：Go 代码改动需 go build + 受影响包测试")
	if n, err := ExtractCandidates(s, "sess-1", first); err != nil || n != 1 {
		t.Fatalf("first extract: n=%d err=%v", n, err)
	}
	// A later session re-states the same rule in slightly different words.
	second := msgsUser("规则（Go 代码改动 = go build + 受影响包测试），已满足")
	if n, err := ExtractCandidates(s, "sess-2", second); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatalf("pending-covered candidate must be deduped, got %d", n)
	}
	if len(PendingCandidates(s)) != 1 {
		t.Fatalf("want 1 pending after dedup, got %d", len(PendingCandidates(s)))
	}
}
