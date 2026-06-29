package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"tianxuan/internal/evidence"
	"tianxuan/internal/jobs"
	"tianxuan/internal/memory"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

func (a *AgentRunner) executeOne(ctx context.Context, call provider.ToolCall) toolOutcome {
	t, ok := a.tools.Get(call.Name)
	if !ok {
		return toolOutcome{
			output: tool.WrapError(tool.CodeUnknownTool, fmt.Sprintf("unknown tool %q", call.Name), nil),
			errMsg: fmt.Sprintf("unknown tool %q", call.Name),
		}
	}

	// Centralised pre-execution checks via the ToolDispatcher (production path).
	// When dispatcher is nil (test/benchmark paths), gate/hooks/planMode are
	// checked inline — preserving backward compatibility with existing tests.
	if a.dispatcher != nil {
		cr := a.dispatcher.Check(ctx, call.Name, json.RawMessage(call.Arguments), t.ReadOnly())
		if !cr.Allowed {
			return toolOutcome{
				output:  cr.Reason,
				blocked: cr.Blocked,
				errMsg:  cr.Reason,
			}
		}
	} else {
		if a.hooks != nil {
			allow, modifiedArgs, reason := a.hooks.PermissionRequest(ctx, call.Name, json.RawMessage(call.Arguments))
			if !allow {
				return toolOutcome{
					output:  "blocked by PermissionRequest hook: " + reason,
					blocked: true,
					errMsg:  "blocked by PermissionRequest hook",
				}
			}
			if len(modifiedArgs) > 0 {
				call.Arguments = string(modifiedArgs)
			}
		}
		if a.planMode.Load() && !t.ReadOnly() {
			return toolOutcome{
				output:  fmt.Sprintf("blocked: %q is a writer tool — currently read-only. Keep exploring with read-only tools, then write your plan as your reply. The user will be asked to approve it before any changes are made.", call.Name),
				blocked: true,
				errMsg:  "blocked: read-only mode",
			}
		}
		if a.gate != nil {
			allow, reason, err := a.gate.Check(ctx, call.Name, json.RawMessage(call.Arguments), t.ReadOnly())
			if err != nil {
				return toolOutcome{
					output:  fmt.Sprintf("blocked: %s (%v)", reason, err),
					blocked: true,
					errMsg:  fmt.Sprintf("blocked: %v", err),
				}
			}
			if !allow {
				return toolOutcome{
					output:  "blocked: " + reason,
					blocked: true,
					errMsg:  "blocked by permission policy",
				}
			}
		}
		if a.hooks != nil {
			if block, msg := a.hooks.PreToolUse(ctx, call.Name, json.RawMessage(call.Arguments)); block {
				if msg == "" {
					msg = "blocked by a PreToolUse hook"
				}
				return toolOutcome{
					output:  "blocked: " + msg,
					blocked: true,
					errMsg:  "blocked by PreToolUse hook",
				}
			}
		}
	}
	// Phase 1 DSpark: 确定性预检查 — 在文件编辑工具实际执行前，
	// 验证 old_string / anchor 是否存在于目标文件中。
	// 预检查命中时返回诊断消息，阻止必然失败的操作，节省一整轮 API 调用。
	// 缓存安全: 纯运行时判断，返回内容作为本轮新 tool_result 追加在末尾。
	if msg := a.precheckTool(call.Name, json.RawMessage(call.Arguments)); msg != "" {
		return toolOutcome{
			output:  msg,
			blocked: true,
			errMsg:  msg,
		}
	}
	// Checkpoint the file this writer is about to change, so the turn can be
	// rewound. Fires after all gating (the edit is cleared to run) and only for
	// tools that can describe their change; a Preview error means the edit will
	// likely fail anyway, so we skip rather than snapshot a stale state.
	if a.onPreEdit != nil && !t.ReadOnly() {
		if pv, ok := t.(tool.Previewer); ok {
			if change, perr := pv.Preview(json.RawMessage(call.Arguments)); perr == nil {
				a.onPreEdit(change)
				a.pendingDiffs = append(a.pendingDiffs, change)
			}
		}
	}
	// V4.2: tool result cache — avoid redundant disk IO for repeat file reads
	if call.Name == "read_file" && a.tc != nil {
		var ra struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
		}
		if err := json.Unmarshal(json.RawMessage(call.Arguments), &ra); err == nil && ra.Path != "" {
			if cached, ok := a.tc.get(ra.Path, ra.Offset); ok {
				return toolOutcome{output: cached}
			}
		}
	}

	cctx := withCallContext(ctx, call.ID, a.sink, a.asker)
	if a.evidence != nil {
		// V10.8: 严格验证只在 Plan Mode 下启用
		a.evidence.SetStrictVerification(a.planMode.Load())
		cctx = evidence.WithLedger(cctx, a.evidence)
	}
	if a.jobs != nil {
		cctx = jobs.WithManager(cctx, a.jobs)
	}
	if a.memQueue != nil {
		cctx = memory.WithQueue(cctx, a.memQueue)
	}
	start := time.Now()
	// V10.12: if the tool implements ContextualTool, pass rich session context
	// alongside the standard context — borrowed from opencode's ToolContext pattern.
	var result string
	var err error
	if ct, ok := t.(tool.ContextualTool); ok {
		tc := tool.ToolContext{
			SessionID:  a.sessionID,
			AgentName:  string(a.agentMode),
			ToolCallID: call.ID,
			Messages:   a.session.Messages,
		}
		result, err = ct.ExecuteWithContext(cctx, tc, json.RawMessage(call.Arguments))
	} else {
		result, err = t.Execute(cctx, json.RawMessage(call.Arguments))
	}
	duration := time.Since(start).Milliseconds()

	// V4.2: cache successful file reads; invalidate writes
	if a.tc != nil {
		switch call.Name {
		case "read_file":
			if err == nil {
				var ra struct {
					Path   string `json:"path"`
					Offset int    `json:"offset"`
				}
				if json.Unmarshal(json.RawMessage(call.Arguments), &ra) == nil && ra.Path != "" {
					a.tc.set(ra.Path, ra.Offset, result)
				}
			}
		case "edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol":
			var wa struct{ Path string `json:"path"` }
			if json.Unmarshal(json.RawMessage(call.Arguments), &wa) == nil && wa.Path != "" {
				a.tc.invalidatePath(wa.Path)
			}
		}
	}

	// V3.2: audit trail — log every tool execution
	if a.auditFunc != nil {
		outcome := "success"
		errMsg := ""
		if err != nil {
			outcome = "error"
			errMsg = err.Error()
		}
		a.auditFunc(call.Name, "", t.ReadOnly(), outcome, errMsg, len(result), duration)
	}

	// V3.0: notify workspace observer of successful edits.
	if err == nil && !t.ReadOnly() && a.dispatcher != nil {
		if path := extractFilePath(call.Name, call.Arguments); path != "" {
			a.dispatcher.NotifyEdit(path)
		}
	}

	if a.evidence != nil {
		if call.Name == "complete_step" {
			if err == nil {
				a.evidence.Record(evidence.ReceiptFromToolCall(call.Name, json.RawMessage(call.Arguments), true, t.ReadOnly()))
			}
		} else {
			a.evidence.Record(evidence.ReceiptFromToolCall(call.Name, json.RawMessage(call.Arguments), err == nil, t.ReadOnly()))
		}
	}
	// PostToolUse hooks observe the result (they can't block); fired whether the
	// call succeeded or errored, since the tool did run.
	if a.hooks != nil {
		a.hooks.PostToolUse(ctx, call.Name, json.RawMessage(call.Arguments), result)
	}
	if err != nil {
		// Errors from tool execution are agent-recoverable (bad args, wrong file,
		// command failed) — the model can fix them on the next turn. Errors from
		// unknown-tool / blocked / panic are NOT recoverable.
		recoverable := true
		env := tool.WrapError(tool.CodeExecError, firstLine(err.Error()), map[string]any{"tool": call.Name, "detail": strings.TrimSpace(result)})
		body, truncMsg := truncateToolOutput(env)
		return toolOutcome{output: body, errMsg: firstLine(err.Error()), recoverable: recoverable, truncated: truncMsg != "", truncMsg: truncMsg}
	}
	// A foreground `task` sub-agent just finished — its result is the final answer.
	if a.hooks != nil && call.Name == "task" && !isBackgroundTaskCall(call.Arguments) {
		a.hooks.SubagentStop(ctx, result)
	}
	result = SmartCompress(call.Name, result)
	env := tool.WrapResult(tool.CodeOK, map[string]any{"tool": call.Name, "result": result})
	body, truncMsg := truncateToolOutput(env)
	return toolOutcome{output: body, truncated: truncMsg != "", truncMsg: truncMsg}
}

// isBackgroundTaskCall reports whether a `task` call set run_in_background.
func isBackgroundTaskCall(args string) bool {
	var p struct {
		RunInBackground bool `json:"run_in_background"`
	}
	_ = json.Unmarshal([]byte(args), &p)
	return p.RunInBackground
}

// toolReadOnly reports a tool's ReadOnly classification by name.
func (a *AgentRunner) toolReadOnly(name string) bool {
	t, ok := a.tools.Get(name)
	return ok && t.ReadOnly()
}
