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

	if a.evidence != nil {
		a.evidence.Reset()
	}
	a.sink.Emit(event.Event{Kind: event.TurnStarted})
	a.session.Add(provider.Message{Role: provider.RoleUser, Content: input})

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

			for step := 0; a.maxSteps <= 0 || step < a.maxSteps; step++ {
		text, reasoning, signature, calls, usage, err := a.stream(ctx, step+1)
		if err != nil {
			a.preWG.Wait() // drain any in-flight pre-execution goroutines before returning
			return err
		}

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
			return nil // all gates passed
		}

		// V4.2: wait for stream() pre-execution goroutines to finish before
		// dispatching the full batch �� avoids races and double-execution.
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

		// V8.0 P0-3: mid-turn steer — detect error patterns and inject corrective hints.
		if a.shouldMidTurnSteer(calls, results) {
			continue // steer injected, skip compaction and continue loop
		}


		// V6.0 P2: �ظ������⡪������ 3 ����ͬ���ߵ���ʱע�� nudge
		if a.detectRepeatedSteps(calls) {
			continue // nudge injected, skip compaction and continue loop
		}

			// V7.1: no mid-turn compaction �� cache grows monotonically within each turn
	}
	// Only reached when a positive maxSteps guard is configured. The work so far
	// is already in the session, so the user can just send another message to pick
	// up where it left off.
	return fmt.Errorf("paused after %d tool-call rounds (agent.max_steps) �� the work so far is saved; send another message to continue, or set max_steps higher or to 0 for no limit", a.maxSteps)
}
