package evidence

import "fmt"

// IncompleteTodos returns the items of a todo list that are not completed.
// (Design adopted from DeepSeek-Reasonix-V1.12)
func IncompleteTodos(todos []TodoItem) []TodoStepMatch {
	incomplete := make([]TodoStepMatch, 0)
	for j, t := range todos {
		status := todoStatus(t.Status)
		// blocked 项等待外部依赖（用户实测/重启服务），不算"未完成"——
		// 否则 taskGate 会反复注入"请继续执行"造成空转。
		if status == "completed" || status == "blocked" {
			continue
		}
		incomplete = append(incomplete, TodoStepMatch{
			Found:      true,
			Index:      j + 1,
			Content:    t.Content,
			Status:     status,
			ActiveForm: t.ActiveForm,
		})
	}
	return incomplete
}

// MatchStep resolves a complete_step.step (number, title, or drift-tolerant
// variant) against a todo list, returning the matched item.
// (Design adopted from DeepSeek-Reasonix-V1.12)
func MatchStep(step string, todos []TodoItem) (TodoStepMatch, bool) {
	m := matchTodoStep(step, todos)
	return m, m.Found
}

// ── V10.99 蒸馏自 Reasonix v1.17.21 — todo 状态机守卫 ──

// ValidateSerialTodos enforces the task-list state machine:
//   - At most one in_progress item at a time
//   - All status values must be valid (pending|in_progress|completed)
//   - A level-1 sub-step must have a level-0 phase header above it
//
// Design adopted from DeepSeek-Reasonix v1.17.21, relaxed for tianxuan's
// gate-at-final-answer design (ordering enforced by finalReadinessCheck).
func ValidateSerialTodos(todos []TodoItem) error {
	ipSeen := false
	for i, todo := range todos {
		switch todoStatus(todo.Status) {
		case "completed", "pending", "blocked":
		case "in_progress":
			if ipSeen {
				return fmt.Errorf("todo %d %q is a second in_progress item; serial task lists allow exactly one current item", i+1, todo.Content)
			}
			ipSeen = true
		default:
			return fmt.Errorf("todo %d %q has invalid status %q", i+1, todo.Content, todo.Status)
		}
	}
	if len(todos) > 0 && todos[0].Level == 1 {
		return fmt.Errorf("todo 1 %q is a level-1 sub-step with no phase above it; add a level-0 phase header or use level 0", todos[0].Content)
	}
	return nil
}
