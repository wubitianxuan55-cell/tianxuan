package agent

import (
	"bytes"
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

	// V10.13: 成功循环检测 — 移植自 Reasonix repeatedSuccessBlock。
	// 写工具在同一用户轮次中重复成功 ≥2 次即阻止，防止模型无意义循环。
	if out, blocked := a.repeatedSuccessBlock(call, t); blocked {
		return toolOutcome{
			output:  out,
			blocked: true,
			errMsg:  "blocked by loop guard",
		}
	}

	// Centralised pre-execution checks via the ToolDispatcher (production path).
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
	if a.sessionSaver != nil {
		cctx = memory.WithSessionSaver(cctx, a.sessionSaver)
	}
	if a.promoter != nil {
		cctx = memory.WithPromoter(cctx, a.promoter)
	}
	var result string
	var err error
	start := time.Now()
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
		detail := strings.TrimSpace(result)
		// V10.13: 参数非法 JSON 时附带工具 schema，帮助模型一次修正。
		// 移植自 Reasonix malformed-args schema echo。
		if !json.Valid([]byte(call.Arguments)) {
			detail = strings.TrimRight(detail, "\n") + "\nThe arguments were not valid JSON. Re-emit them exactly per this schema:\n" + string(t.Schema())
		}
		env := tool.WrapError(tool.CodeExecError, firstLine(err.Error()), map[string]any{"tool": call.Name, "detail": detail})
		body, truncMsg := truncateToolOutput(env)
		return toolOutcome{output: body, errMsg: firstLine(err.Error()), recoverable: recoverable, truncated: truncMsg != "", truncMsg: truncMsg}
	}
	// V10.13: 记录成功签名用于循环检测
	a.recordRepeatSuccess(call, t)
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

// ── V10.13: 成功循环检测 — 移植自 Reasonix ──────────────────────────

// repeatSuccessAllowed 是同一写工具签名允许成功的最大次数。
// 2 次给模型自我修正的空间；第 3 次通常是空转/写循环，应阻止。
const repeatSuccessAllowed = 2

// repeatedSuccessBlock 检测写工具是否在同轮中重复成功过多次。
// 命中时返回阻止消息，防止模型无意义循环消耗 token。
func (a *AgentRunner) repeatedSuccessBlock(call provider.ToolCall, t tool.Tool) (string, bool) {
	sig, ok := repeatSuccessSignature(call, t)
	if !ok || a.repeatSuccessCounts == nil {
		return "", false
	}
	count := a.repeatSuccessCounts[sig]
	if count < repeatSuccessAllowed {
		return "", false
	}
	return fmt.Sprintf(
		"blocked: [loop guard] %q has already succeeded %d times with the same write-like arguments in this user turn. Re-running it is unlikely to help and may burn tokens or repeat file writes. Change approach: use edit_file or multi_edit for file changes, verify with a read/test command, or explain the blocker in your final answer.",
		call.Name, count), true
}

// recordRepeatSuccess 记录一次成功的写工具调用，用于循环检测。
func (a *AgentRunner) recordRepeatSuccess(call provider.ToolCall, t tool.Tool) {
	sig, ok := repeatSuccessSignature(call, t)
	if !ok {
		return
	}
	if a.repeatSuccessCounts == nil {
		a.repeatSuccessCounts = make(map[string]int)
	}
	a.repeatSuccessCounts[sig]++
}

// repeatSuccessSignature 为写工具调用计算可比较的签名。
// 只读工具不参与（不会修改文件状态）；仅对写文件工具和写入型 bash 签名。
func repeatSuccessSignature(call provider.ToolCall, t tool.Tool) (string, bool) {
	if t.ReadOnly() {
		return "", false
	}
	switch call.Name {
	case "write_file", "edit_file", "multi_edit", "delete_range", "delete_symbol":
		return call.Name + "\x00" + canonicalToolArgs(call.Arguments), true
	case "bash":
		var p struct {
			Command         string `json:"command"`
			RunInBackground bool   `json:"run_in_background"`
		}
		if err := json.Unmarshal([]byte(call.Arguments), &p); err != nil {
			return "", false
		}
		if p.RunInBackground || !isShellFileWriteCommand(p.Command) {
			return "", false
		}
		return "bash\x00" + normalizeShellCommand(p.Command), true
	default:
		return "", false
	}
}

// canonicalToolArgs 将 JSON 参数规范化为紧凑可比较形式。
func canonicalToolArgs(raw string) string {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return strings.TrimSpace(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, b); err != nil {
		return string(b)
	}
	return compact.String()
}

// normalizeShellCommand 规范化 shell 命令（合并空白）。
func normalizeShellCommand(command string) string {
	return strings.Join(strings.Fields(command), " ")
}

// isShellFileWriteCommand 判断 shell 命令是否会写入文件。
func isShellFileWriteCommand(command string) bool {
	lower := strings.ToLower(command)
	switch {
	case shellPythonOpenWrites(lower):
		return true
	case strings.Contains(lower, "set-content") || strings.Contains(lower, "add-content") || strings.Contains(lower, "out-file"):
		return true
	case strings.Contains(lower, "sed -i") || strings.Contains(lower, "perl -pi"):
		return true
	case hasShellWriteRedirect(command):
		return true
	default:
		return false
	}
}

// shellPythonOpenWrites 检测 Python open() 调用是否以写模式打开文件。
func shellPythonOpenWrites(lower string) bool {
	if !strings.Contains(lower, "open(") {
		return false
	}
	if strings.Contains(lower, ".write(") {
		return true
	}
	for _, marker := range []string{", 'w", `, "w`, ", 'a", `, "a`, ", 'x", `, "x`, "mode='w", `mode="w`, "mode='a", `mode="a`, "mode='x", `mode="x`} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// hasShellWriteRedirect 检测 shell 命令是否包含写重定向（> 非 2>）。
func hasShellWriteRedirect(command string) bool {
	var quote rune
	var prev rune
	for _, r := range command {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			prev = r
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			prev = r
			continue
		}
		if r == '>' {
			if prev == '2' {
				prev = r
				continue
			}
			return true
		}
		prev = r
	}
	return false
}
