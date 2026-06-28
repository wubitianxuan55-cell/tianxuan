package builtin

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEditFileCRLFAdaptation verifies that edit_file's line-ending adaptation
// correctly matches LF old_string against a CRLF file.
func TestEditFileCRLFAdaptation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crlf_test.txt")

	// Write a CRLF file
	content := "line one\r\nline two\r\nline three\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ef := editFile{roots: []string{dir}}

	// Edit with LF old_string — should be auto-adapted to CRLF
	result, err := ef.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":       path,
		"old_string": "line two\n",
		"new_string": "line TWO\n",
	}))
	if err != nil {
		t.Fatalf("edit with LF old_string on CRLF file failed: %v", err)
	}
	if result != "edited "+path {
		t.Errorf("unexpected result: %s", result)
	}

	// Verify the file was correctly edited
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := "line one\r\nline TWO\r\nline three\r\n"
	if string(got) != expected {
		t.Errorf("after edit:\n  got: %q\n want: %q", string(got), expected)
	}
}

// TestEditFileCRLFNewlineAdaptation verifies that new_string LF is also
// adapted to CRLF when the file uses CRLF.
func TestEditFileCRLFNewlineAdaptation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crlf_newline.txt")

	content := "old line\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ef := editFile{roots: []string{dir}}

	result, err := ef.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":       path,
		"old_string": "old line\n",
		"new_string": "new line\nstill new\n",
	}))
	if err != nil {
		t.Fatalf("edit with multi-line LF new_string on CRLF file failed: %v", err)
	}
	_ = result

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := "new line\r\nstill new\r\n"
	if string(got) != expected {
		t.Errorf("multi-line new_string adaptation:\n  got: %q\n want: %q", string(got), expected)
	}
}

// TestAdaptLineEndings verifies the adaptLineEndings helper directly.
func TestAdaptLineEndings(t *testing.T) {
	tests := []struct {
		input  string
		target string
		want   string
	}{
		{"hello\nworld\n", "\r\n", "hello\r\nworld\r\n"},
		{"hello\r\nworld\r\n", "\r\n", "hello\r\nworld\r\n"}, // already CRLF
		{"hello\nworld\n", "\n", "hello\nworld\n"},             // LF target = no change
		{"hello\nworld\n", "", "hello\nworld\n"},                // no target = no change
		{"no newlines", "\r\n", "no newlines"},                  // no newlines
		{"line\n", "\r\n", "line\r\n"},                          // single line
		{"", "\r\n", ""},                                        // empty
	}
	for _, tt := range tests {
		got := adaptLineEndings(tt.input, tt.target)
		if got != tt.want {
			t.Errorf("adaptLineEndings(%q, %q) = %q, want %q", tt.input, tt.target, got, tt.want)
		}
	}
}

// TestDetectLineEnding verifies line ending detection.
func TestDetectLineEnding(t *testing.T) {
	tests := []struct {
		content string
		want    string
	}{
		{"hello\r\nworld\r\n", "\r\n"},
		{"hello\nworld\n", "\n"},
		{"mixed\r\nline\n", "\r\n"}, // first detected wins
		{"no newlines", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := detectLineEnding(tt.content)
		if got != tt.want {
			t.Errorf("detectLineEnding(%q) = %q, want %q", tt.content, got, tt.want)
		}
	}
}
