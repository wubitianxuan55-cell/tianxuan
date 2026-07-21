package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"tianxuan/internal/evidence"
	"tianxuan/internal/provider"
	"tianxuan/internal/strutil"
)

// makeTodoMsg creates an assistant message with a todo_write tool call.
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

// ── taskGate ──

func TestTaskGateSkipsInPlannerMode(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s, plannerMode: true}
	if a.taskGate() {
		t.Fatal("taskGate should skip in plannerMode")
	}
}

func TestTaskGateSkipsWhenDisabledVerify(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s, disableVerify: true}
	if a.taskGate() {
		t.Fatal("taskGate should skip when disableVerify is set")
	}
}

func TestTaskGateReentryCap(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}
	// Seed incomplete todos so the gate has something to fire about.
	pending := []evidence.TodoItem{{Content: "step 1", Status: "pending"}}
	a.setTodoState(pending)
	// First 3 calls should fire.
	for i := 0; i < 3; i++ {
		if !a.taskGate() {
			t.Fatalf("call %d should fire", i+1)
		}
	}
	// 4th call — reentry cap hit.
	if a.taskGate() {
		t.Fatal("4th call should not fire (reentry cap)")
	}
}

func TestTaskGateSkipsWhenNoTodos(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}
	// No todo state set — incompleteCanonicalTodos returns (nil, false).
	if a.taskGate() {
		t.Fatal("taskGate should skip when no todos exist")
	}
}

func TestTaskGateSkipsWhenAllCompleted(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}
	allDone := []evidence.TodoItem{
		{Content: "step 1", Status: "completed"},
		{Content: "step 2", Status: "completed"},
	}
	a.setTodoState(allDone)
	if a.taskGate() {
		t.Fatal("taskGate should skip when all todos are completed")
	}
}

func TestTaskGateFiresWithIncompleteTodos(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}
	pending := []evidence.TodoItem{
		{Content: "step 1", Status: "completed"},
		{Content: "step 2", Status: "pending"},
		{Content: "step 3", Status: "in_progress", ActiveForm: "正在执行步骤3"},
	}
	a.setTodoState(pending)

	if !a.taskGate() {
		t.Fatal("taskGate should fire with incomplete todos")
	}

	// Verify the nudge message was injected.
	last := s.Messages[len(s.Messages)-1]
	if last.Role != provider.RoleUser {
		t.Fatalf("expected user-role nudge, got %s", last.Role)
	}
	if !strings.Contains(last.Content, "step 2") || !strings.Contains(last.Content, "步骤3") {
		t.Fatalf("nudge should list incomplete steps, got: %s", last.Content)
	}
	if !strings.Contains(last.Content, "complete_step") {
		t.Fatal("nudge should mention complete_step")
	}
}

// ── goalGate ──

func TestGoalGateSkipsInPlannerMode(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s, plannerMode: true, goal: "fix all bugs"}
	if a.goalGate() {
		t.Fatal("goalGate should skip in plannerMode")
	}
}

func TestGoalGateSkipsWhenNoGoal(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}
	if a.goalGate() {
		t.Fatal("goalGate should skip when goal is empty")
	}
}

func TestGoalGateReentryCap(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s, goal: "implement user login"}
	for i := 0; i < 3; i++ {
		if !a.goalGate() {
			t.Fatalf("call %d should fire", i+1)
		}
	}
	if a.goalGate() {
		t.Fatal("4th call should not fire (reentry cap)")
	}
}

func TestGoalGateFiresWithGoal(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s, goal: "implement user login"}
	if !a.goalGate() {
		t.Fatal("goalGate should fire when goal is set")
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Role != provider.RoleUser {
		t.Fatalf("expected user-role nudge, got %s", last.Role)
	}
	if !strings.Contains(last.Content, "implement user login") {
		t.Fatalf("nudge should include goal text, got: %s", last.Content)
	}
}

// ── finalReadinessCheck ────────────────────────────────────────────

func TestFinalReadinessCheck_NilEvidence(t *testing.T) {
	a := &AgentRunner{} // evidence is nil by default
	blocked, _ := a.finalReadinessCheck()
	if blocked {
		t.Fatal("nil evidence should not block")
	}
}

func TestFinalReadinessCheck_NoBaseline(t *testing.T) {
	ledger := evidence.NewLedger()
	a := &AgentRunner{evidence: ledger}
	blocked, _ := a.finalReadinessCheck()
	if blocked {
		t.Fatal("no baseline should not block")
	}
}

// TestFinalReadinessCheck_UnverifiedTodosWithNilCurrent verifies the current
// behavior: UnverifiedCompletedTodos(nil) means "no current todos to check".
// With nil current, even unverified completed todos in the ledger don't block.
// See canonical_todo.go: recordTodoState updates the ledger with a synthetic
// baseline when complete_step fires, so by the time finalReadinessCheck runs
// the ledger's latest todo_write baseline has already been patched.
func TestFinalReadinessCheck_UnverifiedTodosWithNilCurrent(t *testing.T) {
	ledger := evidence.NewLedger()
	// A todo_write that marks steps as completed without complete_step.
	// In production, recordTodoState would have already patched the ledger
	// by the time finalReadinessCheck runs. Since we bypass that path,
	// and finalReadinessCheck passes nil as current, this should not block.
	ledger.Record(evidence.Receipt{
		ToolName: "todo_write",
		Success:  true,
		Todos: []evidence.TodoItem{
			{Content: "Step 1", Status: "completed", ActiveForm: "Step 1"},
			{Content: "Step 2", Status: "completed", ActiveForm: "Step 2"},
		},
	})
	a := &AgentRunner{evidence: ledger}
	blocked, _ := a.finalReadinessCheck()
	if blocked {
		t.Fatal("nil current should not block even with unverified todos (recordTodoState patches baseline first)")
	}
}

func TestFinalReadinessCheck_VerifiedTodos(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.Record(evidence.Receipt{
		ToolName: "todo_write",
		Success:  true,
		Todos: []evidence.TodoItem{
			{Content: "Step 1", Status: "completed", ActiveForm: "Step 1"},
		},
	})
	ledger.Record(evidence.Receipt{
		ToolName: "complete_step",
		Success:  true,
		Step:     "1",
	})
	a := &AgentRunner{evidence: ledger}
	blocked, _ := a.finalReadinessCheck()
	if blocked {
		t.Fatal("verified todos should not block")
	}
}
