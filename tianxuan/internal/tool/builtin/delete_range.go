package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"tianxuan/internal/diff"
	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(deleteRange{}) }

type deleteRange struct {
	roots   []string
	workDir string
}

func (deleteRange) Name() string { return "delete_range" }

func (deleteRange) Description() string {
	return "Delete a contiguous text range from a file using exact start/end text anchors. Each anchor must match exactly one line. Returns unified diff on success. Use for large deletions — smaller changes should use edit_file."
}

func (deleteRange) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"path":{"type":"string","description":"File path"},
			"start_anchor":{"type":"string","description":"Exact text of the first line to delete (must be unique in the file)"},
			"end_anchor":{"type":"string","description":"Exact text of the last line to delete (must be unique in the file)"},
			"inclusive":{"type":"boolean","description":"Whether to include the anchor lines in the deletion (default true)"}
		},
		"required":["path","start_anchor","end_anchor"]
	}`)
}

func (deleteRange) ReadOnly() bool { return false }
func (deleteRange) Kind() tool.ToolKind { return tool.KindDelete }

func (deleteRange) CompactDescription() string { return compactDesc["delete_range"] }
func (deleteRange) CompactSchema() json.RawMessage   { return compactSchema["delete_range"] }

func (d deleteRange) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	change, err := d.preview(args)
	if err != nil {
		return "", err
	}
	// Preserve original file encoding so non-UTF-8 files (GB18030, UTF-16, etc.)
	// are written back in their original charset.
	_, enc, _ := readFileEncoded(change.Path)
	// Preserve original file permissions.
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(change.Path); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := writeFileEncoded(change.Path, change.NewText, enc, mode); err != nil {
		return "", fmt.Errorf("write %s: %w", change.Path, err)
	}
	return change.Diff, nil
}

func (d deleteRange) Preview(args json.RawMessage) (diff.Change, error) {
	return d.preview(args)
}

func (d deleteRange) preview(args json.RawMessage) (diff.Change, error) {
	var p struct {
		Path        string `json:"path"`
		StartAnchor string `json:"start_anchor"`
		EndAnchor   string `json:"end_anchor"`
		Inclusive   *bool  `json:"inclusive"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return diff.Change{}, fmt.Errorf("invalid args: %w", err)
	}
	if p.Path == "" {
		return diff.Change{}, fmt.Errorf("path is required")
	}
	if p.StartAnchor == "" {
		return diff.Change{}, fmt.Errorf("start_anchor is required")
	}
	if p.EndAnchor == "" {
		return diff.Change{}, fmt.Errorf("end_anchor is required")
	}

	inclusive := true
	if p.Inclusive != nil {
		inclusive = *p.Inclusive
	}

	p.Path = resolveIn(d.workDir, p.Path)
	if err := confine(d.roots, p.Path); err != nil {
		return diff.Change{}, err
	}

	b, err := os.ReadFile(p.Path)
	if err != nil {
		return diff.Change{}, fmt.Errorf("read %s: %w", p.Path, err)
	}
	original := string(b)

	// Detect line ending style so we can preserve it on write.
	lineSep := "\n"
	if strings.Contains(original, "\r\n") {
		lineSep = "\r\n"
	}

	// Strip \r\n to \n for matching (normalize CRLF→LF without removing
	// legitimate \r characters inside string literals or comments).
	lines := strings.Split(strings.ReplaceAll(original, "\r\n", "\n"), "\n")
	startLine := findUniqueLine(lines, p.StartAnchor)
	if startLine == -2 {
		return diff.Change{}, fmt.Errorf("start_anchor is not unique in %s; add more surrounding context", p.Path)
	}
	if startLine == -1 {
		return diff.Change{}, fmt.Errorf("start_anchor not found in %s", p.Path)
	}
	endLine := findUniqueLine(lines, p.EndAnchor)
	if endLine == -2 {
		return diff.Change{}, fmt.Errorf("end_anchor is not unique in %s; add more surrounding context", p.Path)
	}
	if endLine == -1 {
		return diff.Change{}, fmt.Errorf("end_anchor not found in %s", p.Path)
	}
	if startLine > endLine {
		return diff.Change{}, fmt.Errorf("start_anchor appears after end_anchor (lines %d and %d)", startLine+1, endLine+1)
	}

	// V10.28: 大括号完整性校验 — 防止删除跨越不完整代码块
	deleteStart := startLine
	deleteEnd := endLine
	if !inclusive {
		deleteStart = startLine + 1
		deleteEnd = endLine - 1
	}
	if err := validateBraceCompleteDeletion(lines, deleteStart, deleteEnd, p.Path); err != nil {
		return diff.Change{}, err
	}

	// Build new content
	var keep []string
	if inclusive {
		keep = append(keep, lines[:startLine]...)
		keep = append(keep, lines[endLine+1:]...)
	} else {
		// Same line for both anchors: the kept prefix and suffix would overlap at
		// that line and duplicate it. There is nothing strictly between a line and
		// itself, so the exclusive deletion is contradictory — reject it.
		if startLine == endLine {
			return diff.Change{}, fmt.Errorf("start_anchor and end_anchor match the same line in %s; with inclusive=false there is nothing between them to delete", p.Path)
		}
		keep = append(keep, lines[:startLine+1]...)
		keep = append(keep, lines[endLine:]...)
	}
	newContent := strings.Join(keep, lineSep)
	// Preserve trailing newline if original had one.
	if newContent != "" && strings.HasSuffix(original, lineSep) && !strings.HasSuffix(newContent, lineSep) {
		newContent += lineSep
	}

	return diff.Build(p.Path, original, newContent, diff.Modify), nil
}

// findUniqueLine returns the index of the line that equals target.
// Returns -1 if not found, -2 if found on multiple lines.
func findUniqueLine(lines []string, target string) int {
	idx := -1
	for i, l := range lines {
		if l == target {
			if idx >= 0 {
				return -2
			}
			idx = i
		}
	}
	return idx
}

// ── Brace-complete deletion validation (V10.28) ──────────────────────
// Ported from Reasonix V1.15 (MIT). Prevents delete_range from cutting a
// code block in half (deleting opening { but not closing }, or vice versa).

func validateBraceCompleteDeletion(lines []string, deleteStart, deleteEnd int, path string) error {
	for _, pair := range bracePairsByLine(lines) {
		if pair.openLine < 0 || pair.closeLine < 0 {
			continue
		}
		openDeleted := pair.openLine >= deleteStart && pair.openLine <= deleteEnd
		closeDeleted := pair.closeLine >= deleteStart && pair.closeLine <= deleteEnd
		switch {
		case openDeleted && !closeDeleted:
			if pair.openLine == deleteEnd {
				return fmt.Errorf("end_anchor in %s appears to open a code block at line %d; delete_range would delete that header but leave its closing line %d outside the range. Use an end_anchor on the block's closing line, or use edit_file/multi_edit", path, pair.openLine+1, pair.closeLine+1)
			}
			return fmt.Errorf("delete_range in %s would cut a code block: opening brace at line %d is deleted but closing brace at line %d is kept. Choose anchors that include the whole block, or use edit_file/multi_edit", path, pair.openLine+1, pair.closeLine+1)
		case !openDeleted && closeDeleted:
			return fmt.Errorf("delete_range in %s would cut a code block: closing brace at line %d is deleted but opening brace at line %d is kept. Choose anchors that include the whole block, or use edit_file/multi_edit", path, pair.closeLine+1, pair.openLine+1)
		}
	}
	return nil
}

type bracePair struct {
	openLine  int
	closeLine int
}

func bracePairsByLine(lines []string) []bracePair {
	var pairs []bracePair
	var stack []int
	inBlockComment := false
	var quote byte
	escaped := false
	for lineNo, line := range lines {
		for i := 0; i < len(line); i++ {
			c := line[i]
			if inBlockComment {
				if c == '*' && i+1 < len(line) && line[i+1] == '/' {
					inBlockComment = false
					i++
				}
				continue
			}
			if quote != 0 {
				if escaped {
					escaped = false
					continue
				}
				if c == '\\' {
					escaped = true
					continue
				}
				if c == quote {
					quote = 0
				}
				continue
			}
			if c == '/' && i+1 < len(line) {
				switch line[i+1] {
				case '/':
					i = len(line)
					continue
				case '*':
					inBlockComment = true
					i++
					continue
				}
			}
			switch c {
			case '\'', '"', '`':
				quote = c
			case '{':
				stack = append(stack, lineNo)
			case '}':
				if len(stack) == 0 {
					pairs = append(pairs, bracePair{openLine: -1, closeLine: lineNo})
					continue
				}
				openLine := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				pairs = append(pairs, bracePair{openLine: openLine, closeLine: lineNo})
			}
		}
		if quote == '\'' || quote == '"' {
			quote = 0
			escaped = false
		}
	}
	for _, openLine := range stack {
		pairs = append(pairs, bracePair{openLine: openLine, closeLine: -1})
	}
	return pairs
}
