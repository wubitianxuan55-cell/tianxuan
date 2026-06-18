//go:build ignore

// long_session_bench 测量单次长会话（20+ 步）的缓存命中率曲线
// 用法: go run long_session_bench.go --v14 <bin> --v15 <bin>
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
)

type perTurn struct {
	Total     int // total tokens
	PromptIn  int // prompt tokens
	Cached    int // cached tokens
	New       int // new/miss tokens
	Completion int
}

func main() {
	v14Bin := flag.String("v14", "", "V1.4 binary")
	v15Bin := flag.String("v15", "", "V1.5 binary")
	model := flag.String("model", "deepseek-v4-flash", "")
	flag.Parse()

	if *v14Bin == "" || *v15Bin == "" {
		fmt.Fprintln(os.Stderr, "usage: --v14 <bin> --v15 <bin>")
		os.Exit(1)
	}

	// 创建一个有 20 个 Go 源文件的项目 — 足够复杂以触发 20+ API 调用
	projectDir := createProject()

	prompt := `请逐步完成以下任务，每步只做一件事：

1. 读取 pkg/math/basic.go，列出所有函数名
2. 读取 pkg/math/advanced.go，列出所有函数名
3. 读取 pkg/strings/ops.go，列出所有函数名
4. 读取 pkg/strings/format.go，列出所有函数名
5. 读取 pkg/data/store.go，列出所有函数名
6. 读取 pkg/data/query.go，列出所有函数名
7. 读取 pkg/auth/login.go，列出所有函数名
8. 读取 pkg/auth/perms.go，列出所有函数名
9. 在 pkg/math/basic.go 的 Add 函数上面添加注释："Add returns the sum of two integers."
10. 在 pkg/strings/ops.go 的 Concat 函数上面添加注释："Concat joins two strings."
11. 为 pkg/math/basic.go 的所有函数编写测试到 pkg/math/basic_test.go
12. 运行 go test ./pkg/math/ 确认测试通过
13. 为 pkg/strings/ops.go 的所有函数编写测试到 pkg/strings/ops_test.go
14. 运行 go test ./pkg/strings/ 确认测试通过
15. 创建一个 README.md，总结所有包中的函数及其用途

每完成一步，标记完成后再做下一步。`

	fmt.Println("=== Single-Session Long-Run Cache Test (20+ steps) ===")
	fmt.Printf("Project: %s\n\n", projectDir)
	defer os.RemoveAll(projectDir)

	// 运行 V1.4
	fmt.Println("--- V1.4 ---")
	v14Turns := runAndParse(*v14Bin, *model, projectDir, prompt, 50)

	// 清理 agent 产生的文件，恢复到初始状态
	resetProject(projectDir)

	// 运行 V1.5
	fmt.Println("\n--- V1.5 ---")
	v15Turns := runAndParse(*v15Bin, *model, projectDir, prompt, 50)

	// 报告
	fmt.Println("\n========================================")
	fmt.Println("  单会话长跑缓存命中率对比 (>20 steps)")
	fmt.Println("========================================")
	fmt.Println()

	// 逐 turn 对比表（取较长的那个）
	maxTurns := len(v14Turns)
	if len(v15Turns) > maxTurns {
		maxTurns = len(v15Turns)
	}

	// 只显示关键 turn
	fmt.Println("| Turn | V1.4 Hit | V1.5 Hit | V1.4 Δ | V1.5 Δ |")
	fmt.Println("|------|----------|----------|--------|--------|")
	for i := 0; i < maxTurns; i++ {
		var r14, r15 string
		if i < len(v14Turns) {
			r14 = fmt.Sprintf("%.0f%%", hitPct(v14Turns[i]))
		} else {
			r14 = "-"
		}
		if i < len(v15Turns) {
			r15 = fmt.Sprintf("%.0f%%", hitPct(v15Turns[i]))
		} else {
			r15 = "-"
		}
		// 跳跃显示：前5轮 + 最后5轮，中间省略
		if i < 5 || i >= maxTurns-5 || maxTurns <= 12 {
			fmt.Printf("| %4d | %8s | %8s | %6s | %6s |\n", i+1, r14, r15, "", "")
		} else if i == 5 {
			fmt.Println("|  ... | ... | ... | ... | ... |")
		}
	}

	// 汇总
	fmt.Println()
	agg14 := aggregate(v14Turns)
	agg15 := aggregate(v15Turns)

	fmt.Println("| 指标 | V1.4 | V1.5 | 变化 |")
	fmt.Println("|------|------|------|------|")
	fmt.Printf("| API 调用数 | %d | %d | — |\n", len(v14Turns), len(v15Turns))
	fmt.Printf("| 总 prompt tokens | %d | %d | %.4f%% |\n",
		agg14.prompt, agg15.prompt, pctChange(agg14.prompt, agg15.prompt))
	fmt.Printf("| 缓存命中 tokens | %d | %d | — |\n", agg14.hit, agg15.hit)
	fmt.Printf("| 缓存未命中 tokens | %d | %d | %.4f%% |\n",
		agg14.miss, agg15.miss, pctChange(agg14.miss, agg15.miss))
	r14 := float64(agg14.hit) / float64(agg14.hit+agg14.miss) * 100
	r15 := float64(agg15.hit) / float64(agg15.hit+agg15.miss) * 100
	fmt.Printf("| **聚合命中率** | **%.4f%%** | **%.4f%%** | %+.4f pp |\n",
		r14, r15, r15-r14)

	// 尾部命中率（最后 3 轮平均，排除冷启动）
	tail14 := tailAvg(v14Turns, 3)
	tail15 := tailAvg(v15Turns, 3)
	fmt.Printf("| 尾部命中率（最后3轮） | %.4f%% | %.4f%% | %+.4f pp |\n", tail14, tail15, tail15-tail14)

	// 逐轮明细
	fmt.Println()
	fmt.Println("## 逐轮命中率明细")
	fmt.Println("| Turn | V1.4 Hit | V1.5 Hit | Δ |")
	fmt.Println("|------|----------|----------|----|")
	for i := 0; i < maxTurns; i++ {
		var r14s, r15s, delta string
		if i < len(v14Turns) {
			r14s = fmt.Sprintf("%.4f%%", hitPct(v14Turns[i]))
		} else {
			r14s = "-"
		}
		if i < len(v15Turns) {
			r15s = fmt.Sprintf("%.4f%%", hitPct(v15Turns[i]))
		} else {
			r15s = "-"
		}
		if i < len(v14Turns) && i < len(v15Turns) {
			d := hitPct(v15Turns[i]) - hitPct(v14Turns[i])
			delta = fmt.Sprintf("%+.4f", d)
		} else {
			delta = "-"
		}
		if i < 5 || i >= maxTurns-5 || maxTurns <= 14 {
			fmt.Printf("| %4d | %s | %s | %s |\n", i+1, r14s, r15s, delta)
		} else if i == 5 {
			fmt.Println("|  ... | ... | ... | ... |")
		}
	}
}

type aggMetrics struct{ prompt, hit, miss int }

func aggregate(turns []perTurn) aggMetrics {
	var a aggMetrics
	for _, t := range turns {
		a.prompt += t.PromptIn
		a.hit += t.Cached
		a.miss += t.New
	}
	return a
}

func tailAvg(turns []perTurn, n int) float64 {
	if len(turns) == 0 {
		return 0
	}
	if n > len(turns) {
		n = len(turns)
	}
	var sum float64
	for i := len(turns) - n; i < len(turns); i++ {
		sum += hitPct(turns[i])
	}
	return sum / float64(n)
}

func hitPct(t perTurn) float64 {
	d := t.Cached + t.New
	if d == 0 {
		return 0
	}
	return float64(t.Cached) / float64(d) * 100
}

func pctChange(old, new int) float64 {
	if old == 0 {
		return 0
	}
	return float64(new-old) / float64(old) * 100
}

// ---- 项目创建 ----

func createProject() string {
	dir, _ := os.MkdirTemp("", "long-session-bench-")

	// go.mod
	writeFile(dir, "go.mod", "module testproject\ngo 1.21\n")

	// tianxuan.toml
	writeFile(dir, "tianxuan.toml",
		`default_model = "deepseek-v4-flash"
[codegraph]
enabled = false
[[providers]]
name = "deepseek-v4-flash"
kind = "openai"
base_url = "https://api.deepseek.com"
model = "deepseek-v4-flash"
api_key_env = "DEEPSEEK_API_KEY"
context_window = 65536
[agent]
system_prompt = "You are a test agent. Be brief. One step at a time."
max_steps = 50
`)

	// 创建多个包和文件
	mkdir(dir, "pkg/math")
	writeFile(dir, "pkg/math/basic.go", `package math

// Add returns the sum of two integers.
func Add(a, b int) int { return a + b }
func Sub(a, b int) int { return a - b }
func Mul(a, b int) int { return a * b }
func Div(a, b int) int { return a / b }
`)

	writeFile(dir, "pkg/math/advanced.go", `package math

func Pow(base, exp int) int {
	r := 1
	for i := 0; i < exp; i++ { r *= base }
	return r
}
func Abs(x int) int { if x < 0 { return -x }; return x }
func Min(a, b int) int { if a < b { return a }; return b }
func Max(a, b int) int { if a > b { return a }; return b }
`)

	mkdir(dir, "pkg/strings")
	writeFile(dir, "pkg/strings/ops.go", `package strings

func Concat(a, b string) string { return a + b }
func Reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 { r[i], r[j] = r[j], r[i] }
	return string(r)
}
func CountChars(s string) int { return len([]rune(s)) }
`)

	writeFile(dir, "pkg/strings/format.go", `package strings

import "fmt"
func Quote(s string) string { return fmt.Sprintf("%q", s) }
func Upper(s string) string {
	r := []rune(s)
	for i := range r {
		if r[i] >= 'a' && r[i] <= 'z' { r[i] -= 32 }
	}
	return string(r)
}
`)

	mkdir(dir, "pkg/data")
	writeFile(dir, "pkg/data/store.go", `package data
var store = map[string]string{}
func Set(k, v string) { store[k] = v }
func Get(k string) string { return store[k] }
func Delete(k string) { delete(store, k) }
func Count() int { return len(store) }
`)

	writeFile(dir, "pkg/data/query.go", `package data
func Keys() []string {
	ks := make([]string, 0, len(store))
	for k := range store { ks = append(ks, k) }
	return ks
}
func Values() []string {
	vs := make([]string, 0, len(store))
	for _, v := range store { vs = append(vs, v) }
	return vs
}
`)

	mkdir(dir, "pkg/auth")
	writeFile(dir, "pkg/auth/login.go", `package auth
type User struct { Name, Pass string }
var users = map[string]string{}
func Register(name, pass string) bool {
	if _, ok := users[name]; ok { return false }
	users[name] = pass
	return true
}
func Login(name, pass string) bool {
	p, ok := users[name]
	return ok && p == pass
}
`)

	writeFile(dir, "pkg/auth/perms.go", `package auth
type Role int
const (RoleNone Role = iota; RoleUser; RoleAdmin)
var roles = map[string]Role{}
func SetRole(user string, r Role) { roles[user] = r }
func GetRole(user string) Role { return roles[user] }
func IsAdmin(user string) bool { return roles[user] == RoleAdmin }
`)

	mkdir(dir, "pkg/util")
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("util%d.go", i)
		writeFile(dir, "pkg/util/"+name, fmt.Sprintf(`package util
func Helper%d(x int) int { return x * %d }
`, i, i+1))
	}

	return dir
}

func resetProject(dir string) {
	// 删除 agent 产生的文件，恢复初始状态
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		n := e.Name()
		if n == "pkg" || n == "go.mod" || n == "tianxuan.toml" {
			continue
		}
		os.RemoveAll(filepath.Join(dir, n))
	}
	// 恢复被修改的 basic.go 和 ops.go
	writeFile(dir, "pkg/math/basic.go", `package math

func Add(a, b int) int { return a + b }
func Sub(a, b int) int { return a - b }
func Mul(a, b int) int { return a * b }
func Div(a, b int) int { return a / b }
`)
	writeFile(dir, "pkg/strings/ops.go", `package strings

func Concat(a, b string) string { return a + b }
func Reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 { r[i], r[j] = r[j], r[i] }
	return string(r)
}
func CountChars(s string) int { return len([]rune(s)) }
`)
	// 删除测试文件
	os.Remove(filepath.Join(dir, "pkg/math/basic_test.go"))
	os.Remove(filepath.Join(dir, "pkg/strings/ops_test.go"))
	os.Remove(filepath.Join(dir, "README.md"))
}

// ---- 运行与解析 ----

func runAndParse(bin, model, dir, prompt string, maxSteps int) []perTurn {
	metricsPath := filepath.Join(dir, ".run-metrics.json")
	os.Remove(metricsPath)

	args := []string{
		"run", "--metrics", metricsPath,
		"--model", model,
		"--max-steps", strconv.Itoa(maxSteps),
		prompt,
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir

	// 捕获 stdout 用于解析 per-turn 数据
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr

	cmd.Start()

	var turns []perTurn
	// 解析格式: "· 8337 tok · in 8254 (0 cached / 8254 new) · out 83 (7 reasoning)"
	re := regexp.MustCompile(`·\s*(\d+)\s*tok\s*·\s*in\s*(\d+)\s*\((\d+)\s*cached\s*/\s*(\d+)\s*new\)\s*·\s*out\s*(\d+)`)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if len(matches) == 6 {
			total, _ := strconv.Atoi(matches[1])
			promptIn, _ := strconv.Atoi(matches[2])
			cached, _ := strconv.Atoi(matches[3])
			newToks, _ := strconv.Atoi(matches[4])
			completion, _ := strconv.Atoi(matches[5])
			turns = append(turns, perTurn{
				Total:      total,
				PromptIn:   promptIn,
				Cached:     cached,
				New:        newToks,
				Completion: completion,
			})
			rate := 0.0
			if cached+newToks > 0 {
				rate = float64(cached) / float64(cached+newToks) * 100
			}
			fmt.Printf("  turn %2d: prompt=%5d cached=%5d new=%5d → %.4f%%\n",
				len(turns), promptIn, cached, newToks, rate)
		}
	}
	cmd.Wait()

	return turns
}

func writeFile(dir, name, content string) {
	os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
}

func mkdir(dir, sub string) {
	os.MkdirAll(filepath.Join(dir, sub), 0755)
}
