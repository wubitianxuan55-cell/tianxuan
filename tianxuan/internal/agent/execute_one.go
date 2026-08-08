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
	"tianxuan/internal/planmode"
	"tianxuan/internal/provider"
	"tianxuan/internal/secrets"
	"tianxuan/internal/tool"
)

func (a *AgentRunner) executeOne(ctx context.Context, call provider.ToolCall) (out toolOutcome) {
	t, ok := a.tools.Get(call.Name)
	if !ok {
		return toolOutcome{
			output: tool.WrapError(tool.CodeUnknownTool, fmt.Sprintf("unknown tool %q", call.Name), nil),
			errMsg: fmt.Sprintf("unknown tool %q", call.Name),
		}
	}

	// V10.167: record every dispatch (success / error / blocked) as one JSONL
	// trace line so offline error-rate analysis sees the full picture, not
	// just failures. The named return lets one defer cover all early-return
	// paths (loop guard, failure guard, plan gate, permission, precheck…).
	start := time.Now()
	defer func() {
		if a.toolTrace == nil {
			return
		}
		outcome := "success"
		if out.errMsg != "" {
			if out.blocked {
				outcome = "blocked"
			} else {
				outcome = "error"
			}
		}
		a.toolTrace.Record(tool.TraceEntry{
			Ts:         time.Now().Format(time.RFC3339),
			SessionID:  a.sessionID,
			TraceID:    TraceID(ctx),
			CallID:     call.ID,
			Tool:       call.Name,
			ReadOnly:   t.ReadOnly(),
			Args:       call.Arguments,
			Outcome:    outcome,
			Error:      out.errMsg,
			OutputLen:  len(out.output),
			DurationMs: time.Since(start).Milliseconds(),
		})
	}()

	// V10.13: 成功循环检测 — 移植自 Reasonix repeatedSuccessBlock。
	// 写工具在同一用户轮次中重复成功 ≥2 次即阻止，防止模型无意义循环。
	if out, blocked := a.repeatedSuccessBlock(call, t); blocked {
		return toolOutcome{
			output:  out,
			blocked: true,
			errMsg:  "blocked by loop guard",
		}
	}

	// V10.148: Auto Failure Guard — host-side failure escalation. Denied
	// calls are blocked before any execution; the block message is fed back
	// as the tool result so the model sees why and switches approach.
	if a.failureGuard != nil {
		if d := a.failureGuard.Check(call.Name, json.RawMessage(call.Arguments), !t.ReadOnly()); d != GuardAllow {
			msg := autoGuardBlockMessage(d)
			return toolOutcome{
				output:  msg,
				blocked: true,
				errMsg:  msg,
			}
		}
	}

	// Plan-mode gate: refuse non-read-only tools while planning.
	// Ported from DeepSeek-Reasonix planmode.Policy.
	if a.planModeGate.Load() {
		safety := planmode.PlanSafetyUnknown
		if pc, ok2 := t.(tool.PlanModeClassifier); ok2 {
			if pc.PlanModeSafe() {
				safety = planmode.PlanSafetySafe
			}
		}
		untrusted := false
		if u, ok2 := t.(tool.PlanModeUntrustedReadOnly); ok2 {
			untrusted = u.PlanModeUntrustedReadOnly()
		}
		if blocked, msg := a.planModeBlocked(call.Name, t.ReadOnly(), untrusted, safety, json.RawMessage(call.Arguments)); blocked {
			return toolOutcome{
				output:  msg,
				blocked: true,
				errMsg:  msg,
			}
		}
	}

	// Centralised pre-execution checks via the ToolDispatcher (production path).
	// When dispatcher is nil (test/benchmark paths), gate/hooks are
	// checked inline — preserving backward compatibility with existing tests.
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
	// V10.154: codex-style schema validation for built-in tools. Missing
	// required / type mismatch / enum violation fail loudly BEFORE execution
	// (the model fixes the args next turn instead of getting a downstream
	// "path is required" style secondary error); schema-unknown fields stay
	// compatible (deliberate aliases) and are reported alongside real errors.
	if _, isBuiltin := tool.LookupBuiltin(call.Name); isBuiltin {
		unknown, verr := tool.ValidateArgs(t.Schema(), json.RawMessage(call.Arguments), builtinAliases(call.Name), builtinExtraFields(call.Name)...)
		if verr != nil {
			detail := verr.Error()
			if len(unknown) > 0 {
				detail += " (schema-unknown fields: " + strings.Join(unknown, ", ") + ")"
			}
			// V10.157: 附正确参数示例 + 跨工具误用提示，让模型一次修正，
			// 而不是反复撞同一个 validation_error（loop guard 是兜底，不是首选）。
			if ex := tool.ExampleFromSchema(t.Schema()); ex != "" {
				detail += "; expected args like: " + ex
			}
			var argsObj map[string]any
			if json.Unmarshal(json.RawMessage(call.Arguments), &argsObj) == nil {
				if hint := tool.MisuseHint(call.Name, argsObj); hint != "" {
					detail += "; " + hint
				}
			}
			msg := tool.WrapError(tool.CodeValidationError, detail, map[string]any{
				"tool":   call.Name,
				"schema": string(t.Schema()),
			})
			return toolOutcome{
				output:      msg,
				errMsg:      detail,
				recoverable: true,
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
	// V10.28: stale anchor 守卫 — 同一轮内已编辑的文件必须先 read_file 才能再编辑
	var staleWarn string
	if !t.ReadOnly() && isFileWriter(call.Name) {
		if path := extractFilePath(call.Name, call.Arguments); path != "" {
			a.staleMu.Lock()
			stale := a.staleWrittenFiles != nil && a.staleWrittenFiles[path] && (a.staleReadFiles == nil || !a.staleReadFiles[path])
			a.staleMu.Unlock()
			if stale {
				// V10.157: 软警告而非硬拦截——写工具自身的锚点匹配
				// （old_string/anchors 对照当前磁盘内容）才是真正的 stale 守卫；
				// 同一轮连续编辑同一文件的不同区域是合法操作，不应被强制 read_file
				// 往返。警告注入到该调用的结果前。
				staleWarn = fmt.Sprintf("note: [stale content] %q was already modified this turn; anchors must match the current file content (re-read with read_file if they don't).", path)
			}
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
			if cached, ok := a.tc.Get(ra.Path, ra.Offset); ok {
				return toolOutcome{output: cached}
			}
		}
	}

	cctx := withCallContext(ctx, call.ID, a.sink, a.asker)
	cctx = withTokensLeft(cctx, a.tokensLeft)
	cctx = withSearchTools(cctx, a.searchToolsProvider)
	if a.evidence != nil {
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
	execStart := time.Now()
	if ct, ok := t.(tool.ContextualTool); ok {
		tc := tool.ToolContext{
			SessionID:  a.sessionID,
			AgentName:  "agent",
			ToolCallID: call.ID,
			Messages:   a.session.Messages,
		}
		result, err = ct.ExecuteWithContext(cctx, tc, json.RawMessage(call.Arguments))
	} else {
		result, err = t.Execute(cctx, json.RawMessage(call.Arguments))
	}
	// Redact credential-like values from tool output before it enters model context.
	// Ported from DeepSeek-Reasonix.
	result = secrets.RedactToolOutput(result)
	duration := time.Since(execStart).Milliseconds()

	// Context offloading: if enabled and output exceeds threshold, save full
	// output to disk and replace with compact reference (path + preview).
	if staleWarn != "" {
		result = staleWarn + "\n" + result
	}
	if err == nil && a.offloadStore != nil {
		result = a.offloadStore.MaybeOffload(call.Name, result, a.offloadThresholdChars)
	}

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
					a.tc.Set(ra.Path, ra.Offset, result)
				}
			}
		case "edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol":
			var wa struct {
				Path string `json:"path"`
			}
			if json.Unmarshal(json.RawMessage(call.Arguments), &wa) == nil && wa.Path != "" {
				a.tc.InvalidatePath(wa.Path)
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
		return toolOutcome{
			output:       body,
			errMsg:       firstLine(err.Error()),
			recoverable:  recoverable,
			truncated:    truncMsg != "",
			truncMsg:     truncMsg,
			guardOutcome: a.guardObserve(call, !t.ReadOnly(), true),
		}
	}
	// V10.13: 记录成功签名用于循环检测
	a.recordRepeatSuccess(call, t)
	// V10.27: 追踪后台任务启停模式，用于检测 start-kill 循环
	switch call.Name {
	case "bash":
		var bp struct {
			RunInBackground bool `json:"run_in_background"`
		}
		if json.Unmarshal([]byte(call.Arguments), &bp) == nil && bp.RunInBackground {
			a.bgJobStartedThisTurn = true
		} else {
			// 前台 bash — 证明模型愿意等结果，重置循环计数
			a.bgStartKillStreak = 0
		}
	case "bash_output", "wait":
		a.bgOutputReadThisTurn = true
		a.bgStartKillStreak = 0 // 读取了输出 — 正常使用模式
	case "kill_shell":
		a.bgJobKilledThisTurn = true
	}
	// V10.28: 追踪 stale anchor — 记录本轮成功的读写操作
	if path := extractFilePath(call.Name, call.Arguments); path != "" {
		a.staleMu.Lock()
		if t.ReadOnly() && call.Name == "read_file" {
			if a.staleReadFiles == nil {
				a.staleReadFiles = make(map[string]bool)
			}
			a.staleReadFiles[path] = true
			if a.staleWrittenFiles != nil {
				delete(a.staleWrittenFiles, path) // 刷新后重置
			}
		} else if !t.ReadOnly() && isFileWriter(call.Name) {
			if a.staleWrittenFiles == nil {
				a.staleWrittenFiles = make(map[string]bool)
			}
			a.staleWrittenFiles[path] = true
		}
		a.staleMu.Unlock()
	}
	// A foreground `task` sub-agent just finished — its result is the final answer.
	if a.hooks != nil && call.Name == "task" && !isBackgroundTaskCall(call.Arguments) {
		a.hooks.SubagentStop(ctx, result)
	}
	result = SmartCompress(call.Name, result)
	env := tool.WrapResult(tool.CodeOK, map[string]any{"tool": call.Name, "result": result})
	body, truncMsg := truncateToolOutput(env)
	return toolOutcome{
		output:       body,
		truncated:    truncMsg != "",
		truncMsg:     truncMsg,
		guardOutcome: a.guardObserve(call, !t.ReadOnly(), isErrorResult(body)),
	}
}

// guardObserve records one executed tool call into the Auto Failure Guard.
// Blocked calls never reach here (they return before execution), so only
// genuine execution failures accumulate — matching Reasonix QualifyingFailure.
func (a *AgentRunner) guardObserve(call provider.ToolCall, mutates, failed bool) GuardOutcome {
	if a.failureGuard == nil {
		return GuardNone
	}
	return a.failureGuard.Observe(call.Name, json.RawMessage(call.Arguments), mutates, failed)
}

// builtinAliases maps deliberate aliases that built-in tools accept to their
// canonical schema property (locked by args_alias_test.go). An alias satisfies
// the canonical field's required check and is validated against its schema.
func builtinAliases(name string) map[string]string {
	switch name {
	case "read_file":
		return map[string]string{"file": "path"}
	case "wait":
		return map[string]string{"job_id": "job_ids", "timeout_ms": "timeout_seconds"}
	default:
		return nil
	}
}

// builtinExtraFields lists schema-less tolerated fields for built-in tools
// (e.g. edit_lines' old_string). They are exempt from the unknown-field report
// but don't satisfy any required check.
func builtinExtraFields(name string) []string {
	switch name {
	case "edit_lines":
		return []string{"old_string"}
	default:
		return nil
	}
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
// 值已迁至 loop_limits.go → RepeatSuccessAllowed。

// repeatedSuccessBlock 检测写工具是否在同轮中重复成功过多次。
// 命中时返回阻止消息，防止模型无意义循环消耗 token。
func (a *AgentRunner) repeatedSuccessBlock(call provider.ToolCall, t tool.Tool) (string, bool) {
	sig, ok := repeatSuccessSignature(call, t)
	if !ok {
		return "", false
	}
	a.repeatMu.Lock()
	if a.repeatSuccessCounts == nil {
		a.repeatMu.Unlock()
		return "", false
	}
	count := a.repeatSuccessCounts[sig]
	a.repeatMu.Unlock()
	if count < RepeatSuccessAllowed {
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
	a.repeatMu.Lock()
	if a.repeatSuccessCounts == nil {
		a.repeatSuccessCounts = make(map[string]int)
	}
	a.repeatSuccessCounts[sig]++
	a.repeatMu.Unlock()
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

// isFileWriter reports whether the tool name targets a specific file for writing.
func isFileWriter(name string) bool {
	switch name {
	case "edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol":
		return true
	}
	return false
}
