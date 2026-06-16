package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
	_ "tianxuan/internal/tool/builtin"
)

// TestTruncateToolOutputUnderCap leaves small payloads alone — the cap should
// never rewrite content that already fits.
func TestTruncateToolOutputUnderCap(t *testing.T) {
	in := strings.Repeat("a", maxToolOutputBytes)
	got, notice := truncateToolOutput(in)
	if got != in {
		t.Errorf("payload at exactly the cap was rewritten")
	}
	if notice != "" {
		t.Errorf("at-cap payload should not emit a notice, got %q", notice)
	}
}

// TestTruncateToolOutputHeadTail V5.10: 新 hygiene 压缩保留信号行 + 首行。
// 无信号关键词时只保留首部，因为尾部无特殊价值。
func TestTruncateToolOutputHeadTail(t *testing.T) {
	head := strings.Repeat("H", maxToolOutputBytes)
	tail := strings.Repeat("T", maxToolOutputBytes)
	in := head + tail
	out, notice := truncateToolOutput(in)
	if !strings.HasPrefix(out, "H") {
		t.Errorf("head not preserved: %q", out[:20])
	}
	if !strings.Contains(out, "cache hygiene") {
		t.Errorf("hygiene marker missing: %q", out[:80])
	}
	if len(out) >= len(in) {
		t.Errorf("output not shorter than input: in=%d out=%d", len(in), len(out))
	}
	if !strings.Contains(notice, "truncated") {
		t.Errorf("notice missing: %q", notice)
	}
}

// TestTruncateToolOutputKeepsSignalLines V5.10: 信号行（error/fatal）在多行输出中应被保留。
func TestTruncateToolOutputKeepsSignalLines(t *testing.T) {
	// 构造超限输出，中间夹一条 error 行
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("line " + itoa(i) + ": normal output text\n")
	}
	sb.WriteString("ERROR: something went wrong at line 501\n")
	for i := 502; i < 1000; i++ {
		sb.WriteString("line " + itoa(i) + ": normal output text\n")
	}
	out, _ := truncateToolOutput(sb.String())
	if !strings.Contains(out, "ERROR") {
		t.Errorf("signal line 'ERROR' should be preserved, got: %s", out[len(out)-200:])
	}
}

// TestTruncateToolOutputRuneBoundaries puts multibyte runes exactly across the
// head and tail cut points; the result must still be valid UTF-8.
func TestTruncateToolOutputRuneBoundaries(t *testing.T) {
	in := strings.Repeat("中", maxToolOutputBytes) // 3 bytes each — guarantees a cut inside a rune
	out, _ := truncateToolOutput(in)
	if !utf8.ValidString(out) {
		t.Errorf("truncated output is not valid UTF-8")
	}
}

// TestFinishReasonMessage only yields a warning for abnormal terminations.
// Normal stops are silent (ok=false) so the per-turn line stays clean.
func TestFinishReasonMessage(t *testing.T) {
	silent := []string{"", "stop", "tool_calls"}
	for _, r := range silent {
		if msg, ok := finishReasonMessage(&provider.Usage{FinishReason: r}); ok {
			t.Errorf("finish_reason=%q should be silent, got %q", r, msg)
		}
	}
	loud := map[string]string{
		"length":                "max output",
		"content_filter":        "content filter",
		"repetition_truncation": "repetition",
	}
	for reason, fragment := range loud {
		msg, ok := finishReasonMessage(&provider.Usage{FinishReason: reason})
		if !ok || !strings.Contains(msg, fragment) {
			t.Errorf("finish_reason=%q: got (%q, %v), want fragment %q", reason, msg, ok, fragment)
		}
	}
}

// --- parallel-dispatch tests ---

// fakeTool is a minimal Tool stand-in for dispatch tests; ReadOnly is
// configurable and Execute sleeps a fixed duration so we can measure
// serial vs parallel behaviour by wall-clock.
type fakeTool struct {
	name     string
	readOnly bool
	delay    time.Duration
	err      error
	calls    *int32 // shared counter to assert all dispatched
}

func (f fakeTool) Name() string            { return f.name }
func (f fakeTool) Description() string     { return "" }
func (f fakeTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f fakeTool) ReadOnly() bool          { return f.readOnly }
func (f fakeTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if f.calls != nil {
		atomic.AddInt32(f.calls, 1)
	}
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if f.err != nil {
		return "", f.err
	}
	return f.name + " done", nil
}

func TestPartitionToolCallsAllReadOnly(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "ro1", readOnly: true})
	reg.Add(fakeTool{name: "ro2", readOnly: true})
	calls := []provider.ToolCall{{Name: "ro1"}, {Name: "ro2"}}
	got := partitionToolCalls(reg, calls)
	want := []toolCallBatch{{start: 0, end: 2, parallel: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("partitionToolCalls = %+v, want %+v", got, want)
	}
}

// TestPartitionToolCallsSegmentsAroundWriters verifies a writer only serializes
// its own provider-order position; read-only runs on either side stay batchable.
func TestPartitionToolCallsSegmentsAroundWriters(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "ro", readOnly: true})
	reg.Add(fakeTool{name: "rw", readOnly: false})
	calls := []provider.ToolCall{{Name: "ro"}, {Name: "rw"}, {Name: "ro"}}
	got := partitionToolCalls(reg, calls)
	want := []toolCallBatch{
		{start: 0, end: 1, parallel: true},
		{start: 1, end: 2},
		{start: 2, end: 3, parallel: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("partitionToolCalls = %+v, want %+v", got, want)
	}
}

// TestPartitionToolCallsUnknownToolSerial keeps unknown-tool errors
// deterministic by forcing unknown calls into single-call serial batches.
func TestPartitionToolCallsUnknownToolSerial(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "ro", readOnly: true})
	calls := []provider.ToolCall{{Name: "ro"}, {Name: "vanished"}, {Name: "ro"}}
	got := partitionToolCalls(reg, calls)
	want := []toolCallBatch{
		{start: 0, end: 1, parallel: true},
		{start: 1, end: 2},
		{start: 2, end: 3, parallel: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("partitionToolCalls = %+v, want %+v", got, want)
	}
}

// TestPartitionToolCallsCompleteStepSerial verifies complete_step never joins a
// parallel read-only run: it reads the turn's receipts, so the prior reads must
// finish (and record) in an earlier batch before it runs in its own serial one.
func TestPartitionToolCallsCompleteStepSerial(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_file", readOnly: true})
	reg.Add(fakeTool{name: "complete_step", readOnly: true})

	calls := []provider.ToolCall{{Name: "read_file"}, {Name: "complete_step"}}
	got := partitionToolCalls(reg, calls)
	want := []toolCallBatch{
		{start: 0, end: 1, parallel: true},
		{start: 1, end: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("partitionToolCalls = %+v, want %+v", got, want)
	}
}

func TestPartitionToolCallsTodoWriteSerial(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_file", readOnly: true})
	reg.Add(fakeTool{name: "todo_write", readOnly: true})

	calls := []provider.ToolCall{{Name: "read_file"}, {Name: "todo_write"}, {Name: "read_file"}}
	got := partitionToolCalls(reg, calls)
	want := []toolCallBatch{
		{start: 0, end: 1, parallel: true},
		{start: 1, end: 2},
		{start: 2, end: 3, parallel: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("partitionToolCalls = %+v, want %+v", got, want)
	}
}

// TestExecuteBatchParallelReadOnly checks that three 80ms read-only calls
// complete in well under 3×80ms — the wall-clock proof of true parallelism.
func TestExecuteBatchParallelReadOnly(t *testing.T) {
	const delay = 80 * time.Millisecond
	calls := int32(0)
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "a", readOnly: true, delay: delay, calls: &calls})
	reg.Add(fakeTool{name: "b", readOnly: true, delay: delay, calls: &calls})
	reg.Add(fakeTool{name: "c", readOnly: true, delay: delay, calls: &calls})

	a := New(nil, reg, NewSession(""), Options{}, event.Discard)

	start := time.Now()
	results := a.executeBatch(context.Background(), []provider.ToolCall{{Name: "a"}, {Name: "b"}, {Name: "c"}})
	elapsed := time.Since(start)

	if calls != 3 {
		t.Errorf("dispatched %d calls, want 3", calls)
	}
	if len(results) != 3 || results[0] != "a done" || results[1] != "b done" || results[2] != "c done" {
		t.Errorf("results out of order or wrong: %v", results)
	}
	// Allow generous slack for CI; even 2x serial would prove we got parallelism.
	if elapsed >= 2*delay {
		t.Errorf("read-only batch took %v (>= %v) — not parallel", elapsed, 2*delay)
	}
}

// TestExecuteBatchSegmentsAroundWrites ensures a write call only serializes its
// own position in the provider-ordered batch: read-only runs before and after it
// may still parallelise within their contiguous segments.
func TestExecuteBatchSegmentsAroundWrites(t *testing.T) {
	const delay = 40 * time.Millisecond
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "ro1", readOnly: true, delay: delay})
	reg.Add(fakeTool{name: "ro2", readOnly: true, delay: delay})
	reg.Add(fakeTool{name: "ro3", readOnly: true, delay: delay})
	reg.Add(fakeTool{name: "ro4", readOnly: true, delay: delay})
	reg.Add(fakeTool{name: "rw", readOnly: false, delay: delay})

	a := New(nil, reg, NewSession(""), Options{}, event.Discard)

	start := time.Now()
	results := a.executeBatch(context.Background(), []provider.ToolCall{
		{Name: "ro1"},
		{Name: "ro2"},
		{Name: "rw"},
		{Name: "ro3"},
		{Name: "ro4"},
	})
	elapsed := time.Since(start)

	want := []string{"ro1 done", "ro2 done", "rw done", "ro3 done", "ro4 done"}
	if len(results) != len(want) {
		t.Fatalf("got %d results, want %d: %v", len(results), len(want), results)
	}
	for i := range want {
		if results[i] != want[i] {
			t.Fatalf("results out of order or wrong: got %v want %v", results, want)
		}
	}
	// Desired shape is roughly 3*delay: (ro1|ro2), then rw, then (ro3|ro4).
	// Old all-serial behaviour is roughly 5*delay and should fail this bound.
	if elapsed >= 4*delay {
		t.Errorf("mixed batch took %v (>= %v) — read-only segments did not parallelise", elapsed, 4*delay)
	}
	if elapsed < 2*delay {
		t.Errorf("mixed batch took only %v — write call appears to have overlapped a read-only segment", elapsed)
	}
}

func TestExecuteBatchFeedsReceiptsToCompleteStep(t *testing.T) {
	completeStep, ok := tool.LookupBuiltin("complete_step")
	if !ok {
		t.Fatal("complete_step builtin not registered")
	}
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "bash", readOnly: false})
	reg.Add(completeStep)
	a := New(nil, reg, NewSession(""), Options{}, event.Discard)

	results := a.executeBatch(context.Background(), []provider.ToolCall{
		{Name: "bash", Arguments: `{"command":"go test ./internal/..."}`},
		{Name: "complete_step", Arguments: `{
			"step":"Run checks",
			"result":"checks passed",
			"evidence":[{"kind":"verification","summary":"tests passed","command":"go test ./internal/..."}]
		}`},
	})

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if !strings.Contains(results[1], "host-verified 1") {
		t.Fatalf("complete_step did not see bash receipt: %q", results[1])
	}
}

func TestExecuteOneFailedReceiptDoesNotVerify(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "bash", readOnly: false, err: errors.New("boom")})
	a := New(nil, reg, NewSession(""), Options{}, event.Discard)

	out := a.executeOne(context.Background(), provider.ToolCall{Name: "bash", Arguments: `{"command":"go test ./..."}`})
	if out.errMsg == "" {
		t.Fatal("failing fake tool should return an error outcome")
	}
	if a.evidence.HasSuccessfulCommand("go test ./...") {
		t.Fatal("failed bash receipt must not verify")
	}
}

func TestRunResetsEvidenceLedger(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "bash", readOnly: false})
	prov := &mockProvider{name: "p", chunks: []provider.Chunk{{Type: provider.ChunkText, Text: "done"}}}
	a := New(prov, reg, NewSession(""), Options{}, event.Discard)

	a.executeOne(context.Background(), provider.ToolCall{Name: "bash", Arguments: `{"command":"go test ./..."}`})
	if !a.evidence.HasSuccessfulCommand("go test ./...") {
		t.Fatal("setup failed to record evidence")
	}

	if err := a.Run(context.Background(), "next turn"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if a.evidence.HasSuccessfulCommand("go test ./...") {
		t.Fatal("new user turn should not inherit previous receipts")
	}
}

// --- cacheBreakDetector tests (V5.30: 诊断修复) ---

func msgSys(content string) provider.Message {
	return provider.Message{Role: provider.RoleSystem, Content: content}
}

func msgUser(content string) provider.Message {
	return provider.Message{Role: provider.RoleUser, Content: content}
}

func toolsForTest(names ...string) []provider.ToolSchema {
	var out []provider.ToolSchema
	for _, n := range names {
		out = append(out, provider.ToolSchema{Name: n, Description: "desc for " + n})
	}
	return out
}

// TestCacheBreakDetectorFirstCall 首次调用应返回 "first call (no baseline)"。
func TestCacheBreakDetectorFirstCall(t *testing.T) {
	var d cacheBreakDetector

	msgs := []provider.Message{msgSys("L1"), msgSys("L2"), msgUser("hello")}
	tools := toolsForTest("read_file", "bash")

	d.record(msgs, tools)
	// 首次 check 无基线 → 不检测断裂
	reason := d.check(&provider.Usage{CacheHitTokens: 1000})
	if reason != "" {
		t.Errorf("first check should return empty, got %q", reason)
	}

	// 第二次 record（同前缀）→ check 无断裂
	d.record(msgs, tools)
	reason = d.check(&provider.Usage{CacheHitTokens: 1000})
	if reason != "" {
		t.Errorf("same prefix should not break, got %q", reason)
	}
}

// TestCacheBreakDetectorSamePrefix 同前缀不应断裂。
func TestCacheBreakDetectorSamePrefix(t *testing.T) {
	var d cacheBreakDetector

	msgs := []provider.Message{msgSys("L1-v1"), msgSys("L2-v1"), msgUser("q1")}
	tools := toolsForTest("a", "b")

	d.record(msgs, tools)
	d.check(&provider.Usage{CacheHitTokens: 5000}) // 建立基线

	d.record(msgs, tools) // 完全相同的消息和工具
	reason := d.check(&provider.Usage{CacheHitTokens: 4900}) // 波动 < 5%
	if reason != "" {
		t.Errorf("same prefix should not detect break: %q", reason)
	}
}

// TestCacheBreakDetectorL1Change 检测 L1 变化 → "client-side prefix drift: L1 changed"。
func TestCacheBreakDetectorL1Change(t *testing.T) {
	var d cacheBreakDetector

	msgs1 := []provider.Message{msgSys("L1-v1"), msgSys("L2"), msgUser("q1")}
	msgs2 := []provider.Message{msgSys("L1-v2"), msgSys("L2"), msgUser("q2")}
	tools := toolsForTest("a")

	d.record(msgs1, tools)
	d.check(&provider.Usage{CacheHitTokens: 5000})

	d.record(msgs2, tools)
	reason := d.check(&provider.Usage{CacheHitTokens: 1000}) // -80% → 断裂
	if !strings.Contains(reason, "client-side") {
		t.Errorf("expected client-side drift, got %q", reason)
	}
	if !strings.Contains(reason, "L1 changed") {
		t.Errorf("expected L1 changed in reason, got %q", reason)
	}
}

// TestCacheBreakDetectorL2Change 检测 L2 变化。
func TestCacheBreakDetectorL2Change(t *testing.T) {
	var d cacheBreakDetector

	msgs1 := []provider.Message{msgSys("L1"), msgSys("L2-v1"), msgUser("q1")}
	msgs2 := []provider.Message{msgSys("L1"), msgSys("L2-v2"), msgUser("q2")}
	tools := toolsForTest("a")

	d.record(msgs1, tools)
	d.check(&provider.Usage{CacheHitTokens: 5000})

	d.record(msgs2, tools)
	reason := d.check(&provider.Usage{CacheHitTokens: 1000})
	if !strings.Contains(reason, "client-side") {
		t.Errorf("expected client-side drift, got %q", reason)
	}
	if !strings.Contains(reason, "L2 changed") {
		t.Errorf("expected L2 changed in reason, got %q", reason)
	}
}

// TestCacheBreakDetectorToolsChange 检测工具集变化。
func TestCacheBreakDetectorToolsChange(t *testing.T) {
	var d cacheBreakDetector

	msgs := []provider.Message{msgSys("L1"), msgSys("L2"), msgUser("q1")}
	tools1 := toolsForTest("a", "b")
	tools2 := toolsForTest("a", "b", "c") // 新增工具

	d.record(msgs, tools1)
	d.check(&provider.Usage{CacheHitTokens: 5000})

	d.record(msgs, tools2)
	reason := d.check(&provider.Usage{CacheHitTokens: 1000})
	if !strings.Contains(reason, "client-side") {
		t.Errorf("expected client-side drift, got %q", reason)
	}
	if !strings.Contains(reason, "tools changed") {
		t.Errorf("expected tools changed in reason, got %q", reason)
	}
}

// TestCacheBreakDetectorMultiChange 同时检测多个变化。
func TestCacheBreakDetectorMultiChange(t *testing.T) {
	var d cacheBreakDetector

	msgs1 := []provider.Message{msgSys("L1-v1"), msgSys("L2-v1"), msgUser("q1")}
	msgs2 := []provider.Message{msgSys("L1-v2"), msgSys("L2-v2"), msgUser("q2")}
	tools1 := toolsForTest("a")
	tools2 := toolsForTest("x")

	d.record(msgs1, tools1)
	d.check(&provider.Usage{CacheHitTokens: 5000})

	d.record(msgs2, tools2)
	reason := d.check(&provider.Usage{CacheHitTokens: 500}) // -90%
	if !strings.Contains(reason, "L1") || !strings.Contains(reason, "L2") || !strings.Contains(reason, "tools") {
		t.Errorf("should detect all drifts, got %q", reason)
	}
}

// TestCacheBreakDetectorServerSide 前缀不变 → "server-side (prefix unchanged)"。
func TestCacheBreakDetectorServerSide(t *testing.T) {
	var d cacheBreakDetector

	msgs := []provider.Message{msgSys("L1"), msgSys("L2"), msgUser("q1")}
	tools := toolsForTest("a", "b")

	d.record(msgs, tools)
	d.check(&provider.Usage{CacheHitTokens: 5000})

	d.record(msgs, tools) // 完全相同的消息和工具 → 前缀不变
	// 模拟服务端缓存 TTL 过期：cache_hit 大幅下降但前缀没变
	reason := d.check(&provider.Usage{CacheHitTokens: 500}) // -90%
	if !strings.Contains(reason, "server-side") {
		t.Errorf("should be server-side break, got %q", reason)
	}
	if strings.Contains(reason, "client-side") {
		t.Errorf("should NOT be client-side, got %q", reason)
	}
}
