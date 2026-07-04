package builtin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"tianxuan/internal/tool"
)

const grepMaxMatches = 500

func init() { tool.RegisterBuiltin(grepTool{}) }

// grepTool searches files by regex. When ripgrep (rg) is available on PATH,
// delegates to it for 10-100x speedup and native .gitignore support.
// workDir, when non-empty, is the directory a relative path resolves against.
type grepTool struct {
	workDir string
}

var grepRgPath string // resolved ripgrep path, set at boot

func (grepTool) Name() string { return "grep" }

func (grepTool) Description() string {
	return "搜索正则表达式匹配的文件或目录。返回匹配行 path:line:text。支持 context_lines 显示匹配行上下文（前后各N行），highlight 高亮匹配部分（>>>match<<<），sort_by=relevance 按匹配密度排序。max_matches 最大 2000。"
}

func (grepTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"正则表达式 (RE2 语法)"},"path":{"type":"string","description":"文件或目录 (默认 \".\")"},"max_matches":{"type":"integer","description":"最大匹配数 (默认 500, 最大 2000)"},"sort_by":{"type":"string","enum":["path","relevance"],"description":"排序方式: path (默认) 或 relevance (按匹配密度)"},"context_lines":{"type":"integer","description":"匹配行四周的上下文行数 (默认 0, 最大 5)"},"highlight":{"type":"boolean","description":"用 >>><<< 包裹匹配文本 (默认 true)"}},"required":["pattern"]}`)
}

func (grepTool) ReadOnly() bool { return true }

func (grepTool) CompactDescription() string { return compactDesc["grep"] }
func (grepTool) CompactSchema() json.RawMessage   { return compactSchema["grep"] }

type grepMatch struct {
	file string
	line int
	text string
	// isContext is true when this match is a surrounding context line, not a
	// direct regex match. Context lines are rendered with a "-" suffix on the
	// line number to distinguish them.
	isContext bool
}

func (g grepTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	type pT struct {
		Pattern      string `json:"pattern"`
		Path         string `json:"path"`
		MaxMatches   *int   `json:"max_matches,omitempty"`
		SortBy       string `json:"sort_by"`
		ContextLines *int   `json:"context_lines,omitempty"`
		Highlight    *bool  `json:"highlight,omitempty"`
	}
	var p pT
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	if p.Path == "" {
		p.Path = "."
	}

	maxMatches := grepMaxMatches
	if p.MaxMatches != nil && *p.MaxMatches > 0 {
		maxMatches = *p.MaxMatches
		if maxMatches > 2000 {
			maxMatches = 2000
		}
	}

	ctxLines := 0
	if p.ContextLines != nil && *p.ContextLines > 0 {
		ctxLines = *p.ContextLines
		if ctxLines > 5 {
			ctxLines = 5
		}
	}

	highlight := true
	if p.Highlight != nil {
		highlight = *p.Highlight
	}

	p.Path = resolveIn(g.workDir, p.Path)

	// V10.29: delegate to ripgrep when available — 10-100x faster, honors .gitignore.
	if grepRgPath != "" && ctxLines == 0 && !highlight && p.SortBy != "relevance" {
		return g.runRipgrep(ctx, p.Pattern, p.Path, maxMatches)
	}

	re, err := regexp.Compile(p.Pattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}

	var out []grepMatch
	truncated := false

	// searchFile returns io.EOF as a sentinel once the cap is reached.
	searchFile := func(file string) error {
		f, err := os.Open(file)
		if err != nil {
			return nil // skip unreadable files
		}
		defer f.Close()

		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		ln := 0

		// When context_lines > 0, maintain a ring buffer of recent lines.
		var ringBuf []string
		ringPos := 0
		var pendingAfter int // context lines to emit after the last match
		emittedLines := map[int]bool{} // tracks emitted line numbers for O(1) dedup

		for sc.Scan() {
			ln++
			line := sc.Text()
			if strings.IndexByte(line, 0) >= 0 {
				return nil // looks binary, skip the file
			}

			matched := re.MatchString(line)

			if ctxLines > 0 {
				// Ring buffer for context lines before a match.
				if cap(ringBuf) < ctxLines {
					ringBuf = make([]string, ctxLines)
				}
				ringBuf[ringPos%ctxLines] = line
				ringPos++

				if matched {
					// Emit preceding context lines from the ring buffer.
					start := ringPos - 1 - ctxLines
					if start < 0 {
						start = 0
					}
					// ringBuf may contain lines not yet flushed that are
					// part of the context. Walk from start to ringPos-1 and
					// emit non-duplicate lines.
					//
					// Simpler approach: compute the exact preceding line
					// numbers we need (ln-ctxLines .. ln-1) and emit them
					// as context, skipping any that were already emitted.
					ctxStart := ln - ctxLines
					if ctxStart < 1 {
						ctxStart = 1
					}
					already := emittedLines
					for ctxLine := ctxStart; ctxLine < ln; ctxLine++ {
						if already[ctxLine] {
							continue
						}
						// Re-read the context line text from the ring buffer.
						offset := ctxLine - ctxStart
						idx := (ringPos - 1 - ctxLines + offset) % ctxLines
						if idx < 0 {
							idx += ctxLines
						}
						ctxTxt := ringBuf[idx]
						if ctxTxt == "" {
							continue
						}
						out = append(out, grepMatch{file: file, line: ctxLine, text: ctxTxt, isContext: true})
						already[ctxLine] = true
						if len(out) >= maxMatches {
							truncated = true
							return io.EOF
						}
					}

					// Emit the actual match.
					out = append(out, grepMatch{file: file, line: ln, text: line, isContext: false})
					if len(out) >= maxMatches {
						truncated = true
						return io.EOF
					}
					pendingAfter = ctxLines
				} else if pendingAfter > 0 {
					// Emit trailing context line.
					out = append(out, grepMatch{file: file, line: ln, text: line, isContext: true})
					pendingAfter--
					if len(out) >= maxMatches {
						truncated = true
						return io.EOF
					}
				}
			} else {
				// No context_lines: simple match.
				if matched {
					out = append(out, grepMatch{file: file, line: ln, text: line})
					if len(out) >= maxMatches {
						truncated = true
						return io.EOF
					}
				}
			}
		}
		return nil
	}

	info, err := os.Stat(p.Path)
	if err != nil {
		return "", fmt.Errorf("grep %s: %w", p.Path, err)
	}

	if info.IsDir() {
		walker := newGitignoreWalker(p.Path)
		_ = filepath.WalkDir(p.Path, func(path string, d os.DirEntry, err error) error {
			// V8.2: 周期性检查 context 取消，防止大目录遍历永久阻塞
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if err != nil {
				return nil
			}
			walker.leave(path)
			name := d.Name()
			isDir := d.IsDir()
			if walker.skip(path, name, isDir) {
				if isDir {
					return filepath.SkipDir
				}
				return nil
			}
			if isDir {
				walker.enter(path)
				return nil
			}
			if searchFile(path) == io.EOF {
				return filepath.SkipAll
			}
			return nil
		})
	} else {
		_ = searchFile(p.Path)
	}

	if len(out) == 0 {
		return "(no matches)", nil
	}

	// V10.7: sort_by 参数 — relevance 模式按文件匹配密度排序
	if p.SortBy == "relevance" {
		sortByRelevance(out)
	}

	var b strings.Builder
	for _, m := range out {
		txt := m.text
		if highlight && !m.isContext {
			txt = highlightMatch(re, txt)
		}
		lineMark := ""
		if m.isContext {
			lineMark = "-" // context line: path:line-:text
		}
		fmt.Fprintf(&b, "%s:%d%s:%s\n", m.file, m.line, lineMark, txt)
	}
	res := strings.TrimSuffix(b.String(), "\n")
	if truncated {
		res += fmt.Sprintf("\n... (truncated at %d results)", maxMatches)
	}
	return res, nil
}

// highlightMatch wraps the leftmost regex match in >>> and <<< markers.
// If the regex doesn't match or the match would empty the string, the
// original text is returned unchanged.
func highlightMatch(re *regexp.Regexp, text string) string {
	loc := re.FindStringIndex(text)
	if loc == nil {
		return text
	}
	return text[:loc[0]] + ">>>" + text[loc[0]:loc[1]] + "<<<" + text[loc[1]:]
}

// sortByRelevance sorts matches by match density per file (descending), then
// within each file by line number. Context lines are sorted adjacent to their
// originating match.
func sortByRelevance(matches []grepMatch) {
	// When context_lines is active, we need to preserve the order of context
	// lines relative to their matches. Use a simple approach: count only
	// non-context matches per file.
	perFile := make(map[string]int)
	for _, m := range matches {
		if !m.isContext {
			perFile[m.file]++
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		ci := perFile[matches[i].file]
		cj := perFile[matches[j].file]
		if ci != cj {
			return ci > cj // more matches = higher relevance
		}
		if matches[i].file != matches[j].file {
			return matches[i].file < matches[j].file
		}
		return matches[i].line < matches[j].line
	})
}

// ── Ripgrep delegation (V10.29) ───────────────────────────────────────

// SetRgPath configures the absolute path to a ripgrep binary for delegation.
// Pass "" to disable. Call once at boot after PATH resolution.
func ResolveRgPath() string {
	if p, err := exec.LookPath("rg"); err == nil {
		grepRgPath = p
		return p
	}
	return ""
}

func (g grepTool) runRipgrep(ctx context.Context, pattern, path string, maxMatches int) (string, error) {
	argv := []string{
		grepRgPath,
		"--no-heading", "--line-number", "--with-filename", "--color", "never",
		"--regexp", pattern,
		"--", path,
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	hideBashWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("ripgrep pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("ripgrep: %w", err)
	}

	var out []string
	truncated := false
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		out = append(out, sc.Text())
		if len(out) >= maxMatches {
			truncated = true
			break
		}
	}
	if truncated {
		_ = cmd.Process.Kill()
	}
	_, _ = io.Copy(io.Discard, stdout)
	_ = cmd.Wait()

	if len(out) == 0 && ctx.Err() != context.DeadlineExceeded {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("ripgrep: %s", msg)
		}
	}
	if len(out) == 0 {
		return "(no matches)", nil
	}
	res := strings.Join(out, "\n")
	if truncated {
		res += fmt.Sprintf("\n... (truncated at %d matches)", maxMatches)
	}
	return res, nil
}

// ── Gitignore-aware walk (V10.30) ────────────────────────────────────
// Ported from Reasonix V1.15 (MIT). When ripgrep is unavailable the pure-Go
// fallback now honours .gitignore rules so results match what rg would return.

// gitignoreFrame holds the cumulative ignore rules for one directory.
type gitignoreFrame struct {
	dir      string
	patterns []string
}

// gitignoreWalker prunes a recursive walk to mirror ripgrep: skips hidden
// entries, vendorDirs, and anything matched by the repository's ignore rules.
type gitignoreWalker struct {
	root     string
	repoRoot string
	disabled bool
	frames   []gitignoreFrame
}

func newGitignoreWalker(root string) *gitignoreWalker {
	w := &gitignoreWalker{root: absClean(root)}
	rr := findGitRoot(w.root)
	if rr == "" {
		return w
	}
	w.repoRoot = rr

	var rootLines []string
	rootLines = append(rootLines, readIgnoreLines(filepath.Join(rr, ".git", "info", "exclude"))...)
	rootLines = append(rootLines, readIgnoreLines(filepath.Join(rr, ".gitignore"))...)
	w.frames = append(w.frames, gitignoreFrame{dir: rr, patterns: rootLines})

	for _, dir := range dirsBetween(rr, w.root) {
		lines := readIgnoreLines(filepath.Join(dir, ".gitignore"))
		if len(lines) == 0 {
			continue
		}
		parent := w.frames[len(w.frames)-1].patterns
		combined := append(append([]string{}, parent...), lines...)
		w.frames = append(w.frames, gitignoreFrame{dir: dir, patterns: combined})
	}

	if isHidden(filepath.Base(w.root)) || w.ignored(w.root, true) {
		w.disabled = true
	}
	return w
}

func (w *gitignoreWalker) enter(path string) {
	if w.disabled || w.repoRoot == "" {
		return
	}
	abs := absClean(path)
	if abs == w.root {
		return
	}
	lines := readIgnoreLines(filepath.Join(abs, ".gitignore"))
	if len(lines) == 0 {
		return
	}
	parent := w.frames[len(w.frames)-1].patterns
	combined := append(append([]string{}, parent...), lines...)
	w.frames = append(w.frames, gitignoreFrame{dir: abs, patterns: combined})
}

func (w *gitignoreWalker) leave(path string) {
	abs := absClean(path)
	for len(w.frames) > 1 && !underDir(w.frames[len(w.frames)-1].dir, abs) {
		w.frames = w.frames[:len(w.frames)-1]
	}
}

func (w *gitignoreWalker) skip(path, name string, isDir bool) bool {
	abs := absClean(path)
	if abs == w.root || w.disabled {
		return false
	}
	if isHidden(name) {
		return true
	}
	if isDir && vendorDirs[name] {
		return true
	}
	return w.ignored(abs, isDir)
}

func (w *gitignoreWalker) ignored(abs string, isDir bool) bool {
	if w.repoRoot == "" || len(w.frames) == 0 {
		return false
	}
	rel, err := filepath.Rel(w.repoRoot, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	relSlash := filepath.ToSlash(rel)
	f := w.frames[len(w.frames)-1]
	matched := false
	for _, p := range f.patterns {
		negate := false
		pat := p
		if strings.HasPrefix(pat, "!") {
			negate = true
			pat = pat[1:]
		}
		dirOnly := strings.HasSuffix(pat, "/")
		if dirOnly {
			pat = strings.TrimSuffix(pat, "/")
		}
		anchor := filepath.ToSlash(strings.TrimPrefix(absClean(f.dir), absClean(w.repoRoot)))
		anchor = strings.TrimPrefix(anchor, "/")
		reanchored := reanchorPattern(pat, anchor, dirOnly)
		if matchGitignorePattern(relSlash, reanchored, isDir) {
			matched = !negate
		}
	}
	return matched
}

func matchGitignorePattern(rel, pattern string, isDir bool) bool {
	pattern = strings.TrimPrefix(pattern, "/")
	testDir := strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	if testDir && !isDir {
		return false
	}
	target := rel
	if isDir {
		target += "/"
	}
	if strings.Contains(pattern, "/") {
		return matchGlobStar(pattern, target)
	}
	if matchBasename(target, pattern) {
		return true
	}
	parts := strings.Split(target, "/")
	for i := 0; i < len(parts)-1; i++ {
		if matchBasename(parts[i], pattern) {
			return true
		}
	}
	return false
}

func matchGlobStar(pattern, target string) bool {
	if pattern == "**" || pattern == "**/" {
		return true
	}
	patParts := strings.Split(pattern, "/")
	tgtParts := strings.Split(strings.TrimSuffix(target, "/"), "/")
	return matchGlobSegments(patParts, tgtParts)
}

func matchGlobSegments(pat, tgt []string) bool {
	if len(pat) == 0 {
		return len(tgt) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(tgt); i++ {
			if matchGlobSegments(pat[1:], tgt[i:]) {
				return true
			}
		}
		return false
	}
	if len(tgt) == 0 {
		return false
	}
	if matchFilepathGlob(pat[0], tgt[0]) {
		return matchGlobSegments(pat[1:], tgt[1:])
	}
	return false
}

func matchFilepathGlob(pattern, name string) bool {
	ok, _ := filepath.Match(pattern, name)
	return ok
}

func matchBasename(path, pattern string) bool {
	idx := strings.LastIndex(path, "/")
	if idx >= 0 {
		return matchFilepathGlob(pattern, path[idx+1:])
	}
	return matchFilepathGlob(pattern, path)
}

func reanchorPattern(line, dirAnchor string, dirOnly bool) string {
	if dirAnchor == "" || dirAnchor == "." {
		return line
	}
	anchored := strings.HasPrefix(line, "/") || strings.Contains(strings.TrimSuffix(line, "/"), "/")
	line = strings.TrimPrefix(line, "/")
	if anchored {
		line = dirAnchor + "/" + line
	} else {
		line = dirAnchor + "/**/" + line
	}
	if dirOnly {
		line += "/"
	}
	return line
}

func findGitRoot(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
		abs = filepath.Dir(abs)
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
			return abs
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return ""
		}
		abs = parent
	}
}

func readIgnoreLines(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func dirsBetween(repoRoot, root string) []string {
	var dirs []string
	for d := root; d != repoRoot && d != filepath.Dir(d); d = filepath.Dir(d) {
		dirs = append(dirs, d)
	}
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return dirs
}

func isHidden(name string) bool {
	return len(name) > 1 && name[0] == '.' && name != ".."
}

func underDir(dir, path string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(os.PathSeparator))
}

func absClean(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}
