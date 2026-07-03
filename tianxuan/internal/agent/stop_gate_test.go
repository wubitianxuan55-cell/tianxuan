package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"tianxuan/internal/provider"
	"tianxuan/internal/strutil"
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

// ── countIncompleteTodos ──

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

func TestCountIncompleteTodosMixed(t *testing.T) {
	msgs := []provider.Message{
		makeTodoMsg("completed", "in_progress", "pending"),
	}
	n := countIncompleteTodos(msgs)
	if n != 2 {
		t.Fatalf("expected 2 incomplete, got %d", n)
	}
}

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

func TestCountIncompleteTodosNoCall(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "hello"},
	}
	n := countIncompleteTodos(msgs)
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

// ── taskGate ──

// ── taskGate ──

func TestTaskGateFiresVerifyWhenDone(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("completed"))
	a := &AgentRunner{session: s}
	blocked := a.taskGate()
	if !blocked {
		t.Fatal("expected true (verify nudge), got false")
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Content != stopGateOrchestrateVerifyNudge {
		t.Fatalf("expected verify nudge, got %q", last.Content)
	}
	// Second call: verify already fired, no more blocks
	blocked = a.taskGate()
	if blocked {
		t.Fatal("second call: expected false (already verified), got true")
	}
}

func TestTaskGateBlocksWhenPending(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("pending", "completed"))
	a := &AgentRunner{session: s}
	blocked := a.taskGate()
	if !blocked {
		t.Fatal("expected true (block), got false")
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Role != provider.RoleUser {
		t.Fatalf("expected user-role nudge, got %s", last.Role)
	}
	if !strings.Contains(last.Content, "unfinished tasks") {
		t.Fatalf("expected task nudge, got %q", last.Content)
	}
}

func TestTaskGateSafetyValve(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("pending"))
	a := &AgentRunner{session: s}

	for i := 0; i < taskGateCap; i++ {
		if !a.taskGate() {
			t.Fatalf("reentry %d: expected true (block)", i+1)
		}
	}
	if a.taskGate() {
		t.Fatal("after cap: expected false (allow), got true")
	}
}

func TestTaskGateNudgeTextIsConstant(t *testing.T) {
	if taskGateNudgeHeader == "" {
		t.Fatal("taskGateNudgeHeader is empty")
	}
	for i := 0; i < len(taskGateNudgeHeader); i++ {
		if taskGateNudgeHeader[i] == '%' {
			t.Fatal("taskGateNudgeHeader contains '%' — must have no format verbs")
		}
	}
}

// ── goalGate ──

func TestGoalGateBlocksWhenGoalNotMet(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("completed"))
	a := &AgentRunner{session: s, goal: "implement user login"}
	blocked := a.goalGate()
	if !blocked {
		t.Fatal("expected true (block for goal), got false")
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Role != provider.RoleUser {
		t.Fatalf("expected user-role nudge, got %s", last.Role)
	}
	if !strings.Contains(last.Content, "Goal not yet met") && last.Content != goalGateNudge {
		t.Fatalf("expected judge reason or goalGateNudge, got %q", last.Content)
	}
}

func TestGoalGateAllowsWhenNoGoal(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("completed"))
	a := &AgentRunner{session: s}
	before := len(s.Messages)
	blocked := a.goalGate()
	if blocked {
		t.Fatal("expected false (no goal, allow), got true")
	}
	if len(s.Messages) != before {
		t.Fatalf("expected no new messages, got +%d", len(s.Messages)-before)
	}
}

func TestGoalGateSafetyValve(t *testing.T) {
	s := NewSession("")
	s.Add(makeTodoMsg("completed"))
	a := &AgentRunner{session: s, goal: "implement user login"}

	for i := 0; i < goalGateCap; i++ {
		if !a.goalGate() {
			t.Fatalf("reentry %d: expected true (block for goal)", i+1)
		}
	}
	if a.goalGate() {
		t.Fatal("after cap: expected false (allow), got true")
	}
}

func TestGoalGateNudgeTextIsConstant(t *testing.T) {
	if goalGateNudge == "" {
		t.Fatal("goalGateNudge is empty")
	}
	for i := 0; i < len(goalGateNudge); i++ {
		if goalGateNudge[i] == '%' {
			t.Fatal("goalGateNudge contains '%' — must have no format verbs")
		}
	}
}

// ── verifyGate ──

func TestVerifyGateFiresOnce(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}
	blocked := a.verifyGate()
	if !blocked {
		t.Fatal("first verifyGate should fire")
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Content != stopGateOrchestrateVerifyNudge {
		t.Fatalf("expected verify nudge, got %q", last.Content)
	}
}

func TestVerifyGateOnlyOnce(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}
	if !a.verifyGate() {
		t.Fatal("first call should fire")
	}
	if a.verifyGate() {
		t.Fatal("second call should not fire")
	}
}

func TestVerifyGateNudgeConstant(t *testing.T) {
	if stopGateOrchestrateVerifyNudge == "" {
		t.Fatal("stopGateOrchestrateVerifyNudge is empty")
	}
	for i := 0; i < len(stopGateOrchestrateVerifyNudge); i++ {
		if stopGateOrchestrateVerifyNudge[i] == '%' {
			t.Fatal("stopGateOrchestrateVerifyNudge contains '%' — must have no format verbs")
		}
	}
}
