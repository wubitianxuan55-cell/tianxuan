package agent

import (
	"context"
	"encoding/json"

	"tianxuan/internal/provider"
)

// --- Gate caps ---
// Each gate has its own reentry cap to prevent infinite loops. Once the cap
// is exceeded the gate allows stopping (fail-open: better to stop than burn
// tokens forever). The caps are intentionally low since the gates are the
// last defense before stop, not a mechanism for driving work.

const (
	taskGateCap         = 3 // max task-gate reentries before allowing stop
	goalGateCap         = 3 // max goal-gate reentries before allowing stop
	stopGateReentryCap  = taskGateCap // legacy alias; use taskGateCap
)

// --- Gate 1: Task gate (V10.3: merged with verify gate) ---

// taskGateNudge is the synthetic user message injected when the model tries
// to stop but has unfinished tasks. Compile-time constant for cache stability.
// V7.0: now lists specific incomplete items instead of a generic nudge.
const taskGateNudgeHeader = "[system] You still have unfinished tasks — " +
	"some items in your todo list are not yet completed. " +
	"Review your todo list, continue working on the next pending item, " +
	"and only stop when every task is truly done."

// taskGate checks whether there are incomplete todos that prevent stopping.
// Returns true when a nudge was injected (caller should continue the loop).
//
// V7.0: separated from goalGate. Uses its own reentry counter (taskGateReentry).
// V10.3: skips when planMode is active (tasks can't be completed without write tools).
// V10.3: in orchestrate mode, verify nudge is merged into taskGate to save one API round-trip.
func (a *AgentRunner) taskGate() bool {
	// explore mode: no task enforcement
	if a.agentMode == "explore" {
		return false
	}
	// plan mode: can't complete tasks without write tools — don't block
	if a.planMode.Load() {
		return false
	}

	incomplete := countIncompleteTodos(a.session.Messages)
	if incomplete == 0 {
		a.taskGateReentry = 0 // reset on clean pass
		// Orchestrate mode — tasks done, fire verify once (merged)
		if a.agentMode == "orchestrate" && !a.verifyGateFired {
			a.verifyGateFired = true
			a.session.Add(provider.Message{
				Role:    provider.RoleUser,
				Content: stopGateOrchestrateVerifyNudge,
			})
			return true
		}
		return false
	}

	a.taskGateReentry++
	if a.taskGateReentry > taskGateCap {
		return false // cap exceeded: allow stop
	}

	// Orchestrate mode: combine task + verify into ONE nudge
	msg := taskGateNudgeHeader
	if a.agentMode == "orchestrate" && !a.verifyGateFired {
		msg = taskGateNudgeHeader + " " + stopGateOrchestrateVerifyNudge
		a.verifyGateFired = true
	}
	a.session.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: msg,
	})
	return true
}

// --- Gate 2: Goal gate ---

// goalGateNudge is injected when the model tries to stop but the session
// goal has not been met. Compile-time constant for cache stability.
const goalGateNudge = "[system] The session goal has not been met yet — " +
	"review the goal and continue working. " +
	"Only stop when you have verifiably achieved the stated goal."

// goalGate checks whether the session goal has been satisfied via an
// independent judge model. Returns true when a nudge was injected.
//
// V7.0: separated from taskGate. Uses its own reentry counter (goalGateReentry).
// The first reentry triggers the judge model call; subsequent reentries
// inject simple nudges.
func (a *AgentRunner) goalGate() bool {
	// explore mode: no goal enforcement
	if a.agentMode == "explore" {
		return false
	}

	if a.goal == "" {
		a.goalGateReentry = 0
		return false
	}

	a.goalGateReentry++
	if a.goalGateReentry > goalGateCap {
		return false // cap exceeded: allow stop
	}

	// First reentry: call independent judge model
	if a.goalGateReentry == 1 {
		verdict, err := a.judgeGoal(context.Background(), a.goal)
		if err == nil {
			if verdict.OK {
				a.goalGateReentry = 0
				return false // judge says goal met
			}
			if verdict.Impossible {
				a.session.Add(provider.Message{
					Role:    provider.RoleUser,
					Content: "[system] Goal impossible: " + verdict.Reason + ". Stop and explain why.",
				})
				return false // allow stop — goal is impossible
			}
			// Judge says not done — inject reason as nudge
			a.session.Add(provider.Message{
				Role:    provider.RoleUser,
				Content: "[system] Goal not yet met: " + verdict.Reason,
			})
			return true
		}
		// Judge call failed — fall through to simple nudge
	}

	a.session.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: goalGateNudge,
	})
	return true
}

// --- Gate 3: Orchestrate verification (V10.3: merged into taskGate, kept as alias) ---

// stopGateOrchestrateVerifyNudge is injected in orchestrate mode when the
// model tries to stop after completing all tasks. Compile-time constant.
const stopGateOrchestrateVerifyNudge = "[system] All tasks appear complete. " +
	"Before declaring done in orchestrate mode, verify your work: " +
	"run the project's test suite (go test ./... or equivalent), " +
	"check for regressions, and confirm the output matches expectations. " +
	"Only stop after tests pass."

// VerifyGate checks whether the model should verify its work before stopping
// (orchestrate mode). Returns true when a nudge was injected.
//
// V7.0: this is now a standalone verify step, not mixed with the stop gates.
// V10.3: functionality merged into taskGate. Kept for backward compatibility.
func (a *AgentRunner) verifyGate() bool {
	if a.agentMode != "orchestrate" {
		return false
	}
	if a.verifyGateFired {
		return false // already fired (either standalone or merged in taskGate)
	}
	a.verifyGateFired = true
	a.session.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: stopGateOrchestrateVerifyNudge,
	})
	return true
}

// --- Legacy stopGate — kept for backward compatibility ---

// stopGateNudge is the old combined nudge. Deprecated: use taskGate + goalGate.
// Kept for backward compat in tests.
const stopGateNudge = "[system] You still have unfinished tasks — " +
	"some items in your todo list are not yet completed. " +
	"Review your todo list, continue working on the next pending item, " +
	"and only stop when every task is truly done. " +
	"For complex multi-phase work, use tree-structured task IDs in todo content: " +
	"prefix phases with T1/T2/T3 and sub-steps with T1.1/T1.2/T2.1 etc."

// stopGateGoalNudge is the old goal nudge. Deprecated: use goalGate.
// Kept for backward compat in tests.
const stopGateGoalNudge = "[system] The session goal has not been met yet — " +
	"review the goal and continue working. " +
	"Only stop when you have verifiably achieved the stated goal."

// stopGate is the legacy combined stop gate. Deprecated: use taskGate() +
// goalGate() + verifyGate() instead. Kept for test backward compatibility.
//
// V7.0: internally delegates to the new gates.
func (a *AgentRunner) stopGate() bool {
	// V6.0 P3: explore mode never blocks
	if a.agentMode == "explore" {
		return false
	}

	incomplete := countIncompleteTodos(a.session.Messages)
	if incomplete > 0 {
		a.taskGateReentry++
		if a.taskGateReentry > taskGateCap {
			return false
		}
		a.session.Add(provider.Message{
			Role:    provider.RoleUser,
			Content: stopGateNudge,
		})
		return true
	}

	// Goal gate delegation
	if a.goal != "" {
		a.goalGateReentry++
		if a.goalGateReentry > goalGateCap {
			return false
		}
		if a.goalGateReentry == 1 {
			verdict, err := a.judgeGoal(context.Background(), a.goal)
			if err == nil {
				if verdict.OK {
					return false
				}
				if verdict.Impossible {
					a.session.Add(provider.Message{
						Role:    provider.RoleUser,
						Content: "[system] Goal impossible: " + verdict.Reason + ". Stop and explain.",
					})
					return false
				}
				a.session.Add(provider.Message{
					Role:    provider.RoleUser,
					Content: "[system] Goal not yet met: " + verdict.Reason,
				})
				return true
			}
		}
		a.session.Add(provider.Message{
			Role:    provider.RoleUser,
			Content: stopGateGoalNudge,
		})
		return true
	}

	// Orchestrate verify
	if a.agentMode == "orchestrate" && !a.verifyGateFired {
		a.verifyGateFired = true
		a.session.Add(provider.Message{
			Role:    provider.RoleUser,
			Content: stopGateOrchestrateVerifyNudge,
		})
		return true
	}

	a.taskGateReentry = 0
	a.goalGateReentry = 0
	return false
}

// --- Helpers ---

// countIncompleteTodos scans session messages for the most recent todo_write
// call and counts items whose status is NOT "completed". It reads the LAST
// todo_write call only (the model re-sends the COMPLETE list each time).
func countIncompleteTodos(msgs []provider.Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != provider.RoleAssistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			if tc.Name != "todo_write" {
				continue
			}
			var p struct {
				Todos []struct {
					Status string `json:"status"`
				} `json:"todos"`
			}
			if err := json.Unmarshal([]byte(tc.Arguments), &p); err != nil {
				continue
			}
			incomplete := 0
			for _, t := range p.Todos {
				if t.Status == "completed" {
					continue
				}
				incomplete++
			}
			return incomplete
		}
	}
	return 0
}
