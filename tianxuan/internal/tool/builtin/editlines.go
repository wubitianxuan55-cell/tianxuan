package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
	return "Replace a range of lines in a file by 1-based line numbers. Use after read_file when you know the exact line range to replace (e.g. start_line=42, end_line=45). new_content becomes the replacement (may be empty to delete lines). The file's original line endings are preserved. Prefer edit_file for single-string replacements — this tool is for line-range edits."
}

func (editLines) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path"},"start_line":{"type":"integer","description":"1-based start line (inclusive)","minimum":1},"end_line":{"type":"integer","description":"1-based end line (inclusive)","minimum":1},"new_content":{"type":"string","description":"Replacement text for the line range (may be empty to delete lines). The file's original line endings are preserved."}},"required":["path","start_line","end_line","new_content"]}`)
}

func (editLines) ReadOnly() bool { return false }

func (editLines) CompactDescription() string { return compactDesc["edit_lines"] }
func (editLines) CompactSchema() json.RawMessage   { return compactSchema["edit_lines"] }

func (el editLines) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path       string `json:"path"`
		StartLine  int    `json:"start_line"`
		EndLine    int    `json:"end_line"`
		NewContent string `json:"new_content"`
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

	b, err := os.ReadFile(p.Path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", p.Path, err)
	}
	content := string(b)

	// Detect and preserve the file's line ending style.
	fileLE := detectLineEnding(content)
	if fileLE == "" {
		fileLE = "\n" // default for files with no newlines
	}

	// Split preserving trailing newline if present.
	hasTrailingNL := strings.HasSuffix(content, fileLE) || strings.HasSuffix(content, "\n")
	lines := strings.Split(content, fileLE)
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

	// Build the new file: lines before the range + new_content + lines after.
	var out []string
	out = append(out, lines[:p.StartLine-1]...)

	if p.NewContent != "" {
		// Split new_content by \n (canonical) and join with the file's line ending.
		newLines := strings.Split(p.NewContent, "\n")
		out = append(out, newLines...)
	}

	out = append(out, lines[p.EndLine:]...)

	// Rejoin with the file's original line ending, preserving trailing newline.
	result := strings.Join(out, fileLE)
	if hasTrailingNL && len(out) > 0 && out[len(out)-1] != "" {
		result += fileLE
	}

	// Preserve original file permissions.
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(p.Path); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.WriteFile(p.Path, []byte(result), mode); err != nil {
		return "", fmt.Errorf("write %s: %w", p.Path, err)
	}
	return fmt.Sprintf("edit_lines %s: replaced lines %d-%d (%d lines) → %d lines", p.Path, p.StartLine, p.EndLine, p.EndLine-p.StartLine+1, len(out)-len(lines)+(p.EndLine-p.StartLine+1)), nil
}
