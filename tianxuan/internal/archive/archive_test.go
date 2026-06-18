package archive

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenEmptyDir(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	if s != nil {
		t.Error("expected nil store for empty dir")
	}
}

func TestRecordMessageNilStore(t *testing.T) {
	var s *Store
	// Must not panic
	s.RecordMessage("s1", "user", "hello", "", 1)
}

func TestRecordMessageEmptySession(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Must not panic
	s.RecordMessage("", "user", "hello", "", 1)
}

func TestRecordAndSearch(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.RecordMessage("s1", "user", "hello world", "", 1)
	s.RecordMessage("s1", "assistant", "hi there", "", 1)
	s.RecordMessage("s2", "user", "goodbye", "", 1)

	results, err := s.SearchMessages([]string{"hello"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'hello', got %d", len(results))
	}
	if results[0].SessionID != "s1" {
		t.Errorf("expected session s1, got %s", results[0].SessionID)
	}

	// Search for "hi" — should match "hi there"
	results, err = s.SearchMessages([]string{"hi"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'hi', got %d", len(results))
	}
}

func TestSearchMultipleKeywords(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.RecordMessage("s1", "user", "hello world", "", 1)

	// Both keywords must match
	results, err := s.SearchMessages([]string{"hello", "world"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'hello'+'world', got %d", len(results))
	}

	// Keywords use OR matching, not AND
	results, err = s.SearchMessages([]string{"hello", "nonexistent"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (hello matches), got %d", len(results))
	}
}

func TestSearchNoResults(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	results, err := s.SearchMessages([]string{"nonexistent"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestListRecentSessions(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.RecordMessage("s1", "user", "msg1", "", 1)
	s.RecordMessage("s2", "user", "msg2", "", 1)

	sessions, err := s.ListRecentSessions(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) < 2 {
		t.Fatalf("expected at least 2 sessions, got %d", len(sessions))
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		max    int
		expect string
	}{
		{"hello", 10, "hello"},
		{"hello world this is long", 5, "hello"},
		{"", 10, ""},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.max)
		if got != tt.expect {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.expect)
		}
	}
}

func TestArchiveFileCreated(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.RecordMessage("test-session", "user", "archived content", "", 1)

	// Check that the file was created
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".jsonl" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected .jsonl file in archive dir")
	}
}
