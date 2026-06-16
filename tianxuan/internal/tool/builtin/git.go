// Package builtin — Git tools (native replacement for "bash git ...").
//
// These tools parse structured git output so the model never has to interpret
// raw terminal text. Four tools cover the daily workflow: status (what's dirty),
// diff (what changed, line by line), commit (stage+commit), log (history).
package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"tianxuan/internal/tool"
)

// --- shared helpers ----------------------------------------------------------

func init() {
	tool.RegisterBuiltin(gitStatus{})
	tool.RegisterBuiltin(gitDiff{})
	tool.RegisterBuiltin(gitCommit{})
	tool.RegisterBuiltin(gitLog{})
}

// runGit runs `git <args>` and returns stdout. stderr is folded into the error
// on non-zero exit.
func runGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
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

// --- git_status --------------------------------------------------------------

// gitStatus reports the working tree status in a structured format: branch name,
// ahead/behind counts, staged files, unstaged files, untracked files, and merge
// conflict files. Each file entry includes the path and a short status code.
type gitStatus struct{}

func (gitStatus) Name() string        { return "git_status" }
func (gitStatus) ReadOnly() bool      { return true }
func (gitStatus) Description() string {
	return "Show working tree status — branch, staged/unstaged/untracked files, merge conflicts. Structured output, each file with path and status."
}
func (gitStatus) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}
func (gitStatus) CompactDescription() string { return compactDesc["git_status"] }
func (gitStatus) CompactSchema() json.RawMessage   { return compactSchema["git_status"] }

func (gitStatus) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	// Branch info
	branchRaw, err := runGit(ctx, "branch", "--show-current")
	if err != nil {
		// Not a git repo
		return "", fmt.Errorf("not a git repository (or git not installed)")
	}
	branch := strings.TrimSpace(branchRaw)

	// Ahead/behind
	var aheadBehind string
	if branch != "" {
		abRaw, err := runGit(ctx, "rev-list", "--left-right", "--count",
			"HEAD...@{upstream}")
		if err == nil {
			parts := strings.Fields(strings.TrimSpace(abRaw))
			if len(parts) == 2 && parts[0] != "0" && parts[1] != "0" {
				aheadBehind = fmt.Sprintf(" (ahead %s, behind %s)", parts[0], parts[1])
			} else if len(parts) == 2 && parts[0] != "0" {
				aheadBehind = fmt.Sprintf(" (ahead %s)", parts[0])
			} else if len(parts) == 2 && parts[1] != "0" {
				aheadBehind = fmt.Sprintf(" (behind %s)", parts[1])
			}
		}
	}

	// Full status --porcelain
	statusRaw, err := runGit(ctx, "status", "--porcelain")
	if err != nil {
		return "", err
	}

	var staged, unstaged, untracked, conflicts []string
	for _, line := range strings.Split(strings.TrimRight(statusRaw, "\n"), "\n") {
		line = strings.TrimRight(line, "\n")
		if len(line) < 3 {
			continue
		}
		xy := line[:2]
		path := strings.TrimSpace(line[2:])
		// Remove "R " prefix for renames: "R  old.go -> new.go"
		if strings.HasPrefix(xy, "R") {
			parts := strings.SplitN(path, " -> ", 2)
			if len(parts) == 2 {
				path = parts[1]
			}
		}

		// Index (staging area) column
		switch xy[0] {
		case '?':
			untracked = append(untracked, path)
			continue
		case 'U', 'D', 'A', 'M':
			staged = append(staged, fmt.Sprintf("%s [%c]", path, xy[0]))
		}

		// Working tree column
		switch xy[1] {
		case 'M':
			unstaged = append(unstaged, path+" [modified]")
		case 'D':
			unstaged = append(unstaged, path+" [deleted]")
		case '?':
			// already handled above
		case 'U':
			conflicts = append(conflicts, path)
		}
	}

	sort.Strings(staged)
	sort.Strings(unstaged)
	sort.Strings(untracked)
	sort.Strings(conflicts)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("branch: %s%s\n", branch, aheadBehind))
	if len(staged) > 0 {
		b.WriteString(fmt.Sprintf("\nstaged (%d):\n", len(staged)))
		for _, f := range staged {
			b.WriteString("  " + f + "\n")
		}
	}
	if len(unstaged) > 0 {
		b.WriteString(fmt.Sprintf("\nunstaged (%d):\n", len(unstaged)))
		for _, f := range unstaged {
			b.WriteString("  " + f + "\n")
		}
	}
	if len(untracked) > 0 {
		b.WriteString(fmt.Sprintf("\nuntracked (%d):\n", len(untracked)))
		for _, f := range untracked {
			b.WriteString("  " + f + "\n")
		}
	}
	if len(conflicts) > 0 {
		b.WriteString(fmt.Sprintf("\n!!! merge conflicts (%d):\n", len(conflicts)))
		for _, f := range conflicts {
			b.WriteString("  " + f + "\n")
		}
	}
	if len(staged)+len(unstaged)+len(untracked)+len(conflicts) == 0 {
		b.WriteString("clean — nothing to commit, working tree clean\n")
	}
	return b.String(), nil
}

// --- git_diff ----------------------------------------------------------------

// gitDiff shows line-level changes. Without --staged it diffs the working tree
// against the index (unstaged changes); with --staged it diffs the index against
// HEAD (staged changes). An optional path filters to one file.
type gitDiff struct{}

func (gitDiff) Name() string        { return "git_diff" }
func (gitDiff) ReadOnly() bool      { return true }
func (gitDiff) Description() string {
	return "Show line-level diff for working tree changes. --staged shows staged (index) diff; path limits to one file."
}
func (gitDiff) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"staged":{"type":"boolean","description":"if true, diff staged changes against HEAD (default: unstaged vs index)"},"path":{"type":"string","description":"optional file path to limit the diff to"}}}`)
}
func (gitDiff) CompactDescription() string { return compactDesc["git_diff"] }
func (gitDiff) CompactSchema() json.RawMessage   { return compactSchema["git_diff"] }

func (gitDiff) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Staged bool   `json:"staged"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	gitArgs := []string{"diff"}
	if p.Staged {
		gitArgs = append(gitArgs, "--staged")
	}
	gitArgs = append(gitArgs, "--no-color")
	if p.Path != "" {
		gitArgs = append(gitArgs, "--", p.Path)
	}
	out, err := runGit(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) == "" {
		return "no diff", nil
	}
	return out, nil
}

// --- git_commit --------------------------------------------------------------

// gitCommit stages (if flag is set) and commits. When message is empty it
// generates one from the staged diff summary.
type gitCommit struct{}

func (gitCommit) Name() string        { return "git_commit" }
func (gitCommit) ReadOnly() bool      { return false }
func (gitCommit) Description() string {
	return "Commit staged changes. If --staged-all is set, stage all tracked files first. With --amend, amend the last commit. With an empty message, auto-generates one from the diff summary."
}
func (gitCommit) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"message":{"type":"string","description":"commit message (empty = auto-generate from diff)"},"stage_all":{"type":"boolean","description":"if true, run ` + "`git add -u`" + ` before committing"},"amend":{"type":"boolean","description":"if true, amend the last commit instead of creating a new one"}},"required":[]}`)
}
func (gitCommit) CompactDescription() string { return compactDesc["git_commit"] }
func (gitCommit) CompactSchema() json.RawMessage   { return compactSchema["git_commit"] }

func (gitCommit) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Message  string `json:"message"`
		StageAll bool   `json:"stage_all"`
		Amend    bool   `json:"amend"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	// Optional: stage all tracked changes first
	if p.StageAll {
		if _, err := runGit(ctx, "add", "-u"); err != nil {
			return "", fmt.Errorf("stage all: %w", err)
		}
	}

	// Check there's something to commit
	status, err := runGit(ctx, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	hasStaged := false
	for _, line := range strings.Split(strings.TrimRight(status, "\n"), "\n") {
		if len(line) >= 2 && line[0] != ' ' && line[0] != '?' {
			hasStaged = true
			break
		}
	}
	if !hasStaged && !p.Amend {
		return "nothing to commit — no staged changes", nil
	}

	commitArgs := []string{"commit"}
	if p.Amend {
		commitArgs = append(commitArgs, "--amend")
	}

	// Use provided message or generate one
	if p.Message != "" {
		commitArgs = append(commitArgs, "-m", p.Message)
	} else if !p.Amend {
		// Auto-generate a message from the staged diff stat
		stat, err := runGit(ctx, "diff", "--staged", "--stat")
		if err != nil {
			return "", err
		}
		msg := autoCommitMessage(stat)
		commitArgs = append(commitArgs, "-m", msg)
	} else {
		// Amend with --no-edit to keep the existing message
		commitArgs = append(commitArgs, "--no-edit")
	}

	out, err := runGit(ctx, commitArgs...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// autoCommitMessage generates a conventional-commit-style message from diff stat.
func autoCommitMessage(stat string) string {
	lines := strings.Split(strings.TrimSpace(stat), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return "chore: update"
	}

	// Count file types
	var goFiles, tsFiles, mdFiles, cfgFiles, otherFiles int
	for _, line := range lines {
		if !strings.Contains(line, "|") {
			continue
		}
		path := strings.TrimSpace(strings.SplitN(line, "|", 2)[0])
		path = strings.TrimSpace(path)
		switch {
		case strings.HasSuffix(path, ".go"):
			goFiles++
		case strings.HasSuffix(path, ".ts"), strings.HasSuffix(path, ".tsx"),
			strings.HasSuffix(path, ".js"), strings.HasSuffix(path, ".jsx"):
			tsFiles++
		case strings.HasSuffix(path, ".md"):
			mdFiles++
		case strings.HasSuffix(path, ".toml"), strings.HasSuffix(path, ".json"),
			strings.HasSuffix(path, ".yaml"), strings.HasSuffix(path, ".yml"):
			cfgFiles++
		default:
			otherFiles++
		}
	}

	// Build type and scope
	scope := ""
	msgType := "chore"
	if goFiles > 0 {
		msgType = "feat"
		if goFiles > tsFiles+mdFiles+cfgFiles+otherFiles {
			scope = "(core)"
		}
	}
	if tsFiles > 0 && tsFiles >= goFiles {
		msgType = "feat"
		scope = "(frontend)"
	}
	if mdFiles > 0 && mdFiles >= goFiles+tsFiles {
		msgType = "docs"
	}
	if cfgFiles > 0 && cfgFiles >= goFiles+tsFiles+mdFiles {
		msgType = "chore"
		scope = "(config)"
	}

	changes := statSummary(lines)
	return fmt.Sprintf("%s%s: %s", msgType, scope, changes)
}

func statSummary(lines []string) string {
	// Count total insertions/deletions from the diff stat footer
	var ins, del int
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "insertion") {
			if n, err := strconv.Atoi(extractNum(line)); err == nil {
				ins += n
			}
		}
		if strings.Contains(line, "deletion") {
			if n, err := strconv.Atoi(extractNum(line)); err == nil {
				del += n
			}
		}
	}

	files := len(lines)
	if files == 0 {
		return "update"
	}

	parts := []string{fmt.Sprintf("%d file(s)", files)}
	if ins > 0 {
		parts = append(parts, fmt.Sprintf("%d insertions", ins))
	}
	if del > 0 {
		parts = append(parts, fmt.Sprintf("%d deletions", del))
	}
	return strings.Join(parts, ", ")
}

func extractNum(s string) string {
	var num string
	for _, c := range s {
		if c >= '0' && c <= '9' {
			num += string(c)
		} else if num != "" {
			break
		}
	}
	return num
}

// --- git_log -----------------------------------------------------------------

// gitLog shows the recent commit log in a compact, structured format.
type gitLog struct{}

func (gitLog) Name() string        { return "git_log" }
func (gitLog) ReadOnly() bool      { return true }
func (gitLog) Description() string {
	return "Show commit history (default: last 10 commits). Supports count, file filter, and author filter."
}
func (gitLog) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer","description":"number of commits to show (default 10)"},"path":{"type":"string","description":"filter by file path"},"author":{"type":"string","description":"filter by author (substring match)"}}}`)
}
func (gitLog) CompactDescription() string { return compactDesc["git_log"] }
func (gitLog) CompactSchema() json.RawMessage   { return compactSchema["git_log"] }

func (gitLog) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Count  int    `json:"count"`
		Path   string `json:"path"`
		Author string `json:"author"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	if p.Count <= 0 {
		p.Count = 10
	}
	if p.Count > 100 {
		p.Count = 100
	}

	// Use a pretty format that's easy to parse
	format := "--pretty=format:commit %h%nAuthor: %an <%ae>%nDate: %ad%n%n%w(0,4,4)%B%n---%n"
	gitArgs := []string{"log", fmt.Sprintf("-%d", p.Count), "--date=format:%Y-%m-%d %H:%M", format}
	if p.Author != "" {
		gitArgs = append(gitArgs, "--author", p.Author)
	}
	if p.Path != "" {
		gitArgs = append(gitArgs, "--", p.Path)
	}

	out, err := runGit(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
