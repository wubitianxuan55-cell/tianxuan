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
			if d.IsDir() {
				if skipWalkDir(p.Path, path, d.Name()) {
					return filepath.SkipDir
				}
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
