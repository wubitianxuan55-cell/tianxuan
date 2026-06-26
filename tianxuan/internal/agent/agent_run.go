package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
)

func (a *AgentRunner) Run(ctx context.Context, input string) error {
	return a.runDirect(ctx, input)
}

// runDirect is the original single-model execution path.
func (a *AgentRunner) runDirect(ctx context.Context, input string) error {
	// V3.3: generate trace ID for this turn
	traceID := NewTraceID()
	ctx = WithTraceID(ctx, traceID)
	defer a.clearSteerQueue() // V10.0: drain any remaining steer on turn exit

	if a.evidence != nil {
		a.evidence.Reset()
	}
	a.sink.Emit(event.Event{Kind: event.TurnStarted})
	// V10.0: wrap user input with transient language preference blocks
	// (Design adopted from DeepSeek-Reasonix-V1.12)
	input = a.withTurnPreferences(input)
	a.session.Add(provider.Message{Role: provider.RoleUser, Content: input})

	// V10.0: rebuild canonical todo state from session history
	// (Design adopted from DeepSeek-Reasonix-V1.12)
	a.rebuildTodoState(a.session.Messages)

	// V8.0 P0-1: reset tool filter from previous turn (prefix must be immutable).
	a.activeSchemasMu.Lock()
	a.activeSchemas = nil
	a.activeSchemasMu.Unlock()



	// V7.5: �Ự��·���Զ�������һ�ξ���·�ɺ����
	if a.flashProv != nil {
		if !a.autoRouteLocked {
						// V7.4: history-aware routing (heuristic + learned)
			heuristic := AutoRouteProvider(input, a.prov, a.flashProv)
			useFlash := heuristic == a.flashProv
			if a.routeHistory != nil {
				useFlash = a.routeHistory.ShouldRouteToFlash(input, useFlash)
			}
			if useFlash && a.flashProv != nil {
				a.activeProv = a.flashProv
			} else {
				a.activeProv = a.prov
			}
			modelName := "pro"
			if a.activeProv == a.flashProv {
				modelName = "flash"
			}
			defer func() {
				if a.routeHistory != nil {
					a.routeHistory.Record(input, modelName, false)
				}
			}()
			a.autoRouteLocked = true
			a.autoRouteDecision = a.activeProv
		} else {
			a.activeProv = a.autoRouteDecision
		}
	} else {
		a.activeProv = a.prov
	}

	// V4.2: reset pre-execution cache and tool result cache for new turn
	a.preMu.Lock()
	a.preOutcomes = make(map[string]toolOutcome)
		a.dedupHashes = nil // V8.0 P0-2: reset dedup hashes each turn
		a.steerCount = 0 // V8.0 P0-3: reset steer counter each turn
	a.pendingDiffs = nil
	a.preMu.Unlock()
	// V5.13: ���ò������籩��·������ turn = ����ͼ��
	if a.paramStorm != nil {
		a.paramStorm.Reset()
	}
	// V5.8: ���� clear()�������� mtime У���Զ�ʧЧ������Ŀ

	// V6.0: �����ٻ����ѡ���������ʾģ�ͼ�����м���
			a.maybeRecallReminder()

			graceRound := false
		// V10.0: stream recovery + empty final detection counters
		streamRecoveries := 0
		const maxStreamRecoveries = 3
		emptyFinalBlocks := 0
		const maxEmptyFinalBlocks = 3
		finalReadinessBlocks := 0
		const maxFinalReadinessBlocks = 3
		for step := 0; a.maxSteps <= 0 || step < a.maxSteps || graceRound; step++ {
		// V10.0: consume a queued mid-turn steer as session guidance
		// (Design adopted from DeepSeek-Reasonix-V1.12)
		if text, ok := a.consumeSteer(); ok {
			a.session.Add(provider.Message{Role: provider.RoleUser, Content: midTurnSteerMessage(text)})
			a.sink.Emit(event.Event{Kind: event.Steer, Text: text})
		}
		text, reasoning, signature, calls, usage, interrupted, err := a.stream(ctx, step+1)
		if err != nil {
			// V10.0: stream recovery — save partial output and inject recovery prompt
			if interrupted && streamRecoveries < maxStreamRecoveries {
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
				a.sink.Emit(event.Event{Kind: event.Retrying, RetryAttempt: streamRecoveries, RetryMax: maxStreamRecoveries})
				step-- // recovery retries do not consume the tool-round budget
				continue
			}
			a.preWG.Wait() // drain any in-flight pre-execution goroutines before returning
			return err
		}
		streamRecoveries = 0

		// V6.0 P1: ������Ƚضϡ���finish_reason="length" ���޹��ߵ���ʱע�� nudge
		if a.maybeContinueOutputLength(usage, calls) {
			continue
		}
		// V6.0 P1: ��Ч������ԡ�����˼��/������غ�����
		if a.maybeRetryInvalidOutput(text, reasoning, calls) {
			continue
		}

		if usage != nil && usage.TotalTokens > 0 {
			a.sink.Emit(event.Event{Kind: event.Usage, Usage: usage, Pricing: a.pricing,
				SessionHit: int(a.sessCacheHit.Load()), SessionMiss: int(a.sessCacheMiss.Load())})
			// V5.15: Ԥ���ſء�������ۼƷ���
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
					return fmt.Errorf("budget exceeded: %s", a.budgetGate.StatusMessage())
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


		// V8.2.5: automatic compaction — truncates history when prompt
		// exceeds the high-water mark. legacyTruncate preserves
		// L1+L2+prefix+summary+tail for maximum cache continuity.
		a.maybeCompact(ctx, usage)

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

		// V7.0: archive the assistant turn for cross-session analysis
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
			// V7.0: ��բ��ֹͣ������ֹģ����ǰֹͣ
			// V10.0: Grace Round — model produced summary, done.
			if graceRound {
				return nil
			}

			// V10.0: empty final detection — model returned no tool calls
			// and no visible text. Inject retry prompt; fail after 3 blocks.
			if strings.TrimSpace(text) == "" {
				emptyFinalBlocks++
				if emptyFinalBlocks >= maxEmptyFinalBlocks {
					return fmt.Errorf("model finished without a visible final answer %d times", emptyFinalBlocks)
				}
				a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
					Text: fmt.Sprintf("empty final answer blocked: retrying (%d/%d)", emptyFinalBlocks, maxEmptyFinalBlocks)})
				a.session.Add(provider.Message{Role: provider.RoleUser, Content: emptyFinalRetryMessage()})
				a.maybeCompact(ctx, usage)
				continue
			}

			// Gate 1: task gate �� ���δ�������

			if a.taskGate() {
				continue
			}
			// Gate 2: goal gate �� ���� judge ģ����֤Ŀ��
			if a.goalGate() {
				continue
			}
			// Gate 3: verify gate �� orchestrate ģʽ��֤
			if a.verifyGate() {
				continue
			}
			// V10.0: final-answer readiness gate — verify evidence before accepting completion
			if blocked, reason := a.finalReadinessCheck(); blocked {
				finalReadinessBlocks++
				if finalReadinessBlocks >= maxFinalReadinessBlocks {
					return fmt.Errorf("final-answer readiness failed %d times: %s", finalReadinessBlocks, reason)
				}
				a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
					Text: "final-answer readiness blocked: " + reason})
				a.session.Add(provider.Message{Role: provider.RoleUser, Content: finalReadinessRetryMessage(reason)})
				a.maybeCompact(ctx, usage)
				continue
			}
			if a.steerQueueLen() > 0 {
				continue // V10.0: more steers pending — another pass
			}
			return nil // all gates passed
		}

		// V4.2: wait for stream() pre-execution goroutines to finish before
		// dispatching the full batch — avoids races and double-execution.
		emptyFinalBlocks = 0 // V10.0: reset empty-final counter when model calls tools successfully
		a.preWG.Wait()
		results := a.executeBatch(ctx, calls)
		// V8.0 P0-2: deterministic pruning — skip duplicate tool results.
		// Hash (toolName + args + result) to detect identical outcomes.
		for i, call := range calls {
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
			// Compute dedup key: toolName | first 64 chars of args | first 64 chars of result
			dk := call.Name + "|" + truncateStr(call.Arguments, 64) + "|" + truncateStr(results[i], 64)
			if a.dedupHashes == nil {
				a.dedupHashes = make(map[string]bool)
			}
			if a.dedupHashes[dk] {
				// Same tool + args + result already seen — skip full result.
				a.session.Add(provider.Message{
					Role:       provider.RoleTool,
					Content:    "[cached — same as previous " + call.Name + " call]",
					ToolCallID: call.ID,
					Name:       call.Name,
				})
			} else {
				a.dedupHashes[dk] = true
				a.session.Add(provider.Message{
					Role:       provider.RoleTool,
					Content:    results[i],
					ToolCallID: call.ID,
					Name:       call.Name,
				})
			}
		}

		// V10.0: advance canonical todo state for successful complete_step calls

		// V10.0: advance canonical todo state for successful complete_step calls
		for i, call := range calls {
			if call.Name == "complete_step" && !strings.HasPrefix(results[i], "error:") && !strings.HasPrefix(results[i], "blocked:") {
				step := extractStepFromArgs(call.Arguments)
				if step != "" {
					a.advanceCanonicalTodo(step)
				}
			}
		}

		// V8.0 P0-3: mid-turn steer — detect error patterns and inject corrective hints.
		if a.shouldMidTurnSteer(calls, results) {
			continue // steer injected, skip compaction and continue loop
		}


		// V6.0 P2: �ظ������⡪������ 3 ����ͬ���ߵ���ʱע�� nudge
		if a.detectRepeatedSteps(calls) {
			continue // nudge injected, skip compaction and continue loop
		}


		// V10.0: Grace Round — when maxSteps is reached, give one extra final turn.
		if a.maxSteps > 0 && step+1 >= a.maxSteps && !graceRound {
			graceRound = true
			nudge := "Do not call any more tools — your tool-call round limit (agent.max_steps) has been reached. Instead, synthesize a final answer from all the work already completed: summarize what was accomplished, what remains to be done, and any decisions the user should make."
			a.session.Add(provider.Message{
				Role:    provider.RoleUser,
				Content: nudge,
			})
			continue
		}

			// V7.1: no mid-turn compaction �� cache grows monotonically within each turn
	}
	// Only reached when a positive maxSteps guard is configured. The work so far
	// is already in the session, so the user can just send another message to pick
	// up where it left off.
	return fmt.Errorf("paused after %d tool-call rounds (agent.max_steps) �� the work so far is saved; send another message to continue, or set max_steps higher or to 0 for no limit", a.maxSteps)
}
