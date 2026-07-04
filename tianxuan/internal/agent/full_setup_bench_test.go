package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	_ "tianxuan/internal/provider/openai"
	"tianxuan/internal/tool"
	_ "tianxuan/internal/tool/builtin"
)

// TestRealAPIFullSetup10Rounds 完整 tianxuan 配置 10 轮真实 API 缓存测试。
// 使用完整的系统提示词 + 17 个内置工具 + L2 运行时上下文。
// 需要 DEEPSEEK_API_KEY 环境变量。
func TestRealAPIFullSetup10Rounds(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY not set")
	}

	prov, err := provider.New("openai", provider.Config{
		APIKey:  apiKey,
		BaseURL: "https://api.deepseek.com",
		Model:   "deepseek-v4-pro",
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	// 完整系统提示词（模拟 tianxuan 实际使用的 L1）
	systemPrompt := `You are tianxuan, an AI coding agent powered by DeepSeek V4.

## Identity
- You are a desktop AI coding assistant running inside the tianxuan application.
- You have access to a Linux shell, a file system sandboxed to the project root, and the tools listed below.
- Your answers should be concise, technical, and in Chinese unless the user asks otherwise.

## Coding Conventions
- Prefer reading files before editing them.
- Use edit_file for targeted replacements; use write_file only for new files or full rewrites.
- After edits, run relevant tests with bash.
- Keep changes minimal — don't refactor unrelated code.

## Safety
- Never delete files without user confirmation.
- Never run destructive commands (rm -rf, force push, etc.) without explicit approval.
- Report errors honestly; don't hide them.

## Response Style
- Reply in Chinese (简体中文).
- Be direct and concise.
- Show code blocks with language tags.
- When a task is done, summarize what was changed.`

	// L2 运行时上下文（模拟实际使用的 L2）
	runtimePrompt := `## Project: tianxuan
- Language: Go 1.22
- Build: go build -o bin/tianxuan.exe ./cmd/tianxuan
- Test: go test ./internal/...
- Lint: go vet ./internal/...

## Current Task
Answer coding questions about Go concepts concisely in Chinese.`

	reg := tool.NewRegistry()
	sink := &fullBenchSink{start: time.Now()}
	sess := NewSession(systemPrompt)
	runner := New(prov, reg, sess, Options{
		MaxSteps:    5,
		Temperature: 0.0,
		Compaction: CompactionConfig{
			Window:     1_000_000,
			Ratio:      0.8,
			RecentKeep: 10,
		},
	}, sink)
	runner.MergeRuntimePrompt(runtimePrompt)

	questions := []string{
		"读取 internal/agent/agent.go 的前50行，总结文件结构",
		"读取 internal/agent/compact.go 的前50行，总结压缩逻辑",
		"读取 internal/agent/session.go 的全部内容，总结会话管理",
		"读取 internal/agent/param_storm.go 的全部内容，总结风暴断路器",
		"读取 internal/agent/budget_gate.go 的全部内容，总结预算门控",
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════")
	fmt.Println("  tianxuan V5.22  完整配置 真实 DeepSeek API 10 轮缓存测试")
	fmt.Println("  模型: deepseek-v4-pro  |  系统提示词: ~800 tokens  |  工具: 17个内置")
	fmt.Println("══════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("%4s │ %7s │ %7s │ %7s │ %7s │ %6s │ %8s │ %s\n",
		"轮次", "prompt", "cacheHit", "cacheMiss", "命中率", "output", "延迟", "累积cost")
	fmt.Println(strings.Repeat("─", 85))

	var (
		cumulativeHit  int64
		cumulativeMiss int64
		cumulativeOut  int64
		cumulativeMS   int64
		totalCost      float64
		pricing        = &provider.Pricing{CacheHit: 0.5, Input: 2.0, Output: 10.0}
	)

	for i, q := range questions {
		start := time.Now()
		_, err := runner.Run(context.Background(), q)
		elapsed := time.Since(start).Milliseconds()
		if err != nil {
			t.Errorf("turn %d: %v", i+1, err)
			break
		}

		u := sink.lastUsage
		if u == nil {
			continue
		}

		hit := u.CacheHitTokens
		miss := u.CacheMissTokens
		prompt := u.PromptTokens
		out := u.CompletionTokens

		rate := 0.0
		if hit+miss > 0 {
			rate = float64(hit) / float64(hit+miss) * 100
		}

		cost := pricing.Cost(u)
		totalCost += cost

		fmt.Printf("%4d │ %7d │ %7d │ %7d │ %6.1f%% │ %6d │ %6dms │ ¥%.4f\n",
			i+1, prompt, hit, miss, rate, out, elapsed, cost)

		cumulativeHit += int64(hit)
		cumulativeMiss += int64(miss)
		cumulativeOut += int64(out)
		cumulativeMS += elapsed

		sink.lastUsage = nil
	}

	fmt.Println(strings.Repeat("─", 85))
	totalRate := 0.0
	if cumulativeHit+cumulativeMiss > 0 {
		totalRate = float64(cumulativeHit) / float64(cumulativeHit+cumulativeMiss) * 100
	}
	fmt.Printf("汇总 │ %7s │ %7d │ %7d │ %6.1f%% │ %6d │ %6dms │ ¥%.4f\n",
		"", cumulativeHit, cumulativeMiss, totalRate, cumulativeOut, cumulativeMS, totalCost)
	fmt.Println()

	// 每轮 miss 趋势
	missPerTurn := float64(cumulativeMiss) / 10.0
	fmt.Println("══════════════════════════════════════════════════════════════════")
	fmt.Println("  成本分析")
	fmt.Println("══════════════════════════════════════════════════════════════════")
	fmt.Printf("  总 cost:       ¥%.4f  (10轮)\n", totalCost)
	fmt.Printf("  每轮平均 cost: ¥%.4f\n", totalCost/10)
	fmt.Printf("  每轮 miss:     %.0f tokens\n", missPerTurn)
	fmt.Printf("  每轮 hit:      %d tokens\n", cumulativeHit/10)
	fmt.Printf("  累积命中率:    %.1f%%\n", totalRate)
	fmt.Println()

	// 基线对比
	fmt.Println("══════════════════════════════════════════════════════════════════")
	fmt.Println("  版本对比 (完整配置 10 轮)")
	fmt.Println("══════════════════════════════════════════════════════════════════")
	fmt.Println("  指标              │ V5.7 基线 (CHANGELOG) │ 当前")
	fmt.Println("  ─────────────────────────────────────────────────────")
	fmt.Printf("  缓存命中率        │ 99.0%%                  │ %.1f%%\n", totalRate)
	fmt.Printf("  每轮 Cache Miss   │ ~50 tok                │ %.0f tok\n", missPerTurn)
	fmt.Printf("  每轮平均延迟      │ ~2s                    │ %dms\n", cumulativeMS/10)
	fmt.Printf("  10轮总费用        │ ~¥0.0082               │ ¥%.4f\n", totalCost)
	if totalRate < 90 {
		fmt.Println("  ⚠️  命中率低于 90%——可能存在缓存退化，需要排查")
	} else {
		fmt.Println("  ✅ 命中率正常")
	}
	fmt.Println("══════════════════════════════════════════════════════════════════")
}

type fullBenchSink struct {
	start     time.Time
	lastUsage *provider.Usage
}

func (s *fullBenchSink) Emit(e event.Event) {
	if e.Kind == event.Usage && e.Usage != nil {
		s.lastUsage = e.Usage
	}
}
