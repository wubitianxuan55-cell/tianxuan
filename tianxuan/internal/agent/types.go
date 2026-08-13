package agent

import (
	"context"
	"encoding/json"
	"tianxuan/internal/event"
)

type Asker interface {
	Ask(ctx context.Context, questions []event.AskQuestion) ([]event.AskAnswer, error)
}

// callContextKey carries the executing tool call's identity into Execute.
type callContextKey struct{}

// callContext is the per-call context a tool can read. parentID is the call being
// executed and sink is the agent's event sink (the `task` tool uses both to nest
// a sub-agent's events under this call); asker lets the `ask` tool reach the user.
type callContext struct {
	parentID string
	sink     event.Sink
	asker    Asker
}

// withCallContext stamps ctx with the executing call's ID, the agent's sink, and
// the asker. executeOne sets this before every Execute; `task` reads it (via
// CallContext) to nest sub-agent events, and `ask` reads the asker to prompt.
func withCallContext(ctx context.Context, parentID string, sink event.Sink, asker Asker) context.Context {
	return context.WithValue(ctx, callContextKey{}, callContext{parentID: parentID, sink: sink, asker: asker})
}

// CallContext returns the executing call's ID, the agent's sink, and the asker,
// if the context was set by an agent's executeOne. ok is false for a plain
// context (headless tool tests, calls made outside the run loop).
func CallContext(ctx context.Context) (parentID string, sink event.Sink, asker Asker, ok bool) {
	cc, ok := ctx.Value(callContextKey{}).(callContext)
	if !ok {
		return "", nil, nil, false
	}
	return cc.parentID, cc.sink, cc.asker, true
}

// StepResult records the outcome of a single complete_step call during a turn.
type StepResult struct {
	Step   string // step name from complete_step args
	Status string // "success", "error", "blocked"
	Result string // result field from complete_step args (truncated)
}

// TurnResult is a structured result produced by an AgentRunner after one turn.
// It lets upstream callers (e.g. PlannerHost) consume execution outcomes without
// having to extract them post-hoc from the agent's session.
type TurnResult struct {
	Plan          string       // the plan that was executed (empty for direct turns)
	FilesCreated  []string     // paths of files newly created this turn (vs. modified)
	FilesModified []string     // paths of files written/edited/moved/deleted this turn
	Summary       string       // agent's final conclusion (last assistant message)
	Success       bool         // true = no tool errors encountered this turn
	Errors        []string     // tool error messages collected during execution (max 5)
	StepResults   []StepResult // per-step outcomes from complete_step calls
}

// Runner carries out one task turn. AgentRunner satisfies it.
// Returns a structured TurnResult even on error so callers can inspect partial results.
type Runner interface {
	Run(ctx context.Context, input string) (*TurnResult, error)
}

// Gate decides, per tool call, whether it may run. The agent consults it at
// execute time (after the plan-mode gate). It is interface-shaped so the agent
// stays independent of the permission package and of how "ask" is resolved
// (silently in headless runs, interactively in the chat TUI). A nil gate means
// no gating �� every call runs, preserving behaviour for callers that don't wire
// one in. reason is fed back to the model when allow is false; a non-nil err
// (e.g. ctx cancelled awaiting approval) is treated as a block for that call.
type Gate interface {
	Check(ctx context.Context, toolName string, args json.RawMessage, readOnly bool) (allow bool, reason string, err error)
}

// ToolHooks fires user-configured shell hooks around each tool call. PreToolUse
// runs before the call and may block it (block=true; message is the reason fed
// back to the model); PostToolUse runs after and only surfaces output to the
// user (it can't block). It is interface-shaped so the agent stays independent
// of the hook package �� a nil hooks field disables hook firing entirely.
type ToolHooks interface {
	PermissionRequest(ctx context.Context, name string, args json.RawMessage) (allow bool, modifiedArgs json.RawMessage, reason string)
	PreToolUse(ctx context.Context, name string, args json.RawMessage) (block bool, message string)
	PostToolUse(ctx context.Context, name string, args json.RawMessage, result string)
	// PostLLMCall fires after each model turn completes (streaming finishes)
	// but before reasoning_content is stored. It returns the (possibly
	// translated) reasoning string �� the original when no hook is configured.
	// HasPostLLMCall reports whether such a hook exists, so the agent keeps
	// streaming reasoning live when none is wired up.
	PostLLMCall(ctx context.Context, reasoning string, turn int) string
	HasPostLLMCall() bool
	// SubagentStop fires when a `task` sub-agent finishes (foreground). PreCompact
	// fires just before a compaction pass and returns extra summary guidance (its
	// hooks' stdout) to fold into the summary prompt; "" when no hook contributes.
	SubagentStop(ctx context.Context, last string)
	PreCompact(ctx context.Context, trigger string) string
}

// AgentRunner drives a single task: a Provider, a tool Registry, and a Session
// wired into the main loop. In ModeDirect it runs the model directly; in
// ModePlanner it delegates classification and planning to a Planner before
// handing off to the executor (itself).
// KeepPolicy is a bitmask controlling which messages are preserved verbatim
// during compaction. Zero means no special retention — only digest summaries
// and small user turns are kept.
type KeepPolicy int

const (
	// KeepErrors preserves tool results that start with "error:" or "blocked:"
	// so critical failure information (build errors, test failures) is never
	// summarized away — the model needs those details to fix the problem.
	KeepErrors KeepPolicy = 1 << iota
	// KeepUserMarked preserves user messages prefixed with [[keep]], [keep],
	// <keep>, or <!-- keep --> markers, letting the user pin facts that must
	// survive compaction.
	KeepUserMarked
	// KeepProtected preserves tool results from protected-tool list (e.g.
	// read_skill, memory_search) whose outputs are foundational context that
	// must survive compaction. Pattern borrowed from opencode.
	KeepProtected
)
