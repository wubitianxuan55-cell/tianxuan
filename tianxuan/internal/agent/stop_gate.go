package agent

import (
	"context"
	"encoding/json"

	"tianxuan/internal/provider"
)

// --- Gate caps ---

const (
	taskGateCap = 3
	goalGateCap = 3
)

// --- Gate 1: Task gate (merged with verify) ---

const taskGateNudgeHeader = "[system] You still have unfinished tasks — " +
	"some items in your todo list are not yet completed. " +
	"Review your todo list, continue working on the next pending item, " +
	"and only stop when every task is truly done."

func (a *AgentRunner) taskGate() bool {

	incomplete := countIncompleteTodos(a.session.Messages)
	if incomplete == 0 {
		a.taskGateReentry = 0
		// V10.22: sub-agents skip orchestrate verify entirely
		if a.disableVerify {
			return false
		}
		// Tasks done — fire verify once
		if !a.verifyGateFired {
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
		return false
	}

	// Combine task + verify into ONE nudge
	msg := taskGateNudgeHeader
	if !a.verifyGateFired {
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

const goalGateNudge = "[system] The session goal has not been met yet — " +
	"review the goal and continue working. " +
	"Only stop when you have verifiably achieved the stated goal."

func (a *AgentRunner) goalGate() bool {
	if a.goal == "" {
		a.goalGateReentry = 0
		return false
	}

	a.goalGateReentry++
	if a.goalGateReentry > goalGateCap {
		return false
	}

	if a.goalGateReentry == 1 {
		verdict, err := a.judgeGoal(context.Background(), a.goal)
		if err == nil {
			if verdict.OK {
				a.goalGateReentry = 0
				return false
			}
			if verdict.Impossible {
				a.session.Add(provider.Message{
					Role:    provider.RoleUser,
					Content: "[system] Goal impossible: " + verdict.Reason + ". Stop and explain why.",
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
		Content: goalGateNudge,
	})
	return true
}

// --- Gate 3: Verify gate (always active) ---

const stopGateOrchestrateVerifyNudge = "[system] All tasks appear complete. " +
	"Before declaring done, verify your work: " +
	"run the project's test suite (go test ./... or equivalent), " +
	"check for regressions, and confirm the output matches expectations. " +
	"Only stop after tests pass."

func (a *AgentRunner) verifyGate() bool {
	if a.verifyGateFired {
		return false
	}
	a.verifyGateFired = true
	a.session.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: stopGateOrchestrateVerifyNudge,
	})
	return true
}

// --- Helpers ---

// --- Helpers ---

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
