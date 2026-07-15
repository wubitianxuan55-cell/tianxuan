package agent

import (
	"tianxuan/internal/provider"
)

// stopGateOrchestrateVerifyNudge prompts the model to run tests before
// declaring completion. It is the sole remaining gate — taskGate and goalGate
// were removed (V10.XX) because Hephaestus is a pure executor; Hermes handles
// verification and decides when the task is complete.
const stopGateOrchestrateVerifyNudge = "[system] All tasks appear complete. " +
	"Before declaring done, verify your work: " +
	"run the project's test suite (go test ./... or equivalent), " +
	"check for regressions, and confirm the output matches expectations. " +
	"Only stop after tests pass."

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
