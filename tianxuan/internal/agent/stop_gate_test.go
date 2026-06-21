package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"tianxuan/internal/strutil"
	"tianxuan/internal/provider"
)

// makeTodoMsg 创建一个包含 todo_write tool call 的 assistant 消息。
func makeTodoMsg(statuses ...string) provider.Message {
	todos := make([]map[string]string, len(statuses))
	for i, s := range statuses {
		todos[i] = map[string]string{"content": "task " + strutil.Itoa(i+1), "status": s}
	}
	args, _ := json.Marshal(map[string]any{"todos": todos})
	return provider.Message{
		Role: provider.RoleAssistant,
		ToolCalls: []provider.ToolCall{{
			ID:        "call_1",
			Name:      "todo_write",
			Arguments: string(args),
		}},
	}
}

// TestCountIncompleteTodosAllDone：全部 completed → 返回 0
func TestCountIncompleteTodosAllDone(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "system"},
		{Role: provider.RoleUser, Content: "hello"},
		makeTodoMsg("completed", "completed"),
	}
	n := countIncompleteTodos(msgs)
	if n != 0 {
		t.Fatalf("expected 0 incomplete, got %d", n)
	}
}

// TestCountIncompleteTodosMixed：混合状态 → 只数非 completed
func TestCountIncompleteTodosMixed(t *testing.T) {
	msgs := []provider.Message{
		makeTodoMsg("completed", "in_progress", "pending"),
	}
	n := countIncompleteTodos(msgs)
	if n != 2 {
		t.Fatalf("expected 2 incomplete, got %d", n)
	}
}

// TestCountIncompleteTodosLastWins：只读最后一个 todo_write
func TestCountIncompleteTodosLastWins(t *testing.T) {
	msgs := []provider.Message{
		makeTodoMsg("pending", "pending", "pending"),
		makeTodoMsg("completed", "completed"),
	}
	n := countIncompleteTodos(msgs)
	if n != 0 {
		t.Fatalf("expected 0 (last wins), got %d", n)
	}
}

// TestCountIncompleteTodosNoCall：没有 todo_write → 返回 0
func TestCountIncompleteTodosNoCall(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "hello"},
	}
	n := countIncompleteTodos(msgs)
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

// TestStopGateAllowsWhenDone：无未完成 → stopGate 返回 false，不注入消息
func TestStopGateAllowsWhenDone(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("completed"))
	a := &AgentRunner{session: s}
	before := len(s.Messages)
	blocked := a.stopGate()
	if blocked {
		t.Fatal("expected false (allow stop), got true")
	}
	if len(s.Messages) != before {
		t.Fatalf("expected no new messages, got +%d", len(s.Messages)-before)
	}
}

// TestStopGateBlocksWhenPending：有未完成 → stopGate 返回 true，注入 nudge
func TestStopGateBlocksWhenPending(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("pending", "completed"))
	a := &AgentRunner{session: s}
	blocked := a.stopGate()
	if !blocked {
		t.Fatal("expected true (block stop), got false")
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Role != provider.RoleUser {
		t.Fatalf("expected user-role nudge, got %s", last.Role)
	}
	if last.Content != stopGateNudge {
		t.Fatalf("expected stopGateNudge constant, got %q", last.Content)
	}
}

// TestStopGateSafetyValve：3 次重入后强制放行
func TestStopGateSafetyValve(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("pending"))
	a := &AgentRunner{session: s}

	for i := 0; i < stopGateReentryCap; i++ {
		if !a.stopGate() {
			t.Fatalf("reentry %d: expected true (block)", i+1)
		}
	}
	if a.stopGate() {
		t.Fatal("after cap: expected false (allow), got true")
	}
}

// TestStopGateNudgeTextIsConstant：验证 nudge 文本是编译期常量
func TestStopGateNudgeTextIsConstant(t *testing.T) {
	if stopGateNudge == "" {
		t.Fatal("stopGateNudge is empty")
	}
	for i := 0; i < len(stopGateNudge); i++ {
		if stopGateNudge[i] == '%' {
			t.Fatal("stopGateNudge contains '%' — must have no format verbs")
		}
	}
}

// TestStopGateBlocksWhenGoalNotMet：todos 完成但有 goal 时调用 judge 验证
func TestStopGateBlocksWhenGoalNotMet(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("completed"))
	a := &AgentRunner{session: s, goal: "implement user login"}
	blocked := a.stopGate()
	if !blocked {
		t.Fatal("expected true (block stop for goal), got false")
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Role != provider.RoleUser {
		t.Fatalf("expected user-role nudge, got %s", last.Role)
	}
	// V7.0: first re-entry calls judge (nil provider → judge reason message)
	if last.Content == stopGateGoalNudge {
		return // fallback nudge — also acceptable
	}
	if !strings.Contains(last.Content, "Goal not yet met") {
		t.Fatalf("expected judge reason or stopGateGoalNudge, got %q", last.Content)
	}
}

// TestStopGateAllowsWhenGoalMet：todos 完成且无 goal → 放行
func TestStopGateAllowsWhenNoGoal(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("completed"))
	a := &AgentRunner{session: s} // goal is empty
	before := len(s.Messages)
	blocked := a.stopGate()
	if blocked {
		t.Fatal("expected false (no goal, allow stop), got true")
	}
	if len(s.Messages) != before {
		t.Fatalf("expected no new messages, got +%d", len(s.Messages)-before)
	}
}

// TestStopGateGoalSafetyValve：goal 闸门 3 次重入后强制放行
func TestStopGateGoalSafetyValve(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("completed"))
	a := &AgentRunner{session: s, goal: "implement user login"}

	for i := 0; i < stopGateReentryCap; i++ {
		if !a.stopGate() {
			t.Fatalf("reentry %d: expected true (block for goal)", i+1)
		}
	}
	if a.stopGate() {
		t.Fatal("after cap: expected false (allow), got true")
	}
}

// TestStopGateGoalNudgeTextIsConstant：验证 goal nudge 文本是编译期常量
func TestStopGateGoalNudgeTextIsConstant(t *testing.T) {
	if stopGateGoalNudge == "" {
		t.Fatal("stopGateGoalNudge is empty")
	}
	for i := 0; i < len(stopGateGoalNudge); i++ {
		if stopGateGoalNudge[i] == '%' {
			t.Fatal("stopGateGoalNudge contains '%' — must have no format verbs")
		}
	}
}

// TestStopGateExploreSkips：explore 模式直接放行（不检查 todos）
func TestStopGateExploreSkips(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("pending", "pending")) // incomplete todos
	a := &AgentRunner{session: s, agentMode: "explore"}
	before := len(s.Messages)
	blocked := a.stopGate()
	if blocked {
		t.Fatal("explore mode should skip stop gate, got blocked")
	}
	if len(s.Messages) != before {
		t.Fatalf("explore mode should not inject messages, got +%d", len(s.Messages)-before)
	}
}

// TestStopGateOrchestrateVerify：orchestrate 模式 todos 完成后注入 verify nudge
func TestStopGateOrchestrateVerify(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("completed"))
	a := &AgentRunner{session: s, agentMode: "orchestrate"}
	blocked := a.stopGate()
	if !blocked {
		t.Fatal("orchestrate mode should inject verify nudge on first stop")
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Content != stopGateOrchestrateVerifyNudge {
		t.Fatalf("expected orchestrate verify nudge, got %q", last.Content)
	}
}

// TestStopGateOrchestrateVerifyOnce：orchestrate verify nudge 只注入一次
func TestStopGateOrchestrateVerifyOnce(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("completed"))
	a := &AgentRunner{session: s, agentMode: "orchestrate"}
	// First call: injects verify nudge
	if !a.stopGate() {
		t.Fatal("first stop should inject verify nudge")
	}
	// Second call: allow stop (stopGateReentry > 0)
	if a.stopGate() {
		t.Fatal("second stop should allow (already verified)")
	}
}

// TestStopGateOrchestrateVerifyNudgeConstant：验证 orchestrate verify nudge 文本
func TestStopGateOrchestrateVerifyNudgeConstant(t *testing.T) {
	if stopGateOrchestrateVerifyNudge == "" {
		t.Fatal("stopGateOrchestrateVerifyNudge is empty")
	}
	for i := 0; i < len(stopGateOrchestrateVerifyNudge); i++ {
		if stopGateOrchestrateVerifyNudge[i] == '%' {
			t.Fatal("stopGateOrchestrateVerifyNudge contains '%' — must have no format verbs")
		}
	}
}
