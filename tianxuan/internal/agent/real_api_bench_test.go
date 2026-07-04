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
	_ "tianxuan/internal/provider/openai" // register openai provider
	"tianxuan/internal/tool"
)

// TestRealAPICache10Rounds 真实 DeepSeek API 10 轮缓存测试。
// 需要设置环境变量 DEEPSEEK_API_KEY。
// 运行: DEEPSEEK_API_KEY=sk-xxx go test -run TestRealAPICache10Rounds -v -timeout 300s
func TestRealAPICache10Rounds(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY not set — skipping real API test")
	}

	prov, err := provider.New("openai", provider.Config{
		APIKey:  apiKey,
		BaseURL: "https://api.deepseek.com",
		Model:   "deepseek-v4-pro",
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	sink := &benchSink{start: time.Now()}
	reg := tool.NewRegistry()
	sess := NewSession("You are a helpful coding assistant. Reply concisely in Chinese.")
	runner := New(prov, reg, sess, Options{
		MaxSteps:    5,
		Temperature: 0.0,
	}, sink)

	questions := []string{
		"什么是 Go 语言的 goroutine？用一句话解释。",
		"什么是 channel？用一句话解释。",
		"什么是 defer？用一句话解释。",
		"什么是 interface？用一句话解释。",
		"什么是 slice？用一句话解释。",
		"什么是 map？用一句话解释。",
		"什么是 struct？用一句话解释。",
		"什么是指针？用一句话解释。",
		"什么是 panic/recover？用一句话解释。",
		"什么是 context 包？用一句话解释。",
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  tianxuan V5.22  真实 DeepSeek API 10 轮缓存测试")
	fmt.Println("  模型: deepseek-v4-pro")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("%4s │ %6s │ %6s │ %6s │ %7s │ %6s │ %8s\n",
		"轮次", "prompt", "cacheH", "cacheM", "命中率", "output", "延迟")
	fmt.Println(strings.Repeat("─", 65))

	var (
		totalHit   int64
		totalMiss  int64
		totalOut   int64
		totalMS    int64
	)

	for i, q := range questions {
		ctx := context.Background()
		start := time.Now()
		_, err := runner.Run(ctx, q)
		elapsed := time.Since(start).Milliseconds()
		if err != nil {
			t.Errorf("turn %d failed: %v", i+1, err)
			break
		}

		u := sink.lastUsage
		if u == nil {
			t.Errorf("turn %d: no usage", i+1)
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

		fmt.Printf("%4d │ %6d │ %6d │ %6d │ %6.1f%% │ %6d │ %5dms\n",
			i+1, prompt, hit, miss, rate, out, elapsed)

		totalHit += int64(hit)
		totalMiss += int64(miss)
		totalOut += int64(out)
		totalMS += elapsed
	}

	// 汇总
	fmt.Println(strings.Repeat("─", 65))
	totalRate := 0.0
	if totalHit+totalMiss > 0 {
		totalRate = float64(totalHit) / float64(totalHit+totalMiss) * 100
	}
	fmt.Printf("汇总 │ %6s │ %6d │ %6d │ %6.1f%% │ %6d │ %5dms\n",
		"", totalHit, totalMiss, totalRate, totalOut, totalMS)
	fmt.Printf("     │  平均每轮: cache hit=%d miss=%d out=%d 延迟=%dms\n",
		totalHit/10, totalMiss/10, totalOut/10, totalMS/10)
	fmt.Println()

	// 基线对比（基于系统提示词大小动态计算）
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  缓存命中率分析 (10轮)")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	// 理论命中率 = sysPromptTokens / (sysPromptTokens + avgNewTokensPerTurn)
	// 本测试使用极简系统提示(~20词/~76 tok)，每轮新增~60-160 tok
	// → 理论命中率约 40-55%，50%+ 即为正常
	fmt.Printf("  本测试系统提示: ~20 词（极简模式，非完整 tianxuan 配置）\n")
	fmt.Printf("  理论命中率上限: ~55%%（前缀仅系统提示词，不含 L2/L3/工具）\n")
	fmt.Printf("  实际命中率:      %.1f%%  Cache Miss %d tok/轮\n", totalRate, totalMiss/10)
	if totalRate < 40 {
		fmt.Println("  ⚠️  命中率低于 40%——可能存在前缀不稳定问题，需要排查")
	} else {
		fmt.Println("  ✅ 命中率正常（极简系统提示下的预期范围）")
	}
	fmt.Println()
	fmt.Println("  💡 完整 tianxuan 配置的缓存测试请运行:")
	fmt.Println("     go test -run TestRealAPIFullSetup10Rounds -v")
	fmt.Println("     该测试使用 ~800 tok 系统提示 + 17 工具，命中率应 >95%")
	fmt.Println("═══════════════════════════════════════════════════════════════")
}

type benchSink struct {
	start     time.Time
	lastUsage *provider.Usage
}

func (s *benchSink) Emit(e event.Event) {
	if e.Kind == event.Usage && e.Usage != nil {
		s.lastUsage = e.Usage
	}
}
