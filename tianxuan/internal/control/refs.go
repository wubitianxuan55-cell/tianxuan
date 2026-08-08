package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// maxFileRefBytes caps how much of an @-referenced file is injected into a
// message, so "@somehuge.log" can't blow the context window. The head is kept
// and the rest noted as truncated.
const maxFileRefBytes = 64 * 1024

// binarySniffBytes is how many bytes to scan for a NUL byte to detect binary files.
const binarySniffBytes = 8192

// refKind distinguishes the two things an @reference can resolve to.
type refKind int

const (
	refResource refKind = iota // an MCP resource: @<server>:<uri>
	refFile                    // a local file or directory: @<path>
	refImage                   // a local image attachment: @.tianxuan/attachments/<file>
	refSession                 // a past session: @session:<id>
)

// ref is a resolved @reference found in a submitted line.
type ref struct {
	kind   refKind
	server string // refResource
	uri    string // refResource
	path   string // refFile
	raw    string // the original token after '@', for labelling
}

// refTokenRe matches an @reference token: '@' then a run of non-space chars.
var refTokenRe = regexp.MustCompile(`@([^\s]+)`)

// parseRefTokens extracts the deduped, punctuation-trimmed tokens following '@'
// in a line. Pure: classification (server? file?) happens in classifyRef.
func parseRefTokens(line string) []string {
	var toks []string
	seen := map[string]bool{}
	for _, g := range refTokenRe.FindAllStringSubmatch(line, -1) {
		t := strings.TrimRight(g[1], ".,;!?)]}")
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		toks = append(toks, t)
	}
	return toks
}

// classifyRef decides what a token refers to. A "server:uri" token whose server
// is connected is an MCP resource; otherwise a token that names an existing path
// is a file. Anything else (an @mention, an email) is not a reference. exists is
// injected so the rule is testable without touching the filesystem.
func classifyRef(token string, known map[string]bool, exists func(string) bool) (ref, bool) {
	// @session:<id> references a persisted conversation for cross-session
	// reuse (Qwen Code @session). Resolved purely by prefix; never a file.
	if id, ok := strings.CutPrefix(token, "session:"); ok && id != "" && !strings.ContainsAny(id, `/\`) {
		return ref{kind: refSession, raw: id}, true
	}
	if i := strings.Index(token, ":"); i > 0 && i+1 < len(token) && known[token[:i]] {
		return ref{kind: refResource, server: token[:i], uri: token[i+1:], raw: token}, true
	}
	if strings.HasPrefix(filepath.ToSlash(token), ".tianxuan/attachments/") && exists(token) {
		return ref{kind: refImage, path: token, raw: token}, true
	}
	if exists(token) {
		return ref{kind: refFile, path: token, raw: token}, true
	}
	return ref{}, false
}

// detectRefs finds the @references in a line: MCP resources for connected
// servers, and local paths that exist on disk.
func (c *Controller) detectRefs(line string) []ref {
	known := map[string]bool{}
	if c.host != nil {
		for _, n := range c.host.ServerNames() {
			known[n] = true
		}
	}
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }

	var refs []ref
	for _, tok := range parseRefTokens(line) {
		if r, ok := classifyRef(tok, known, exists); ok {
			refs = append(refs, r)
		}
	}
	return refs
}

// HasRefs reports whether a line contains any resolvable @references, so a
// frontend can decide to resolve off its event loop only when needed.
func (c *Controller) HasRefs(line string) bool {
	return len(c.detectRefs(line)) > 0
}

// ResolveRefs resolves the @references in a line into a single tagged context
// block (file/dir contents, MCP resource bodies), plus per-reference error
// strings for any that failed. An empty block means no references resolved.
// Safe to call off a frontend's event loop; honours ctx for the resource reads.
func (c *Controller) ResolveRefs(ctx context.Context, line string) (block string, errs []string) {
	var b strings.Builder
	for _, r := range c.detectRefs(line) {
		switch r.kind {
		case refResource:
			text, err := c.host.ReadResource(ctx, r.server, r.uri)
			if err != nil {
				errs = append(errs, "@"+r.raw+" — "+err.Error())
				continue
			}
			appendRefBlock(&b, "resource", `ref="@`+r.raw+`"`, text)
		case refFile:
			text, isDir, err := readFileRef(r.path)
			if err != nil {
				errs = append(errs, "@"+r.raw+" — "+err.Error())
				continue
			}
			tag := "file"
			if isDir {
				tag = "dir"
			}
			appendRefBlock(&b, tag, `path="`+r.path+`"`, text)
		case refImage:
			appendRefBlock(&b, "image", `path="`+r.path+`"`, "[image attachment available at @"+r.path+"; image bytes are not inlined — invoke the vision skill (run_skill name=vision) with this path to see it]")
		case refSession:
			path := filepath.Join(c.sessionDir, r.raw+".jsonl")
			text, err := sessionDigest(path, maxSessionRefBytes)
			if err != nil {
				errs = append(errs, "@session:"+r.raw+" — "+err.Error())
				continue
			}
			appendRefBlock(&b, "session", `ref="@session:`+r.raw+`"`, text)
		}
	}
	return b.String(), errs
}

// maxSessionRefBytes caps how much of a referenced session is injected,
// mirroring Qwen Code's 8k token budget for @session summaries: the newest
// content is kept, older parts are omitted with a marker.
const maxSessionRefBytes = 8 * 1024

// sessionDigest renders a deterministic, read-only summary of a JSONL session:
// user/assistant text is kept, tool results are compressed to a one-line
// status, and only the newest content within budget is retained (older parts
// marked omitted). No LLM call; failures are returned to the caller.
func sessionDigest(path string, budgetBytes int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	type sessionMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
		Name    string `json:"name"`
	}
	var msgs []sessionMsg
	dec := json.NewDecoder(f)
	for {
		var m sessionMsg
		if err := dec.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("decode session %s: %w", path, err)
		}
		msgs = append(msgs, m)
	}

	var kept []string
	used := 0
	omitted := false
	for i := len(msgs) - 1; i >= 0; i-- {
		line := renderSessionLine(msgs[i])
		if line == "" {
			continue
		}
		if budgetBytes > 0 && used+len(line) > budgetBytes {
			omitted = true
			break
		}
		kept = append(kept, line)
		used += len(line)
	}

	var b strings.Builder
	if omitted {
		b.WriteString("[earlier content omitted — deterministic session summary]\n")
	}
	for i := len(kept) - 1; i >= 0; i-- {
		b.WriteString(kept[i])
		b.WriteString("\n")
	}
	return b.String(), nil
}

// renderSessionLine turns one message into a digest line: text roles are kept
// verbatim (trimmed), tool results collapse to a one-line status.
func renderSessionLine(m struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name"`
}) string {
	switch m.Role {
	case "user", "assistant":
		if t := strings.TrimSpace(m.Content); t != "" {
			return m.Role + ": " + t
		}
		return ""
	case "tool":
		name := m.Name
		if name == "" {
			name = "tool"
		}
		n := len([]rune(strings.TrimSpace(m.Content)))
		return "[tool:" + name + "] output " + strconv.Itoa(n) + " chars"
	default:
		return ""
	}
}

func appendRefBlock(b *strings.Builder, tag, attr, body string) {
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	fmt.Fprintf(b, "<%s %s>\n%s\n</%s>", tag, attr, body, tag)
}

// readFileRef reads an @-referenced path for injection. A directory yields a
// recursive listing (walked depth-first so the model sees the full tree); a
// binary file (NUL in the first 8 KiB) is noted rather than dumped; a large file
// is truncated to maxFileRefBytes with a marker. isDir lets the caller pick the
// wrapping tag. Common noise directories (.git, node_modules, .DS_Store) are
// skipped during the walk.
func readFileRef(path string) (content string, isDir bool, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false, err
	}
	if info.IsDir() {
		var b strings.Builder
		err := filepath.WalkDir(path, func(p string, d os.DirEntry, wErr error) error {
			if wErr != nil {
				return wErr
			}
			// Skip the root itself — we only list its children.
			if p == path {
				return nil
			}
			name := d.Name()
			// Skip common noise directories.
			if d.IsDir() {
				switch name {
				case ".git", "node_modules", ".DS_Store", "__pycache__", ".idea", ".vscode":
					return filepath.SkipDir
				}
			}
			// Render the path relative to the referenced directory so the
			// listing is concise and unambiguous. Use forward slashes for
			// cross-platform consistency.
			rel, rErr := filepath.Rel(path, p)
			if rErr != nil {
				rel = p
			}
			rel = strings.ReplaceAll(rel, string(os.PathSeparator), "/")
			if d.IsDir() {
				rel += "/"
			}
			b.WriteString(rel)
			b.WriteByte('\n')
			return nil
		})
		if err != nil {
			return "", true, err
		}
		return b.String(), true, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	buf := make([]byte, maxFileRefBytes+1)
	n, rerr := io.ReadFull(f, buf)
	if rerr != nil && rerr != io.ErrUnexpectedEOF && rerr != io.EOF {
		return "", false, rerr
	}
	data := buf[:n]

	if mime := imageMime(data, path); mime != "" {
		return fmt.Sprintf("[image file %s, mime=%s, %d bytes — image bytes are not inlined. Use an available MCP image/OCR/vision tool with this path when visual understanding is needed.]", path, mime, info.Size()), false, nil
	}
	if bytes.IndexByte(data[:min(n, binarySniffBytes)], 0) >= 0 {
		return fmt.Sprintf("[binary file %s, %d bytes — not shown]", path, info.Size()), false, nil
	}
	if n > maxFileRefBytes {
		return string(data[:maxFileRefBytes]) + fmt.Sprintf("\n…[truncated; file is %d bytes]…", info.Size()), false, nil
	}
	return string(data), false, nil
}

func imageMime(data []byte, path string) string {
	mime := http.DetectContentType(data[:min(len(data), 512)])
	if strings.HasPrefix(mime, "image/") {
		return mime
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".tiff", ".tif":
		return "image/tiff"
	}
	return ""
}
