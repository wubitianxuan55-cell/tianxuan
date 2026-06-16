package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// parseLocations decodes the three shapes textDocument/definition and
// /references may return: Location, Location[], or LocationLink[].
func parseLocations(raw json.RawMessage) []Location {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil
	}
	if s[0] == '[' {
		var arr []Location
		if json.Unmarshal(raw, &arr) == nil && len(arr) > 0 && arr[0].URI != "" {
			return arr
		}
		var links []struct {
			TargetURI   string `json:"targetUri"`
			TargetRange Range  `json:"targetRange"`
		}
		if json.Unmarshal(raw, &links) == nil {
			out := make([]Location, 0, len(links))
			for _, l := range links {
				if l.TargetURI != "" {
					out = append(out, Location{URI: l.TargetURI, Range: l.TargetRange})
				}
			}
			return out
		}
		return nil
	}
	var one Location
	if json.Unmarshal(raw, &one) == nil && one.URI != "" {
		return []Location{one}
	}
	return nil
}

// parseHover decodes a Hover.contents, which may be a MarkupContent, a single
// MarkedString, or an array of them.
func parseHover(raw json.RawMessage) string {
	var h struct {
		Contents json.RawMessage `json:"contents"`
	}
	if json.Unmarshal(raw, &h) != nil || len(h.Contents) == 0 {
		return ""
	}
	return strings.TrimSpace(markedToText(h.Contents))
}

func markedToText(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	switch s[0] {
	case '"':
		var str string
		_ = json.Unmarshal(raw, &str)
		return str
	case '{':
		var mc struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		}
		if json.Unmarshal(raw, &mc) == nil && mc.Value != "" {
			return mc.Value
		}
		var ms struct {
			Language string `json:"language"`
			Value    string `json:"value"`
		}
		_ = json.Unmarshal(raw, &ms)
		return ms.Value
	case '[':
		var parts []json.RawMessage
		_ = json.Unmarshal(raw, &parts)
		var out []string
		for _, p := range parts {
			if t := markedToText(p); t != "" {
				out = append(out, t)
			}
		}
		return strings.Join(out, "\n")
	}
	return ""
}

var severityName = map[int]string{1: "error", 2: "warning", 3: "info", 4: "hint"}

func formatDiagnostics(rel string, diags []Diagnostic) string {
	if len(diags) == 0 {
		return "no diagnostics for " + rel
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d diagnostic(s) in %s:\n", len(diags), rel)
	for _, d := range diags {
		sev := severityName[d.Severity]
		if sev == "" {
			sev = "error"
		}
		src := ""
		if d.Source != "" {
			src = " [" + d.Source + "]"
		}
		fmt.Fprintf(&b, "%d:%d %s%s %s\n",
			d.Range.Start.Line+1, d.Range.Start.Character+1, sev, src,
			strings.TrimSpace(d.Message))
	}
	return strings.TrimRight(b.String(), "\n")
}

// completionItem mirrors the LSP CompletionItem shape for parsing.
type completionItem struct {
	Label      string `json:"label"`
	Detail     string `json:"detail"`
	InsertText string `json:"insertText"`
	Kind       int    `json:"kind"`
}

// formatCompletions parses an LSP textDocument/completion response
// (CompletionItem[] or CompletionList) into a readable summary.
func formatCompletions(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return "no completions"
	}

	// Try CompletionList first: {"isIncomplete": bool, "items": [...]}
	var list struct {
		Items []completionItem `json:"items"`
	}
	if json.Unmarshal(raw, &list) == nil && len(list.Items) > 0 {
		return formatCompletionItems(list.Items)
	}

	// Try bare array
	var items []completionItem
	if json.Unmarshal(raw, &items) == nil {
		return formatCompletionItems(items)
	}

	return "no completions"
}

func formatCompletionItems(items []completionItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d completion(s):\n", len(items))
	limit := 30
	if len(items) < limit {
		limit = len(items)
	}
	for _, it := range items[:limit] {
		b.WriteString("  " + it.Label)
		if it.Detail != "" {
			b.WriteString(" \u2014 " + it.Detail)
		}
		b.WriteByte('\n')
	}
	if len(items) > limit {
		fmt.Fprintf(&b, "  ... and %d more\n", len(items)-limit)
	}
	return strings.TrimRight(b.String(), "\n")
}

// textEdit is one atomic change within a WorkspaceEdit.
type textEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// applyWorkspaceEdit applies a WorkspaceEdit returned by textDocument/rename.
// It handles the changes map format (uri → []TextEdit), applying each edit
// in reverse order so earlier positions stay valid.
func applyWorkspaceEdit(raw json.RawMessage) (string, error) {
	var edit struct {
		Changes map[string][]textEdit `json:"changes"`
	}
	if err := json.Unmarshal(raw, &edit); err != nil || len(edit.Changes) == 0 {
		// Try documentChanges format (newer LSP protocol)
		var docEdit struct {
			DocumentChanges []struct {
				TextDocument struct {
					URI     string `json:"uri"`
					Version int    `json:"version"`
				} `json:"textDocument"`
				Edits []textEdit `json:"edits"`
			} `json:"documentChanges"`
		}
		if json.Unmarshal(raw, &docEdit) == nil && len(docEdit.DocumentChanges) > 0 {
			return applyDocumentChanges(docEdit.DocumentChanges)
		}
		return "", fmt.Errorf("rename: no changes produced by the language server")
	}

	return applyChangesMap(edit.Changes)
}

func applyChangesMap(changes map[string][]textEdit) (string, error) {
	filesChanged := 0
	for uri, edits := range changes {
		path := uriToPath(uri)
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		// Apply edits in reverse order so positions stay valid
		text := string(content)
		for i := len(edits) - 1; i >= 0; i-- {
			text = applyTextEditToString(text, edits[i])
		}
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			return "", fmt.Errorf("write %s: %w", path, err)
		}
		filesChanged++
	}
	return fmt.Sprintf("renamed: %d file(s) updated", filesChanged), nil
}

func applyDocumentChanges(changes []struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version int    `json:"version"`
	} `json:"textDocument"`
	Edits []textEdit `json:"edits"`
}) (string, error) {
	changesMap := map[string][]textEdit{}
	for _, dc := range changes {
		changesMap[dc.TextDocument.URI] = dc.Edits
	}
	return applyChangesMap(changesMap)
}

// applyTextEditToString applies one TextEdit to a string.
// Edits MUST be applied in reverse order (last to first) so positions stay valid.
func applyTextEditToString(text string, e textEdit) string {
	lines := strings.Split(text, "\n")
	startLine := e.Range.Start.Line
	endLine := e.Range.End.Line
	startChar := e.Range.Start.Character
	endChar := e.Range.End.Character

	if startLine == endLine && len(lines) > startLine {
		line := lines[startLine]
		if startChar <= len(line) && endChar <= len(line) {
			lines[startLine] = line[:startChar] + e.NewText + line[endChar:]
		}
	} else if startLine < endLine && endLine < len(lines) {
		first := lines[startLine][:startChar]
		last := lines[endLine][endChar:]
		lines = append(lines[:startLine], append([]string{first + e.NewText + last}, lines[endLine+1:]...)...)
	}
	return strings.Join(lines, "\n")
}
