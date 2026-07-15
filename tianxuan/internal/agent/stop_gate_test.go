package agent

import (
	"encoding/json"
	"testing"

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
