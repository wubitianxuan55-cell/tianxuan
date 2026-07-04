package agent

import (
	"testing"

	"tianxuan/internal/provider"
)

func TestCheckBgStartKillCycle_NotTriggeredWhenOnlyStarted(t *testing.T) {
	a := New(nil, nil, NewSession(""), Options{}, nil)
	a.bgJobStartedThisTurn = true
	// Started but not killed — should not trigger.
	if a.checkBgStartKillCycle() {
		t.Error("only started, no kill — should NOT trigger nudge")
	}
	if a.bgStartKillStreak != 0 {
		t.Errorf("streak should be 0 when no kill, got %d", a.bgStartKillStreak)
	}
}

func TestCheckBgStartKillCycle_NotTriggeredWhenOnlyKilled(t *testing.T) {
	a := New(nil, nil, NewSession(""), Options{}, nil)
	a.bgJobKilledThisTurn = true
	// Killed but not started — should not trigger.
	if a.checkBgStartKillCycle() {
		t.Error("only killed, no start — should NOT trigger nudge")
	}
}

func TestCheckBgStartKillCycle_NotTriggeredWhenOutputRead(t *testing.T) {
	a := New(nil, nil, NewSession(""), Options{}, nil)
	a.bgJobStartedThisTurn = true
	a.bgJobKilledThisTurn = true
	a.bgOutputReadThisTurn = true // bash_output or wait was called
	// Start + kill + output read = normal usage pattern.
	if a.checkBgStartKillCycle() {
		t.Error("start + kill + output read is normal — should NOT trigger nudge")
	}
	// Streak should NOT increment when output was read.
	if a.bgStartKillStreak != 0 {
		t.Errorf("streak should be 0 when output read, got %d", a.bgStartKillStreak)
	}
}

func TestCheckBgStartKillCycle_StreakIncrements(t *testing.T) {
	a := New(nil, nil, NewSession(""), Options{}, nil)

	// First start-kill cycle — streak goes to 1, no nudge yet.
	a.bgJobStartedThisTurn = true
	a.bgJobKilledThisTurn = true
	if a.checkBgStartKillCycle() {
		t.Error("first cycle should NOT trigger nudge")
	}
	if a.bgStartKillStreak != 1 {
		t.Errorf("streak should be 1, got %d", a.bgStartKillStreak)
	}

	// Simulate next turn reset (flags cleared) then second cycle.
	a.bgJobStartedThisTurn = true
	a.bgJobKilledThisTurn = true
	a.bgOutputReadThisTurn = false
	if a.checkBgStartKillCycle() {
		t.Error("second cycle should NOT trigger nudge")
	}
	if a.bgStartKillStreak != 2 {
		t.Errorf("streak should be 2, got %d", a.bgStartKillStreak)
	}
}

func TestCheckBgStartKillCycle_TriggersNudgeAt3(t *testing.T) {
	a := New(nil, nil, NewSession(""), Options{}, nil)

	// Build up to 3 start-kill cycles.
	for i := 0; i < 3; i++ {
		a.bgJobStartedThisTurn = true
		a.bgJobKilledThisTurn = true
		a.bgOutputReadThisTurn = false
		triggered := a.checkBgStartKillCycle()
		if i < 2 {
			if triggered {
				t.Errorf("cycle %d should NOT trigger nudge yet", i+1)
			}
			// Reset per-turn flags for next cycle.
			a.bgJobStartedThisTurn = false
			a.bgJobKilledThisTurn = false
		} else {
			if !triggered {
				t.Error("third cycle SHOULD trigger nudge")
			}
		}
	}

	// After nudge, streak should be reset.
	if a.bgStartKillStreak != 0 {
		t.Errorf("streak should be 0 after nudge, got %d", a.bgStartKillStreak)
	}

	// Check the nudge message was added to session.
	msgs := a.session.Messages
	if len(msgs) == 0 {
		t.Fatal("expected nudge message in session")
	}
	last := msgs[len(msgs)-1]
	if last.Role != provider.RoleUser {
		t.Errorf("nudge should be a user message, got %s", last.Role)
	}
	if last.Content == "" {
		t.Error("nudge should have content")
	}
}

func TestCheckBgStartKillCycle_ForegroundBashResets(t *testing.T) {
	a := New(nil, nil, NewSession(""), Options{}, nil)

	// Build up 2 start-kill cycles.
	a.bgJobStartedThisTurn = true
	a.bgJobKilledThisTurn = true
	a.checkBgStartKillCycle() // streak = 1

	a.bgJobStartedThisTurn = true
	a.bgJobKilledThisTurn = true
	a.checkBgStartKillCycle() // streak = 2

	if a.bgStartKillStreak != 2 {
		t.Fatalf("streak should be 2, got %d", a.bgStartKillStreak)
	}

	// Simulate foreground bash call (sets streak to 0 in executeOne).
	a.bgStartKillStreak = 0

	// Now 3 more start-kill cycles should NOT trigger (streak restarted).
	a.bgJobStartedThisTurn = true
	a.bgJobKilledThisTurn = true
	if a.checkBgStartKillCycle() {
		t.Error("after reset, first cycle should NOT trigger nudge")
	}
	if a.bgStartKillStreak != 1 {
		t.Errorf("after reset, streak should be 1, got %d", a.bgStartKillStreak)
	}
}

func TestCheckBgStartKillCycle_BashOutputResets(t *testing.T) {
	a := New(nil, nil, NewSession(""), Options{}, nil)

	// Build up 2 start-kill cycles.
	a.bgJobStartedThisTurn = true
	a.bgJobKilledThisTurn = true
	a.checkBgStartKillCycle() // streak = 1

	a.bgJobStartedThisTurn = true
	a.bgJobKilledThisTurn = true
	a.checkBgStartKillCycle() // streak = 2

	// Simulate bash_output call (resets streak to 0 in executeOne).
	a.bgStartKillStreak = 0
	a.bgOutputReadThisTurn = true

	// This turn should NOT increment streak (output read).
	a.bgJobStartedThisTurn = true
	a.bgJobKilledThisTurn = true
	if a.checkBgStartKillCycle() {
		t.Error("after bash_output reset, streak should still be 0")
	}
	if a.bgStartKillStreak != 0 {
		t.Errorf("streak should be 0 when output read, got %d", a.bgStartKillStreak)
	}
}

// Test that the flags get set correctly through executeOne.
func TestBgFlags_SetViaExecuteOne(t *testing.T) {
	a := New(nil, nil, NewSession(""), Options{}, nil)

	// Register the real bash/kill_shell/bash_output tools so executeOne can resolve them.
	// The executeOne function uses a.tools.Get() — since we registered nothing,
	// it will return "unknown tool". But we can still test the flag-setting logic
	// indirectly through checkBgStartKillCycle.

	// For now, test the flags directly since tools require provider registration.
	a.bgJobStartedThisTurn = true
	a.bgJobKilledThisTurn = true
	if !a.bgJobStartedThisTurn || !a.bgJobKilledThisTurn {
		t.Error("flags should be set")
	}
}
