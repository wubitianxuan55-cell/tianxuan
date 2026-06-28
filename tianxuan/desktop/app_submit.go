package main

import (
	"tianxuan/internal/config"
	"tianxuan/internal/event"
)

// --- bound command surface (frontend → controller) ---
// Each method guards on a nil controller so a pre-startup or failed-build call is
// a no-op, never a panic.

// Submit runs raw user input as a turn; slash commands and @-references are
// resolved by the controller. Output arrives asynchronously on eventChannel.
func (a *App) Submit(input string) {
	if ctrl := a.ctrlByTabID(""); ctrl != nil {
		ctrl.Submit(input)
	}
}

// SubmitDisplay runs input as a turn while recording a shorter UI-only display
// string for the saved desktop transcript. The model still receives input.
func (a *App) SubmitDisplay(display, input string) {
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return
	}
	_ = recordSessionDisplay(config.WorkspaceSessionDir(""), ctrl.SessionPath(), input, display)
	ctrl.Submit(input)
}

// Cancel aborts the in-flight turn.
func (a *App) Cancel() {
	if ctrl := a.ctrlByTabID(""); ctrl != nil {
		ctrl.Cancel()
	}
}

// Approve answers a pending approval_request by ID: allow runs the call, session
// also remembers the grant for the rest of the session.
func (a *App) Approve(id string, allow, session bool) {
	if ctrl := a.ctrlByTabID(""); ctrl != nil {
		ctrl.Approve(id, allow, session)
	}
}

// SetPlanMode toggles read-only plan mode.
func (a *App) SetPlanMode(on bool) {
	if ctrl := a.ctrlByTabID(""); ctrl != nil {
		ctrl.SetPlanMode(on)
	}
}

// QuestionAnswer is the frontend's reply to one question in an ask_request.
type QuestionAnswer struct {
	QuestionID string   `json:"questionId"`
	Selected   []string `json:"selected"`
}

// AnswerQuestion resolves a pending ask_request (the `ask` tool) by ID with the
// user's selections per question.
func (a *App) AnswerQuestion(id string, answers []QuestionAnswer) {
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return
	}
	out := make([]event.AskAnswer, len(answers))
	for i, an := range answers {
		out[i] = event.AskAnswer{QuestionID: an.QuestionID, Selected: an.Selected}
	}
	ctrl.AnswerQuestion(id, out)
}

// Compact runs one compaction pass on demand.
// Compact runs a plain compaction pass (the "compact now" button). Focus-guided
// compaction goes through Submit("/compact <focus>") instead.
func (a *App) Compact() error {
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return nil
	}
	return ctrl.Compact(a.ctx, "")
}

// SetAgentMode switches the agent runtime mode: "explore" (read-only research),
// "develop" (full tools, default), or "orchestrate" (plan→execute→verify).
func (a *App) SetAgentMode(mode string) {
	if ctrl := a.ctrlByTabID(""); ctrl != nil {
		ctrl.SetAgentMode(mode)
	}
}

// AgentMode returns the current agent runtime mode, for the status-bar indicator.
func (a *App) AgentMode() string {
	if ctrl := a.ctrlByTabID(""); ctrl != nil {
		return ctrl.AgentMode()
	}
	return ""
}

// SetBypass toggles YOLO mode for the session: auto-approve every tool call
// (writers and bash run without asking). Deny rules still apply. Runtime-only —
// not written to config, so it resets on relaunch.
func (a *App) SetBypass(on bool) {
	if ctrl := a.ctrlByTabID(""); ctrl != nil {
		ctrl.SetBypass(on)
	}
}
