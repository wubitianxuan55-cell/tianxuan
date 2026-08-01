package builtin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// TestProbeGitCandidates locks git executable discovery: when the process
// PATH snapshot predates a git install (tianxuan-desktop keeps the PATH it
// started with), the git tools must fall back to probing well-known install
// locations instead of failing with "git not found".
func TestProbeGitCandidates(t *testing.T) {
	dir := t.TempDir()
	fakeGit := filepath.Join(dir, "git.exe")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "nope.exe")

	// 第一个存在的候选被选中；不存在的跳过。
	got := probeGitCandidates([]string{missing, fakeGit})
	if got != fakeGit {
		t.Fatalf("probeGitCandidates = %q, want %q", got, fakeGit)
	}
	// 全部不存在返回空串 → 调用方回落 "git" 让错误自然浮现。
	if got := probeGitCandidates([]string{missing}); got != "" {
		t.Fatalf("probeGitCandidates(empty) = %q, want \"\"", got)
	}
}

// TestGitCandidatePathsWindowsInstall locks that the default candidate list
// covers the standard Git for Windows location, so a desktop process started
// before git was installed still finds it.
func TestGitCandidatePathsWindowsInstall(t *testing.T) {
	found := false
	for _, c := range gitCandidatePaths {
		if strings.Contains(c, `Program Files\Git\cmd\git.exe`) {
			found = true
		}
	}
	if !found {
		t.Fatal("gitCandidatePaths must include C:\\Program Files\\Git\\cmd\\git.exe")
	}
}

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func writeLines(t *testing.T, path string, n int, suffix string) {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %06d%s\n", i, suffix)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGitDiffLongOutputTruncated locks truncation protection for git_diff:
// a large working-tree diff must not be dumped into context in full — the
// output is cut head+tail with an explicit notice so the model can narrow
// with path= instead of guessing.
func TestGitDiffLongOutputTruncated(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	t.Chdir(dir) // git 工具继承进程 cwd（无 workDir 字段）——模拟 agent 运行在仓库根
	gitCmd(t, dir, "init")
	f := filepath.Join(dir, "big.txt")
	writeLines(t, f, 6000, "")
	gitCmd(t, dir, "add", "big.txt")
	writeLines(t, f, 6000, " changed") // every line modified → full-file diff

	out := runTool(t, gitDiff{}, map[string]any{})
	if !strings.Contains(out, "[git output truncated") {
		t.Fatalf("long diff should carry a truncation notice, got %d bytes", len(out))
	}
	// git 把全部删除行（-）放在前、新增行（+）放在后：头部断言删除行，尾部断言新增行。
	if !strings.Contains(out, "-line 000001") {
		t.Errorf("head of diff lost: %.200s", out)
	}
	if !strings.Contains(out, "+line 006000 changed") {
		t.Errorf("tail of diff lost: ...%.200s", out[len(out)-200:])
	}
	if strings.Contains(out, "line 030000 changed") {
		t.Errorf("middle of diff should be elided")
	}
}

// TestGitLogLongOutputTruncated locks the same protection for git_log:
// oversized commit messages must not flood the context window.
func TestGitLogLongOutputTruncated(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	t.Chdir(dir) // git 工具继承进程 cwd（无 workDir 字段）——模拟 agent 运行在仓库根
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.email", "test@tianxuan.dev")
	gitCmd(t, dir, "config", "user.name", "test")
	f := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	longMsg := strings.Repeat("commit body line\n", 6000)
	// 超长 message 经 stdin 传入（`-F -`），避免 Windows 命令行长度限制。
	cmd := exec.Command("git", "commit", "-F", "-")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("subject\n\n" + longMsg)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	out := runTool(t, gitLog{}, map[string]any{"count": 1})
	if !strings.Contains(out, "[git output truncated") {
		t.Fatalf("long log should carry a truncation notice, got %d bytes", len(out))
	}
	if strings.Contains(out, "commit body line\ncommit body line\ncommit body line\ncommit body line\ncommit body line\ncommit body line") {
		t.Errorf("middle of long commit message should be elided")
	}
}
