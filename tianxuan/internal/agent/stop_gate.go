package agent

import (
	"fmt"
	"strings"

	"tianxuan/internal/provider"
)

// ── Stop gates ──────────────────────────────────────────────────────────
// Triple gate for solo (single-model) mode. All three skip in plannerMode.
// Each gate has a reentry cap of 3 to prevent infinite loops.

// stopGateOrchestrateVerifyNudge prompts the model to run the check that
// matches the change before declaring completion (V10.141: scoped verification —
// simple/frontend/docs changes no longer force the full backend suite).
const stopGateOrchestrateVerifyNudge = "[system] All tasks appear complete. " +
	"Before declaring done, verify your work with the check that matches the change: " +
	"Go code changed -> go build + affected package tests (go test ./<pkg>); " +
	"frontend files changed -> tsc --noEmit / frontend tests / build (no backend tests needed); " +
	"docs/config only -> content check, no test suite required; " +
	"cross-module or refactor -> full suite (go test ./... or equivalent). " +
	"Check for regressions and confirm the output matches expectations. " +
	"Only stop after the matching checks pass."

// taskGate checks the canonical todo list for incomplete items. When any
// remain, it injects a nudge listing them and asking the model to complete
// and sign off with complete_step. Reentry cap = 3. Skips in plannerMode
// and when disableVerify is set (sub-agents).
func (a *AgentRunner) taskGate() bool {
	if a.plannerMode || a.disableVerify {
		return false
	}
	if a.taskGateReentry >= TaskGateReentryLimit {
		return false
	}
	incomplete, ok := a.incompleteCanonicalTodos()
	if !ok || len(incomplete) == 0 {
		return false
	}
	a.taskGateReentry++

	var names []string
	for _, t := range incomplete {
		name := t.Content
		if t.ActiveForm != "" {
			name = t.ActiveForm
		}
		names = append(names, fmt.Sprintf("  - %s [%s]", name, t.Status))
	}

	// 单模型（strictVerify=false）不强制 complete_step 签收，更新 todo 状态即可；
	// 双模型保留 complete_step 引导（Hermes 需要证据回报）。
	signOff := "complete_step 标记每个步骤。"
	if !a.evidence.StrictVerification() {
		signOff = "将完成的步骤在 todo_write 中更新为 completed。"
	}
	a.session.Add(provider.Message{
		Role: provider.RoleUser,
		Content: fmt.Sprintf(
			"[system] 以下任务尚未完成，请继续执行：\n%s\n\n"+
				"完成后使用 "+signOff,
			strings.Join(names, "\n")),
	})
	return true
}

// goalGate checks whether a session goal is set and, if so, injects a nudge
// asking the model to verify the goal has been met before declaring
// completion. Reentry cap = 3. Skips in plannerMode.
func (a *AgentRunner) goalGate() bool {
	if a.plannerMode || a.disableVerify {
		return false
	}
	if a.goal == "" {
		return false
	}
	if a.goalGateReentry >= GoalGateReentryLimit {
		return false
	}
	a.goalGateReentry++

	a.session.Add(provider.Message{
		Role: provider.RoleUser,
		Content: fmt.Sprintf(
			"[system] 会话目标：%s\n\n"+
				"在声明任务完成前，请确认以上目标是否已达成。如果尚未达成，请继续工作直到目标实现。",
			a.goal),
	})
	return true
}

// verifyGate injects a single nudge to run tests before the model declares
// completion. It fires at most once per turn. Sub-agents and planner mode
// skip the verify gate (disableVerify / plannerMode).
func (a *AgentRunner) verifyGate() bool {
	if a.verifyGateFired {
		return false
	}
	if a.disableVerify {
		return false
	}
	a.verifyGateFired = true
	a.session.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: stopGateOrchestrateVerifyNudge,
	})
	return true
}
