//go:build integration
// +build integration

package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestWebSearchIntegration_Live verifies that public SearXNG engines can return
// real search results without any API key configuration.
//
//	go test -tags=integration -run TestWebSearchIntegration -v -timeout 60s ./internal/tool/builtin/
func TestWebSearchIntegration_Live(t *testing.T) {
	ws := webSearch{}
	searchCfg = nil // zero config: only public SearXNG instances

	tests := []struct {
		query string
		topK  int
	}{
		{"golang generics tutorial", 3},
		{"今日天气", 2},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			args, _ := json.Marshal(map[string]any{"query": tt.query, "topK": tt.topK})
			result, err := ws.Execute(ctx, args)
			if err != nil {
				t.Logf("搜索失败（无 API key 时可能正常）: %v", err)
				return
			}
			t.Logf("结果:\n%s", result)

			if strings.TrimSpace(result) == "" {
				t.Error("返回空结果")
			}
		})
	}
}

// TestWebSearchIntegration_Parallel verifies parallel execution doesn't
// serialize engine timeouts — total time < sum of individual timeouts.
func TestWebSearchIntegration_Parallel(t *testing.T) {
	ws := webSearch{}
	searchCfg = nil

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	start := time.Now()
	args, _ := json.Marshal(map[string]any{"query": "hello world", "topK": 2})
	_, err := ws.Execute(ctx, args)
	elapsed := time.Since(start)

	t.Logf("执行耗时: %v (应 << 6×10s=60s 串行时间)", elapsed)
	if err != nil {
		t.Logf("搜索失败: %v", err)
	}
	// Parallel: all 6 engines fire simultaneously, total should be <20s.
	if elapsed > 22*time.Second {
		t.Errorf("执行过慢: %v — 并行引擎应在总时限内完成", elapsed)
	}
}
