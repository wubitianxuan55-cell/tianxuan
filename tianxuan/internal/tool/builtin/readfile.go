// Package builtin provides Tianxuan's compile-time built-in tools. Each tool
// self-registers via init(); main blank-imports this package to wire them in.
package builtin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(readFile{}) }

// readFile reads a text file. workDir, when non-empty, is the directory a
// relative path is resolved against (see resolveIn); the zero value registered
// at init resolves against the process working directory.
type readFile struct{ workDir string }

const (
	readFileDefaultLimit = 2000     // lines returned when limit is unset
	readFileBinaryPeek   = 8 * 1024 // bytes scanned for a NUL to flag binary
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
	n, _ := io.ReadFull(f, peek)
	peek = peek[:n]

	// A leading BOM marks encoded text whose bytes the NUL check below would
	// otherwise misread: UTF-16 encodes ASCII as paired bytes with a 0x00 half,
	// so a NUL is normal, not a binary signal. Decode such files to UTF-8 and
	// scan that; only the BOM-less common case stays on the streaming path.
	var src io.Reader = f
	if enc := bomEncoding(peek); enc != encUTF8Plain {
		rest, err := io.ReadAll(f)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", p.Path, err)
		}
		src = bytes.NewReader(decodeBOM(append(peek, rest...), enc))
	} else {
		// V5.9: 二进制文件 → 尝试用 markitdown 转为 Markdown
		// 支持 PDF/DOCX/XLSX/PPTX/EPUB/HTML 等文档格式。
		// markitdown 不在 PATH 则回退到原有错误提示。
		if bytes.IndexByte(peek, 0) >= 0 {
			if markdown, ok := tryMarkItDown(p.Path); ok {
				return markdown, nil
			}
			return "", fmt.Errorf(
				"binary file %s (NUL byte detected); install markitdown (pip install markitdown) to auto-convert PDF/Word/Excel/PPT to readable Markdown, or use `bash hexdump` for raw bytes",
				p.Path,
			)
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek %s: %w", p.Path, err)
		}
	}

	// Scan up to offset+limit+1 lines (the extra is just to know whether
	// trimming a trailer is warranted). 1 MB per-line cap matches what other
	// scanners in this package allow — well above any reasonable source line.
	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	upTo := p.Offset + p.Limit + 1

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
	// Drain any remainder to learn the true total without buffering the rest.
	remaining := 0
	for scanner.Scan() {
		remaining++
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read %s: %w", p.Path, err)
	}
	totalSeen := lineNo + remaining

	if totalSeen == 0 {
		return "(empty file)", nil
	}
	if len(collected) == 0 {
		return fmt.Sprintf("(offset %d is past EOF — file has %d lines)", p.Offset, totalSeen), nil
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

	more := totalSeen - (p.Offset + len(collected))
	if more > 0 {
		if showLineNumbers {
			fmt.Fprintf(&b, "\n[%d more line(s); pass offset=%d to continue]\n",
				more, p.Offset+len(collected))
		} else {
			fmt.Fprintf(&b, "\n[%d more line(s); pass offset=%d to continue]\n",
				more, p.Offset+len(collected))
		}
	}
	return b.String(), nil
}

type bomKind int

const (
	encUTF8Plain bomKind = iota // no BOM (or none we special-case)
	encUTF8BOM
	encUTF16LE
	encUTF16BE
)

func bomEncoding(b []byte) bomKind {
	switch {
	case len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF:
		return encUTF8BOM
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		return encUTF16LE
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		return encUTF16BE
	}
	return encUTF8Plain
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

// decodeBOM strips a UTF-8 BOM or decodes UTF-16 to UTF-8, given the kind
// bomEncoding already identified from the same leading bytes.
func decodeBOM(b []byte, enc bomKind) []byte {
	switch enc {
	case encUTF8BOM:
		return b[3:]
	case encUTF16LE, encUTF16BE:
		order := binary.ByteOrder(binary.LittleEndian)
		if enc == encUTF16BE {
			order = binary.BigEndian
		}
		b = b[2:]
		u := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			u = append(u, order.Uint16(b[i:i+2]))
		}
		return []byte(string(utf16.Decode(u)))
	}
	return b
}
