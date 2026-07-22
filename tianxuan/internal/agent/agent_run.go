package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"tianxuan/internal/event"
	"tianxuan/internal/evidence"
	"tianxuan/internal/provider"
)

func (a *AgentRunner) Run(ctx context.Context, input string) (*TurnResult, error) {
	return a.runDirect(ctx, input)
}

// runDirect is the original single-model execution path.
func (a *AgentRunner) runDirect(ctx context.Context, input string) (*TurnResult, error) {
	// generate trace ID for this turn
	traceID := NewTraceID()
	ctx = WithTraceID(ctx, traceID)
	defer func() { a.steerMu.Lock(); a.steerQueue = nil; a.steerConsumed = false; a.steerMu.Unlock() }() // drain any remaining steer on turn exit

	if a.evidence != nil {
		a.evidence.Reset()
	}
	a.sink.Emit(event.Event{Kind: event.TurnStarted})
	// wrap user input with transient language preference blocks
	// (Design adopted from DeepSeek-Reasonix-V1.12)
	// V10.46: planner skips language wrappers — its output is a plan, not user text.
	if !a.plannerMode {
		input = a.withTurnPreferences(input)
	}
	// V10.49: Hermes pre-injects the original user input (origInput) before
	// the formatHandoff call, so the handoff never enters the session as a
	// user-visible message. Both messages reach the model; History() skips
	// the handoff text by prefix. See app_session.go History().
	a.session.Add(provider.Message{Role: provider.RoleUser, Content: input})

	// rebuild canonical todo state from session history
	// (Design adopted from DeepSeek-Reasonix-V1.12)
	// V10.46: planner doesn't use todo_write.
	if !a.plannerMode {
		a.rebuildTodoState(a.session.Messages)
	}

	// P0-1: reset tool filter from previous turn (prefix must be immutable).
	a.activeSchemasMu.Lock()
	a.activeSchemas = nil
	a.activeSchemasMu.Unlock()

	// reset pre-execution cache and tool result cache for new turn
	a.preMu.Lock()
	a.preOutcomes = make(map[string]toolOutcome)
	a.dedupHashes = nil            // P0-2: reset dedup hashes each turn
	a.bgJobStartedThisTurn = false // 每轮重置启停标志
	a.bgOutputReadThisTurn = false
	a.bgJobKilledThisTurn = false
	a.bgStartKillStreak = 0   // 新用户轮次重置循环计数
	a.staleWrittenFiles = nil // 每轮重置 stale anchor 追踪
	a.staleReadFiles = nil
	a.pendingDiffs = nil
	a.preMu.Unlock()
	a.repeatSuccessCounts = nil // 每轮重置成功循环计数
	a.toolFeedbackCount = 0     // V10.89: 每轮重置工具反馈计数
	// per-turn TurnResult tracking — accumulated here and returned by Run().
	var turnFilesCreated []string
	var turnFilesModified []string
	var turnToolErrors []string
	var turnToolErrorsTruncated int // V10.87: count of errors beyond the 5 we keep
	var turnLastSummary string
	var turnStepResults []StepResult
	if a.paramStorm != nil {
		a.paramStorm.Reset()
	}
	// the clear() method resets mtime caches auto-expired entries

	// recall-reminder lets the model know mid-turn context remains
	// V10.46: planner doesn't need recall reminders.
	if !a.plannerMode {
		a.maybeRecallReminder()
	}

	graceRound := false
	// stream recovery + empty final detection counters
	streamRecoveries := 0
	emptyFinalBlocks := 0
	finalReadinessBlocks := 0
	for step := 0; a.maxSteps <= 0 || step < a.maxSteps || graceRound; step++ {
		// consume a queued mid-turn steer as session guidance
		// (Design adopted from DeepSeek-Reasonix-V1.12)
		// V10.46: planner doesn't accept mid-turn steers.
		if !a.plannerMode {
			if text, ok := a.consumeSteer(); ok {
				a.session.Add(provider.Message{Role: provider.RoleUser, Content: midTurnSteerMessage(text)})
				a.sink.Emit(event.Event{Kind: event.Steer, Text: text})
			}
		}
		text, reasoning, signature, calls, usage, interrupted, err := a.stream(ctx, step+1)
		if err != nil {
			// stream recovery — save partial output and inject recovery prompt
			if interrupted && streamRecoveries < MaxStreamRecoveries {
				streamRecoveries++
				if strings.TrimSpace(text) != "" {
					a.session.Add(provider.Message{
						Role:               provider.RoleAssistant,
						Content:            text,
						ReasoningContent:   reasoning,
						ReasoningSignature: signature,
					})
				}
				a.session.Add(provider.Message{
					Role:    provider.RoleUser,
					Content: streamRecoveryMessage(strings.TrimSpace(text) != ""),
				})
				a.sink.Emit(event.Event{Kind: event.Retrying, RetryAttempt: streamRecoveries, RetryMax: MaxStreamRecoveries})
				step-- // recovery retries do not consume the tool-round budget
				continue
			}
			a.preWG.Wait() // drain any in-flight pre-execution goroutines before returning
			return buildTurnResult(turnFilesCreated, turnFilesModified, turnToolErrors, turnLastSummary, turnStepResults), err
		}
		streamRecoveries = 0

		// length-truncation — inject nudge when finish_reason="length" and no tool calls
		if a.maybeContinueOutputLength(usage, calls) {
			continue
		}
		// invalid output — handle empty reasoning/text after retry
		if a.maybeRetryInvalidOutput(text, reasoning, calls) {
			continue
		}

		if usage != nil && usage.TotalTokens > 0 {
			a.sink.Emit(event.Event{Kind: event.Usage, Usage: usage, Pricing: a.pricing,
				SessionHit: int(a.sessCacheHit.Load()), SessionMiss: int(a.sessCacheMiss.Load()),
				UsageSource: event.UsageSourceExecutor})
			// budget gate — cumulative fee check
			if a.budgetGate != nil {
				status := a.budgetGate.Check(a.pricing, usage)
				if status == BudgetWarn {
					a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
						Text: a.budgetGate.StatusMessage()})
				}
				if status == BudgetBlock {
					a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
						Text: a.budgetGate.StatusMessage()})
					a.preWG.Wait()
					return buildTurnResult(turnFilesCreated, turnFilesModified, turnToolErrors, turnLastSummary, turnStepResults), fmt.Errorf("budget exceeded: %s", a.budgetGate.StatusMessage())
				}
			}
		}
		// Phase 3: compute cache-shape fingerprint for TCCA diagnostics
		if a.prefixFingerprintSet {
			shape := a.CaptureShape()
			a.lastPrefixShape = shape
		}

		if msg, ok := finishReasonMessage(usage); ok {
			a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: msg})
		}

		// automatic compaction — truncates history when prompt
		// exceeds the high-water mark. legacyTruncate preserves
		// L1+L2+prefix+summary+tail for maximum cache continuity.
		// Skip during grace round: compaction rewrites the message
		// array (LogRewriteVersion++), zeroing the prefix cache
		// right before the turn ends — pure waste.
		if !graceRound {
			a.maybeCompact(ctx, usage)
		}

		// Keep reasoning_content on the assistant turn for display and session
		// archive. It is NOT re-uploaded to the API: the openai provider drops it
		// when building the request, since re-sent reasoning is billable prompt
		// input for no cache or coherence gain.
		a.session.Add(provider.Message{
			Role:               provider.RoleAssistant,
			Content:            text,
			ReasoningContent:   reasoning,
			ReasoningSignature: signature,
			ToolCalls:          calls,
		})
		// capture last assistant text for TurnResult
		turnLastSummary = text

		// archive the assistant turn for cross-session analysis
		if a.archive != nil {
			tcJSON := "[]"
			if len(calls) > 0 {
				if b, err := json.Marshal(calls); err == nil {
					tcJSON = string(b)
				}
			}
			a.archive.RecordMessage(a.sessionID, string(provider.RoleAssistant), text, tcJSON, step+1)
		}

		if len(calls) == 0 {
			// finish-gate — prevent premature model stop
			// Grace Round — model produced summary, done.
			if graceRound {
				// clean up grace-round nudge from session before exit
				if len(a.session.Messages) > 0 {
					a.session.Messages = a.session.Messages[:len(a.session.Messages)-1]
				}
				return buildTurnResult(turnFilesCreated, turnFilesModified, turnToolErrors, turnLastSummary, turnStepResults), nil
			}

			// empty final detection — model returned no tool calls
			// and no visible text. Inject retry prompt; fail after 3 blocks.
			if strings.TrimSpace(text) == "" {
				emptyFinalBlocks++
				if emptyFinalBlocks >= MaxEmptyFinalBlocks {
					return buildTurnResult(turnFilesCreated, turnFilesModified, turnToolErrors, turnLastSummary, turnStepResults), fmt.Errorf("model finished without a visible final answer %d times", emptyFinalBlocks)
				}
				a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
					Text: fmt.Sprintf("empty final answer blocked: retrying (%d/%d)", emptyFinalBlocks, MaxEmptyFinalBlocks)})
				a.session.Add(provider.Message{Role: provider.RoleUser, Content: emptyFinalRetryMessage()})
				a.maybeCompact(ctx, usage)
				continue
			}

			// ── Stop gates (solo mode) ───────────────────────────────────
			// Triple gate: taskGate → goalGate → verifyGate.
			// All three fire only in solo mode (!plannerMode); plannerMode
			// (Hermes planner) skips them because Hermes handles task tracking
			// and verification via its own plan/confirm/verify loop.
			// V10.87: taskGate and goalGate restored for solo (single-model) runs.
			// Both check plannerMode internally, so no external guard needed.
			if a.taskGate() {
				continue
			}
			if a.goalGate() {
				continue
			}
			if !a.plannerMode && a.verifyGate() {
				continue
			}

			// final-answer readiness gate — verify evidence before accepting completion
			if !a.plannerMode {
				if blocked, reason := a.finalReadinessCheck(); blocked {
					finalReadinessBlocks++
					if finalReadinessBlocks >= MaxFinalReadinessBlocks {
						return buildTurnResult(turnFilesCreated, turnFilesModified, turnToolErrors, turnLastSummary, turnStepResults), fmt.Errorf("final-answer readiness failed %d times: %s", finalReadinessBlocks, reason)
					}
					a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
						Text: "final-answer readiness blocked: " + reason})
					a.session.Add(provider.Message{Role: provider.RoleUser, Content: finalReadinessRetryMessage(reason)})
					a.maybeCompact(ctx, usage)
					continue
				}
			}
			if a.steerQueueLen() > 0 {
				continue // more steers pending — another pass
			}
			return buildTurnResult(turnFilesCreated, turnFilesModified, turnToolErrors, turnLastSummary, turnStepResults), nil // all gates passed
		}

		// wait for stream() pre-execution goroutines to finish before
		// dispatching the full batch — avoids races and double-execution.
		emptyFinalBlocks = 0 // reset empty-final counter when model calls tools successfully
		// Grace Round guard — if model still calls tools during grace round, exit.
		// Ported from Reasonix to prevent infinite loops under MaxSteps limit.
		if graceRound {
			a.preWG.Wait() // drain pre-exec goroutines started during grace streaming
			// clean up grace-round nudge to prevent leaking to next user turn
			if len(a.session.Messages) > 0 {
				a.session.Messages = a.session.Messages[:len(a.session.Messages)-1]
			}
			return buildTurnResult(turnFilesCreated, turnFilesModified, turnToolErrors, turnLastSummary, turnStepResults), fmt.Errorf("paused after %d tool-call rounds (agent.max_steps) — the model continued calling tools during the grace round; the work so far is saved. Send another message to continue, or increase max_steps", a.maxSteps)
		}
		a.preWG.Wait()
		results := a.executeBatch(ctx, calls)
		// P0-2: deterministic pruning — skip duplicate tool results.
		// only dedup ReadOnly tools — bash/git_commit etc. may produce
		// different results on repeated calls (state changed between calls).
		for i, call := range calls {
			// track writer tool paths for TurnResult
			switch call.Name {
			case "write_file":
				if p := extractFilePath(call.Name, call.Arguments); p != "" {
					turnFilesCreated = append(turnFilesCreated, p)
				}
			case "edit_file", "move_file", "delete_range", "delete_symbol":
				if p := extractFilePath(call.Name, call.Arguments); p != "" {
					turnFilesModified = append(turnFilesModified, p)
				}
			}
			// collect tool errors for TurnResult (max 5, with truncation notice)
			if isErrorResult(results[i]) {
				turnToolErrorsTruncated++
				if len(turnToolErrors) < 5 {
					turnToolErrors = append(turnToolErrors, results[i])
				}
			}
			// Skip suppressed calls (already have placeholder result).
			if strings.HasPrefix(results[i], "suppressed:") {
				a.session.Add(provider.Message{
					Role:       provider.RoleTool,
					Content:    results[i],
					ToolCallID: call.ID,
					Name:       call.Name,
				})
				continue
			}
			// Only dedup read-only tools — writers may legitimately change state
			dedupOK := false
			if t, ok := a.tools.Get(call.Name); ok {
				dedupOK = t.ReadOnly()
			}
			if dedupOK {
				dk := call.Name + "|" + truncateStr(call.Arguments, 64) + "|" + truncateStr(results[i], 64)
				if a.dedupHashes == nil {
					a.dedupHashes = make(map[string]bool)
				}
				if a.dedupHashes[dk] {
					results[i] = "[cached — same as previous " + call.Name + " call]"
				} else {
					a.dedupHashes[dk] = true
				}
			}
			a.session.Add(provider.Message{
				Role:       provider.RoleTool,
				Content:    results[i],
				ToolCallID: call.ID,
				Name:       call.Name,
			})
		}
		// surface truncation notice when more than 5 tool errors occurred
		if turnToolErrorsTruncated > 5 && len(turnToolErrors) == 5 {
			turnToolErrors[4] = fmt.Sprintf("%s（还有 %d 个额外错误被截断）", turnToolErrors[4], turnToolErrorsTruncated-5)
		}

		// advance canonical todo state for successful complete_step calls
		// Also sync todo state from successful todo_write calls that ran
		// in the current batch — rebuildTodoState at turn start can't see them.
		for i, call := range calls {
			if call.Name == "todo_write" && !strings.HasPrefix(results[i], "error:") && !strings.HasPrefix(results[i], "blocked:") {
				rec := evidence.ReceiptFromToolCall(call.Name, json.RawMessage(call.Arguments), true, false)
				if len(rec.Todos) > 0 {
					a.setTodoState(rec.Todos)
				}
			}
		}
		for i, call := range calls {
			if call.Name == "complete_step" && !strings.HasPrefix(results[i], "error:") && !strings.HasPrefix(results[i], "blocked:") {
				step := extractStepFromArgs(call.Arguments)
				if step != "" {
					a.advanceCanonicalTodo(step)
				}
				// V10.87: collect step results for TurnResult
				status := "success"
				if strings.HasPrefix(results[i], "error:") {
					status = "error"
				} else if strings.HasPrefix(results[i], "blocked:") {
					status = "blocked"
				}
				turnStepResults = append(turnStepResults, StepResult{
					Step:   step,
					Status: status,
					Result: extractStepResult(call.Arguments),
				})
			}
		}

		// V10.89: 工具失败的结构化反馈 — 2+ errors 时注入归纳性消息，
		// 帮助 LLM 理解失败模式（参考 Aider reflected_message）。
		if a.maybeInjectToolFeedback(calls, results) {
			a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
				Text: "部分工具执行失败，已注入错误分析反馈"})
		}

		// bg start-kill cycle — detect repeated background job start→kill
		// without reading output, inject corrective nudge after 3 cycles.
		// V10.46: planner never starts background jobs.
		if !a.plannerMode && a.checkBgStartKillCycle() {
			continue
		}

		// repeat-detection — inject nudge after 3 same-tool calls
		// V10.46: planner repeating tool calls is normal research behaviour.
		if !a.plannerMode && a.detectRepeatedSteps(calls) {
			continue // nudge injected, skip compaction and continue loop
		}

		// Grace Round — when maxSteps is reached, give one extra final turn.
		// V10.46: planner uses its own maxSteps; no grace round needed.
		if !a.plannerMode && a.maxSteps > 0 && step+1 >= a.maxSteps && !graceRound {
			graceRound = true
			nudge := "Do not call any more tools — your tool-call round limit (agent.max_steps) has been reached. Instead, synthesize a final answer from all the work already completed: summarize what was accomplished, what remains to be done, and any decisions the user should make."
			a.session.Add(provider.Message{
				Role:    provider.RoleUser,
				Content: nudge,
			})
			continue
		}

		// no mid-turn compaction — cache grows monotonically within each turn
	}
	// Only reached when a positive maxSteps guard is configured. The work so far
	// is already in the session, so the user can just send another message to pick
	// up where it left off.
	return buildTurnResult(turnFilesCreated, turnFilesModified, turnToolErrors, turnLastSummary, turnStepResults), fmt.Errorf("paused after %d tool-call rounds (agent.max_steps)", a.maxSteps)
}

// buildTurnResult assembles a TurnResult from per-turn tracking variables.
// Used by runDirect at every return point so callers get partial results
// even when the turn ends with an error.
func buildTurnResult(created []string, modified []string, errors []string, summary string, stepResults []StepResult) *TurnResult {
	return &TurnResult{
		FilesCreated:  uniqFiles(created),
		FilesModified: uniqFiles(modified),
		Summary:       summary,
		Success:       len(errors) == 0,
		Errors:        errors,
		StepResults:   stepResults,
	}
}

func uniqFiles(files []string) []string {
	seen := make(map[string]bool, len(files))
	uniq := make([]string, 0, len(files))
	for _, f := range files {
		if !seen[f] {
			seen[f] = true
			uniq = append(uniq, f)
		}
	}
	return uniq
}

// isErrorResult checks if a tool result indicates an error or blocked condition.
// Detects multiple error prefix formats.
func isErrorResult(result string) bool {
	return strings.HasPrefix(result, "error:") ||
		strings.HasPrefix(result, "Error:") ||
		strings.HasPrefix(result, "blocked:") ||
		strings.HasPrefix(result, "[error")
}
