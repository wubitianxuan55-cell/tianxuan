package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- read_file extended tests ---

func TestReadFileMissing(t *testing.T) {
	_, err := readFile{}.Execute(context.Background(), argsJSON(t, map[string]any{"path": "/nonexistent/file.txt"}))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Errorf("error should mention 'read': %v", err)
	}
}

func TestReadFileMissingPath(t *testing.T) {
	_, err := readFile{}.Execute(context.Background(), argsJSON(t, map[string]any{}))
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestReadFileLargeFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "large.txt")
	var b strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	os.WriteFile(f, []byte(b.String()), 0o644)

	// Read with small limit.
	out := runTool(t, readFile{}, map[string]any{"path": f, "offset": 0, "limit": 3})
	if !strings.Contains(out, "1→line 1") {
		t.Errorf("missing first line: %s", out)
	}
	if !strings.Contains(out, "3→line 3") {
		t.Errorf("missing third line: %s", out)
	}
	if strings.Contains(out, "4→line 4") {
		t.Errorf("should not contain fourth line: %s", out)
	}
	// Pagination hint (exact count unknown without draining file).
	if !strings.Contains(out, "more lines available") {
		t.Errorf("pagination hint missing: %s", out)
	}
}

// V10.96: 阶梯式阅读门控 — 大文件盲读时注入降级建议。
func TestReadFileRungLadderHint(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "large.txt")
	var b strings.Builder
	for i := 1; i <= 250; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	os.WriteFile(f, []byte(b.String()), 0o644)

	// 默认 limit=2000, offset=0, 文件>200行 → 应触发阶梯式提示
	out := runTool(t, readFile{}, map[string]any{"path": f})
	if !strings.Contains(out, "阶梯式阅读提示") {
		t.Errorf("rung ladder hint missing for large file blind read: %s", out)
	}

	// 指定了 offset → 不触发阶梯式提示
	out2 := runTool(t, readFile{}, map[string]any{"path": f, "offset": 100, "limit": 10})
	if strings.Contains(out2, "阶梯式阅读提示") {
		t.Errorf("rung ladder hint should not trigger with explicit offset/limit: %s", out2)
	}
}

func TestReadFileOffsetPastEOF(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "short.txt")
	os.WriteFile(f, []byte("one\ntwo\n"), 0o644)

	out := runTool(t, readFile{}, map[string]any{"path": f, "offset": 100, "limit": 10})
	if !strings.Contains(out, "past EOF") {
		t.Errorf("should report past EOF: %s", out)
	}
}

func TestReadFileInvalidArgs(t *testing.T) {
	_, err := readFile{}.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- ls extended tests ---

func TestLsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	out := runTool(t, listDir{}, map[string]any{"path": dir})
	if !strings.Contains(out, "(empty directory)") {
		t.Errorf("empty dir should report (empty directory): %s", out)
	}
}

func TestLsMissingDir(t *testing.T) {
	_, err := listDir{}.Execute(context.Background(), argsJSON(t, map[string]any{"path": "/nonexistent"}))
	if err == nil {
		t.Fatal("expected error for missing dir")
	}
}

func TestLsDefaultPath(t *testing.T) {
	// Default path "." should list the current directory without error.
	out := runTool(t, listDir{}, map[string]any{})
	if out == "" {
		t.Error("ls with default path should return something")
	}
}

func TestLsInvalidArgs(t *testing.T) {
	_, err := listDir{}.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- grep extended tests ---

func TestGrepSingleFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "code.go")
	os.WriteFile(f, []byte("func Foo() {}\nfunc Bar() {}\nvar x = 1\n"), 0o644)

	out := runTool(t, grepTool{}, map[string]any{"pattern": "func ", "path": f})
	if !strings.Contains(out, "Foo") || !strings.Contains(out, "Bar") {
		t.Errorf("should find both functions: %s", out)
	}
	if strings.Contains(out, "var x") {
		t.Errorf("should not include non-matching line: %s", out)
	}
}

func TestGrepNoMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\n"), 0o644)

	out := runTool(t, grepTool{}, map[string]any{"pattern": "xyzzy", "path": dir})
	if !strings.Contains(out, "(no matches)") {
		t.Errorf("expected (no matches): %s", out)
	}
}

func TestGrepInvalidPattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("test\n"), 0o644)

	_, err := grepTool{}.Execute(context.Background(), argsJSON(t, map[string]any{"pattern": "[invalid", "path": dir}))
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestGrepInvalidArgs(t *testing.T) {
	_, err := grepTool{}.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGrepMissingPattern(t *testing.T) {
	_, err := grepTool{}.Execute(context.Background(), argsJSON(t, map[string]any{"path": "."}))
	if err == nil {
		t.Fatal("expected error for missing pattern")
	}
}

func TestGrepSkipsGitDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("secret = true\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)

	out := runTool(t, grepTool{}, map[string]any{"pattern": "secret", "path": dir})
	if strings.Contains(out, ".git") {
		t.Errorf("grep should skip .git directory: %s", out)
	}
}

func TestGrepTruncation(t *testing.T) {
	dir := t.TempDir()
	// Create a file with many matching lines.
	var b strings.Builder
	for i := 0; i < 600; i++ {
		fmt.Fprintf(&b, "match %d\n", i)
	}
	os.WriteFile(filepath.Join(dir, "many.txt"), []byte(b.String()), 0o644)

	out := runTool(t, grepTool{}, map[string]any{"pattern": "match", "path": dir})
	if !strings.Contains(out, "truncated") {
		t.Errorf("should mention truncation: %s", out)
	}
}

// --- glob extended tests ---

func TestGlobEmptyPattern(t *testing.T) {
	_, err := globTool{}.Execute(context.Background(), argsJSON(t, map[string]any{"pattern": ""}))
	if err == nil {
		t.Fatal("expected error for empty pattern")
	}
}

func TestGlobInvalidArgs(t *testing.T) {
	_, err := globTool{}.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGlobCharClass(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(dir, "c.log"), []byte("c"), 0o644)

	out := runTool(t, globTool{}, map[string]any{"pattern": filepath.Join(dir, "[ab].txt")})
	if !strings.Contains(out, "a.txt") || !strings.Contains(out, "b.txt") {
		t.Errorf("should match [ab].txt: %s", out)
	}
	if strings.Contains(out, "c.log") {
		t.Errorf("should not match c.log: %s", out)
	}
}

func TestGlobQuestionMark(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "ab.txt"), []byte("ab"), 0o644)

	out := runTool(t, globTool{}, map[string]any{"pattern": filepath.Join(dir, "?.txt")})
	if !strings.Contains(out, "a.txt") {
		t.Errorf("should match a.txt: %s", out)
	}
	if strings.Contains(out, "ab.txt") {
		t.Errorf("?.txt should not match ab.txt: %s", out)
	}
}

// --- verify_gate tests --- V10.97: headsign phase gate

func TestVerifyGateAllPass(t *testing.T) {
	out := runTool(t, verifyGate{}, map[string]any{
		"checks": []map[string]any{
			{"name": "truth", "command": "exit 0"},
			{"name": "echo test", "command": "echo ok"},
		},
	})
	if !strings.Contains(out, "GATE PASSED") {
		t.Errorf("all-pass gate should report PASSED: %s", out)
	}
}

func TestVerifyGateFirstFail(t *testing.T) {
	out := runTool(t, verifyGate{}, map[string]any{
		"checks": []map[string]any{
			{"name": "fail check", "command": "exit 1"},
			{"name": "never runs", "command": "echo should not run"},
		},
	})
	if !strings.Contains(out, "GATE FAILED") {
		t.Errorf("should report FAILED: %s", out)
	}
	if strings.Contains(out, "never runs") || strings.Contains(out, "should not run") {
		t.Errorf("second check should not run after first failure: %s", out)
	}
}

func TestVerifyGateEmptyChecks(t *testing.T) {
	_, err := verifyGate{}.Execute(context.Background(), argsJSON(t, map[string]any{"checks": []any{}}))
	if err == nil {
		t.Fatal("expected error for empty checks")
	}
}

func TestVerifyGateInvalidArgs(t *testing.T) {
	_, err := verifyGate{}.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestVerifyGateLongFailureKeepsTailDetails locks the truncation behavior for
// long check output: go test 类长输出（>2000 字符）的 FAIL 详情位于输出尾部，
// 头部截断会丢掉「哪个测试失败、为什么失败」，模型只能盲目重跑或猜测。
// 截断必须保留尾部关键信息。
func TestVerifyGateLongFailureKeepsTailDetails(t *testing.T) {
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = `(for /L %i in (1,1,250) do @echo PASS line %i) & echo --- FAIL: TestFoo & echo foo_test.go:12: expected 2, got 3 & exit /b 1`
	} else {
		cmd = `for i in $(seq 1 250); do echo PASS line $i; done; echo '--- FAIL: TestFoo'; echo 'foo_test.go:12: expected 2, got 3'; echo FAIL; exit 1`
	}
	out := runTool(t, verifyGate{}, map[string]any{
		"checks": []map[string]any{
			{"name": "unit tests", "command": cmd},
		},
	})
	if !strings.Contains(out, "GATE FAILED") {
		t.Fatalf("should report FAILED: %s", out)
	}
	if !strings.Contains(out, "--- FAIL: TestFoo") {
		t.Errorf("FAIL header lost by truncation: %s", out)
	}
	if !strings.Contains(out, "foo_test.go:12: expected 2, got 3") {
		t.Errorf("failure detail lost by truncation: %s", out)
	}
}
