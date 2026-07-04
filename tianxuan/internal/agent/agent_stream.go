package agent

import (
	"context"
	"encoding/json"
	"strings"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
)

func (a *AgentRunner) stream(ctx context.Context, turn int) (string, string, string, []provider.ToolCall, *provider.Usage, bool, error) {
	// Build tools and messages. L4 conversation messages always come from
	// Session �� NOT from ctxMgr.AssemblePrompt() which reads FlowLayer.
	// FlowLayer is only updated during compact, so reading it on every turn
	// would send stale/empty messages to the model (information pollution).
	tools := a.tools.Schemas()
	a.activeSchemasMu.RLock()
	if a.activeSchemas != nil {
		tools = a.activeSchemas
	}
	a.activeSchemasMu.RUnlock()
	msgs := a.session.Messages

	// V5.10: ImmutablePrefix guard — capture prefix shape before API call,
	// compare against session baseline; emit Notice on drift (not panic).
	prevShape := a.verifyPrefixAndShape()

	ch, err := a.prov.Stream(ctx, provider.Request{
		Messages:    msgs,
		Tools:       tools,
		Temperature: a.temperature,
	})
	if err != nil {
		return "", "", "", nil, nil, false, err
	}

	// A PostLLMCall hook rewrites the whole reasoning block, so when one is wired
	// up we buffer reasoning silently and emit the transformed text once after the
	// stream. With no such hook the reasoning streams live, chunk by chunk, as
	// before �� the common case must not lose its live "thinking��" display.
	transformReasoning := a.hooks != nil && a.hooks.HasPostLLMCall()

	var text, reasoning strings.Builder
	var signature string // provider-issued proof for the reasoning (Anthropic thinking)
	var calls []provider.ToolCall
	var usage *provider.Usage
	batcher := newStreamBatcher(a.sink)
	for chunk := range ch {
		switch chunk.Type {
		case provider.ChunkReasoning:
			reasoning.WriteString(chunk.Text)
			if chunk.Signature != "" {
				signature = chunk.Signature
			}
			if chunk.Text != "" && !transformReasoning {
				batcher.addReasoning(chunk.Text)
			}
		case provider.ChunkText:
			text.WriteString(chunk.Text)
			batcher.addText(chunk.Text)
		case provider.ChunkToolCallStart:
			batcher.flushNow()
			// Surface the tool card as soon as the call begins �� before its
			// (possibly large) arguments finish streaming �� so the user sees it
			// working instead of a stall. executeBatch emits the full dispatch
			// (with args) once the call completes; the frontend merges by ID.
			if tc := chunk.ToolCall; tc != nil {
				a.sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{
					ID: tc.ID, Name: tc.Name, ReadOnly: a.toolReadOnly(tc.Name), Partial: true,
				}})
			}
		case provider.ChunkToolCall:
			calls = append(calls, *chunk.ToolCall)
			// V4.2: pre-execute read-only tools as each call completes streaming,
			// overlapping tool execution with the model's remaining output.
			// Skip complete_step/todo_write (ordering-dependent on prior calls)
			// and calls with empty IDs (lookup key collision).
			if tc := chunk.ToolCall; tc != nil && tc.ID != "" && a.toolReadOnly(tc.Name) &&
				tc.Name != "complete_step" && tc.Name != "todo_write" {
				a.preWG.Add(1)
				go func(call provider.ToolCall) {
					defer a.preWG.Done()
					o := a.executeOne(ctx, call)
					a.preMu.Lock()
					a.preOutcomes[call.ID] = o
					a.preMu.Unlock()
				}(*tc)
			}
		case provider.ChunkUsage:
			usage = chunk.Usage
			a.lastUsage.Store(chunk.Usage)
			a.sessCacheHit.Add(int64(chunk.Usage.CacheHitTokens)); chunk.Usage.SessionCacheHitTokens = int(a.sessCacheHit.Load())
			a.sessCacheMiss.Add(int64(chunk.Usage.CacheMissTokens)); chunk.Usage.SessionCacheMissTokens = int(a.sessCacheMiss.Load())
			batcher.flushNow()
			// Phase 3: CompareShape diagnostics — explain cache behaviour
			if chunk.Usage != nil {
				postShape := a.CaptureShape()
				diag := CompareShape(prevShape, postShape, chunk.Usage)
				if diag.PrefixChanged {
					a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
						Text: diag.Format()})
				}
				a.lastPrefixShape = postShape
			}
		case provider.ChunkError:
			batcher.flushNow()
			if provider.IsStreamInterrupted(chunk.Err) {
				return text.String(), reasoning.String(), signature, calls, usage, true, chunk.Err
			}
			return "", "", "", nil, nil, false, chunk.Err
		}
	}
	batcher.flushAll()
	// With a PostLLMCall hook, the live stream was suppressed above; transform the
	// full reasoning now and emit it once so the sink never sees the untranslated
	// text. Without a hook this is skipped �� the chunk-by-chunk events already fired.
	original := reasoning.String()
	display := original
	if transformReasoning && original != "" {
		display = a.hooks.PostLLMCall(ctx, original, turn)
		if display != "" {
			a.sink.Emit(event.Event{Kind: event.Reasoning, Text: display})
		}
	}
	// Store the transformed reasoning �� except when a provider signature pins it to
	// the original text (Anthropic extended thinking). That signed thinking block is
	// replayed verbatim on the next tool-call turn; re-uploading transformed text
	// under the original signature is rejected, so keep the original for storage
	// while the user still sees the transformed version live.
	stored := display
	if signature != "" {
		stored = original
	}
	// Close the text stream: a sink may re-render the streamed raw text as
	// styled markdown now that it is complete. Reasoning rides along so the sink
	// has the full chain if it wants it.
	if text.Len() > 0 || display != "" {
		a.sink.Emit(event.Event{Kind: event.Message, Text: text.String(), Reasoning: display})
	}
	return text.String(), stored, signature, calls, usage, false, nil
}

// repairArguments applies Tool-Call-Repair fixes (flatten wrappers, scavenge
// JSON strings, truncate oversized strings) to a tool call's arguments in-place.
// Called once per call in executeBatch; executeOne no longer repeats it.
func repairArguments(args *string, readOnly bool) {
	if *args == "" {
		return
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(*args), &raw); err != nil {
		return
	}
	opts := ToolArgumentRepairOptions{PreserveLongStrings: !readOnly}
	repaired := RepairDispatchToolArguments(raw, opts)
	if len(repaired.Notes) > 0 {
		if fixed, err := json.Marshal(repaired.Arguments); err == nil {
			*args = string(fixed)
		}
	}
}

// executeBatch runs a set of tool calls, parallelising read-only calls (all
// writers serialised). Repair runs first (once per call), then paramStorm
// detection, then execution. executeOne no longer repeats repair.
// ToolResult events are emitted after the batch in call order.
// nil usage. The sink renders the message; the "! " prefix is presentation.
func finishReasonMessage(u *provider.Usage) (string, bool) {
	if u == nil {
		return "", false
	}
	switch u.FinishReason {
	case "length":
		return "response truncated: hit max output tokens", true
	case "content_filter":
		return "response blocked by content filter", true
	case "repetition_truncation":
		return "response truncated: model repetition detected", true
	default:
		return "", false
	}
}
