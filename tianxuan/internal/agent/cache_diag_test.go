package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	_ "tianxuan/internal/provider/openai"
	"tianxuan/internal/tool"
	_ "tianxuan/internal/tool/builtin"
)

// TestCachePrefixStabilityDiagnostic 诊断测试：捕获每轮实际发送给 API 的字节，
// 逐轮对比前缀哈希，精确定位缓存断裂位置。
func TestCachePrefixStabilityDiagnostic(t *testing.T) {
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

	systemPrompt := "You are a helpful coding assistant. Reply concisely in Chinese."
	runtimePrompt := "## Project: tianxuan\nFocus on Go programming."

	reg := tool.NewRegistry()
	sink := &diagSink{}
	sess := NewSession(systemPrompt)
	runner := New(prov, reg, sess, Options{
		MaxSteps:    5,
		Temperature: 0.0,
	}, sink)
	runner.MergeRuntimePrompt(runtimePrompt)

	// 捕获 5 轮的消息内容
	type roundData struct {
		turn     int
		sysHash  string // L1 hash
		l2Hash   string // L2 hash
		msgCount int
		msgHash  string // 所有消息拼接的 hash
		hit      int
		miss     int
	}
	var rounds []roundData

	questions := []string{
		"用一句话解释 Go 的 goroutine",
		"用一句话解释 Go 的 channel",
		"用一句话解释 Go 的 defer",
		"用一句话解释 Go 的 interface",
		"用一句话解释 Go 的 slice",
	}

	for i, q := range questions {
		sink.lastUsage = nil
		err := runner.Run(context.Background(), q)
		if err != nil {
			t.Fatalf("turn %d: %v", i+1, err)
		}

		msgs := runner.Session().Messages
		var l1, l2 string
		if len(msgs) > 0 && msgs[0].Role == provider.RoleSystem {
			l1 = msgs[0].Content
		}
		if len(msgs) > 1 && msgs[1].Role == provider.RoleSystem {
			l2 = msgs[1].Content
		}

		// 计算所有消息（包括 L2 注入后）的哈希
		var allParts []string
		for _, m := range msgs {
			allParts = append(allParts, string(m.Role)+":"+m.Content)
		}

		rd := roundData{
			turn:     i + 1,
			sysHash:  hashStr(l1),
			l2Hash:   hashStr(l2),
			msgCount: len(msgs),
			msgHash:  hashStr(strings.Join(allParts, "|")),
		}
		if u := sink.lastUsage; u != nil {
			rd.hit = u.CacheHitTokens
			rd.miss = u.CacheMissTokens
		}
		rounds = append(rounds, rd)
	}

	// 输出诊断表
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  缓存前缀稳定性诊断")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("%4s │ %12s │ %12s │ %5s │ %12s │ %5s │ %5s\n",
		"轮次", "L1 hash", "L2 hash", "消息数", "消息 hash", "hit", "miss")
	fmt.Println(strings.Repeat("─", 80))

	for _, rd := range rounds {
		fmt.Printf("%4d │ %12s │ %12s │ %5d │ %12s │ %5d │ %5d\n",
			rd.turn, rd.sysHash, rd.l2Hash, rd.msgCount, rd.msgHash, rd.hit, rd.miss)
	}

	// 检查前缀稳定性
	fmt.Println()
	l1First := rounds[0].sysHash
	l2First := rounds[0].l2Hash
	l1Stable := true
	l2Stable := true
	for _, rd := range rounds[1:] {
		if rd.sysHash != l1First {
			l1Stable = false
		}
		if rd.l2Hash != l2First {
			l2Stable = false
		}
	}

	fmt.Println("═══════════════════════════════════════════════════════════════")
	if l1Stable {
		fmt.Println("  ✅ L1 (系统提示词) 稳定——所有轮次 hash 一致")
	} else {
		fmt.Println("  ❌ L1 (系统提示词) 不稳定——存在字节变化!")
	}
	if l2Stable {
		fmt.Println("  ✅ L2 (运行时上下文) 稳定——所有轮次 hash 一致")
	} else {
		fmt.Println("  ❌ L2 (运行时上下文) 不稳定——存在字节变化!")
	}
	fmt.Println("═══════════════════════════════════════════════════════════════")
}

func hashStr(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:12]
}

type diagSink struct {
	lastUsage *provider.Usage
}

func (s *diagSink) Emit(e event.Event) {
	if e.Kind == event.Usage && e.Usage != nil {
		s.lastUsage = e.Usage
	}
}
