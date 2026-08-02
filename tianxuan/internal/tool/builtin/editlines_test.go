package builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEditLinesCRLFContentNoRLeak 验证：CRLF 文件中，AI 发送的 new_content
// 若自带 \r\n（Windows 环境常见），edit_lines 不得把 \r 泄漏进文件
// （历史 bug：产生 \r\r\n，文件被污染后 AI 只能整文件重写）。
func TestEditLinesCRLFContentNoRLeak(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crlf.txt")
	orig := "line one\r\nline two\r\nline three\r\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  2,
		"end_line":    2,
		"new_content": "line TWO\r\n", // 带 CRLF 的输入必须被归一化
	})); err != nil {
		t.Fatalf("edit_lines: %v", err)
	}

	got := readTestFile(t, path)
	want := "line one\r\nline TWO\r\nline three\r\n"
	if got != want {
		t.Errorf("CRLF file edited with CRLF new_content:\n  got: %q\n want: %q", got, want)
	}
	if strings.Contains(got, "\r\r") {
		t.Errorf("CR leaked (found \\r\\r): %q", got)
	}
}

// TestEditLinesCRLFWithLFContent 回归保护：CRLF 文件 + LF new_content
// 是最常见路径，必须保持纯 CRLF 输出。
func TestEditLinesCRLFWithLFContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crlf_lf.txt")
	orig := "line one\r\nline two\r\nline three\r\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  2,
		"end_line":    2,
		"new_content": "line TWO\n",
	})); err != nil {
		t.Fatalf("edit_lines: %v", err)
	}

	got := readTestFile(t, path)
	want := "line one\r\nline TWO\r\nline three\r\n"
	if got != want {
		t.Errorf("CRLF file edited with LF new_content:\n  got: %q\n want: %q", got, want)
	}
}

// TestEditLinesMixedLineEndings 验证：混合换行文件按 \n 计行（与 read_file
// 的 bufio.Scanner 一致），编辑后输出统一为检测到的风格，不吞行。
func TestEditLinesMixedLineEndings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mixed.txt")
	// 3 行，第 1、3 行 CRLF，第 2 行 LF
	orig := "line one\r\nline two\nline three\r\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	// 改第 2 行 —— 若按 \r\n 分割则 totalLines=2，会吞掉第 3 行
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  2,
		"end_line":    2,
		"new_content": "line TWO",
	})); err != nil {
		t.Fatalf("edit_lines: %v", err)
	}

	got := readTestFile(t, path)
	want := "line one\r\nline TWO\r\nline three\r\n" // 输出统一 CRLF，第 3 行保留
	if got != want {
		t.Errorf("mixed file edited:\n  got: %q\n want: %q", got, want)
	}
}

// TestEditLinesLFWithCRLFContent 验证：LF 文件 + CRLF new_content
// 不得产生混合换行。
func TestEditLinesLFWithCRLFContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lf.txt")
	orig := "line one\nline two\nline three\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  2,
		"end_line":    2,
		"new_content": "line TWO\r\n", // 输入 CRLF，输出必须保持纯 LF
	})); err != nil {
		t.Fatalf("edit_lines: %v", err)
	}

	got := readTestFile(t, path)
	want := "line one\nline TWO\nline three\n"
	if got != want {
		t.Errorf("LF file edited with CRLF new_content:\n  got: %q\n want: %q", got, want)
	}
}

// TestEditLinesNoTrailingNewline 验证：文件末尾无换行时编辑正常。
func TestEditLinesNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notrail.txt")
	orig := "line one\nline two\nline three" // 无尾随换行
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  3,
		"end_line":    3,
		"new_content": "line THREE",
	})); err != nil {
		t.Fatalf("edit_lines: %v", err)
	}

	got := readTestFile(t, path)
	want := "line one\nline two\nline THREE" // 无尾随换行保持无尾随换行
	if got != want {
		t.Errorf("no-trailing-newline file edited:\n  got: %q\n want: %q", got, want)
	}
}

// TestEditLinesDeleteRangeCRLF 验证：CRLF 文件中删除行不产生 \r 泄漏。
func TestEditLinesDeleteRangeCRLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "del.txt")
	orig := "line one\r\nline two\r\nline three\r\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  2,
		"end_line":    2,
		"new_content": "",
	})); err != nil {
		t.Fatalf("edit_lines delete: %v", err)
	}

	got := readTestFile(t, path)
	want := "line one\r\nline three\r\n"
	if got != want {
		t.Errorf("delete line in CRLF file:\n  got: %q\n want: %q", got, want)
	}
	if strings.Contains(got, "\r\r") {
		t.Errorf("CR leaked (found \\r\\r): %q", got)
	}
}

// TestEditLinesRepeatedEditsNoAccumulation 验证：连续多次编辑（其中含
// 带 \r\n 的 new_content）不会累积换行错误。
func TestEditLinesRepeatedEditsNoAccumulation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repeat.txt")
	orig := "line one\r\nline two\r\nline three\r\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	edits := []struct {
		start, end int
		content    string
	}{
		{1, 1, "line ONE\r\n"},
		{2, 2, "line TWO"},
		{3, 3, "line THREE\r\n"},
	}
	for i, e := range edits {
		if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
			"path":        path,
			"start_line":  e.start,
			"end_line":    e.end,
			"new_content": e.content,
		})); err != nil {
			t.Fatalf("edit %d: %v", i+1, err)
		}
	}

	got := readTestFile(t, path)
	want := "line ONE\r\nline TWO\r\nline THREE\r\n"
	if got != want {
		t.Errorf("repeated edits:\n  got: %q\n want: %q", got, want)
	}
	if strings.Contains(got, "\r\r") {
		t.Errorf("CR leaked (found \\r\\r): %q", got)
	}
}

// TestEditLinesAppendAtEnd 验证：end_line 顶到文件末尾时可追加新行。
func TestEditLinesAppendAtEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "append.txt")
	orig := "line one\nline two\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  2,
		"end_line":    2,
		"new_content": "line two\nline three",
	})); err != nil {
		t.Fatalf("edit_lines append: %v", err)
	}

	got := readTestFile(t, path)
	want := "line one\nline two\nline three\n"
	if got != want {
		t.Errorf("append at end:\n  got: %q\n want: %q", got, want)
	}
}

// TestEditLinesPreserveTrailingEmptyLine 复现"吞行"bug：文件末尾存在空行
// （以两个换行结尾）时，编辑中间行会把末尾空行吞掉。
// 根因：重建尾部时用 `out` 最后一个元素是否为 "" 判断"是否已有尾随换行"，
// 但 `out` 以 "" 结尾表示的是"文件末尾是空行"，join 后仅产生一个换行，
// 缺少该空行所需的第二个换行。
func TestEditLinesPreserveTrailingEmptyLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trailing_empty_lf.txt")
	// 3 行：第 3 行为空行
	orig := "line one\nline two\n\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  2,
		"end_line":    2,
		"new_content": "line TWO",
	})); err != nil {
		t.Fatalf("edit_lines: %v", err)
	}

	got := readTestFile(t, path)
	want := "line one\nline TWO\n\n" // 末尾空行必须保留
	if got != want {
		t.Errorf("trailing empty line swallowed:\n  got: %q\n want: %q", got, want)
	}
}

// TestEditLinesPreserveTrailingEmptyLineCRLF CRLF 变体：同样的空行吞没问题，
// 且不得产生 \r 泄漏。
func TestEditLinesPreserveTrailingEmptyLineCRLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trailing_empty_crlf.txt")
	// 3 行：第 3 行为空行
	orig := "line one\r\nline two\r\n\r\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  2,
		"end_line":    2,
		"new_content": "line TWO",
	})); err != nil {
		t.Fatalf("edit_lines: %v", err)
	}

	got := readTestFile(t, path)
	want := "line one\r\nline TWO\r\n\r\n"
	if got != want {
		t.Errorf("trailing empty line swallowed (CRLF):\n  got: %q\n want: %q", got, want)
	}
	if strings.Contains(got, "\r\r") {
		t.Errorf("CR leaked (found \\r\\r): %q", got)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
