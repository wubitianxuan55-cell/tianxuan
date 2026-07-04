package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tianxuan/internal/provider"
)

// --- New ---

func TestNewEmpty(t *testing.T) {
	s := New("")
	if len(s.Messages) != 0 {
		t.Errorf("empty session should have 0 messages, got %d", len(s.Messages))
	}
}

func TestNewWithSystem(t *testing.T) {
	s := New("You are a helpful assistant.")
	if len(s.Messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(s.Messages))
	}
	if s.Messages[0].Role != provider.RoleSystem {
		t.Errorf("role = %q, want system", s.Messages[0].Role)
	}
	if s.Messages[0].Content != "You are a helpful assistant." {
		t.Errorf("content = %q", s.Messages[0].Content)
	}
}

// --- Session.Add ---

func TestAdd(t *testing.T) {
	s := New("")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "hello"})
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "hi there"})
	if len(s.Messages) != 2 {
		t.Fatalf("want 2 messages, got %d", len(s.Messages))
	}
	if s.Messages[0].Role != provider.RoleUser {
		t.Errorf("first role = %q", s.Messages[0].Role)
	}
	if s.Messages[1].Role != provider.RoleAssistant {
		t.Errorf("second role = %q", s.Messages[1].Role)
	}
}

// --- Session.HasContent ---

func TestHasContentEmpty(t *testing.T) {
	s := New("")
	if s.HasContent() {
		t.Error("empty session should not have content")
	}
}

func TestHasContentSystemOnly(t *testing.T) {
	s := New("system prompt")
	if s.HasContent() {
		t.Error("system-only session should not have content")
	}
}

func TestHasContentWithUser(t *testing.T) {
	s := New("system")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "hello"})
	if !s.HasContent() {
		t.Error("session with user message should have content")
	}
}

func TestHasContentWithAssistant(t *testing.T) {
	s := New("")
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "response"})
	if !s.HasContent() {
		t.Error("session with assistant message should have content")
	}
}

func TestHasContentWithTool(t *testing.T) {
	s := New("")
	s.Add(provider.Message{Role: provider.RoleTool, Content: "result", ToolCallID: "tc1"})
	if !s.HasContent() {
		t.Error("session with tool message should have content")
	}
}

// --- Save / Load round-trip ---

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	s := New("system prompt")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "hello"})
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "world"})
	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Messages) != 3 {
		t.Fatalf("want 3 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "system prompt" {
		t.Errorf("system = %q", loaded.Messages[0].Content)
	}
	if loaded.Messages[1].Content != "hello" {
		t.Errorf("user = %q", loaded.Messages[1].Content)
	}
	if loaded.Messages[2].Content != "world" {
		t.Errorf("assistant = %q", loaded.Messages[2].Content)
	}
}

func TestSaveEmptyPath(t *testing.T) {
	s := New("")
	if err := s.Save(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestSaveCreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "session.jsonl")
	s := New("")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "test"})
	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("session file should exist")
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := Load("/nonexistent/session.jsonl")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("error should be os.IsNotExist, got %v", err)
	}
}

func TestLoadMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")
	os.WriteFile(path, []byte("not valid json\n"), 0o644)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed JSONL")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error should mention decode: %v", err)
	}
}

// --- List ---

func TestListMissingDirReturnsNil(t *testing.T) {
	sessions, err := List("/nonexistent/dir")
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got %v", err)
	}
	if sessions != nil {
		t.Errorf("expected nil sessions, got %v", sessions)
	}
}

func TestListEmptyDir(t *testing.T) {
	dir := t.TempDir()
	sessions, err := List(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("want 0 sessions, got %d", len(sessions))
	}
}

func TestListSorted(t *testing.T) {
	dir := t.TempDir()
	s1 := New("")
	s1.Add(provider.Message{Role: provider.RoleUser, Content: "first"})
	s1.Save(filepath.Join(dir, "a.jsonl"))

	s2 := New("")
	s2.Add(provider.Message{Role: provider.RoleUser, Content: "second"})
	s2.Save(filepath.Join(dir, "b.jsonl"))

	sessions, err := List(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(sessions))
	}
	if sessions[0].ModTime.Before(sessions[1].ModTime) {
		t.Error("sessions should be sorted newest first")
	}
}

func TestListSkipsEmpty(t *testing.T) {
	dir := t.TempDir()
	s := New("system only")
	s.Save(filepath.Join(dir, "empty.jsonl"))

	sessions, err := List(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("empty sessions should be skipped, got %d", len(sessions))
	}
}

func TestListSkipsNonJSONL(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a session"), 0o644)
	s := New("")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "real"})
	s.Save(filepath.Join(dir, "real.jsonl"))

	sessions, err := List(dir)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("want 1 session, got %d", len(sessions))
	}
}

// --- previewSession ---

func TestPreviewSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	s := New("system")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "Help me debug the auth module"})
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "Sure, let me look..."})
	s.Save(path)

	preview, turns := previewSession(path)
	if turns != 1 {
		t.Errorf("turns = %d, want 1", turns)
	}
	if !strings.Contains(preview, "debug") {
		t.Errorf("preview = %q", preview)
	}
}

func TestPreviewSessionLongMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	s := New("")
	s.Add(provider.Message{Role: provider.RoleUser, Content: strings.Repeat("a", 200)})
	s.Save(path)

	preview, _ := previewSession(path)
	if len([]rune(preview)) > 80 {
		t.Errorf("preview should be capped at 80 runes, got %d", len([]rune(preview)))
	}
	if !strings.HasSuffix(preview, "…") {
		t.Errorf("truncated preview should end with …, got %q", preview)
	}
}

func TestPreviewSessionMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")
	os.WriteFile(path, []byte("not json\n"), 0o644)
	preview, turns := previewSession(path)
	if turns != 0 {
		t.Errorf("turns = %d, want 0", turns)
	}
	if preview != "" {
		t.Errorf("preview = %q, want empty", preview)
	}
}

// --- NewPath ---

func TestNewPath(t *testing.T) {
	dir := t.TempDir()
	path := NewPath(dir, "deepseek-chat")
	if !strings.HasSuffix(path, ".jsonl") {
		t.Errorf("should end with .jsonl: %s", path)
	}
	if !strings.Contains(path, "deepseek-chat") {
		t.Errorf("should contain model name: %s", path)
	}
	if !strings.HasPrefix(path, dir) {
		t.Errorf("should be under dir: %s", path)
	}
}

func TestNewPathSanitizesSlashes(t *testing.T) {
	path := NewPath("/dir", "provider/model")
	base := filepath.Base(path)
	if strings.Contains(base, "/") {
		t.Errorf("filename should not contain /: %s", base)
	}
	if !strings.Contains(base, "provider-model") {
		t.Errorf("slashes should be replaced: %s", base)
	}
}

func TestNewPathEmptyModel(t *testing.T) {
	path := NewPath("/dir", "")
	if !strings.Contains(path, "session") {
		t.Errorf("empty model should use 'session' fallback: %s", path)
	}
}
