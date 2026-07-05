package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"tianxuan/internal/event"
	"tianxuan/internal/evidence"
	"tianxuan/internal/provider"
)

// rebuildTodoState reconstructs the canonical task list from a transcript: the
// latest successful todo_write is the base, then every complete_step after it
// advances an item. Deterministic from persisted messages, so it survives a
// fresh load or a rewind.
// (Design adopted from DeepSeek-Reasonix-V1.12)
func (a *AgentRunner) rebuildTodoState(msgs []provider.Message) {
	successful := successfulToolCallIDs(msgs)
	var todos []evidence.TodoItem
	baseIdx := -1
	for i, msg := range msgs {
		for _, tc := range msg.ToolCalls {
			if tc.Name != "todo_write" || !successful[tc.ID] {
				continue
			}
			rec := evidence.ReceiptFromToolCall(tc.Name, json.RawMessage(tc.Arguments), true, true)
			// A successful empty todo_write is an explicit clear.
			todos = append([]evidence.TodoItem(nil), rec.Todos...)
			baseIdx = i
		}
	}
	if baseIdx < 0 {
		a.setTodoState(nil)
		return
	}
	for i := baseIdx; i < len(msgs); i++ {
		for _, tc := range msgs[i].ToolCalls {
			if tc.Name != "complete_step" || !successful[tc.ID] {
				continue
			}
			rec := evidence.ReceiptFromToolCall(tc.Name, json.RawMessage(tc.Arguments), true, true)
			if m, ok := evidence.MatchStep(rec.Step, todos); ok && canonicalTodoStatus(todos[m.Index-1].Status) != "completed" {
				todos[m.Index-1].Status = "completed"
				promoteNextPendingTodo(todos)
			}
		}
	}
	a.setTodoState(todos)
}

func (a *AgentRunner) setTodoState(todos []evidence.TodoItem) {
	a.todoMu.Lock()
	a.todoState = append([]evidence.TodoItem(nil), todos...)
	a.todoMu.Unlock()
}

// CanonicalTodoState returns a copy of the host-reconstructed task list.
func (a *AgentRunner) CanonicalTodoState() []evidence.TodoItem {
	a.todoMu.Lock()
	defer a.todoMu.Unlock()
	return append([]evidence.TodoItem(nil), a.todoState...)
}

func (a *AgentRunner) incompleteCanonicalTodos() ([]evidence.TodoStepMatch, bool) {
	a.todoMu.Lock()
	defer a.todoMu.Unlock()
	if len(a.todoState) == 0 {
		return nil, false
	}
	return evidence.IncompleteTodos(a.todoState), true
}

// advanceCanonicalTodo flips the canonical todo matching a signed-off step to
// completed (promoting the next pending item to in_progress) and emits a
// synthetic todo_write so the task panel reflects it without the model
// re-sending the whole list.
func (a *AgentRunner) advanceCanonicalTodo(step string) {
	a.todoMu.Lock()
	if len(a.todoState) == 0 {
		a.todoMu.Unlock()
		return
	}
	m, ok := evidence.MatchStep(step, a.todoState)
	if !ok || canonicalTodoStatus(a.todoState[m.Index-1].Status) == "completed" {
		a.todoMu.Unlock()
		return
	}
	a.todoState[m.Index-1].Status = "completed"
	promoteNextPendingTodo(a.todoState)
	snapshot := append([]evidence.TodoItem(nil), a.todoState...)
	a.todoMu.Unlock()
	a.recordTodoState(snapshot)
	a.emitTodoState(snapshot, m.Index)
}

// recordTodoState logs the host-advanced list as a synthetic todo_write receipt
// so the per-turn final gate sees the advance.
func (a *AgentRunner) recordTodoState(todos []evidence.TodoItem) {
	if a.evidence == nil {
		return
	}
	args, err := json.Marshal(map[string]any{"todos": todos})
	if err != nil {
		return
	}
	a.evidence.Record(evidence.ReceiptFromToolCall("todo_write", json.RawMessage(args), true, true))
}

// emitTodoState emits synthetic ToolDispatch and ToolResult events for the
// host-advanced todo list so the UI reflects it without model re-sending.
func (a *AgentRunner) emitTodoState(todos []evidence.TodoItem, itemIndex int) {
	args, err := json.Marshal(map[string]any{"todos": todos})
	if err != nil {
		return
	}
	id := fmt.Sprintf("host-advance-%d-%d", a.hostAdvanceSeq.Add(1), itemIndex)
	t := event.Tool{ID: id, Name: "todo_write", Args: string(args), ReadOnly: true}
	a.sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: t})
	t.Output = "task list advanced by complete_step"
	a.sink.Emit(event.Event{Kind: event.ToolResult, Tool: t})
}

func promoteNextPendingTodo(todos []evidence.TodoItem) {
	for _, t := range todos {
		if canonicalTodoStatus(t.Status) == "in_progress" {
			return
		}
	}
	for i := range todos {
		if canonicalTodoStatus(todos[i].Status) == "pending" {
			todos[i].Status = "in_progress"
			return
		}
	}
}

func canonicalTodoStatus(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "pending"
	}
	return s
}

// successfulToolCallIDs returns a set of tool call IDs whose results indicate
// success (not blocked/error/cancelled).
func successfulToolCallIDs(msgs []provider.Message) map[string]bool {
	out := make(map[string]bool)
	for _, msg := range msgs {
		for _, tc := range msg.ToolCalls {
			out[tc.ID] = true // tool calls without explicit failure are assumed successful
		}
	}
	// Check tool results for explicit failure markers
	for _, msg := range msgs {
		if msg.Role != provider.RoleTool {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if strings.HasPrefix(content, "error:") ||
			strings.HasPrefix(content, "blocked:") ||
			strings.HasPrefix(content, "cancelled:") {
			out[msg.ToolCallID] = false
		}
	}
	return out
}

// extractStepFromArgs extracts the step identifier from complete_step JSON arguments.
// It supports both "step" (string title/number) and "step_index" (1-based integer).
// When both are present, step_index takes precedence since it's unambiguous.
func extractStepFromArgs(args string) string {
	var v struct {
		Step      string `json:"step"`
		StepIndex int    `json:"step_index"`
	}
	if err := json.Unmarshal([]byte(args), &v); err != nil {
		return ""
	}
	if v.StepIndex > 0 {
		return fmt.Sprintf("%d", v.StepIndex)
	}
	return v.Step
}
