package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(editLines{}) }

// editLines replaces a range of lines in a file by line number. Useful when
// read_file's line-numbered output makes it natural to target "lines 42-45".
// roots confines the target to the workspace; workDir resolves relative paths.
type editLines struct {
	roots   []string
	workDir string
}

func (editLines) Name() string { return "edit_lines" }

func (editLines) Description() string {
	return "Replace a range of lines in a file by 1-based line numbers. Use after read_file when you know the exact line range to replace (e.g. start_line=42, end_line=45). new_content becomes the replacement (may be empty to delete lines). The file's original line endings are preserved. start_anchor/end_anchor are the expected exact content of the start/end lines (without trailing newline); a mismatch rejects the edit without writing, protecting against stale line numbers after prior edits. After writing, .go files are syntax-checked with gofmt -e and .ts/.tsx files with a project-local tsc --noEmit --skipLibCheck; a failed check rolls the file back (set validate=false to skip). Prefer edit_file for single-string replacements - this tool is for line-range edits."
}

func (editLines) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path"},"start_line":{"type":"integer","description":"1-based start line (inclusive)","minimum":1},"end_line":{"type":"integer","description":"1-based end line (inclusive)","minimum":1},"new_content":{"type":"string","description":"Replacement text for the line range (may be empty to delete lines). The file's original line endings are preserved."},"start_anchor":{"type":"string","description":"Expected exact content of the start_line (without trailing newline). A mismatch rejects the edit without writing."},"end_anchor":{"type":"string","description":"Expected exact content of the end_line (without trailing newline). A mismatch rejects the edit without writing."},"validate":{"type":"boolean","description":"Post-edit quick syntax/type check with automatic rollback on failure. Default true: .go is checked with gofmt -e, .ts/.tsx with a project-local tsc --noEmit --skipLibCheck."}},"required":["path","start_line","end_line","new_content"]}`)
}

func (editLines) ReadOnly() bool { return false }
func (editLines) Kind() tool.ToolKind { return tool.KindEdit }

func (editLines) CompactDescription() string { return compactDesc["edit_lines"] }
func (editLines) CompactSchema() json.RawMessage   { return compactSchema["edit_lines"] }

func (el editLines) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path        string `json:"path"`
		StartLine   int    `json:"start_line"`
		EndLine     int    `json:"end_line"`
		NewContent  string `json:"new_content"`
		OldString   string `json:"old_string"`
		StartAnchor string `json:"start_anchor"`
		EndAnchor   string `json:"end_anchor"`
		Validate    *bool  `json:"validate"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if p.StartLine < 1 {
		return "", fmt.Errorf("start_line must be >= 1")
	}
	if p.EndLine < p.StartLine {
		return "", fmt.Errorf("end_line (%d) must be >= start_line (%d)", p.EndLine, p.StartLine)
	}
	p.Path = resolveIn(el.workDir, p.Path)
	if err := confine(el.roots, p.Path); err != nil {
		return "", err
	}

	originalContent, enc, err := readFileEncoded(p.Path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", p.Path, err)
	}
	content := originalContent

	// Detect and preserve the file's dominant line ending style.
	fileLE := detectLineEnding(content)
	if fileLE == "" {
		fileLE = "\n" // default for files with no newlines
	}

	// Normalise CRLF to LF before splitting so line numbers match read_file
	// (which scans on \n) even for mixed-line-ending files, and so a \r can
	// never leak into the output as \r\r\n. The join below re-applies fileLE.
	content = strings.ReplaceAll(content, "\r\n", "\n")
	hasTrailingNL := strings.HasSuffix(content, "\n")
	lines := strings.Split(content, "\n")
	// If the file ends with the line ending, Split produces an empty trailing
	// element — trim it so line numbers work correctly.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	totalLines := len(lines)
	if p.StartLine > totalLines {
		return "", fmt.Errorf("start_line %d exceeds file length (%d lines)", p.StartLine, totalLines)
	}
	if p.EndLine > totalLines {
		p.EndLine = totalLines // clamp
	}

	// The model often passes old_string together with line numbers (a habit
	// from text-anchor edit tools). It is NOT a locator here — line numbers
	// win — so verify it against the actual range: a mismatch means the
	// numbers are stale/wrong, and silently proceeding would replace (and
	// swallow) the wrong lines. Fail loudly and leave the file untouched.
	if p.OldString != "" {
		old := strings.ReplaceAll(p.OldString, "\r\n", "\n")
		old = strings.TrimSuffix(old, "\n")
		rangeText := strings.Join(lines[p.StartLine-1:p.EndLine], "\n")
		if old != rangeText {
			preview := rangeText
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			return "", fmt.Errorf(
				"old_string does not match lines %d-%d (actual range: %q); "+
					"re-read the file with read_file and re-check start_line/end_line, "+
					"or use edit_file for exact-string edits",
				p.StartLine, p.EndLine, preview)
		}
	}

	// Content anchors: the model asserts what the start/end lines contain right
	// now. A mismatch means the line numbers are stale (prior edits shifted the
	// file), and proceeding would edit the wrong lines - reject without writing.
	if p.StartAnchor != "" {
		want := normalizeAnchorLine(p.StartAnchor)
		got := normalizeAnchorLine(lines[p.StartLine-1])
		if want != got {
			return "", fmt.Errorf(
				"start_anchor does not match line %d (actual: %q); "+
					"re-read the file with read_file and re-check start_line/end_line",
				p.StartLine, got)
		}
	}
	if p.EndAnchor != "" {
		want := normalizeAnchorLine(p.EndAnchor)
		got := normalizeAnchorLine(lines[p.EndLine-1])
		if want != got {
			return "", fmt.Errorf(
				"end_anchor does not match line %d (actual: %q); "+
					"re-read the file with read_file and re-check start_line/end_line",
				p.EndLine, got)
		}
	}

	// Build the new file: lines before the range + new_content + lines after.
	var out []string
	out = append(out, lines[:p.StartLine-1]...)

	if p.NewContent != "" {
		// Normalise CRLF in new_content to LF so joining with the file's line
		// ending never produces \r\r\n or mixed line endings. The trailing
		// empty element from a new_content ending in \n is trimmed so it can't
		// inject a blank line mid-file (the join below re-applies fileLE).
		newContent := strings.ReplaceAll(p.NewContent, "\r\n", "\n")
		newLines := strings.Split(newContent, "\n")
		if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
			newLines = newLines[:len(newLines)-1]
		}
		out = append(out, newLines...)
	}

	out = append(out, lines[p.EndLine:]...)

	// Rejoin with the file's original line ending, preserving trailing newline.
	result := strings.Join(out, fileLE)
	// `out` 以空字符串结尾表示一个真实的空行（如 "a\n\n" 的第 2 行），
	// Join 只会在元素之间插入分隔符，此时仍需追加换行，否则末尾空行会被吞掉。
	if hasTrailingNL && len(out) > 0 {
		result += fileLE
	}

	// Preserve original file permissions.
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(p.Path); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := writeFileEncoded(p.Path, result, enc, mode); err != nil {
		return "", fmt.Errorf("write %s: %w", p.Path, err)
	}
	if p.Validate == nil || *p.Validate {
		if out, verr := validateEditedFile(ctx, p.Path); verr != nil {
			if werr := writeFileEncoded(p.Path, originalContent, enc, mode); werr != nil {
				return "", fmt.Errorf("post-edit validation failed (%v: %s); rollback failed: %v", verr, out, werr)
			}
			return "", fmt.Errorf("post-edit validation failed: %v: %s (file rolled back)", verr, out)
		}
	}
	return fmt.Sprintf("edit_lines %s: replaced lines %d-%d (%d lines) → %d lines", p.Path, p.StartLine, p.EndLine, p.EndLine-p.StartLine+1, len(out)-len(lines)+(p.EndLine-p.StartLine+1)), nil
}

// normalizeAnchorLine normalises a model-supplied line anchor: CRLF to LF, then
// strips one trailing newline/CR so "line two" matches a line whose content is
// "line two" regardless of the file's line-ending style.
func normalizeAnchorLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	return strings.TrimSuffix(s, "\r")
}

// quickCheckTimeout caps post-edit validation so a slow tsc never blocks a turn
// indefinitely (the edit is rolled back on timeout too).
const quickCheckTimeout = 60 * time.Second

// validateEditedFile runs a quick syntax/type check appropriate to the file:
// .go is checked with gofmt -e; .ts/.tsx with a project-local tsc
// --noEmit --skipLibCheck when a tsconfig.json and node_modules/.bin/tsc exist
// above the file. A nil error means the file is valid or no check applies; the
// caller rolls the file back when err != nil.
func validateEditedFile(ctx context.Context, path string) (string, error) {
	switch {
	case strings.HasSuffix(path, ".go"):
		gofmt, err := exec.LookPath("gofmt")
		if err != nil {
			return "", nil // gofmt unavailable - skip rather than block edits
		}
		return runQuickCheck(ctx, gofmt, "-e", path)
	case strings.HasSuffix(path, ".ts"), strings.HasSuffix(path, ".tsx"):
		tsc, err := findLocalTsc(path)
		if err != nil {
			return "", nil // no local tsc - skip
		}
		return runQuickCheckTsc(ctx, tsc)
	}
	return "", nil
}

// runQuickCheck runs name with args under a 60s timeout and returns trimmed
// combined output; a non-zero exit is returned as an error.
func runQuickCheck(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, quickCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

// runQuickCheckTsc runs the project-local tsc against its project. On Windows
// the .cmd shim must go through cmd.exe (CreateProcess cannot run batch files).
func runQuickCheckTsc(ctx context.Context, tsc string) (string, error) {
	args := []string{"--noEmit", "--skipLibCheck", "-p", filepath.Dir(filepath.Dir(tsc))}
	if runtime.GOOS == "windows" {
		return runQuickCheck(ctx, "cmd", append([]string{"/c", tsc}, args...)...)
	}
	return runQuickCheck(ctx, tsc, args...)
}

// findLocalTsc walks up from path to the nearest tsconfig.json and requires a
// project-local node_modules/.bin/tsc there. Not found returns an error, which
// the caller treats as "skip the check".
func findLocalTsc(path string) (string, error) {
	dir := filepath.Dir(path)
	for {
		if _, err := os.Stat(filepath.Join(dir, "tsconfig.json")); err == nil {
			bin := "tsc"
			if runtime.GOOS == "windows" {
				bin = "tsc.cmd"
			}
			candidate := filepath.Join(dir, "node_modules", ".bin", bin)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
			return "", fmt.Errorf("tsconfig.json found but no local %s", candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no tsconfig.json above %s", path)
		}
		dir = parent
	}
}
