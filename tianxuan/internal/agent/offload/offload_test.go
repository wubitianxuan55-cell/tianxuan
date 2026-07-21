package offload

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaybeOffload_BelowThreshold(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, "test-session")
	if err != nil {
		t.Fatal(err)
	}
	raw := "short output"
	result := s.MaybeOffload("bash", raw, 100)
	if result != raw {
		t.Errorf("output below threshold should be unchanged, got %q", result)
	}
}

func TestMaybeOffload_AboveThreshold(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, "test-session")
	if err != nil {
		t.Fatal(err)
	}
	// Build a string > 100 chars.
	raw := strings.Repeat("abcdefghij", 20) // 200 chars
	result := s.MaybeOffload("bash", raw, 100)

	if result == raw {
		t.Fatal("output above threshold should be offloaded")
	}
	if !strings.Contains(result, "[Large output offloaded:") {
		t.Errorf("result should contain offload marker, got: %s", result)
	}
	if !strings.Contains(result, "Preview:") {
		t.Errorf("result should contain preview, got: %s", result)
	}
	if !strings.Contains(result, "search_large_output") {
		t.Errorf("result should mention search_large_output tool")
	}

	// Verify file was written.
	entries, err := os.ReadDir(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}

	data, err := os.ReadFile(filepath.Join(s.Dir(), entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != raw {
		t.Errorf("offloaded content mismatch: got %q, want %q", string(data), raw)
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, "list-test")
	if err != nil {
		t.Fatal(err)
	}

	// Create two offloaded files.
	s.MaybeOffload("bash", strings.Repeat("a", 200), 100)
	s.MaybeOffload("grep", strings.Repeat("b", 200), 100)

	files, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	// Order: newest first.
	if files[0].ModTime.Before(files[1].ModTime) {
		t.Error("files should be newest-first")
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, "read-test")
	if err != nil {
		t.Fatal(err)
	}

	raw := "hello world\nline two\n"
	s.MaybeOffload("bash", strings.Repeat("x", 200), 100)

	files, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	// We can't read the real file name since it's auto-generated, but we can
	// read the list entry's name.
	content, err := s.Read(files[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(content, strings.Repeat("x", 200)) {
		t.Errorf("content mismatch: got %q", strings.TrimSpace(content))
	}
	_ = raw
}

func TestRead_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, "traversal-test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Read("../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "invalid name") {
		t.Errorf("expected 'invalid name' error, got: %v", err)
	}
}

func TestSearch(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, "search-test")
	if err != nil {
		t.Fatal(err)
	}

	// Create two files with known content.
	s.MaybeOffload("bash", strings.Repeat("x", 150)+"IMPORTANT: found it\n"+strings.Repeat("y", 50), 100)
	s.MaybeOffload("grep", strings.Repeat("z", 150)+"another line\n", 100)

	result, err := s.Search("IMPORTANT", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "IMPORTANT") {
		t.Errorf("search should find IMPORTANT, got: %s", result)
	}

	// Search for non-existent.
	result, err = s.Search("NONEXISTENT", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "(no matches)") {
		t.Errorf("search for non-existent should return 'no matches', got: %s", result)
	}
}

func TestRemoveAll(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, "remove-test")
	if err != nil {
		t.Fatal(err)
	}

	s.MaybeOffload("bash", strings.Repeat("a", 200), 100)

	if err := s.RemoveAll(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Dir()); !os.IsNotExist(err) {
		t.Error("directory should be removed")
	}
}

func TestSanitizeSessionID(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"abc-123", "abc-123"},
		{"session/../etc", "session____etc"},
		{"hello world!", "hello_world_"},
	}
	for _, tc := range tests {
		got := sanitizeSessionID(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeSessionID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
