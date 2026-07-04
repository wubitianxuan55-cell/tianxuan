package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(editFile{}) }

// editFile replaces an exact string in a file. roots confines the target to the
// workspace when non-empty (see writeFile); workDir, when non-empty, is the
// directory a relative path resolves against (see resolveIn).
type editFile struct {
	roots   []string
	workDir string
}

func (editFile) Name() string { return "edit_file" }

func (editFile) Description() string {
	return "Replace an exact string in a file with another. old_string must occur exactly once; add surrounding context to disambiguate. Line endings are auto-adapted: if the file uses CRLF, your LF old_string/new_string are automatically converted. Use for targeted edits instead of rewriting the whole file."
}

func (editFile) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path"},"old_string":{"type":"string","description":"Exact text to replace (must be unique in the file). Line endings auto-adapted."},"new_string":{"type":"string","description":"Replacement text (may be empty to delete). Line endings auto-adapted."}},"required":["path","old_string","new_string"]}`)
}

func (editFile) ReadOnly() bool { return false }

func (editFile) CompactDescription() string { return compactDesc["edit_file"] }
func (editFile) CompactSchema() json.RawMessage   { return compactSchema["edit_file"] }

// detectLineEnding reports the dominant line-ending style in content.
// Returns "\r\n" for CRLF, "\n" for LF, "" for no-newlines.
func detectLineEnding(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	if strings.Contains(content, "\n") {
		return "\n"
	}
	return ""
}

// adaptLineEndings replaces standalone \n (not preceded by \r) with the target
// line ending. This prevents the most common edit_file failure: the LLM sends
// old_string with LF, but the file uses CRLF (or vice versa).
func adaptLineEndings(s string, target string) string {
	if target == "\n" || target == "" {
		return s // nothing to adapt — LF is the canonical form
	}
	// Replace \n that is NOT preceded by \r with \r\n
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && (i == 0 || s[i-1] != '\r') {
			b.WriteString(target)
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func (e editFile) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if p.OldString == "" {
		return "", fmt.Errorf("old_string is required")
	}
	p.Path = resolveIn(e.workDir, p.Path)
	if err := confine(e.roots, p.Path); err != nil {
		return "", err
	}

	content, enc, err := readFileEncoded(p.Path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", p.Path, err)
	}

	result := applyOldStringEdit(content, p.OldString, p.NewString, false)
	switch {
	case result.matches == 0:
		return "", oldStringNotFoundError(p.Path, p.OldString, content)
	case result.matches > 1:
		return "", oldStringNotUniqueError(p.Path, p.OldString, content, result.matches, false)
	}

	// Preserve the original file permission bits (e.g. executable scripts).
	fi, statErr := os.Stat(p.Path)
	mode := os.FileMode(0o644)
	if statErr == nil {
		mode = fi.Mode().Perm()
	}
	if err := writeFileEncoded(p.Path, result.updated, enc, mode); err != nil {
		return "", fmt.Errorf("write %s: %w", p.Path, err)
	}
	return fmt.Sprintf("edited %s", p.Path), nil
}
