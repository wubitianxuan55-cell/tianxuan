package agent

import (
	"strings"
	"testing"

	"tianxuan/internal/strutil"
)

// TestCompressGrepGroupsByFile 验证 grep 结果按文件分组，每文件保留首尾。
func TestCompressGrepGroupsByFile(t *testing.T) {
	input := strings.Join([]string{
		"src/a.go:1: package main",
		"src/a.go:5: func main() {",
		"src/a.go:10: return nil",
		"src/b.go:2: import fmt",
		"src/b.go:8: fmt.Println(x)",
	}, "\n")

	result := compressGrep(input)

	// 每个文件至少出现一次，首条和末条应保留
	if !strings.Contains(result, "src/a.go:1:") {
		t.Error("missing first line of a.go")
	}
	if !strings.Contains(result, "src/a.go:10:") {
		t.Error("missing last line of a.go")
	}
	if !strings.Contains(result, "src/b.go:2:") {
		t.Error("missing first line of b.go")
	}
	if !strings.Contains(result, "src/b.go:8:") {
		t.Error("missing last line of b.go")
	}
}

// TestCompressGrepKeepsErrorLines 验证错误行被优先保留。
func TestCompressGrepKeepsErrorLines(t *testing.T) {
	input := strings.Join([]string{
		"log.go:1: INFO startup",
		"log.go:2: DEBUG config loaded",
		"log.go:3: ERROR connection refused",
		"log.go:4: FATAL cannot proceed",
		"log.go:5: INFO retry",
		"log.go:6: WARN timeout",
		"log.go:7: DEBUG done",
	}, "\n")

	result := compressGrep(input)

	// 错误/致命行必须保留
	if !strings.Contains(result, "ERROR") {
		t.Error("ERROR line should be kept")
	}
	if !strings.Contains(result, "FATAL") {
		t.Error("FATAL line should be kept")
	}
}

// TestCompressGrepGlobalCap 验证全局上限（30条匹配，15个文件）。
func TestCompressGrepGlobalCap(t *testing.T) {
	var lines []string
	// 生成 20 个文件，每个 10 行
	for f := 0; f < 20; f++ {
		fname := string(rune('a'+f%26)) + ".go"
		if f >= 26 {
			fname = string(rune('a'+f/26-1)) + string(rune('a'+f%26)) + ".go"
		}
		for l := 1; l <= 10; l++ {
			lines = append(lines, fname+":"+strutil.Itoa(l)+": line content here some text")
		}
	}
	input := strings.Join(lines, "\n")

	result := compressGrep(input)

	// 不应超过 30 条匹配
	matchCount := 0
	for _, line := range strings.Split(result, "\n") {
		if strings.Contains(line, ": line content") {
			matchCount++
		}
	}
	if matchCount > 30 {
		t.Errorf("got %d matches, want at most 30", matchCount)
	}
}

// TestCompressGrepPassthrough 验证少量结果原样返回。
func TestCompressGrepPassthrough(t *testing.T) {
	input := "src/main.go:5: func main()"

	result := compressGrep(input)

	if result != input {
		t.Errorf("passthrough failed:\ngot:  %s\nwant: %s", result, input)
	}
}

// TestCompressGrepEmpty 验证空输入。
func TestCompressGrepEmpty(t *testing.T) {
	result := compressGrep("")
	if result != "" {
		t.Errorf("empty input should return empty, got %q", result)
	}
}

// TestCompressGrepWindowsPath 验证 Windows 盘符路径不会被误解析。
func TestCompressGrepWindowsPath(t *testing.T) {
	input := strings.Join([]string{
		`C:\Users\dev\src\app.go:10: func handler() {`,
		`C:\Users\dev\src\app.go:25: return result`,
		`D:\other\lib.go:5: import "fmt"`,
	}, "\n")

	result := compressGrep(input)

	if !strings.Contains(result, `C:\Users\dev\src\app.go:10:`) {
		t.Error("Windows path with drive letter not preserved")
	}
	if !strings.Contains(result, `C:\Users\dev\src\app.go:25:`) {
		t.Error("last line of Windows path not preserved")
	}
}

// TestCompressGrepDashInFilename 验证文件名含横线不会被误解析。
func TestCompressGrepDashInFilename(t *testing.T) {
	input := strings.Join([]string{
		"pre-commit-config.yaml:42: repos:",
		"pre-commit-config.yaml:55: hooks:",
	}, "\n")

	result := compressGrep(input)

	if !strings.Contains(result, "pre-commit-config.yaml:42:") {
		t.Error("filename with dash not preserved")
	}
}

// TestCompressTreeCollapsesNodeModules 验证折叠 node_modules 等噪声目录。
func TestCompressTreeCollapsesNodeModules(t *testing.T) {
	input := strings.Join([]string{
		"src/",
		"  main.go",
		"  utils.go",
		"node_modules/",
		"  express/",
		"    index.js",
		"  lodash/",
		"    lodash.js",
		"dist/",
		"  bundle.js",
	}, "\n")

	result := compressTree(input)

	if strings.Contains(result, "node_modules/") && !strings.Contains(result, "hidden") {
		t.Error("node_modules should be collapsed with hidden marker")
	}
	if strings.Contains(result, "dist/") && !strings.Contains(result, "hidden") {
		t.Error("dist should be collapsed with hidden marker")
	}
	if !strings.Contains(result, "src/") {
		t.Error("src/ should be preserved")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("main.go should be preserved")
	}
}

// TestCompressTreePassthrough 验证无明显噪声时不修改。
func TestCompressTreePassthrough(t *testing.T) {
	input := strings.Join([]string{
		"src/",
		"  agent/",
		"    agent.go",
		"  cache/",
		"    runtime.go",
	}, "\n")

	result := compressTree(input)
	if result != input {
		t.Errorf("clean tree should pass through unchanged:\ngot:  %s\nwant: %s", result, input)
	}
}

// TestCompressGrepEmptyLines 验证空行被跳过。
func TestCompressGrepEmptyLines(t *testing.T) {
	input := "app.go:1: line1\n\napp.go:2: line2\n\n"
	result := compressGrep(input)

	if strings.Count(result, "\n") > 1 {
		t.Errorf("empty lines should be skipped, got %d lines", strings.Count(result, "\n")+1)
	}
}
