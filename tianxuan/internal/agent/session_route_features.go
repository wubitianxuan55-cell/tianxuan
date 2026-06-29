package agent

import (
	"tianxuan/internal/provider"
)

// SessionRouteFeatures captures the current session's complexity signals
// for model routing — analogous to DSpark's extract_context_feature() which
// pulls intermediate hidden states from the target model.
//
// 缓存安全: 纯运行时读取，不修改任何进入 API 消息数组的内容。
type SessionRouteFeatures struct {
	TurnCount     int // total turns this session
	RecentErrors  int // tool errors in last 3 turns
	PendingTodos  int // incomplete todo items
	FilesModified int // distinct files edited this session
}

// IsComplex reports whether the session state indicates a complex task
// that warrants the pro model regardless of the current input text.
// Thresholds are deliberately conservative — flash is the default,
// pro is the escalation.
func (f SessionRouteFeatures) IsComplex() bool {
	if f.TurnCount > 10 {
		return true
	}
	if f.RecentErrors >= 3 {
		return true
	}
	if f.PendingTodos > 3 {
		return true
	}
	if f.FilesModified > 8 {
		return true
	}
	return false
}

// AutoRouteWithSession extends the heuristic AutoRoute with session-state
// signals. When the session is already complex (long, error-prone, multi-step),
// the pro model is selected even for short follow-up inputs like "继续".
// Otherwise falls back to the input-text-only heuristic.
func AutoRouteWithSession(input string, features SessionRouteFeatures) AutoRouteModel {
	if features.IsComplex() {
		return AutoRoutePro
	}
	return AutoRoute(input)
}

// AutoRouteProviderWithSession selects the provider using both input text
// and session features. flashProv=nil disables auto-routing (always defaultProv).
func AutoRouteProviderWithSession(input string, features SessionRouteFeatures, defaultProv, flashProv provider.Provider) provider.Provider {
	if flashProv == nil {
		return defaultProv
	}
	route := AutoRouteWithSession(input, features)
	if route == AutoRouteFlash {
		return flashProv
	}
	return defaultProv
}

// collectSessionRouteFeatures gathers complexity signals from the agent's
// current state. Called once per turn before routing.
func (a *AgentRunner) collectSessionRouteFeatures() SessionRouteFeatures {
	msgs := a.session.Messages
	f := SessionRouteFeatures{}

	// Count turns: each user message is one turn.
	for _, m := range msgs {
		if m.Role == provider.RoleUser {
			f.TurnCount++
		}
	}

	// Count recent errors: tool errors in last 6 tool-result messages (≈3 turns).
	recent := 0
	for i := len(msgs) - 1; i >= 0 && recent < 6; i-- {
		if msgs[i].Role != provider.RoleTool {
			continue
		}
		recent++
		c := msgs[i].Content
		if len(c) > 0 && (c[0] == 'e' || c[0] == 'b' || c[0] == 'p') {
			// "error:", "blocked:", "precheck blocked:"
			if len(c) >= 6 && (c[:6] == "error:" || c[:6] == "blocke" || c[:6] == "preche") {
				f.RecentErrors++
			}
		}
	}

	// Count pending todos from canonical state.
	a.todoMu.Lock()
	for _, t := range a.todoState {
		if t.Status == "pending" || t.Status == "in_progress" {
			f.PendingTodos++
		}
	}
	a.todoMu.Unlock()

	// Count distinct edited files from assistant tool calls (not tool results).
	// Previously this iterated RoleTool messages and passed m.Content to
	// extractFilePath — but m.Content is the execution result string (e.g.
	// "File edited successfully"), NOT the JSON arguments. extractFilePath
	// scans for JSON-encoded "path" keys so it always returned "", making
	// FilesModified permanently 0 and the Pro-model auto-escalation dead.
	//
	// Fix: walk assistant messages, iterate their ToolCalls, and extract
	// the path from ToolCall.Arguments (which IS the raw JSON parameters).
	seen := map[string]bool{}
	for _, m := range msgs {
		if m.Role == provider.RoleAssistant && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				for _, name := range []string{"edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol"} {
					if tc.Name == name {
						path := extractFilePath(tc.Name, tc.Arguments)
						if path != "" {
							seen[path] = true
						}
					}
				}
			}
		}
	}
	f.FilesModified = len(seen)

	return f
}
