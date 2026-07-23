package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(gitWorktree{}) }

// gitWorktree manages git worktrees for isolated parallel development.
// Supports add (create a new worktree on a branch), remove (delete a worktree),
// and list (show all worktrees).
type gitWorktree struct{}

func (gitWorktree) Name() string { return "git_worktree" }

func (gitWorktree) Description() string {
	return "Manage git worktrees for isolated parallel development. Use add to create a new worktree on a branch (with an optional base commit/branch), remove to delete a worktree (and optionally its branch), and list to show all worktrees with their branches and paths."
}

func (gitWorktree) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "action":{"type":"string","enum":["add","remove","list"],"description":"add: create a new worktree. remove: delete a worktree. list: show all worktrees."},
  "path":{"type":"string","description":"For add: the directory path for the new worktree (parent dir will be created). For remove: the path of the worktree to delete."},
  "branch":{"type":"string","description":"For add: name of the new branch. For remove: if set, also delete the branch with git branch -D."},
  "base":{"type":"string","description":"For add: base commit/branch/tag for the new worktree (default: HEAD)."}
},
"required":["action"]
}`)
}

func (gitWorktree) ReadOnly() bool { return false }
func (gitWorktree) Kind() tool.ToolKind { return tool.KindExecute }

func (gitWorktree) CompactDescription() string { return compactDesc["git_worktree"] }
func (gitWorktree) CompactSchema() json.RawMessage   { return compactSchema["git_worktree"] }

func (gitWorktree) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Action string `json:"action"`
		Path   string `json:"path"`
		Branch string `json:"branch"`
		Base   string `json:"base"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	switch p.Action {
	case "add":
		return worktreeAdd(ctx, p.Path, p.Branch, p.Base)
	case "remove":
		return worktreeRemove(ctx, p.Path, p.Branch)
	case "list":
		return worktreeList(ctx)
	default:
		return "", fmt.Errorf("unknown action %q; use add, remove, or list", p.Action)
	}
}

func worktreeAdd(ctx context.Context, dir, branch, base string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("path is required for add")
	}
	if branch == "" {
		return "", fmt.Errorf("branch is required for add")
	}

	// Check that the parent directory exists
	parent := filepath.Dir(dir)
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		return "", fmt.Errorf("parent directory %s does not exist", parent)
	}

	args := []string{"worktree", "add", dir, "-b", branch}
	if base != "" {
		args = append(args, base)
	}
	out, err := runGit(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("worktree add: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func worktreeRemove(ctx context.Context, dir, branch string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("path is required for remove")
	}

	// First list to find the worktree
	listOut, err := runGit(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("worktree list: %w", err)
	}
	if !strings.Contains(listOut, "worktree "+filepath.ToSlash(dir)) {
		return "", fmt.Errorf("no worktree found at %s", dir)
	}

	// Remove the worktree (--force to handle dirty state)
	args := []string{"worktree", "remove", dir, "--force"}
	out, err := runGit(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("worktree remove: %w", err)
	}

	// Optionally delete the branch
	if branch != "" {
		if _, err := runGit(ctx, "branch", "-D", branch); err != nil {
			return "", fmt.Errorf("worktree removed but failed to delete branch %s: %w", branch, err)
		}
	}

	result := strings.TrimSpace(out)
	if result == "" {
		result = fmt.Sprintf("removed worktree at %s", dir)
	}
	if branch != "" {
		result += fmt.Sprintf(" (branch %s deleted)", branch)
	}
	return result, nil
}

func worktreeList(ctx context.Context) (string, error) {
	out, err := runGit(ctx, "worktree", "list")
	if err != nil {
		return "", fmt.Errorf("worktree list: %w", err)
	}
	return strings.TrimSpace(out), nil
}
