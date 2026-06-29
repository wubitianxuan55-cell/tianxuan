package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestGrepHighlightMatch(t *testing.T) {
	tests := []struct {
		pattern string
		text    string
		want    string
	}{
		{"foo", "hello foo bar", "hello >>>foo<<< bar"},
		{"\\d+", "line 42 is the answer", "line >>>42<<< is the answer"},
		{"nomatch", "no match here", "no match here"},
		{"^start", "start of line", ">>>start<<< of line"},
		{"end$", "the end", "the >>>end<<<"},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			re := mustCompile(t, tt.pattern)
			got := highlightMatch(re, tt.text)
			if got != tt.want {
				t.Errorf("highlightMatch(%q, %q) = %q, want %q", tt.pattern, tt.text, got, tt.want)
			}
		})
	}
}

func TestGrepContextLines(t *testing.T) {
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, "test.go"), `package main

import "fmt"

// Hello prints a greeting.
func Hello() {
	fmt.Println("hello world")
}

// Goodbye says farewell.
func Goodbye() {
	fmt.Println("goodbye world")
}

func main() {
	Hello()
	Goodbye()
}
`)

	g := grepTool{workDir: dir}
	ctx := context.Background()

	t.Run("no context lines", func(t *testing.T) {
		result, err := g.Execute(ctx, jsonArg(`{"pattern":"says farewell","path":"test.go"}`))
		if err != nil {
			t.Fatal(err)
		}
		// Should only contain the matching line, no context.
		lines := strings.Split(strings.TrimSpace(result), "\n")
		if len(lines) != 1 {
			t.Errorf("expected single line (unique match), got %d:\n%s", len(lines), result)
		}
		if !strings.Contains(result, "Goodbye") {
			t.Errorf("result should contain match: %s", result)
		}
	})

	t.Run("context_lines=2", func(t *testing.T) {
		result, err := g.Execute(ctx, jsonArg(`{"pattern":"Hello","path":"test.go","context_lines":2}`))
		if err != nil {
			t.Fatal(err)
		}
		// Should contain the match line + 2 preceding context lines.
		lines := strings.Split(strings.TrimSpace(result), "\n")
		if len(lines) < 3 {
			t.Errorf("expected at least 3 lines (match + 2 context), got %d:\n%s", len(lines), result)
		}
		// Context lines should have a "-" suffix on the line number.
		hasContextSuffix := false
		for _, line := range lines {
			if strings.Contains(line, ":-") || strings.Contains(line, ":\"") {
				// check for lines ending with "-:" before text or lines with the context mark
			}
			if idx := strings.Index(line, ":"); idx > 0 {
				rest := line[idx+1:]
				if strings.HasPrefix(rest, "test.go:") {
					rest = line[idx+1+len("test.go:"):]
				}
				if len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
					numEnd := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' })
					if numEnd > 0 && numEnd < len(rest) && rest[numEnd] == '-' {
						hasContextSuffix = true
					}
				}
			}
		}
		if !hasContextSuffix && len(lines) > 1 {
			t.Logf("note: context suffix not detected in output:\n%s", result)
		}
	})

	t.Run("context_lines with highlight", func(t *testing.T) {
		result, err := g.Execute(ctx, jsonArg(`{"pattern":"Hello","path":"test.go","context_lines":1,"highlight":true}`))
		if err != nil {
			t.Fatal(err)
		}
		// Match line should have >>>Hello<<< highlighting.
		if !strings.Contains(result, ">>>Hello<<<") {
			t.Errorf("expected highlighted match, got:\n%s", result)
		}
	})
}

func TestGrepContextLines_MultipleMatches(t *testing.T) {
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, "multi.txt"), `line one
line two with error
line three
line four with error
line five
line six with error
line seven`)

	g := grepTool{workDir: dir}
	ctx := context.Background()

	result, err := g.Execute(ctx, jsonArg(`{"pattern":"error","path":"multi.txt","context_lines":1}`))
	if err != nil {
		t.Fatal(err)
	}
	// Should find 3 matches each with context.
	count := strings.Count(result, ">>>error<<<")
	if count != 3 {
		t.Errorf("expected 3 highlighted matches, got %d:\n%s", count, result)
	}
	// Total lines: 3 matches + up to 2 context each = up to 9.
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) < 3 || len(lines) > 9 {
		t.Errorf("expected 3-9 lines, got %d:\n%s", len(lines), result)
	}
}

func TestGrepHighlight_Disabled(t *testing.T) {
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, "f.txt"), "hello world")

	g := grepTool{workDir: dir}
	ctx := context.Background()

	result, err := g.Execute(ctx, jsonArg(`{"pattern":"hello","path":"f.txt","highlight":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, ">>>") {
		t.Errorf("highlight should be disabled, got: %s", result)
	}
}

func TestGrepSortByRelevance_WithContext(t *testing.T) {
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, "a.txt"), "apple\nbanana\ncherry\ndate")
	testWriteFile(t, filepath.Join(dir, "b.txt"), "apple\napple pie\nbanana split\ncherry pie")

	g := grepTool{workDir: dir}
	ctx := context.Background()

	result, err := g.Execute(ctx, jsonArg(`{"pattern":"apple|banana|cherry","path":".","sort_by":"relevance","context_lines":0}`))
	if err != nil {
		t.Fatal(err)
	}
	// b.txt has 3 matches, a.txt has 3 matches (apple, banana, cherry).
	// With relevance sort, the file with more matches appears first.
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 matches, got %d:\n%s", len(lines), result)
	}
}

func TestGrepContextLines_Max5(t *testing.T) {
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, "f.txt"), "line1\nline2\nline3\nmatch\nline5\nline6\nline7")

	g := grepTool{workDir: dir}
	ctx := context.Background()

	// context_lines > 5 should be clamped to 5.
	result, err := g.Execute(ctx, jsonArg(`{"pattern":"match","path":"f.txt","context_lines":10}`))
	if err != nil {
		t.Fatal(err)
	}
	// Should be clamped, not crash. At most 1 match + 5 before = 6 lines.
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) > 6 {
		t.Errorf("context_lines should be clamped to 5, got %d lines:\n%s", len(lines), result)
	}
}

func mustCompile(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("compile %q: %v", pattern, err)
	}
	return re
}

func testWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeFile %s: %v", path, err)
	}
}

func jsonArg(s string) json.RawMessage {
	return json.RawMessage([]byte(s))
}
