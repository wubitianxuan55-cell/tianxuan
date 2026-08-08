package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	fileenc "tianxuan/internal/fileutil/encoding"
	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(applyPatch{}) }

// applyPatch edits one or more files from a codex-style patch text (V10.171,
// distilled from codex's apply_patch freeform tool). The model writes the
// patch as plain text inside the JSON patch argument, eliminating
// old_string/new_string exact-match failures: lines are matched with leading
// and trailing whitespace ignored, an "@@ <anchor>" narrows the position, and
// all hunks apply atomically (a failure leaves every file untouched).
// roots confines the targets to the workspace when non-empty (see writeFile);
// workDir, when non-empty, is the directory a relative path resolves against.
type applyPatch struct {
	roots   []string
	workDir string
}

const (
	beginPatchMarker = "*** Begin Patch"
	endPatchMarker   = "*** End Patch"
	addFileMarker    = "*** Add File: "
	deleteFileMarker = "*** Delete File: "
	updateFileMarker = "*** Update File: "
	contextMarker    = "@@"
	eofMarker        = "*** End of File"
)

func (applyPatch) Name() string { return "apply_patch" }

func (applyPatch) Description() string {
	return "Apply a patch to one or more files in a single call. The patch is plain text delimited by '*** Begin Patch' and '*** End Patch'. Supported hunks: '*** Add File: <path>' followed by lines prefixed with '+'; '*** Delete File: <path>'; and '*** Update File: <path>' followed by change lines: '@@ [anchor]' starts a block, '-' removes a line, '+' inserts a line, ' ' (leading space) keeps a context line, and '*** End of File' pins the block to the end of the file. Lines match ignoring leading/trailing whitespace; without an '@@ <anchor>' a removed block must be unique. Every hunk is validated before any file is written, so a bad hunk leaves all files untouched."
}

func (applyPatch) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"patch":{"type":"string","description":"Patch text in apply_patch format: *** Begin Patch ... *** End Patch. Hunks: *** Add File: <path> (+ lines), *** Delete File: <path>, *** Update File: <path> (@@ anchor, - removed, + added, ' ' context, *** End of File)."}},"required":["patch"]}`)
}

func (applyPatch) ReadOnly() bool         { return false }
func (applyPatch) Kind() tool.ToolKind    { return tool.KindEdit }
func (applyPatch) CompactDescription() string { return compactDesc["apply_patch"] }
func (applyPatch) CompactSchema() json.RawMessage { return compactSchema["apply_patch"] }

// patchHunk is one parsed hunk of an apply_patch payload.
type patchHunk struct {
	kind     string // "add" | "delete" | "update"
	path     string
	contents string       // add hunk content
	chunks   []patchChunk // update hunk change blocks
}

// patchChunk is one change block inside an update hunk, positioned by an
// optional "@@ anchor". lines preserves the original order of the block:
// context lines (" ") are kept on both sides, old lines ("-") are removed,
// new lines ("+") are inserted.
type patchChunk struct {
	contextText string
	lines       []changeLine
	atEOF       bool
}

// changeLine is one ordered line of a patchChunk.
type changeLine struct {
	kind string // "ctx" | "old" | "new"
	text string
}

func hunkErr(line int, msg string) error {
	return fmt.Errorf("invalid hunk at line %d: %s", line, msg)
}

// isMarkerLine reports whether a line starts a new hunk or the patch
// boundaries, i.e. it terminates the body of the current hunk.
func isMarkerLine(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, addFileMarker) ||
		strings.HasPrefix(t, deleteFileMarker) ||
		strings.HasPrefix(t, updateFileMarker) ||
		t == beginPatchMarker ||
		t == endPatchMarker
}

// parsePatch parses a codex-style patch text into ordered hunks. It mirrors
// the lenient mode of codex's apply-patch parser: marker lines tolerate
// leading/trailing whitespace and stray blank lines between hunks are
// skipped. Parse errors carry the 1-based line number so the model can fix
// the exact spot.
func parsePatch(patch string) ([]patchHunk, error) {
	if strings.TrimSpace(patch) == "" {
		return nil, fmt.Errorf("invalid patch: empty patch")
	}
	lines := strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n")
	first := 0
	for first < len(lines) && strings.TrimSpace(lines[first]) == "" {
		first++
	}
	last := len(lines) - 1
	for last >= first && strings.TrimSpace(lines[last]) == "" {
		last--
	}
	if last < first {
		return nil, fmt.Errorf("invalid patch: empty patch")
	}
	if strings.TrimSpace(lines[first]) != beginPatchMarker {
		return nil, fmt.Errorf("invalid patch: first line must be '*** Begin Patch'")
	}
	if strings.TrimSpace(lines[last]) != endPatchMarker {
		return nil, fmt.Errorf("invalid patch: last line must be '*** End Patch'")
	}

	var hunks []patchHunk
	for i := first + 1; i < last; i++ {
		trimmed := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(trimmed, addFileMarker):
			h := patchHunk{kind: "add", path: strings.TrimSpace(strings.TrimPrefix(trimmed, addFileMarker))}
			if h.path == "" {
				return nil, hunkErr(i+1, "add file path is empty")
			}
			j := i + 1
			var content strings.Builder
			for j < last && !isMarkerLine(lines[j]) {
				if !strings.HasPrefix(lines[j], "+") {
					return nil, hunkErr(j+1, "add file content must start with '+'")
				}
				content.WriteString(strings.TrimPrefix(lines[j], "+"))
				content.WriteString("\n")
				j++
			}
			if content.Len() == 0 {
				return nil, hunkErr(i+1, fmt.Sprintf("add file hunk for path %q is empty", h.path))
			}
			h.contents = content.String()
			hunks = append(hunks, h)
			i = j - 1

		case strings.HasPrefix(trimmed, deleteFileMarker):
			path := strings.TrimSpace(strings.TrimPrefix(trimmed, deleteFileMarker))
			if path == "" {
				return nil, hunkErr(i+1, "delete file path is empty")
			}
			hunks = append(hunks, patchHunk{kind: "delete", path: path})

		case strings.HasPrefix(trimmed, updateFileMarker):
			path := strings.TrimSpace(strings.TrimPrefix(trimmed, updateFileMarker))
			if path == "" {
				return nil, hunkErr(i+1, "update file path is empty")
			}
			chunks, j, err := parseChangeChunks(lines, i+1, last)
			if err != nil {
				return nil, err
			}
			if len(chunks) == 0 {
				return nil, hunkErr(i+1, fmt.Sprintf("update file hunk for path %q is empty", path))
			}
			hunks = append(hunks, patchHunk{kind: "update", path: path, chunks: chunks})
			i = j - 1

		default:
			return nil, hunkErr(i+1, fmt.Sprintf("unknown line %q (expected '*** Add File:', '*** Delete File:' or '*** Update File:')", trimmed))
		}
	}
	return hunks, nil
}

// parseChangeChunks parses the change lines of an update hunk between start
// (exclusive) and end (exclusive of the patch end marker), returning the
// chunks and the index of the line that terminated the hunk (next marker).
func parseChangeChunks(lines []string, start, end int) ([]patchChunk, int, error) {
	var chunks []patchChunk
	cur := patchChunk{}
	inChunk := false
	j := start
	for j < end && !isMarkerLine(lines[j]) {
		line := lines[j]
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, contextMarker):
			if inChunk && !chunkHasDeletion(cur) {
				return nil, 0, hunkErr(j+1, "change chunk has no deletion lines (a block needs '-' lines to locate itself)")
			}
			if inChunk {
				chunks = append(chunks, cur)
			}
			cur = patchChunk{contextText: strings.TrimSpace(strings.TrimPrefix(trimmed, contextMarker))}
			inChunk = true
		case strings.HasPrefix(line, "+"):
			inChunk = true
			cur.lines = append(cur.lines, changeLine{kind: "new", text: strings.TrimPrefix(line, "+")})
		case strings.HasPrefix(line, "-"):
			inChunk = true
			cur.lines = append(cur.lines, changeLine{kind: "old", text: strings.TrimPrefix(line, "-")})
		case len(line) > 0 && line[0] == ' ':
			inChunk = true
			cur.lines = append(cur.lines, changeLine{kind: "ctx", text: strings.TrimSpace(line)})
		case trimmed == "":
			// stray blank line between hunks — skip, do not treat as context
		case trimmed == eofMarker:
			if !inChunk {
				return nil, 0, hunkErr(j+1, "'*** End of File' marker outside a change chunk")
			}
			cur.atEOF = true
			k := j + 1
			for k < end && !isMarkerLine(lines[k]) {
				if strings.TrimSpace(lines[k]) != "" {
					return nil, 0, hunkErr(k+1, "'*** End of File' must be the last line of the hunk")
				}
				k++
			}
		default:
			return nil, 0, hunkErr(j+1, fmt.Sprintf("unknown change line %q (expected '@@', '+', '-' or a line starting with a space)", trimmed))
		}
		j++
	}
	if inChunk {
		if !chunkHasDeletion(cur) {
			return nil, 0, hunkErr(j, "change chunk has no deletion lines (a block needs '-' lines to locate itself)")
		}
		chunks = append(chunks, cur)
	}
	return chunks, j, nil
}

// chunkHasDeletion reports whether a change block contains at least one "-"
// line, which is what locates the block in the target file.
func chunkHasDeletion(ch patchChunk) bool {
	for _, l := range ch.lines {
		if l.kind == "old" {
			return true
		}
	}
	return false
}

func normaliseLine(s string) string { return strings.TrimSpace(s) }

// findSequence returns the first index at which seq occurs in lines starting
// at from, and the total number of occurrences. Matching ignores leading and
// trailing whitespace per line (codex's seek_sequence behaviour), which is
// what lets apply_patch survive read_file line prefixes and trailing spaces.
func findSequence(lines []string, from int, seq []string) (int, int) {
	if len(seq) == 0 || from+len(seq) > len(lines) {
		return -1, 0
	}
	first := -1
	count := 0
	for i := from; i+len(seq) <= len(lines); i++ {
		ok := true
		for j := range seq {
			if normaliseLine(lines[i+j]) != normaliseLine(seq[j]) {
				ok = false
				break
			}
		}
		if ok {
			count++
			if first < 0 {
				first = i
			}
		}
	}
	return first, count
}

func chunkNotFoundError(ci int, matchSeq []string, lines []string, from int) error {
	msg := fmt.Sprintf("chunk %d: lines not found (expected %d line(s) starting with %q)",
		ci+1, len(matchSeq), strings.TrimSpace(matchSeq[0]))
	if from < len(lines) {
		msg += fmt.Sprintf("; nearest line %d: %q", from+1, strings.TrimSpace(lines[from]))
	}
	return fmt.Errorf("%s", msg)
}

// applyUpdateChunks applies an update hunk's change blocks to file content in
// memory. CRLF input is normalised to LF for matching and restored on output;
// the result is a full new file content (never written by this function).
func applyUpdateChunks(content string, chunks []patchChunk) (string, error) {
	lineEnding := detectLineEnding(content)
	norm := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(norm, "\n")

	pos := 0
	for ci, ch := range chunks {
		searchFrom := pos
		if strings.TrimSpace(ch.contextText) != "" {
			anchor := -1
			for i := pos; i < len(lines); i++ {
				if strings.Contains(lines[i], strings.TrimSpace(ch.contextText)) {
					anchor = i
					break
				}
			}
			if anchor < 0 {
				return "", fmt.Errorf("chunk %d: anchor %q not found", ci+1, ch.contextText)
			}
			// Search from the anchor line itself: the model usually repeats the
			// anchor as a context line, but searching from it also tolerates a
			// patch that jumps straight into the change lines after '@@'.
			searchFrom = anchor
		}

		var matchSeq, newSeq []string
		for _, l := range ch.lines {
			switch l.kind {
			case "ctx":
				matchSeq = append(matchSeq, l.text)
				newSeq = append(newSeq, l.text)
			case "old":
				matchSeq = append(matchSeq, l.text)
			case "new":
				newSeq = append(newSeq, l.text)
			}
		}
		idx, count := findSequence(lines, searchFrom, matchSeq)
		if count == 0 {
			return "", chunkNotFoundError(ci, matchSeq, lines, searchFrom)
		}
		if count > 1 && strings.TrimSpace(ch.contextText) == "" {
			return "", fmt.Errorf("chunk %d: %d matches found, not unique (add an '@@ <anchor>' line to disambiguate)", ci+1, count)
		}
		if ch.atEOF {
			effectiveLen := len(lines)
			if effectiveLen > 0 && lines[effectiveLen-1] == "" {
				effectiveLen--
			}
			if idx+len(matchSeq) != effectiveLen {
				return "", fmt.Errorf("chunk %d: '*** End of File' requires the block to end at the last line (block ends at line %d, file has %d lines)", ci+1, idx+len(matchSeq), effectiveLen)
			}
		}

		replaced := make([]string, 0, len(lines)-len(matchSeq)+len(newSeq))
		replaced = append(replaced, lines[:idx]...)
		replaced = append(replaced, newSeq...)
		replaced = append(replaced, lines[idx+len(matchSeq):]...)
		lines = replaced
		pos = idx + len(newSeq)
	}

	out := strings.Join(lines, "\n")
	if lineEnding == "\r\n" {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return out, nil
}

func (ap applyPatch) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if strings.TrimSpace(p.Patch) == "" {
		return "", fmt.Errorf("patch is required")
	}
	hunks, err := parsePatch(p.Patch)
	if err != nil {
		return "", fmt.Errorf("invalid patch: %w", err)
	}
	if len(hunks) == 0 {
		return "", fmt.Errorf("patch contains no hunks")
	}

	// Phase 1: compute every target's new content in memory. Nothing touches
	// the filesystem until every hunk has been validated (atomicity).
	type filePlan struct {
		path    string
		content string
		action  string // "add" | "update"
		mode    os.FileMode
		enc     fileenc.Kind
	}
	plans := map[string]*filePlan{}
	var deletes []string

	for _, h := range hunks {
		resolved := resolveIn(ap.workDir, h.path)
		if err := confine(ap.roots, resolved); err != nil {
			return "", err
		}
		key := filepath.Clean(resolved)
		switch h.kind {
		case "add":
			plans[key] = &filePlan{path: resolved, content: h.contents, action: "add"}
		case "delete":
			deletes = append(deletes, resolved)
		case "update":
			cur, ok := plans[key]
			if !ok {
				content, enc, err := readFileEncoded(resolved)
				if err != nil {
					return "", fmt.Errorf("apply_patch: read %s: %w", resolved, err)
				}
				fi, statErr := os.Stat(resolved)
				mode := os.FileMode(0o644)
				if statErr == nil {
					mode = fi.Mode().Perm()
				}
				cur = &filePlan{path: resolved, content: content, action: "update", mode: mode, enc: enc}
				plans[key] = cur
			}
			newContent, err := applyUpdateChunks(cur.content, h.chunks)
			if err != nil {
				return "", fmt.Errorf("apply_patch: %s: %w", resolved, err)
			}
			cur.content = newContent
			cur.action = "update"
		}
	}

	// Phase 2: validate delete targets (existence, not a directory, and no
	// add/update on the same path).
	for _, d := range deletes {
		if _, ok := plans[filepath.Clean(d)]; ok {
			return "", fmt.Errorf("apply_patch: %s: same path is both written and deleted in one patch", d)
		}
		fi, err := os.Lstat(d)
		if err != nil {
			return "", fmt.Errorf("apply_patch: delete %s: %w", d, err)
		}
		if fi.IsDir() {
			return "", fmt.Errorf("apply_patch: delete %s: is a directory, refusing", d)
		}
	}

	// Phase 3: commit — every validation passed, so write/delete all targets.
	var added, modified, deleted []string
	for _, pl := range plans {
		if pl.action == "add" {
			if err := writeFileEncoded(pl.path, pl.content, fileenc.UTF8, 0o644); err != nil {
				return "", fmt.Errorf("apply_patch: write %s: %w", pl.path, err)
			}
			added = append(added, pl.path)
		} else {
			if err := writeFileEncoded(pl.path, pl.content, pl.enc, pl.mode); err != nil {
				return "", fmt.Errorf("apply_patch: write %s: %w", pl.path, err)
			}
			modified = append(modified, pl.path)
		}
	}
	for _, d := range deletes {
		if err := os.Remove(d); err != nil {
			return "", fmt.Errorf("apply_patch: delete %s: %w", d, err)
		}
		deleted = append(deleted, d)
	}

	var b strings.Builder
	b.WriteString("Success. Updated the following files:\n")
	for _, p := range added {
		fmt.Fprintf(&b, "  added: %s\n", p)
	}
	for _, p := range modified {
		fmt.Fprintf(&b, "  modified: %s\n", p)
	}
	for _, p := range deleted {
		fmt.Fprintf(&b, "  deleted: %s\n", p)
	}
	return b.String(), nil
}
