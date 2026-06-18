package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
)

// EditObserver is notified when a file-editing tool succeeds. Implementations
// (e.g. ContextManager) use it to track session workspace state.
type EditObserver interface {
	OnFileEdited(path string)
}

// ToolDispatcher centralizes the pre-execution checks (plan mode, permission
// gate, hooks) that every tool call must pass through. It sits between the
// agent's run loop and individual tool execution.
type ToolDispatcher struct {
	planMode *atomic.Bool
	gate     Gate
	hooks    ToolHooks
	observer EditObserver // V3.0: workspace state observer
}

// NewToolDispatcher creates a dispatcher.
func NewToolDispatcher(planMode *atomic.Bool, gate Gate, hooks ToolHooks) *ToolDispatcher {
	return &ToolDispatcher{
		planMode: planMode,
		gate:     gate,
		hooks:    hooks,
	}
}

// SetObserver installs a workspace state observer (V3.0).
func (d *ToolDispatcher) SetObserver(o EditObserver) {
	d.observer = o
}

// NotifyEdit informs the observer of a successful file edit.
func (d *ToolDispatcher) NotifyEdit(path string) {
	if d.observer != nil {
		d.observer.OnFileEdited(path)
	}
}

// CheckResult summarises the outcome of the pre-execution checks.
type CheckResult struct {
	Allowed bool
	Blocked bool
	Reason  string // human-readable block reason (empty when allowed)
}

// Check runs plan-mode, gate, and hook checks for a single tool call.
func (d *ToolDispatcher) Check(ctx context.Context, name string, args json.RawMessage, readOnly bool) CheckResult {
	// 0. PermissionRequest hooks — run before any gate, can modify args. V8.0 P2-12.
	if d.hooks != nil {
		allow, modifiedArgs, reason := d.hooks.PermissionRequest(ctx, name, args)
		if !allow {
			return CheckResult{
				Allowed: false,
				Blocked: true,
				Reason:  "blocked by PermissionRequest hook: " + reason,
			}
		}
		if len(modifiedArgs) > 0 {
			args = modifiedArgs
		}
	}

	// 1. Plan mode (read-only gate)
	if d.planMode != nil && d.planMode.Load() && !readOnly {
		return CheckResult{
			Allowed: false,
			Blocked: true,
			Reason:  fmt.Sprintf("blocked: %q is a writer tool and plan mode is read-only. Keep exploring with read-only tools, then write your plan as your reply — the user will be asked to approve it before any changes are made.", name),
		}
	}

	// 2. Permission gate
	if d.gate != nil {
		allow, reason, err := d.gate.Check(ctx, name, args, readOnly)
		if err != nil {
			return CheckResult{
				Allowed: false,
				Blocked: true,
				Reason:  fmt.Sprintf("blocked: %s (%v)", reason, err),
			}
		}
		if !allow {
			return CheckResult{
				Allowed: false,
				Blocked: true,
				Reason:  "blocked: " + reason,
			}
		}
	}

	// 3. PreToolUse hooks
	if d.hooks != nil {
		if block, msg := d.hooks.PreToolUse(ctx, name, args); block {
			if msg == "" {
				msg = "blocked by a PreToolUse hook"
			}
			return CheckResult{
				Allowed: false,
				Blocked: true,
				Reason:  "blocked: " + msg,
			}
		}
	}

	return CheckResult{Allowed: true}
}
