package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tianxuan/internal/evidence"
	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(todoWrite{}) }

// todoWrite records the agent's running task list. It has no host side effects —
// the full list lives in the call's args (the model re-sends it whole on every
// update), which a frontend renders as a checklist. Execute just validates the
// shape and acks with a count, so the model gets a stable confirmation. The agent
// keeps one item in_progress at a time and flips each to completed as it finishes.
type todoWrite struct{}

type todoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm,omitempty"`
	Level      int    `json:"level,omitempty"`
}

func (todoWrite) Name() string { return "todo_write" }

func (todoWrite) Description() string {
	return "Record and update a structured task list for the current work. Send the COMPLETE list every call — it replaces the previous one. Use it to plan multi-step work and show progress: keep exactly one item in_progress at a time, and flip an item to completed the moment it's done (don't batch completions). Skip it for trivial single-step tasks. The list is two-level: a `level` 0 item is a PHASE (a milestone) and the `level` 1 items after it are its concrete sub-steps; omit `level` (0) for a flat list."
}

func (todoWrite) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "todos":{
    "type":"array",
    "description":"The complete task list, in order. Replaces any previous list.",
    "items":{
      "type":"object",
      "properties":{
        "content":{"type":"string","description":"Imperative description of the task."},
        "status":{"type":"string","enum":["pending","in_progress","completed"],"description":"Task state. Keep at most one in_progress."},
        "activeForm":{"type":"string","description":"Present-continuous form shown while the task is in progress (e.g. \"Running tests\")."},
        "level":{"type":"integer","enum":[0,1],"description":"Nesting level: 0 = phase/milestone, 1 = a sub-step of the phase above it. Omit for a flat list."}
      },
      "required":["content","status"]
    }
  }
},
"required":["todos"]
}`)
}

// ReadOnly is true: todo_write only records a list (no filesystem or process
// effect), so it never needs approval and stays available in plan mode — where
// laying out a plan as todos is exactly the point.
func (todoWrite) ReadOnly() bool { return true }
func (todoWrite) Kind() tool.ToolKind { return tool.KindOther }

func (todoWrite) CompactDescription() string { return compactDesc["todo_write"] }
func (todoWrite) CompactSchema() json.RawMessage   { return compactSchema["todo_write"] }

func (todoWrite) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Todos []todoItem `json:"todos"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	var done, active, pending int
	for i, t := range p.Todos {
		if t.Content == "" {
			return "", fmt.Errorf("todo %d: content is required", i+1)
		}
		if t.Level < 0 || t.Level > 1 {
			return "", fmt.Errorf("todo %d: invalid level %d (want 0 phase | 1 sub-step)", i+1, t.Level)
		}
		switch t.Status {
		case "completed":
			done++
		case "in_progress":
			active++
		case "pending", "":
			pending++
		default:
			return "", fmt.Errorf("todo %d: invalid status %q (want pending|in_progress|completed)", i+1, t.Status)
		}
	}
	if err := verifyTodoCompletionTransitions(ctx, p.Todos); err != nil {
		return "", err
	}

	// V10.6: 计划进度持久化 — 每次 todo_write 同步写入 .tianxuan/progress.md
	// compaction 后丢失 todo 状态时，系统提示会引导 agent 读取此文件恢复进度
	saveProgressMarkdown(p.Todos)

	return fmt.Sprintf("Todos updated: %d total — %d completed, %d in progress, %d pending.",
		len(p.Todos), done, active, pending), nil
}

// saveProgressMarkdown writes the current todo list to .tianxuan/progress.md
// in the project root (discovered by walking up from cwd). This survives
// compaction and lets the agent recover its plan after context resets.
func saveProgressMarkdown(todos []todoItem) {
	// Find project root by looking for .tianxuan/ directory
	dir, err := os.Getwd()
	if err != nil {
		return // can't save without knowing where we are
	}
	root := findTianxuanDir(dir)
	if root == "" {
		return // no .tianxuan/ found
	}

	path := filepath.Join(root, "progress.md")
	var b strings.Builder
	b.WriteString("# 任务进度\n\n")
	b.WriteString(fmt.Sprintf("> 最后更新: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	b.WriteString("| 状态 | 任务 |\n")
	b.WriteString("|------|------|\n")

	for _, t := range todos {
		icon := "⬜"
		switch t.Status {
		case "in_progress":
			icon = "🔄"
		case "completed":
			icon = "✅"
		}
		prefix := ""
		if t.Level == 1 {
			prefix = "  └─ "
		}
		b.WriteString(fmt.Sprintf("| %s | %s%s |\n", icon, prefix, t.Content))
	}

	_ = os.WriteFile(path, []byte(b.String()), 0o644)
}

// findTianxuanDir walks up from dir looking for a .tianxuan/ directory.
// Returns the path to the .tianxuan directory, or "" if not found.
func findTianxuanDir(dir string) string {
	for {
		candidate := filepath.Join(dir, ".tianxuan")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // reached root
		}
		dir = parent
	}
}

func verifyTodoCompletionTransitions(ctx context.Context, todos []todoItem) error {
	ledger, ok := evidence.FromContext(ctx)
	if !ok {
		return nil
	}
	// V10.8: 只在严格验证模式（Plan Mode）下强制 complete_step 验证
	// 普通模式下允许自由标记 todo 状态，由 complete_step 自行验证
	strictMode := false
	if ledger, ok := evidence.FromContext(ctx); ok {
		strictMode = ledger.StrictVerification()
	}
	if !strictMode {
		return nil
	}
	missing, hasBaseline := ledger.UnverifiedCompletedTodos(toEvidenceTodos(todos))
	if !hasBaseline || len(missing) == 0 {
		return nil
	}
	if len(missing) == 1 {
		m := missing[0]
		return fmt.Errorf("todo %d %q is newly completed but has no matching successful complete_step receipt in this turn", m.Index, m.Content)
	}
	return fmt.Errorf("%d todos are newly completed but have no matching successful complete_step receipts in this turn", len(missing))
}

func toEvidenceTodos(todos []todoItem) []evidence.TodoItem {
	out := make([]evidence.TodoItem, 0, len(todos))
	for _, t := range todos {
		out = append(out, evidence.TodoItem{
			Content:    t.Content,
			Status:     t.Status,
			ActiveForm: t.ActiveForm,
			Level:      t.Level,
		})
	}
	return out
}
