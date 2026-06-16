// cross_session_bench 测量同一目录下两次 tianxuan run 的跨会话缓存命中率
// 用法: go run cross_session_bench.go --v14 <v14-bin> --v15 <v15-bin>
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type runMetrics struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	CacheHitTokens   int     `json:"cache_hit_tokens"`
	CacheMissTokens  int     `json:"cache_miss_tokens"`
	Steps            int     `json:"steps"`
	Cost             float64 `json:"cost"`
}

func main() {
	v14Bin := flag.String("v14", "", "path to V1.4 tianxuan binary")
	v15Bin := flag.String("v15", "", "path to V1.5 tianxuan binary")
	model := flag.String("model", "deepseek-v4-flash", "model name")
	flag.Parse()

	if *v14Bin == "" || *v15Bin == "" {
		fmt.Fprintln(os.Stderr, "usage: --v14 <path> --v15 <path>")
		os.Exit(1)
	}

	// Create a shared project directory with some .go files
	projectDir, err := os.MkdirTemp("", "cross-session-bench-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(projectDir)

	// Seed the directory with files so Profile scan finds content
	writeFile(projectDir, "main.go", `package main
import "fmt"
func main() { fmt.Println("hello") }
`)
	writeFile(projectDir, "go.mod", `module testproject
go 1.21
`)
	writeFile(projectDir, "tianxuan.toml", fmt.Sprintf(`
default_model = "%s"

[codegraph]
enabled = false

[[providers]]
name = "%s"
kind = "openai"
base_url = "https://api.deepseek.com"
model = "%s"
api_key_env = "DEEPSEEK_API_KEY"
context_window = 65536

[agent]
system_prompt = "You are a test agent. Be brief."
`, *model, *model, *model))
	writeFile(projectDir, "lib.go", `package main
func add(a, b int) int { return a + b }
`)

	prompt := "read main.go and lib.go, then write a test for add() in main_test.go"

	fmt.Println("=== Cross-Session Cache Benchmark ===")
	fmt.Printf("Project dir: %s\n", projectDir)
	fmt.Printf("Model: %s\n\n", *model)

	// Test V1.4
	fmt.Println("--- V1.4 ---")
	run1V14 := runAgent(*v14Bin, *model, projectDir, prompt)
	time.Sleep(500 * time.Millisecond) // let DeepSeek cache settle
	run2V14 := runAgent(*v14Bin, *model, projectDir, prompt)

	// Test V1.5
	fmt.Println("\n--- V1.5 ---")
	run1V15 := runAgent(*v15Bin, *model, projectDir, prompt)
	time.Sleep(500 * time.Millisecond)
	run2V15 := runAgent(*v15Bin, *model, projectDir, prompt)

	// Report
	fmt.Println("\n========================================")
	fmt.Println("           CROSS-SESSION CACHE COMPARISON")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("| Metric | V1.4 Run1 | V1.4 Run2 | V1.5 Run1 | V1.5 Run2 |")
	fmt.Println("|--------|-----------|-----------|-----------|-----------|")

	printRow := func(label, v14r1, v14r2, v15r1, v15r2 string) {
		fmt.Printf("| %s | %s | %s | %s | %s |\n", label, v14r1, v14r2, v15r1, v15r2)
	}

	printRow("Steps", itoa(run1V14.Steps), itoa(run2V14.Steps), itoa(run1V15.Steps), itoa(run2V15.Steps))
	printRow("Prompt tokens", itoa(run1V14.PromptTokens), itoa(run2V14.PromptTokens), itoa(run1V15.PromptTokens), itoa(run2V15.PromptTokens))
	printRow("Cache hit", itoa(run1V14.CacheHitTokens), itoa(run2V14.CacheHitTokens), itoa(run1V15.CacheHitTokens), itoa(run2V15.CacheHitTokens))
	printRow("Cache miss", itoa(run1V14.CacheMissTokens), itoa(run2V14.CacheMissTokens), itoa(run1V15.CacheMissTokens), itoa(run2V15.CacheMissTokens))

	hitPct := func(m runMetrics) string {
		d := m.CacheHitTokens + m.CacheMissTokens
		if d == 0 {
			return "n/a"
		}
		return fmt.Sprintf("%.4f%%", 100*float64(m.CacheHitTokens)/float64(d))
	}
	printRow("Hit rate", hitPct(run1V14), hitPct(run2V14), hitPct(run1V15), hitPct(run2V15))
	printRow("Cost", fmt.Sprintf("¥%.4f", run1V14.Cost), fmt.Sprintf("¥%.4f", run2V14.Cost), fmt.Sprintf("¥%.4f", run1V15.Cost), fmt.Sprintf("¥%.4f", run2V15.Cost))

	// Improvement metrics
	fmt.Println()
	v14Gain := 0.0
	v15Gain := 0.0
	if run1V14.CacheMissTokens > 0 {
		v14Gain = 100 * float64(run1V14.CacheMissTokens-run2V14.CacheMissTokens) / float64(run1V14.CacheMissTokens)
	}
	if run1V15.CacheMissTokens > 0 {
		v15Gain = 100 * float64(run1V15.CacheMissTokens-run2V15.CacheMissTokens) / float64(run1V15.CacheMissTokens)
	}
	fmt.Printf("V1.4 cross-session miss reduction: %.4f%%\n", v14Gain)
	fmt.Printf("V1.5 cross-session miss reduction: %.4f%%\n", v15Gain)

	if v15Gain > v14Gain {
		fmt.Println("\n✅ V1.5 shows BETTER cross-session caching than V1.4")
	} else if v14Gain > v15Gain {
		fmt.Println("\n⚠️  V1.4 shows better cross-session caching than V1.5")
	} else {
		fmt.Println("\n➡️  Both versions show similar cross-session caching")
	}
}

func runAgent(bin, model, dir, prompt string) runMetrics {
	metricsPath := filepath.Join(dir, ".run-metrics.json")
	os.Remove(metricsPath)

	args := []string{"run", "--metrics", metricsPath, "--model", model, prompt}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stderr // stream to stderr so it's visible
	cmd.Stderr = os.Stderr
	_ = cmd.Run() // ignore exit code (may fail on "verify" tasks)

	data, err := os.ReadFile(metricsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  WARN: cannot read metrics: %v\n", err)
		return runMetrics{}
	}
	var m runMetrics
	json.Unmarshal(data, &m)
	return m
}

func writeFile(dir, name, content string) {
	os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
}

func itoa(n int) string { return fmt.Sprint(n) }
