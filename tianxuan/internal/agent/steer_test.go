package agent

import (
	"testing"

	"tianxuan/internal/provider"
)

func TestShouldMidTurnSteer_AllBlockedIsNotFailure(t *testing.T) {
	a := New(nil, nil, NewSession(""), Options{}, nil)
	calls := []provider.ToolCall{
		{Name: "write_file", Arguments: `{"path":"/x"}`},
		{Name: "edit_file", Arguments: `{"old":"a","new":"b"}`},
	}
	// All "blocked:" results — plan mode, completely normal.
	results := []string{
		"blocked: write_file is a writer tool and plan mode is read-only",
		"blocked: edit_file is a writer tool and plan mode is read-only",
	}
	if a.shouldMidTurnSteer(calls, results) {
		t.Error("all-blocked batch should NOT trigger steer — it is normal in plan mode")
	}
}

func TestShouldMidTurnSteer_RealFailuresTrigger(t *testing.T) {
	a := New(nil, nil, NewSession(""), Options{}, nil)
	calls := []provider.ToolCall{
		{Name: "read_file", Arguments: `{"path":"/x"}`},
		{Name: "read_file", Arguments: `{"path":"/y"}`},
	}
	// Real errors should trigger.
	results := []string{
		"error: no such file",
		"error: permission denied",
	}
	if !a.shouldMidTurnSteer(calls, results) {
		t.Error("real failures should trigger steer")
	}
}

func TestShouldMidTurnSteer_MixedBlockedAndFailedTriggers(t *testing.T) {
	a := New(nil, nil, NewSession(""), Options{}, nil)
	calls := []provider.ToolCall{
		{Name: "write_file", Arguments: `{"path":"/x"}`},
		{Name: "read_file", Arguments: `{"path":"/y"}`},
		{Name: "read_file", Arguments: `{"path":"/z"}`},
	}
	// One blocked, two real failures — non-blocked failures >= 2 should trigger.
	results := []string{
		"blocked: writer in plan mode",
		"error: no such file",
		"error: permission denied",
	}
	if !a.shouldMidTurnSteer(calls, results) {
		t.Error("mixed blocked+real failures (2 real) should trigger steer")
	}
}

func TestShouldMidTurnSteer_SingleFailureNotTriggered(t *testing.T) {
	a := New(nil, nil, NewSession(""), Options{}, nil)
	calls := []provider.ToolCall{
		{Name: "read_file", Arguments: `{"path":"/x"}`},
		{Name: "read_file", Arguments: `{"path":"/y"}`},
	}
	// One failure, one success — not enough for steer.
	results := []string{
		"error: no such file",
		"file content here",
	}
	if a.shouldMidTurnSteer(calls, results) {
		t.Error("single failure should NOT trigger steer")
	}
}
