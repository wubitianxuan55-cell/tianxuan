package agent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"tianxuan/internal/provider"
)

// CheckpointData is a deterministic snapshot of session state saved before
// compaction. Serialized as JSON — same input always produces the same bytes,
// so the prefix after a checkpoint rebuild stays cache-stable.
type CheckpointData struct {
	Summary       string           `json:"summary"`
	Todos         []checkpointTodo `json:"todos,omitempty"`
	EditFiles     []string         `json:"edit_files,omitempty"`
	Goal          string           `json:"goal,omitempty"`
	TruncateCount int              `json:"truncate_count"`
}

type checkpointTodo struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

// extractTodos scans session messages for the last todo_write call and
// returns a snapshot of the todo items.
func extractTodos(msgs []provider.Message) []checkpointTodo {
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
				Todos []checkpointTodo `json:"todos"`
			}
			if err := json.Unmarshal([]byte(tc.Arguments), &p); err != nil {
				continue
			}
			return p.Todos
		}
	}
	return nil
}

// extractEditFiles scans messages for file edit operations.
func extractEditFiles(msgs []provider.Message) []string {
	seen := make(map[string]bool)
	var files []string
	for _, msg := range msgs {
		for _, tc := range msg.ToolCalls {
			switch tc.Name {
			case "edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol":
				path := extractFilePath(tc.Name, tc.Arguments)
				if path != "" && !seen[path] {
					files = append(files, path)
					seen[path] = true
				}
			}
		}
	}
	return files
}

// WriteCheckpoint saves the current session state to dir/checkpoint.json.
// Called before compaction. Returns nil when dir is empty (disabled).
func (a *AgentRunner) WriteCheckpoint(dir string) error {
	if dir == "" {
		return nil
	}
	msgs := a.session.Messages
	keep := a.compaction.RecentKeep
	if keep < minRecentKeep {
		keep = minRecentKeep
	}
	keepFrom := len(msgs) - keep
	if keepFrom < 1 {
		keepFrom = 1
	}

	cp := CheckpointData{
		Summary:       BuildCompactSummary(msgs[1:keepFrom]),
		Todos:         extractTodos(msgs),
		EditFiles:     extractEditFiles(msgs[1:keepFrom]),
		TruncateCount: a.compaction.TruncateCount,
	}
	if cp.Summary == "" && len(cp.Todos) == 0 && len(cp.EditFiles) == 0 {
		return nil
	}

	data, err := json.Marshal(cp)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "checkpoint.json"), data, 0644)
}

// LoadCheckpoint reads the last checkpoint from dir. Returns nil when
// no checkpoint exists or dir is empty.
func LoadCheckpoint(dir string) *CheckpointData {
	if dir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "checkpoint.json"))
	if err != nil {
		return nil
	}
	var cp CheckpointData
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil
	}
	if cp.Summary == "" && len(cp.Todos) == 0 {
		return nil
	}
	return &cp
}

// formatForLLM renders the checkpoint as a structured user message for
// injection during rebuild. Deterministic output for a given CheckpointData.
func (cp *CheckpointData) formatForLLM() string {
	var parts []string
	parts = append(parts, "[Earlier conversation checkpoint snapshot:")

	if cp.Summary != "" {
		parts = append(parts, cp.Summary)
	}
	if len(cp.Todos) > 0 {
		parts = append(parts, "- Active todo list:")
		for _, t := range cp.Todos {
			icon := " "
			switch t.Status {
			case "completed":
				icon = "✓"
			case "in_progress":
				icon = "▶"
			case "pending":
				icon = "○"
			}
			parts = append(parts, "  "+icon+" "+t.Content)
		}
	}
	if cp.Goal != "" {
		parts = append(parts, "- Goal: "+cp.Goal)
	}
	if len(cp.EditFiles) > 0 {
		parts = append(parts, "- Files modified: "+joinStr(cp.EditFiles, ", "))
	}
	parts = append(parts, "]")
	return joinStr(parts, "\n")
}

// joinStr joins strings with a separator (stdlib strings.Join without import).
func joinStr(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for _, s := range ss[1:] {
		result += sep + s
	}
	return result
}
