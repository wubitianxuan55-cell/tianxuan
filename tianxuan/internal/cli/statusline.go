package cli

import (
	"charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	"strings"
	"tianxuan/internal/hook"
	"time"
)

// statuslineMsg carries the latest custom status-line output (one line, ""
// when none/failed).
type statuslineMsg struct{ out string }

// runStatusline runs the user's custom status-line command off the event loop,
// feeding it a small JSON context on stdin and returning its first stdout line.
// A no-op (nil) when no command is configured. Tight timeout so a slow script
// can't stall the UI; failures collapse to an empty line rather than an error.
func (m chatTUI) runStatusline() tea.Cmd {
	cmd := m.statuslineCmd
	if cmd == "" {
		return nil
	}
	used, window := m.ctrl.ContextSnapshot()
	payload, _ := json.Marshal(map[string]any{
		"model":         m.label,
		"contextUsed":   used,
		"contextWindow": window,
	})
	return func() tea.Msg { return statuslineMsg{out: runStatuslineCmd(cmd, string(payload))} }
}

// runStatuslineCmd runs a status-line command with the JSON context on stdin and
// returns its first stdout line (status lines are a single row). A tight timeout
// keeps a slow script from stalling the UI; any failure collapses to "".
func runStatuslineCmd(cmd, stdinPayload string) string {
	res := hook.DefaultSpawner(context.Background(), hook.SpawnInput{
		Command: cmd,
		Stdin:   stdinPayload + "\n",
		Timeout: 2 * time.Second,
	})
	out := strings.TrimSpace(res.Stdout)
	if i := strings.IndexByte(out, '\n'); i >= 0 {
		out = out[:i]
	}
	return out
}
