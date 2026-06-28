package builtin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"tianxuan/internal/tool"
)

const grepMaxMatches = 500

func init() { tool.RegisterBuiltin(grepTool{}) }

// grepTool searches files by regex. workDir, when non-empty, is the directory a
// relative path resolves against (see resolveIn).
type grepTool struct{ workDir string }

func (grepTool) Name() string { return "grep" }

func (grepTool) Description() string {
	return "Search for a regular expression in a file, or recursively under a directory. Returns matching lines as path:line:text. Set sort_by=relevance to rank results by match density (most relevant first) instead of the default path-based order. Set max_matches to increase limit (default 500, max 2000)."
}

func (grepTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Regular expression (RE2 syntax)"},"path":{"type":"string","description":"File or directory to search (default \".\")"},"max_matches":{"type":"integer","description":"Maximum matches to return (default 500, max 2000)"},"sort_by":{"type":"string","enum":["path","relevance"],"description":"Sort order: path (default, by file path then line number) or relevance (by match density, most relevant first)"}},"required":["pattern"]}`)
}

func (grepTool) ReadOnly() bool { return true }

func (grepTool) CompactDescription() string { return compactDesc["grep"] }
func (grepTool) CompactSchema() json.RawMessage   { return compactSchema["grep"] }

type grepMatch struct {
	file string
	line int
	text string
}

func (g grepTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		MaxMatches *int   `json:"max_matches,omitempty"`
		SortBy     string `json:"sort_by"`
	}
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

	p.Path = resolveIn(g.workDir, p.Path)
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
		for sc.Scan() {
			ln++
			line := sc.Text()
			if strings.IndexByte(line, 0) >= 0 {
				return nil // looks binary, skip the file
			}
			if re.MatchString(line) {
				out = append(out, grepMatch{file: file, line: ln, text: line})
				if len(out) >= maxMatches {
					truncated = true
					return io.EOF
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
		fmt.Fprintf(&b, "%s:%d:%s\n", m.file, m.line, m.text)
	}
	res := strings.TrimSuffix(b.String(), "\n")
	if truncated {
		res += fmt.Sprintf("\n... (truncated at %d matches)", maxMatches)
	}
	return res, nil
}

// sortByRelevance sorts matches by match density per file (descending), then
// within each file by line number. Files with more matches relative to their
// size appear first — this surfaces the most relevant files at the top.
func sortByRelevance(matches []grepMatch) {
	// Count matches per file
	perFile := make(map[string]int)
	for _, m := range matches {
		perFile[m.file]++
	}

	// Sort: by match count per file (desc), then file path, then line number
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
