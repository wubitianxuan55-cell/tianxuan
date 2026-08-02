package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"tianxuan/internal/agent/testutil"
	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// ─── V10.148: Auto Failure Guard（蒸馏自 Reasonix internal/recovery）───
//
// 宿主侧失败升级决策：同一精确操作（工具+参数指纹）失败 MaxOperationFailures
// 次后宿主停止该操作；回合内累计 MaxEpisodeFailures 次失败（无真实进展）后
// 宿主停止回合的变更/验证，仅保留只读诊断。成功的变更/验证清零回合预算。

func TestOperationFingerprint_StableAcrossKeyOrder(t *testing.T) {
	a := json.RawMessage(`{"path":"a.go","content":"x","offset":3}`)
	b := json.RawMessage(`{"offset":3,"content":"x","path":"a.go"}`)
	fa := OperationFingerprint("edit_file", a)
	fb := OperationFingerprint("edit_file", b)
	if fa != fb {
		t.Errorf("key order changed fingerprint: %s != %s", fa, fb)
	}
	if fa == "" {
		t.Fatal("fingerprint must not be empty")
	}
}

func TestOperationFingerprint_DiffersAcrossCalls(t *testing.T) {
	a := OperationFingerprint("edit_file", json.RawMessage(`{"path":"a.go","content":"x"}`))
	b := OperationFingerprint("edit_file", json.RawMessage(`{"path":"a.go","content":"y"}`))
	c := OperationFingerprint("write_file", json.RawMessage(`{"path":"a.go","content":"x"}`))
	if a == b {
		t.Errorf("different args produced same fingerprint: %s", a)
	}
	if a == c {
		t.Errorf("different tools produced same fingerprint: %s", a)
	}
}

func TestGuard_SameOperationFailuresStopOperation(t *testing.T) {
	g := NewFailureGuard()
	args := json.RawMessage(`{"path":"a.go"}`)
	for i := 0; i < MaxOperationFailures; i++ {
		if out := g.Observe("edit_file", args, true, true); out != GuardNone && i < MaxOperationFailures-1 {
			t.Fatalf("early stop at %d: %v", i, out)
		}
	}
	// 第 4 次提出同一操作：宿主拒绝。
	if d := g.Check("edit_file", args, true); d != GuardDenyOperation {
		t.Errorf("stopped operation must be denied, got %v", d)
	}
	// 同工具不同参数：不受影响。
	if d := g.Check("edit_file", json.RawMessage(`{"path":"b.go"}`), true); d != GuardAllow {
		t.Errorf("different args must be allowed, got %v", d)
	}
}

func TestGuard_ParamChangeResetsOperationCount(t *testing.T) {
	g := NewFailureGuard()
	for i := 0; i < MaxOperationFailures; i++ {
		g.Observe("edit_file", json.RawMessage(`{"path":"a.go"}`), true, true)
	}
	// 参数变化后是全新操作，允许且重新计数。
	if d := g.Check("edit_file", json.RawMessage(`{"path":"b.go"}`), true); d != GuardAllow {
		t.Fatalf("new operation denied: %v", d)
	}
	for i := 0; i < MaxOperationFailures-1; i++ {
		g.Observe("edit_file", json.RawMessage(`{"path":"b.go"}`), true, true)
	}
	if d := g.Check("edit_file", json.RawMessage(`{"path":"b.go"}`), true); d != GuardAllow {
		t.Errorf("operation b.go stopped too early: %v", d)
	}
	g.Observe("edit_file", json.RawMessage(`{"path":"b.go"}`), true, true)
	if d := g.Check("edit_file", json.RawMessage(`{"path":"b.go"}`), true); d != GuardDenyOperation {
		t.Errorf("operation b.go must stop after %d failures, got %v", MaxOperationFailures, d)
	}
}

func TestGuard_ReadOnlyFailuresNotCounted(t *testing.T) {
	g := NewFailureGuard()
	for i := 0; i < MaxEpisodeFailures+2; i++ {
		g.Observe("read_file", json.RawMessage(`{"path":"missing.go"}`), false, true)
	}
	if d := g.Check("edit_file", json.RawMessage(`{"path":"a.go"}`), true); d != GuardAllow {
		t.Errorf("read-only failures must not accumulate episode budget: %v", d)
	}
}

func TestGuard_EpisodeFailuresStopTurn(t *testing.T) {
	g := NewFailureGuard()
	files := []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go"}
	for i := 0; i < MaxEpisodeFailures; i++ {
		out := g.Observe("edit_file", json.RawMessage(`{"path":"`+files[i]+`"}`), true, true)
		if i < MaxEpisodeFailures-1 && out != GuardNone {
			t.Fatalf("early episode stop at %d: %v", i, out)
		}
	}
	if d := g.Check("edit_file", json.RawMessage(`{"path":"b.go"}`), true); d != GuardDenyTurn {
		t.Errorf("mutating call must be denied after episode stop, got %v", d)
	}
	if d := g.Check("read_file", json.RawMessage(`{"path":"x.go"}`), false); d != GuardAllow {
		t.Errorf("read-only diagnosis must remain available, got %v", d)
	}
}

func TestGuard_SuccessMutationClearsEpisodeBudget(t *testing.T) {
	g := NewFailureGuard()
	for i := 0; i < MaxEpisodeFailures-2; i++ {
		g.Observe("edit_file", json.RawMessage(`{"path":"a.go"}`), true, true)
	}
	// 真实进展：成功变更清零回合预算。
	g.Observe("write_file", json.RawMessage(`{"path":"b.go"}`), true, false)
	for i := 0; i < MaxEpisodeFailures-1; i++ {
		g.Observe("edit_file", json.RawMessage(`{"path":"a.go"}`), true, true)
	}
	if d := g.Check("edit_file", json.RawMessage(`{"path":"c.go"}`), true); d != GuardAllow {
		t.Errorf("episode budget not cleared after successful mutation: %v", d)
	}
}

func TestGuard_StoppedOperationStaysStopped(t *testing.T) {
	g := NewFailureGuard()
	args := json.RawMessage(`{"path":"a.go"}`)
	for i := 0; i < MaxOperationFailures; i++ {
		g.Observe("edit_file", args, true, true)
	}
	// 成功的其他变更不解除已停止的操作。
	g.Observe("write_file", json.RawMessage(`{"path":"b.go"}`), true, false)
	if d := g.Check("edit_file", args, true); d != GuardDenyOperation {
		t.Errorf("stopped operation resumed after progress: %v", d)
	}
}

func TestGuard_ResetClearsAll(t *testing.T) {
	g := NewFailureGuard()
	args := json.RawMessage(`{"path":"a.go"}`)
	for i := 0; i < MaxOperationFailures; i++ {
		g.Observe("edit_file", args, true, true)
	}
	g.Reset()
	if d := g.Check("edit_file", args, true); d != GuardAllow {
		t.Errorf("reset must clear stopped operations, got %v", d)
	}
}

// ─── 接线集成测试（真实 runDirect 主循环）───

// mutFailTool 是变更型失败工具：ReadOnly=false，执行必然失败。
type mutFailTool struct{ name string }

func (m mutFailTool) Name() string            { return m.name }
func (m mutFailTool) Description() string     { return "always fails (mutating)" }
func (m mutFailTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (m mutFailTool) ReadOnly() bool          { return false }
func (m mutFailTool) Execute(context.Context, json.RawMessage) (string, error) {
	return "", errors.New("boom")
}

// mutOkTool 是变更型成功工具：代表真实进展。
type mutOkTool struct{ name string }

func (m mutOkTool) Name() string            { return m.name }
func (m mutOkTool) Description() string     { return "always succeeds (mutating)" }
func (m mutOkTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (m mutOkTool) ReadOnly() bool          { return false }
func (m mutOkTool) Execute(context.Context, json.RawMessage) (string, error) {
	return "ok", nil
}

func guardSessionContains(s *Session, content string) bool {
	for _, m := range s.Messages {
		if m.Content == content {
			return true
		}
	}
	return false
}

// TestGuardIntegrated_StopsRepeatedOperation 走真实主循环：同一变更操作连续
// 失败 MaxOperationFailures 次后，宿主拒绝第 4 次调用并注入引导消息。
func TestGuardIntegrated_StopsRepeatedOperation(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(mutFailTool{name: "edit_file"})
	mp := testutil.NewMock("m",
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"a.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"a.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"a.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"a.go"}`}}},
		testutil.Turn{Text: "done"},
	)
	sess := NewSession("sys")
	a := New(mp, reg, sess, Options{DisableVerify: true}, event.Discard)
	if _, err := a.Run(context.Background(), "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !guardSessionContains(sess, autoGuardOperationStoppedMsg) {
		t.Error("operation-stopped guidance message was not injected into the session")
	}
	// 第 4 次调用（脚本第 4 轮）应被宿主拒绝，结果携带 auto guard 标记。
	var blocked bool
	for _, m := range sess.Messages {
		if m.Role == provider.RoleTool && strings.Contains(m.Content, "auto guard") {
			blocked = true
		}
	}
	if !blocked {
		t.Error("4th identical operation call was not blocked by the host")
	}
}

// TestGuardIntegrated_EpisodeStopsTurn 走真实主循环：6 个不同变更操作各失败
// 一次后，回合变更被宿主暂停（只读仍可用），并注入回合级引导消息。
func TestGuardIntegrated_EpisodeStopsTurn(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(mutFailTool{name: "edit_file"})
	reg.Add(okTool{name: "read_file"})
	mp := testutil.NewMock("m",
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"a.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"b.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"c.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"d.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"e.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"f.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"g.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "read_file", Arguments: `{"path":"x.go"}`}}},
		testutil.Turn{Text: "done"},
	)
	sess := NewSession("sys")
	a := New(mp, reg, sess, Options{DisableVerify: true}, event.Discard)
	if _, err := a.Run(context.Background(), "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !guardSessionContains(sess, autoGuardEpisodeStoppedMsg) {
		t.Error("episode-stopped guidance message was not injected into the session")
	}
	// g.go 变更调用应被拒绝；x.go 只读调用应正常执行。
	var mutBlocked, readAllowed bool
	for _, m := range sess.Messages {
		if m.Role != provider.RoleTool {
			continue
		}
		if strings.Contains(m.Content, "auto guard") {
			mutBlocked = true
		}
		if strings.Contains(m.Content, "ok") {
			readAllowed = true
		}
	}
	if !mutBlocked {
		t.Error("mutating call after episode stop was not blocked")
	}
	if !readAllowed {
		t.Error("read-only diagnosis was not allowed after episode stop")
	}
}

// TestGuardIntegrated_SuccessClearsEpisode 走真实主循环：成功变更清零回合
// 预算，后续失败不会过早触发回合停止。
func TestGuardIntegrated_SuccessClearsEpisode(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(mutFailTool{name: "edit_file"})
	reg.Add(mutOkTool{name: "write_file"})
	mp := testutil.NewMock("m",
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"a.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"b.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"c.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"d.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "write_file", Arguments: `{"path":"ok.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"e.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"f.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"g.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"h.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"i.go"}`}}},
		testutil.Turn{ToolCalls: []provider.ToolCall{{Name: "edit_file", Arguments: `{"path":"j.go"}`}}},
		testutil.Turn{Text: "done"},
	)
	sess := NewSession("sys")
	a := New(mp, reg, sess, Options{DisableVerify: true}, event.Discard)
	if _, err := a.Run(context.Background(), "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// j.go 是成功后的第 6 次失败（预算已清零后 5 次 + 第 6 次），必须仍被放行，
	// 否则说明成功没有清零回合预算。
	for _, m := range sess.Messages {
		if m.Role == provider.RoleTool && strings.Contains(m.Content, "auto guard") {
			t.Error("mutation was blocked although a successful mutation cleared the episode budget")
		}
	}
}
