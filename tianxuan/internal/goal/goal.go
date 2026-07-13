// Package goal provides a finite-state machine for session-scoped goals
// with persistence support. Ported from DeepSeek-Reasonix.
package goal

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"tianxuan/internal/evidence"
)

const (
	GoalStatusRunning  = "running"
	GoalStatusComplete = "complete"
	GoalStatusBlocked  = "blocked"
	GoalStatusStopped  = "stopped"

	GoalResearchAuto = iota
	GoalResearchOn
	GoalResearchOff

	maxGoalAutoTurns = 50
	maxGoalIdleTurns = 2

	goalContinueTurn  = "Continue pursuing the active goal under its task contract. If it is complete, provide the concise final result and end with [goal:complete]. If progress genuinely requires user-only information, an irreversible or externally visible operation, or a changed scope, end with [goal:blocked:<short reason>]. Otherwise use sensible defaults, do the next useful work, and end with [goal:continue]."
	goalSelfCheckTurn  = "The agent signaled goal completion and all tasks are marked done. Before finalizing, perform a brief quality self-check:\n1. Verify any changed files compile or parse correctly\n2. Run the relevant tests if applicable\n3. Confirm the original request, output format, constraints, and success criteria are met\nIf everything checks out, signal [goal:complete]. If issues are found, fix them and signal [goal:complete] when done."
	goalCompleteNotice = "goal complete"
)

// Machine owns the active goal's finite-state machine and its persistence.
type Machine struct {
	mu            sync.Mutex
	goal          string
	status        string
	researchMode  int
	turns         int
	blocks        int
	block         string
	interceptMsg  string
	intercepts    int
	strict        bool
	selfCheckDone bool
	idleTurns     int

	// statePath is the persisted goal-state sidecar; empty disables persistence.
	statePath string
	// writeMu serialises goal-state disk writes.
	writeMu sync.Mutex
}

// State is the serializable form of a goal state.
type State struct {
	Goal    string              `json:"goal,omitempty"`
	Status  string              `json:"status,omitempty"`
	Turns   int                 `json:"turns,omitempty"`
	Blocks  int                 `json:"blocks,omitempty"`
	Block   string              `json:"block,omitempty"`
	Strict  bool                `json:"strict,omitempty"`
	Todos   []evidence.TodoItem `json:"todos,omitempty"`
}

// AdvanceInput carries everything the FSM needs for one continuation step.
type AdvanceInput struct {
	Status     string
	Reason     string
	ToolCalled bool
	Todos      []evidence.TodoItem
	Readiness  string
}

// AdvanceResult reports the FSM step's outcome.
type AdvanceResult struct {
	Notice string
	Cont   bool
}

// SetStatePath sets the persisted state sidecar path.
func (g *Machine) SetStatePath(path string) {
	g.mu.Lock()
	g.statePath = path
	g.mu.Unlock()
}

// GoalStatePath derives a session's persisted goal-state sidecar.
func GoalStatePath(sessionPath string) string {
	return filepath.Join(filepath.Dir(sessionPath), "."+filepath.Base(sessionPath)+".goal.json")
}

// Snapshot returns goal text and status for turn injection.
func (g *Machine) Snapshot() (goal, status string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.goal, g.status
}

// Active reports whether a goal is currently running.
func (g *Machine) Active() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return strings.TrimSpace(g.goal) != "" && g.status == GoalStatusRunning
}

// StatusForDisplay maps empty status to "stopped".
func (g *Machine) StatusForDisplay() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.status == "" {
		return GoalStatusStopped
	}
	return g.status
}

// Set installs a session-scoped goal. Returns state to persist.
func (g *Machine) Set(goalText string, todos []evidence.TodoItem) (path string, data []byte, ok bool) {
	goalText = strings.TrimSpace(goalText)
	g.mu.Lock()
	defer g.mu.Unlock()
	if goalText != "" && g.goal == goalText && g.status == GoalStatusRunning {
		return "", nil, false
	}
	g.turns, g.blocks, g.block = 0, 0, ""
	g.interceptMsg, g.intercepts = "", 0
	g.selfCheckDone, g.idleTurns, g.strict = false, 0, false
	if goalText == "" {
		g.goal, g.status = "", GoalStatusStopped
	} else {
		g.goal, g.status = goalText, GoalStatusRunning
	}
	return g.buildStateLocked(todos)
}

// SetStrict enables or disables strict goal mode.
func (g *Machine) SetStrict(strict bool, todos []evidence.TodoItem) (path string, data []byte, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.strict = strict
	return g.buildStateLocked(todos)
}

// Stop transitions a running goal to the given terminal status.
func (g *Machine) Stop(status string, todos []evidence.TodoItem) (path string, data []byte, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if strings.TrimSpace(g.goal) != "" && g.status == GoalStatusRunning {
		g.status = status
	}
	g.interceptMsg, g.intercepts = "", 0
	g.selfCheckDone, g.idleTurns = false, 0
	return g.buildStateLocked(todos)
}

// TakeIntercept consumes a pending continuation-turn override.
func (g *Machine) TakeIntercept() (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.interceptMsg == "" {
		return "", false
	}
	msg := g.interceptMsg
	g.interceptMsg = ""
	return msg, true
}

// Advance runs one continuation step of the goal FSM.
func (g *Machine) Advance(in AdvanceInput) AdvanceResult {
	g.mu.Lock()
	defer g.mu.Unlock()
	if strings.TrimSpace(g.goal) == "" || g.status != GoalStatusRunning {
		return AdvanceResult{Cont: false}
	}
	g.turns++
	var notice string
	switch in.Status {
	case GoalStatusComplete:
		if incomplete := formatIncompleteTodos(in.Todos, in.Readiness); len(incomplete) > 0 && (g.strict || g.intercepts == 0) {
			g.intercepts++
			g.interceptMsg = incomplete
			break
		}
		if g.strict && !g.selfCheckDone {
			g.selfCheckDone = true
			g.interceptMsg = goalSelfCheckTurn
			break
		}
		g.intercepts, g.selfCheckDone, g.idleTurns = 0, false, 0
		g.goal, g.status = "", GoalStatusComplete
		g.blocks, g.block, g.interceptMsg = 0, "", ""
		notice = goalCompleteNotice
	case GoalStatusBlocked:
		r := cleanGoalBlockReason(in.Reason)
		if r == "" {
			r = "blocked"
		}
		if sameGoalBlock(g.block, r) {
			g.blocks++
		} else {
			g.blocks, g.block = 1, r
		}
		if g.blocks >= 3 {
			g.status = GoalStatusBlocked
			notice = "goal blocked: " + r
		}
	default:
		g.blocks, g.block, g.intercepts, g.selfCheckDone, g.idleTurns = 0, "", 0, false, 0
	}
	if notice == "" && g.interceptMsg == "" {
		if in.ToolCalled {
			g.idleTurns = 0
		} else {
			g.idleTurns++
			if g.idleTurns >= maxGoalIdleTurns {
				g.idleTurns = 0
				g.interceptMsg = "No tool calls in recent turns. Either make progress with tools or signal [goal:blocked:<reason>]."
			}
		}
	}
	if notice == "" && g.turns >= maxGoalAutoTurns {
		g.status = GoalStatusBlocked
		g.block, g.intercepts, g.selfCheckDone, g.interceptMsg, g.idleTurns = "goal continuation limit reached", 0, false, "", 0
		notice = g.block
	}
	return AdvanceResult{Notice: notice, Cont: notice == ""}
}

// Text returns the current goal text.
func (g *Machine) Text() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.goal
}

// WriteState persists pre-marshaled goal-state bytes to disk.
func (g *Machine) WriteState(path string, data []byte) {
	if path == "" || data == nil {
		return
	}
	g.writeMu.Lock()
	defer g.writeMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		slog.Warn("goal: state dir", "err", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		slog.Warn("goal: write state", "err", err)
	}
}

// PersistWithTodos re-persists goal state with the given todos.
func (g *Machine) PersistWithTodos(todos []evidence.TodoItem) {
	g.mu.Lock()
	path, data, ok := g.buildStateLocked(todos)
	g.mu.Unlock()
	if ok {
		g.WriteState(path, data)
	}
}

// TerminalTodosFromState reads the persisted goal-state sidecar and returns its
// todo snapshot only after the goal has reached a terminal state.
func TerminalTodosFromState(sessionPath string) ([]evidence.TodoItem, bool) {
	if strings.TrimSpace(sessionPath) == "" {
		return nil, false
	}
	path := GoalStatePath(sessionPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("goal: read state", "err", err)
		}
		return nil, false
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		slog.Warn("goal: parse state", "err", err)
		return nil, false
	}
	switch state.Status {
	case GoalStatusComplete, GoalStatusBlocked, GoalStatusStopped:
	default:
		return nil, false
	}
	if len(state.Todos) == 0 {
		return nil, false
	}
	return append([]evidence.TodoItem(nil), state.Todos...), true
}

// RestoreRunningFromState reloads the active running goal from the persisted sidecar.
func (g *Machine) RestoreRunningFromState(sessionPath string) {
	if strings.TrimSpace(sessionPath) == "" {
		return
	}
	path := GoalStatePath(sessionPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("goal: read state", "err", err)
		}
		return
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		slog.Warn("goal: parse state", "err", err)
		return
	}
	if state.Status != GoalStatusRunning || strings.TrimSpace(state.Goal) == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.goal = strings.TrimSpace(state.Goal)
	g.status = GoalStatusRunning
	g.turns = state.Turns
	g.blocks = state.Blocks
	g.block = state.Block
	g.strict = state.Strict
	g.interceptMsg, g.intercepts = "", 0
	g.selfCheckDone, g.idleTurns = false, 0
}

// buildStateLocked marshals the current goal state for persistence.
func (g *Machine) buildStateLocked(todos []evidence.TodoItem) (path string, data []byte, ok bool) {
	if g.statePath == "" {
		g.statePath = GoalStatePath(filepath.Join(os.TempDir(), "goal.json"))
	}
	state := State{
		Goal:   g.goal,
		Status: g.status,
		Turns:  g.turns,
		Blocks: g.blocks,
		Block:  g.block,
		Strict: g.strict,
		Todos:  todos,
	}
	b, err := json.Marshal(state)
	if err != nil {
		slog.Warn("goal: marshal state", "err", err)
		return "", nil, false
	}
	return g.statePath, b, true
}

// --- parser & helpers ---

// ParseGoalStatusMarker extracts [goal:complete] / [goal:blocked:<reason>] / [goal:continue].
func ParseGoalStatusMarker(text string) (status, reason string, ok bool) {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch lower {
		case "[goal:complete]":
			return GoalStatusComplete, "", true
		case "[goal:continue]":
			return GoalStatusRunning, "", true
		}
		const blockedPrefix = "[goal:blocked:"
		if strings.HasPrefix(lower, blockedPrefix) && strings.HasSuffix(line, "]") {
			return GoalStatusBlocked, strings.TrimSpace(line[len(blockedPrefix) : len(line)-1]), true
		}
		return "", "", false
	}
	return "", "", false
}

func formatIncompleteTodos(todos []evidence.TodoItem, readiness string) string {
	var parts []string
	if len(todos) > 0 {
		if incomplete := evidence.IncompleteTodos(todos); len(incomplete) > 0 {
			var b strings.Builder
			b.WriteString("the following tasks are still incomplete:")
			for _, t := range incomplete {
				fmt.Fprintf(&b, "\n  - %s (%s)", t.Content, t.Status)
			}
			parts = append(parts, b.String())
		}
	}
	if readiness != "" {
		parts = append(parts, readiness)
	}
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Goal signaled complete but issues remain:\n")
	for _, p := range parts {
		b.WriteString("- ")
		b.WriteString(p)
		b.WriteString("\n")
	}
	b.WriteString("Fix or use todo_write/complete_step to mark done, then [goal:complete] again.")
	return b.String()
}

func sameGoalBlock(a, b string) bool {
	return normalizeGoalBlockReason(a) == normalizeGoalBlockReason(b)
}

func cleanGoalBlockReason(reason string) string {
	return strings.Trim(strings.TrimSpace(reason), " \t\r\n:：,，.。;；!！?？-—_[]()（）")
}

func normalizeGoalBlockReason(reason string) string {
	reason = strings.ToLower(cleanGoalBlockReason(reason))
	var b strings.Builder
	lastSpace := true
	for _, r := range reason {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// ShortGoalForNotice collapses whitespace and truncates for UI.
func ShortGoalForNotice(goalText string) string {
	goalText = strings.Join(strings.Fields(goalText), " ")
	runes := []rune(goalText)
	const max = 160
	if len(runes) <= max {
		return goalText
	}
	return string(runes[:max]) + "..."
}
