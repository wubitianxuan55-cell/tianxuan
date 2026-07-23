// Package builtin provides Tianxuan's compile-time built-in tools. Each tool
// self-registers via init(); main blank-imports this package to wire them in.
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
	"strings"
	"time"

	"golang.org/x/text/transform"

	fileenc "tianxuan/internal/fileutil/encoding"
	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(readFile{}) }

// readFile reads a text file. workDir, when non-empty, is the directory a
// relative path is resolved against (see resolveIn); the zero value registered
// at init resolves against the process working directory.
type readFile struct{ workDir string }

const (
	readFileDefaultLimit  = 2000      // lines returned when limit is unset
	readFileBinaryPeek    = 8 * 1024  // bytes scanned for a NUL to flag binary
	readFileDetectSample  = 256 << 10 // bytes sampled for encoding detection
)

func (readFile) Name() string { return "read_file" }

func (readFile) Description() string {
	return "Read a text file with optional line offset/limit. By default each line is prefixed with its 1-based number (e.g. `   42→...`). Set line_numbers=false to get raw text — useful when copying content for edit_file. Use `offset` and `limit` to page through large files."
}

func (readFile) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "path":{"type":"string","description":"File path"},
  "offset":{"type":"integer","description":"0-based line offset to start reading from (default 0)","minimum":0},
  "limit":{"type":"integer","description":"Maximum lines to return (default 2000)","minimum":1},
  "line_numbers":{"type":"boolean","description":"Prefix each line with its 1-based line number (default true). Set false for raw text."}
},
"required":["path"]
}`)
}

func (readFile) ReadOnly() bool { return true }
func (readFile) Kind() tool.ToolKind { return tool.KindRead }

func (readFile) CompactDescription() string { return compactDesc["read_file"] }
func (readFile) CompactSchema() json.RawMessage   { return compactSchema["read_file"] }

func (r readFile) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path        string `json:"path"`
		Offset      int    `json:"offset,omitempty"`
		Limit       int    `json:"limit,omitempty"`
		LineNumbers *bool  `json:"line_numbers,omitempty"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	p.Path = resolveIn(r.workDir, p.Path)
	if p.Offset < 0 {
		p.Offset = 0
	}
	if p.Limit <= 0 {
		p.Limit = readFileDefaultLimit
	}
	const readFileMaxLimit = 10000
	if p.Limit > readFileMaxLimit {
		p.Limit = readFileMaxLimit
	}
	// V10.5: line_numbers defaults to true (backward-compatible)
	showLineNumbers := true
	if p.LineNumbers != nil {
		showLineNumbers = *p.LineNumbers
	}

	// A directory can be os.Open'd but not read as text — catch it up front with
	// an actionable message (and avoid the doubled "read X: read X:" the scanner's
	// error would otherwise produce) so the model switches to the ls tool.
	if info, err := os.Stat(p.Path); err == nil && info.IsDir() {
		return "", fmt.Errorf("%s is a directory, not a file — use the ls tool to list it, or read a specific file inside it", p.Path)
	}

	f, err := os.Open(p.Path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", p.Path, err)
	}
	defer f.Close()

	peek := make([]byte, readFileBinaryPeek)
	pn, perr := io.ReadFull(f, peek)
	peek = peek[:pn]
	peekEOF := perr != nil // whole file fit in the peek (EOF / ErrUnexpectedEOF)

	var src io.Reader

	// BOM check first: UTF-16 files contain 0x00 for every ASCII character, so a
	// naive NUL check would misidentify them as binary.
	if k := fileenc.DetectQuick(peek); k != fileenc.UTF8 {
		// UTF-16 is not self-synchronising — buffer it fully.
		rest, rerr := io.ReadAll(f)
		if rerr != nil {
			return "", fmt.Errorf("read %s: %w", p.Path, rerr)
		}
		src = bytes.NewReader(fileenc.Decode(append(peek, rest...), fileenc.DetectQuick(append(peek, rest...))))
	} else if k, ok := fileenc.DetectUTF16NoBOM(peek); ok {
		// BOM-less UTF-16 (Windows source files) — recognise by NUL pattern.
		rest, rerr := io.ReadAll(f)
		if rerr != nil {
			return "", fmt.Errorf("read %s: %w", p.Path, rerr)
		}
		src = bytes.NewReader(fileenc.Decode(append(peek, rest...), k))
	} else {
		// V5.9: 二进制文件 → 尝试用 markitdown 转为 Markdown
		if bytes.IndexByte(peek, 0) >= 0 {
			if markdown, ok := tryMarkItDown(p.Path); ok {
				return markdown, nil
			}
			return "", fmt.Errorf(
				"binary file %s (NUL byte detected); install markitdown (pip install markitdown) to auto-convert PDF/Word/Excel/PPT to readable Markdown, or use `bash hexdump` for raw bytes",
				p.Path,
			)
		}

		// Read up to a bounded sample for encoding detection (GB18030/GBK).
		head := peek
		if !peekEOF {
			more := make([]byte, readFileDetectSample-len(peek))
			mn, _ := io.ReadFull(f, more)
			head = append(peek, more[:mn]...)
		}
		sample := head
		enc, _ := fileenc.Detect(sample)
		switch enc {
		case fileenc.UTF8, fileenc.LossyUTF8:
			// Plain UTF-8 — stream directly.
			src = io.MultiReader(bytes.NewReader(head), f)
		case fileenc.GB18030:
			// GB18030/GBK — decode on the fly via streaming decoder.
			src = transform.NewReader(io.MultiReader(bytes.NewReader(head), f),
				fileenc.Decoder(enc))
		default:
			// Other encodings — full decode via Decoder.
			src = io.MultiReader(bytes.NewReader(head), f)
			if dec := fileenc.Decoder(enc); dec != nil {
				src = transform.NewReader(src, dec)
			}
		}
	}

	// Scan up to offset+limit+1 lines (the extra is just to know whether
	// trimming a trailer is warranted). 1 MB per-line cap matches what other
	// scanners in this package allow — well above any reasonable source line.
	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	upTo := p.Offset + p.Limit + 1

	// Check for cancellation before potentially long scan
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	var collected []string
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if lineNo > p.Offset && len(collected) < p.Limit {
			collected = append(collected, scanner.Text())
		}
		if lineNo >= upTo {
			// Keep counting to know how many more lines remain.
			break
		}
	}
	// Quick check for remaining lines without draining the entire file.
	// (Avoids O(n) scan over huge files just to compute "more lines" count.)
	hasMore := scanner.Scan()
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read %s: %w", p.Path, err)
	}

	if lineNo == 0 && !hasMore {
		return "(empty file)", nil
	}
	if len(collected) == 0 {
		return fmt.Sprintf("(offset %d is past EOF — file has %d lines)", p.Offset, lineNo), nil
	}

	var b strings.Builder

	if showLineNumbers {
		// Right-align line numbers to the largest one we'll print, so the arrow
		// "→" column lines up. Add 1 for the 1-based display.
		maxShown := p.Offset + len(collected)
		w := len(fmt.Sprint(maxShown))
		for i, line := range collected {
			fmt.Fprintf(&b, "%*d→%s\n", w, p.Offset+i+1, line)
		}
	} else {
		// V10.5: raw text mode — no line numbers, useful for copying to edit_file
		for _, line := range collected {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	if hasMore {
		if showLineNumbers {
			fmt.Fprintf(&b, "\n[more lines available; pass offset=%d to continue]\n",
				p.Offset+len(collected))
		} else {
			fmt.Fprintf(&b, "\n[more lines available; pass offset=%d to continue]\n",
				p.Offset+len(collected))
		}
	}
	return b.String(), nil
}


// V5.9: markitdown 集成 —— 将二进制文档自动转为 Markdown

// 支持的文档扩展名列表
var markitdownExtensions = map[string]bool{
	".pdf": true, ".docx": true, ".xlsx": true, ".xls": true,
	".pptx": true, ".epub": true, ".html": true, ".htm": true,
	".csv": true, ".ipynb": true,
}

// tryMarkItDown 尝试用 markitdown CLI 将文件转为 Markdown。
// 返回 (markdown, true) 表示成功，(_, false) 表示不可用或转换失败。
func tryMarkItDown(path string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	if !markitdownExtensions[ext] {
		return "", false // 不支持的格式
	}

	// 优先用 markitdown CLI，未安装则回退到 `python -m markitdown`
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var out []byte
	var runErr error
	if p, err := exec.LookPath("markitdown"); err == nil {
		cmd := exec.CommandContext(ctx, p, path)
		hideBashWindow(cmd)
		out, runErr = cmd.Output()
	} else if p, err := exec.LookPath("python3"); err == nil {
		cmd := exec.CommandContext(ctx, p, "-m", "markitdown", path)
		hideBashWindow(cmd)
		out, runErr = cmd.Output()
	} else if p, err := exec.LookPath("python"); err == nil {
		cmd := exec.CommandContext(ctx, p, "-m", "markitdown", path)
		hideBashWindow(cmd)
		out, runErr = cmd.Output()
	} else {
		return "", false
	}
	if runErr != nil {
		return "", false
	}

	result := strings.TrimSpace(string(out))
	if result == "" {
		return "", false
	}
	return result, true
}

