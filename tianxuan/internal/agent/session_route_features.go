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
	TurnCount       int  // total turns this session
	RecentErrors    int  // tool errors in last 3 turns
	PendingTodos    int  // incomplete todo items
	FilesModified   int  // distinct files edited this session
	HasWrittenFiles bool // any write tool (edit_file/write_file/multi_edit/bash) used
	HasUsedSubAgent bool // any sub-agent spawned (task tool)
}

// IsComplex reports whether the session state indicates a complex task
// that warrants the pro model regardless of the current input text.
// Thresholds are deliberately conservative — flash is the default,
// pro is the escalation.
func (f SessionRouteFeatures) IsComplex() bool {
	// Turn-based: 5+ turns indicates a sustained conversation likely doing real work.
	if f.TurnCount > 5 {
		return true
	}
	// Write tools: the session has already modified files, so subsequent turns
	// are likely refactoring / editing — flash may not handle complex edits.
	if f.HasWrittenFiles {
		return true
	}
	// Sub-agent usage: spawning sub-agents is a strong signal of complex workflow.
	if f.HasUsedSubAgent {
		return true
	}
	// Error-prone: consistently failing tool calls may indicate model capability gap.
	if f.RecentErrors >= 3 {
		return true
	}
	// Multi-step planning: many pending todos suggest complex orchestration.
	if f.PendingTodos > 3 {
		return true
	}
	// Extensive editing: touching 9+ distinct files is a large-scale change.
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

	// Count distinct edited files + write-tool + sub-agent signals from
	// assistant tool calls.
	// Iterates assistant messages with ToolCalls.Arguments (JSON parameters),
	// not RoleTool.Content (execution result string — see V10.12.0 bugfix).
	seen := map[string]bool{}
	for _, m := range msgs {
		if m.Role == provider.RoleAssistant && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				// Write tools detection: any session that has used a write tool
				// is likely doing real development work.
				if tc.Name == "edit_file" || tc.Name == "write_file" ||
					tc.Name == "multi_edit" || tc.Name == "delete_range" ||
					tc.Name == "delete_symbol" || tc.Name == "bash" {
					f.HasWrittenFiles = true
				}
				// Sub-agent detection: task tool spawns child agents.
				if tc.Name == "task" {
					f.HasUsedSubAgent = true
				}
				// File counting (for FilesModified signal).
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
