package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeView 是 git worktree 列表项（Codex Local environments / worktree
// 隔离开发的桌面管理蒸馏）。
type WorktreeView struct {
	Path     string `json:"path"`
	Branch   string `json:"branch"`   // 当前分支名；detached 时为空
	Detached bool   `json:"detached"` // HEAD detached
	Current  bool   `json:"current"`  // agent 当前所在 worktree
}

// gitCmd 在当前工作区目录跑任意 git 命令（显式 -C 语义，避免进程 cwd 歧义）。
func gitCmd(ctx context.Context, args ...string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

// gitWorktreeCmd 跑 git worktree 子命令。
func gitWorktreeCmd(ctx context.Context, args ...string) (string, error) {
	return gitCmd(ctx, append([]string{"worktree"}, args...)...)
}

// parseWorktreePorcelain 解析 `git worktree list --porcelain` 输出。
// 格式：空行分隔的块，每块含 worktree 路径、HEAD、branch refs/heads/x
// （或 detached），当前 worktree 额外有一行裸的 main/detached。
func parseWorktreePorcelain(out string) []WorktreeView {
	var views []WorktreeView
	for _, block := range strings.Split(out, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var v WorktreeView
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				v.Path = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
			case strings.HasPrefix(line, "branch refs/heads/"):
				v.Branch = strings.TrimPrefix(line, "branch refs/heads/")
			case line == "detached":
				v.Detached = true
				v.Current = true
			case line == "main":
				v.Current = true
			}
		}
		if v.Path != "" {
			views = append(views, v)
		}
	}
	return views
}

// Worktrees 列出当前工作区 git 仓库的全部 worktree。
func (a *App) Worktrees() ([]WorktreeView, error) {
	out, err := gitWorktreeCmd(context.Background(), "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktreePorcelain(out), nil
}

// AddWorktree 创建新 worktree：git worktree add <path> -b <branch> [base]。
func (a *App) AddWorktree(path, branch, base string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path is required")
	}
	if strings.TrimSpace(branch) == "" {
		return errors.New("branch is required")
	}
	parent := filepath.Dir(path)
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		return fmt.Errorf("parent directory %s does not exist", parent)
	}
	args := []string{"add", path, "-b", branch}
	if strings.TrimSpace(base) != "" {
		args = append(args, base)
	}
	_, err := gitWorktreeCmd(context.Background(), args...)
	return err
}

// RemoveWorktree 删除 worktree（force 处理脏状态）；branch 非空时一并删分支。
func (a *App) RemoveWorktree(path, branch string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path is required")
	}
	if _, err := gitWorktreeCmd(context.Background(), "remove", path, "--force"); err != nil {
		return err
	}
	if strings.TrimSpace(branch) != "" {
		if _, err := gitCmd(context.Background(), "branch", "-D", branch); err != nil {
			return fmt.Errorf("worktree removed but failed to delete branch %s: %w", branch, err)
		}
	}
	return nil
}
