package agent

import (
	"strings"

	"tianxuan/internal/provider"
)

// recallReminderNudge is injected at the start of the first user turn when the
// session has memory artifacts. Compile-time constant for cache stability.
const recallReminderNudge = "[system] This session has saved memory — " +
	"prior context, preferences, and project facts are stored as markdown files. " +
	"Before answering, check memory with read_file or search. " +
	"Don't ask the user about something that memory may already record."

// maybeRecallReminder injects a recall nudge once when the session has memory.
// Only fires on the first call, and only when memory content is actually present
// in the session (detected via <memory-update> blocks or memQueue being non-nil
// AND the session has progressed beyond the first turn).
func (a *AgentRunner) maybeRecallReminder() {
	if a.memQueue == nil {
		return
	}
	// One-shot: only remind once per session
	if a.recallReminderFired {
		return
	}
	a.recallReminderFired = true

	// Only inject if there's actual memory content in the session
	hasMemory := false
	for _, m := range a.session.Messages {
		if strings.Contains(m.Content, "<memory-update>") ||
			strings.Contains(m.Content, "TIANXUAN.md") ||
			strings.Contains(m.Content, "project memory") {
			hasMemory = true
			break
		}
	}
	if !hasMemory {
		return
	}

	a.session.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: recallReminderNudge,
	})
}
