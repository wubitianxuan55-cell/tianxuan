package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// ─── V5.13a: ParamStormBreaker 测试 (Kun tool-storm-breaker.ts 移植) ─────

// fakeEchoTool 是一个只读工具，返回 args 的 JSON。
type fakeEchoTool struct{ name string }

func (f fakeEchoTool) Name() string                 { return f.name }
func (f fakeEchoTool) Description() string           { return "echo tool" }
func (f fakeEchoTool) Schema() json.RawMessage       { return json.RawMessage(`{"type":"object"}`) }
func (f fakeEchoTool) ReadOnly() bool                { return true }
func (f fakeEchoTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return string(args), nil
}

// fakeWriteTool 是一个写入工具。
type fakeWriteTool struct{ name string }

func (f fakeWriteTool) Name() string       { return f.name }
func (f fakeWriteTool) Description() string { return "write tool" }
func (f fakeWriteTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f fakeWriteTool) ReadOnly() bool      { return false }
func (f fakeWriteTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return "ok", nil
}

// TestParamStormBreakerSuppressIdentical 验证第3次完全相同的调用被抑制。
// Kun semantics: threshold=3 → 前2次通过，第3次抑制。
func TestParamStormBreakerSuppressIdentical(t *testing.T) {
	psb := NewParamStormBreaker(ParamStormOptions{WindowSize: 8, Threshold: 3})

	call := provider.ToolCall{Name: "read_file", Arguments: `{"path":"/tmp/x"}`}

	// 前2次不抑制
	for i := 0; i < 2; i++ {
		res := psb.Inspect(call.Name, call.Arguments, true)
		if res.Suppress {
			t.Fatalf("call %d should NOT be suppressed", i+1)
		}
	}
	// 第3次抑制
	res := psb.Inspect(call.Name, call.Arguments, true)
	if !res.Suppress {
		t.Fatal("3rd identical call should be suppressed")
	}
	if !strings.Contains(res.Reason, "identical arguments") {
		t.Errorf("reason should mention identical arguments, got: %q", res.Reason)
	}
}

// TestParamStormBreakerDifferentArgs 验证不同参数各自独立计数。
// /a 和 /b 是不同的调用，互不影响。窗口内 /a 出现2次后第3次被抑制。
func TestParamStormBreakerDifferentArgs(t *testing.T) {
	psb := NewParamStormBreaker(ParamStormOptions{WindowSize: 8, Threshold: 3})

	psb.Inspect("read_file", `{"path":"/a"}`, true) // /a #1
	psb.Inspect("read_file", `{"path":"/a"}`, true) // /a #2 — 不抑制
	psb.Inspect("read_file", `{"path":"/b"}`, true) // /b #1 — 不同参数，不影响/a计数

	// /a #3 —— 抑制（window内仍有2个/a）
	res := psb.Inspect("read_file", `{"path":"/a"}`, true)
	if !res.Suppress {
		t.Fatal("/a should be suppressed — 2 previous /a calls still in window")
	}
	// /b #2 —— 不抑制
	res2 := psb.Inspect("read_file", `{"path":"/b"}`, true)
	if res2.Suppress {
		t.Fatal("/b should NOT be suppressed — only 1 previous /b call")
	}
}

// TestParamStormBreakerWriteClearsReadHistory 验证写入操作清零只读历史。
func TestParamStormBreakerWriteClearsReadHistory(t *testing.T) {
	psb := NewParamStormBreaker(ParamStormOptions{WindowSize: 8, Threshold: 3})

	// 只读调用（不触发抑制只有2次）
	psb.Inspect("read_file", `{"path":"/x"}`, true)
	psb.Inspect("read_file", `{"path":"/x"}`, true)

	// 写入操作 —— 清零只读历史
	psb.Inspect("write_file", `{"content":"hello"}`, false)

	// 写入后重新开始，2次不会触发
	psb.Inspect("read_file", `{"path":"/x"}`, true)
	res := psb.Inspect("read_file", `{"path":"/x"}`, true)
	if res.Suppress {
		t.Fatal("write should have cleared read history — 2nd read after write should NOT be suppressed")
	}
}

// TestParamStormBreakerReset 验证 Reset 清零所有历史。
func TestParamStormBreakerReset(t *testing.T) {
	psb := NewParamStormBreaker(ParamStormOptions{WindowSize: 8, Threshold: 3})

	psb.Inspect("read_file", `{"path":"/x"}`, true)
	psb.Inspect("read_file", `{"path":"/x"}`, true)
	psb.Reset()

	// 重置后：2次不会触发（threshold=3要求2次历史+当前=3次）
	psb.Inspect("read_file", `{"path":"/x"}`, true)
	res := psb.Inspect("read_file", `{"path":"/x"}`, true)
	if res.Suppress {
		t.Fatal("after Reset, 2nd call should NOT be suppressed (need 3 total)")
	}
}

// TestParamStormBreakerCanonicalArgs 验证参数规范化——key顺序不同但语义相同视为相同。
func TestParamStormBreakerCanonicalArgs(t *testing.T) {
	psb := NewParamStormBreaker(ParamStormOptions{WindowSize: 8, Threshold: 3})

	// 相同语义但 key 顺序不同
	psb.Inspect("read_file", `{"path":"/x","offset":10}`, true)
	psb.Inspect("read_file", `{"offset":10,"path":"/x"}`, true)

	res := psb.Inspect("read_file", `{"path":"/x","offset":10}`, true)
	if !res.Suppress {
		t.Fatal("canonicalized args should match regardless of key order")
	}
}

// TestParamStormBreakerExemptTools 验证豁免工具不触发抑制。
func TestParamStormBreakerExemptTools(t *testing.T) {
	psb := NewParamStormBreaker(ParamStormOptions{
		WindowSize:   8,
		Threshold:    3,
		ExemptTools:  []string{"ask", "todo_write"},
	})

	for i := 0; i < 5; i++ {
		res := psb.Inspect("ask", `{"question":"test"}`, true)
		if res.Suppress {
			t.Fatalf("ask should be exempt from storm breaker at call %d", i+1)
		}
	}
}

// TestParamStormBreakerWindowEvict 验证窗口满时最旧的调用被驱逐。
func TestParamStormBreakerWindowEvict(t *testing.T) {
	psb := NewParamStormBreaker(ParamStormOptions{WindowSize: 3, Threshold: 3})

	psb.Inspect("read_file", `{"path":"/a"}`, true) // slot 0
	psb.Inspect("read_file", `{"path":"/b"}`, true) // slot 1
	psb.Inspect("read_file", `{"path":"/c"}`, true) // slot 2 (满)
	// /a 被驱逐
	psb.Inspect("read_file", `{"path":"/a"}`, true) // 重新加入

	// /a 只出现过2次（第1次被驱逐了），不应抑制
	res := psb.Inspect("read_file", `{"path":"/a"}`, true)
	if res.Suppress {
		t.Fatal("evicted call should not accumulate count")
	}
}

// TestIntegrationParamStormBreaker 集成测试：在 executeBatch 中验证抑制。
func TestIntegrationParamStormBreaker(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeEchoTool{name: "read_file"})
	reg.Add(fakeWriteTool{name: "write_file"})

	a := New(nil, reg, NewSession(""), Options{
		ParamStorm: &ParamStormOptions{WindowSize: 8, Threshold: 3},
	}, nil) // nil sink — suppress notices

	call := provider.ToolCall{Name: "read_file", Arguments: `{"path":"/x"}`}

	// 前2次正常执行（threshold=3 → 第3次抑制）
	for i := 0; i < 2; i++ {
		result := a.executeBatch(context.Background(), []provider.ToolCall{call})
		if strings.Contains(result[0], "suppressed") {
			t.Fatalf("call %d should execute normally, got: %s", i+1, result[0])
		}
	}

	// 第3次被抑制
	result := a.executeBatch(context.Background(), []provider.ToolCall{call})
	if !strings.Contains(result[0], "suppressed") {
		t.Fatalf("3rd call should be suppressed, got: %s", result[0])
	}
}
