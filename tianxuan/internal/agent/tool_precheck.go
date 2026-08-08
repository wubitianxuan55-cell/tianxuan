package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"tianxuan/internal/tool/builtin"
)

// precheckTool runs a fast deterministic check before a writer tool executes.
// Returns "" when the call is likely to succeed, or a diagnostic message when
// it is predictably going to fail — saving the model an entire API roundtrip.
//
// 缓存安全: 纯运行时判断，不修改任何进入 API 消息数组的内容。
// 阻止调用时返回的消息作为本轮新的 tool_result 追加在消息末尾，
// 不改变已有前缀。
func (a *AgentRunner) precheckTool(name string, args json.RawMessage) string {
	switch name {
	case "edit_file":
		return a.precheckEditFile(args)
	case "multi_edit":
		return a.precheckMultiEdit(args)
	case "delete_range":
		return a.precheckDeleteRange(args)
	}
	return ""
}

// precheckEditFile verifies that old_string exists in the target file before
// letting edit_file run. Uses the toolCache when available; falls back to a
// direct read. This catches the single most common agent failure pattern:
// the model sends an old_string that doesn't match the current file content.
func (a *AgentRunner) precheckEditFile(raw json.RawMessage) string {
	var p struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.Path == "" || p.OldString == "" {
		return "" // can't check — let the real Execute handle it
	}

	content, ok := a.readFileForPrecheck(p.Path)
	if !ok {
		return "" // can't read — let the real Execute report the error
	}

	if strings.Contains(content, p.OldString) {
		return "" // found — let it proceed
	}
	// V10.103: try the same fuzzy matching that Execute uses.
	// Without this, precheck rejects edits that Execute could handle
	// (trailing whitespace, tab/space differences, read_file line prefixes),
	// causing the model to give up on edit_file and rewrite the whole file.
	if fuzzyPrecheckMatch(content, p.OldString) {
		return ""
	}

	// old_string not found — give the model actionable diagnostics
	preview := p.OldString
	if len([]rune(preview)) > 100 {
		preview = truncateString(preview, 100) + "..."
	}
	filePreview := content
	if len([]rune(filePreview)) > 200 {
		filePreview = truncateString(filePreview, 200) + "..."
	}
	nearest := ""
	if line, text, ok := builtin.NearestContentLine(p.OldString, content); ok {
		nearest = fmt.Sprintf("  nearest line %d: %q\n", line, text)
	}
	return fmt.Sprintf(
		"precheck blocked: old_string not found in %s.\n"+
			"  searched for: %q\n"+
			"%s"+
			"  file content (first 200 chars): %q\n"+
			"  suggestion: use read_file to see the current content, then retry with the exact string.",
		p.Path, preview, nearest, filePreview,
	)
}

// precheckMultiEdit checks each edit in a multi_edit batch against the target
// file. Returns "" when all old_strings are present.
func (a *AgentRunner) precheckMultiEdit(raw json.RawMessage) string {
	var p struct {
		Path  string `json:"path"`
		Edits []struct {
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
		} `json:"edits"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.Path == "" || len(p.Edits) == 0 {
		return ""
	}

	content, ok := a.readFileForPrecheck(p.Path)
	if !ok {
		return ""
	}

	for i, e := range p.Edits {
		if e.OldString != "" && !strings.Contains(content, e.OldString) {
			preview := e.OldString
			if len([]rune(preview)) > 80 {
				preview = truncateString(preview, 80) + "..."
			}
			nearest := ""
			if line, text, ok := builtin.NearestContentLine(e.OldString, content); ok {
				nearest = fmt.Sprintf(" nearest line %d: %q.", line, text)
			}
			return fmt.Sprintf(
				"precheck blocked: multi_edit[%d] old_string not found in %s: %q.%s Re-read the file and retry.",
				i, p.Path, preview, nearest,
			)
		}
	}
	return ""
}

// precheckDeleteRange verifies that start_anchor and end_anchor both exist in
// the target file.
func (a *AgentRunner) precheckDeleteRange(raw json.RawMessage) string {
	var p struct {
		Path        string `json:"path"`
		StartAnchor string `json:"start_anchor"`
		EndAnchor   string `json:"end_anchor"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.Path == "" {
		return ""
	}
	if p.StartAnchor == "" || p.EndAnchor == "" {
		return ""
	}

	content, ok := a.readFileForPrecheck(p.Path)
	if !ok {
		return ""
	}

	missing := []string{}
	if !strings.Contains(content, p.StartAnchor) {
		missing = append(missing, "start_anchor")
	}
	if !strings.Contains(content, p.EndAnchor) {
		missing = append(missing, "end_anchor")
	}
	if len(missing) == 0 {
		return ""
	}

	return fmt.Sprintf(
		"precheck blocked: %s not found in %s. Re-read the file and retry.",
		strings.Join(missing, " and "), p.Path,
	)
}

// readFileForPrecheck returns file content for precheck purposes. Uses the
// toolCache when available (faster, no disk IO); falls back to a direct read.
func (a *AgentRunner) readFileForPrecheck(path string) (string, bool) {
	// Try the tool cache first (cached read_file results from this turn).
	if a.tc != nil {
		if content, hit := a.tc.Get(path, 0); hit {
			return content, true
		}
	}
	// Fall back to a direct read.
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	content := string(b)
	// Populate the tool cache so the subsequent Execute() call (edit_file,
	// multi_edit, etc.) reuses this read instead of hitting the disk again.
	// V10.13: 消除 precheck→execute 的重复文件 I/O，对大文件效果显著。
	if a.tc != nil {
		a.tc.Set(path, 0, content)
	}
	return content, true
}

// fuzzyPrecheckMatch mirrors the fuzzy matching that Execute uses in
// encoding_helpers.go (trimTrailing, expandTabs, stripOldReadPrefixes).
// Returns true if old can be matched in content under any fuzzy mode —
// prevents precheck from falsely blocking edits that Execute would accept.
func fuzzyPrecheckMatch(content, old string) bool {
	if old == "" || content == "" {
		return false
	}
	contentLines := strings.Split(content, "\n")
	oldLines := strings.Split(old, "\n")
	if len(oldLines) == 0 || len(oldLines) > len(contentLines) {
		return false
	}

	oldHasReadPrefixes := true
	for _, line := range oldLines {
		if !strings.HasPrefix(strings.TrimLeft(line, " \t"), "→") &&
			!hasReadFileNumberPrefix(line) {
			oldHasReadPrefixes = false
			break
		}
	}

	// Mirror the fuzzy modes from encoding_helpers.go fuzzyEditRanges.
	type mode struct {
		trimTrailing, expandTabs, stripPrefixes bool
	}
	modes := []mode{
		{trimTrailing: true},
		{trimTrailing: true, expandTabs: true},
	}
	if oldHasReadPrefixes {
		modes = append(modes,
			mode{trimTrailing: true, stripPrefixes: true},
			mode{trimTrailing: true, expandTabs: true, stripPrefixes: true},
		)
	}

	for _, m := range modes {
		normOld := make([]string, len(oldLines))
		for i, line := range oldLines {
			normOld[i] = normLine(line, m)
		}
		normContent := make([]string, len(contentLines))
		for i, line := range contentLines {
			normContent[i] = normLine(line, m)
		}
		// Sliding window match: find oldLines consecutively in contentLines.
		for i := 0; i <= len(normContent)-len(normOld); i++ {
			match := true
			for j := 0; j < len(normOld); j++ {
				if normContent[i+j] != normOld[j] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

func normLine(line string, m struct{ trimTrailing, expandTabs, stripPrefixes bool }) string {
	s := line
	if m.stripPrefixes {
		s = stripReadPrefix(s)
	}
	if m.expandTabs {
		s = strings.ReplaceAll(s, "\t", "    ")
	}
	if m.trimTrailing {
		s = strings.TrimRight(s, " \t")
	}
	return s
}

func stripReadPrefix(s string) string {
	// Strip " 123→" read_file line number prefixes.
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		s = s[1:]
	}
	if len(s) > 0 {
		// Check for "→" (Unicode arrow used by read_file output).
		r, size := utf8.DecodeRuneInString(s)
		if r == '→' || r == '>' {
			s = s[size:]
		}
	}
	return s
}

func hasReadFileNumberPrefix(line string) bool {
	s := strings.TrimLeft(line, " \t")
	for len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		s = s[1:]
	}
	if len(s) > 0 {
		r, _ := utf8.DecodeRuneInString(s)
		return r == '→' || r == '>'
	}
	return false
}
