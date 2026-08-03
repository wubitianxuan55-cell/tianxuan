package builtin

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tianxuan/internal/event"
	"tianxuan/internal/jobs"
)

// TestReadFileAcceptsFileAlias: the model often emits {"file": ...} instead of
// {"path": ...}; the alias must be honored, not silently dropped.
func TestReadFileAcceptsFileAlias(t *testing.T) {
	f := filepath.Join(t.TempDir(), "alias.txt")
	if err := os.WriteFile(f, []byte("hello alias"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := runTool(t, readFile{}, map[string]any{"file": f})
	if !strings.Contains(out, "hello alias") {
		t.Fatalf("file alias output = %q, want content", out)
	}
}

// TestWaitAcceptsJobIDSingular: the model often emits {"job_id": "..."} while
// the schema says job_ids (array); the singular alias must be honored and must
// NOT degrade to "wait for every job" (which would block on unrelated ones).
func TestWaitAcceptsJobIDSingular(t *testing.T) {
	m := jobs.NewManager(event.Discard)
	defer m.Close()
	ctx := jobs.WithManager(context.Background(), m)

	fast := m.Start("bash", "fast", func(_ context.Context, out io.Writer) (string, error) {
		fmt.Fprint(out, "job-done")
		return "job-done", nil
	})
	slow := m.Start("bash", "slow", func(jobCtx context.Context, _ io.Writer) (string, error) {
		select {
		case <-time.After(10 * time.Second):
		case <-jobCtx.Done():
		}
		return "", nil
	})
	defer m.Kill(slow.ID)

	start := time.Now()
	out, err := waitJob{}.Execute(ctx, argsJSON(t, map[string]any{"job_id": fast.ID}))
	if err != nil {
		t.Fatalf("wait(job_id) failed: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "job-done") {
		t.Fatalf("output = %q, want job-done", out)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("job_id waited for an unrelated job: %v", elapsed)
	}
}

// TestWaitAcceptsTimeoutMs: the model sometimes emits timeout_ms; it must be
// converted to seconds and actually bound the wait.
func TestWaitAcceptsTimeoutMs(t *testing.T) {
	m := jobs.NewManager(event.Discard)
	defer m.Close()
	ctx := jobs.WithManager(context.Background(), m)

	job := m.Start("bash", "sleep", func(jobCtx context.Context, _ io.Writer) (string, error) {
		select {
		case <-time.After(10 * time.Second):
		case <-jobCtx.Done():
		}
		return "", nil
	})
	defer m.Kill(job.ID)

	start := time.Now()
	_, err := waitJob{}.Execute(ctx, argsJSON(t, map[string]any{"job_ids": []string{job.ID}, "timeout_ms": 300}))
	if err != nil {
		t.Fatalf("wait timeout_ms failed: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("timeout_ms not honored: waited %v", elapsed)
	}
}

// TestEditFileReplaceAll: replace_all replaces every occurrence (multi_edit
// semantics); without it, multiple matches still error as "not unique".
func TestEditFileReplaceAll(t *testing.T) {
	f := filepath.Join(t.TempDir(), "r.txt")
	if err := os.WriteFile(f, []byte("a=1\na=2\nb=3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTool(t, editFile{}, map[string]any{
		"path": f, "old_string": "a=", "new_string": "x=", "replace_all": true,
	})
	got, _ := os.ReadFile(f)
	if want := "x=1\nx=2\nb=3\n"; string(got) != want {
		t.Fatalf("after replace_all = %q, want %q", got, want)
	}

	f2 := filepath.Join(t.TempDir(), "r2.txt")
	if err := os.WriteFile(f2, []byte("a=1\na=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (editFile{}).Execute(context.Background(), argsJSON(t, map[string]any{
		"path": f2, "old_string": "a=", "new_string": "x=",
	})); err == nil {
		t.Fatal("multiple matches without replace_all should still error")
	}
}

// TestGrepGlobFilter: glob restricts the searched files.
func TestGrepGlobFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("needle\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("needle\n"), 0o644)

	out := runTool(t, grepTool{}, map[string]any{"pattern": "needle", "path": dir, "glob": "*.go"})
	if strings.Contains(out, "b.txt") {
		t.Fatalf("glob filter leaked b.txt: %q", out)
	}
	if !strings.Contains(out, "a.go") {
		t.Fatalf("glob filter dropped a.go: %q", out)
	}
}

// TestGrepIncludeLimitAliases: include is a glob alias, limit maps to
// max_matches (both are common model emissions that used to be dropped).
func TestGrepIncludeLimitAliases(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.go"), []byte("needle\nneedle\nneedle\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "y.txt"), []byte("needle\n"), 0o644)

	out := runTool(t, grepTool{}, map[string]any{
		"pattern": "needle", "path": dir, "include": "*.go", "limit": 2,
	})
	if strings.Contains(out, "y.txt") {
		t.Fatalf("include filter leaked y.txt: %q", out)
	}
	if got := strings.Count(out, "needle"); got != 2 {
		t.Fatalf("limit alias not honored: got %d matches in %q", got, out)
	}
}

// TestEditLinesOldStringMismatchNoSwallow: when the model passes both
// old_string and line numbers, the old_string must be verified against the
// actual range. A mismatch must fail loudly WITHOUT mutating the file —
// silently ignoring old_string used to let a wrong range swallow the
// declaration line.
func TestEditLinesOldStringMismatchNoSwallow(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.go")
	orig := "func foo() {\n  body\n}\n"
	if err := os.WriteFile(f, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := (editLines{}).Execute(context.Background(), argsJSON(t, map[string]any{
		"path": f, "start_line": 1, "end_line": 1,
		"new_content": "  replaced", "old_string": "  body\n}",
	}))
	if err == nil {
		t.Fatal("mismatched old_string must fail loudly (would swallow the declaration line)")
	}
	if !strings.Contains(err.Error(), "old_string") || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err = %v, want old_string mismatch hint", err)
	}
	got, _ := os.ReadFile(f)
	if string(got) != orig {
		t.Fatalf("file mutated despite mismatch: %q", got)
	}
}

// TestEditLinesOldStringMatchExecutes: when old_string equals the actual
// range, the anchor is consistent with the line numbers and the edit proceeds
// (the model's anchor habit keeps working instead of being rejected).
func TestEditLinesOldStringMatchExecutes(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.go")
	// The fixture must be a syntactically valid Go file: edit_lines now
	// post-validates .go edits with gofmt -e and rolls back invalid files.
	if err := os.WriteFile(f, []byte("package main\n\nfunc foo() {\n  body\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTool(t, editLines{}, map[string]any{
		"path": f, "start_line": 4, "end_line": 4,
		"new_content": "  replaced", "old_string": "  body",
	})
	got, _ := os.ReadFile(f)
	if want := "package main\n\nfunc foo() {\n  replaced\n}\n"; string(got) != want {
		t.Fatalf("match edit = %q, want %q", got, want)
	}
}
