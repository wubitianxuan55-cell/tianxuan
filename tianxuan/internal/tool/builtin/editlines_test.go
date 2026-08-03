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

// TestEditLinesStartAnchorMismatch 锚点校验：start_anchor 与实际 start_line 行
// 内容不一致时必须拒绝，且文件保持原样（防止行号偏移后误删/误改行）。
func TestEditLinesStartAnchorMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anchor.txt")
	orig := "line one\nline two\nline three\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	_, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  2,
		"end_line":    2,
		"start_anchor": "line nope", // 与实际 "line two" 不一致
		"new_content": "line TWO",
	}))
	if err == nil {
		t.Fatal("expected anchor mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "start_anchor") {
		t.Errorf("error should mention start_anchor, got: %v", err)
	}
	if got := readTestFile(t, path); got != orig {
		t.Errorf("file modified on anchor mismatch:\n  got: %q\n want: %q", got, orig)
	}
}

// TestEditLinesEndAnchorMismatch end_anchor 不匹配时同样拒绝且不写文件。
func TestEditLinesEndAnchorMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anchor_end.txt")
	orig := "line one\nline two\nline three\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	_, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  1,
		"end_line":    2,
		"end_anchor":  "line WRONG",
		"new_content": "replacement",
	}))
	if err == nil {
		t.Fatal("expected end_anchor mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "end_anchor") {
		t.Errorf("error should mention end_anchor, got: %v", err)
	}
	if got := readTestFile(t, path); got != orig {
		t.Errorf("file modified on end_anchor mismatch:\n  got: %q\n want: %q", got, orig)
	}
}

// TestEditLinesAnchorsMatch 锚点与实际内容一致时正常执行替换。
func TestEditLinesAnchorsMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anchor_ok.txt")
	orig := "line one\nline two\nline three\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":         path,
		"start_line":   2,
		"end_line":     2,
		"start_anchor": "line two",
		"end_anchor":   "line two",
		"new_content":  "line TWO",
	})); err != nil {
		t.Fatalf("edit_lines with matching anchors: %v", err)
	}

	got := readTestFile(t, path)
	want := "line one\nline TWO\nline three\n"
	if got != want {
		t.Errorf("matching anchors edit:\n  got: %q\n want: %q", got, want)
	}
}

// TestEditLinesAnchorCRLFNormalized CRLF 文件 + LF 锚点：锚点比对前做换行
// 归一化，行内容本身不含 \r 时依然匹配。
func TestEditLinesAnchorCRLFNormalized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anchor_crlf.txt")
	orig := "line one\r\nline two\r\nline three\r\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":         path,
		"start_line":   2,
		"end_line":     2,
		"start_anchor": "line two",
		"new_content":  "line TWO",
	})); err != nil {
		t.Fatalf("CRLF file with LF anchor: %v", err)
	}

	got := readTestFile(t, path)
	want := "line one\r\nline TWO\r\nline three\r\n"
	if got != want {
		t.Errorf("CRLF anchor edit:\n  got: %q\n want: %q", got, want)
	}
}

// TestEditLinesValidateGoSyntaxRollback .go 文件编辑后语法错误：gofmt -e 校验
// 失败，自动回滚到编辑前内容并返回错误。
func TestEditLinesValidateGoSyntaxRollback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	orig := "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	_, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  4,
		"end_line":    4,
		"new_content": "func broken(", // 语法错误
	}))
	if err == nil {
		t.Fatal("expected syntax validation error, got nil")
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Errorf("error should mention rollback, got: %v", err)
	}
	if got := readTestFile(t, path); got != orig {
		t.Errorf("syntax-broken edit not rolled back:\n  got: %q\n want: %q", got, orig)
	}
}

// TestEditLinesValidateGoSyntaxOK .go 文件编辑后语法合法：校验通过，正常写入。
func TestEditLinesValidateGoSyntaxOK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main_ok.go")
	orig := "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  4,
		"end_line":    4,
		"new_content": `println("HELLO")`,
	})); err != nil {
		t.Fatalf("valid go edit: %v", err)
	}

	got := readTestFile(t, path)
	if !strings.Contains(got, `println("HELLO")`) {
		t.Errorf("valid edit not applied:\n  got: %q", got)
	}
}

// TestEditLinesValidateDisabled validate=false 时跳过自动语法校验（显式关闭）。
func TestEditLinesValidateDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main_off.go")
	orig := "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{dir}}
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        path,
		"start_line":  4,
		"end_line":    4,
		"new_content": "func broken(",
		"validate":    false,
	})); err != nil {
		t.Fatalf("validate=false should skip check: %v", err)
	}
	if got := readTestFile(t, path); !strings.Contains(got, "func broken(") {
		t.Errorf("validate=false edit not applied:\n  got: %q", got)
	}
}

// writeFakeTSProject 构造一个最小 TS 项目：tsconfig.json + node_modules/.bin/
// tsc.cmd（Windows）。fake tsc 在项目根存在 fail.flag 时退出 1，否则退出 0，
// 用于隔离测试"编辑后 tsc 校验失败回滚"的完整流程。
func writeFakeTSProject(t *testing.T) (projectDir, file string) {
	t.Helper()
	projectDir = filepath.Join(t.TempDir(), "web")
	binDir := filepath.Join(projectDir, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "tsconfig.json"),
		[]byte(`{"compilerOptions":{"strict":true},"files":["app.ts"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	file = filepath.Join(projectDir, "app.ts")
	if err := os.WriteFile(file, []byte("const x: number = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "@echo off\r\nif exist \"%~dp0..\\..\\fail.flag\" exit /b 1\r\nexit /b 0\r\n"
	if err := os.WriteFile(filepath.Join(binDir, "tsc.cmd"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	return projectDir, file
}

// TestEditLinesValidateTSRollback .ts 文件 + 项目内 tsc 校验失败（退出 1）：
// 自动回滚。
func TestEditLinesValidateTSRollback(t *testing.T) {
	proj, file := writeFakeTSProject(t)
	if err := os.WriteFile(filepath.Join(proj, "fail.flag"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	el := editLines{roots: []string{proj}}
	_, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        file,
		"start_line":  1,
		"end_line":    1,
		"new_content": "const y: number = 2;",
	}))
	if err == nil {
		t.Fatal("expected tsc validation error, got nil")
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Errorf("error should mention rollback, got: %v", err)
	}
	if got := readTestFile(t, file); got != "const x: number = 1;\n" {
		t.Errorf("tsc-failed edit not rolled back:\n  got: %q", got)
	}
}

// TestEditLinesValidateTSOK .ts 文件 + 项目内 tsc 校验通过：正常写入。
func TestEditLinesValidateTSOK(t *testing.T) {
	_, file := writeFakeTSProject(t)

	el := editLines{roots: []string{filepath.Dir(file)}}
	if _, err := el.Execute(t.Context(), argsJSON(t, map[string]any{
		"path":        file,
		"start_line":  1,
		"end_line":    1,
		"new_content": "const y: number = 2;",
	})); err != nil {
		t.Fatalf("tsc-ok edit: %v", err)
	}
	if got := readTestFile(t, file); got != "const y: number = 2;\n" {
		t.Errorf("tsc-ok edit not applied:\n  got: %q", got)
	}
}
