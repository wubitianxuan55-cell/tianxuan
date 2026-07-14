package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

func (a *AgentRunner) executeBatch(ctx context.Context, calls []provider.ToolCall) []string {
	// V7.7.1: repair always runs first — executeOne no longer repeats it.
	for i := range calls {
		t, ok := a.tools.Get(calls[i].Name)
		repairArguments(&calls[i].Arguments, ok && !t.ReadOnly())
	}

	// V5.13: param storm breaker — after repair, inspect for duplicate patterns.
	suppressed := make([]bool, len(calls))
	if a.paramStorm != nil {
		for i := range calls {
			c := &calls[i]
			t, ok := a.tools.Get(c.Name)
			readOnly := ok && t.ReadOnly()
			res := a.paramStorm.Inspect(c.Name, c.Arguments, readOnly)
			if res.Suppress {
				suppressed[i] = true
				a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
					Text: "param storm breaker: " + res.Reason})
			}
		}
	}

	// V7.7.1: repair already done above; executeOne skips it.

	for i, c := range calls {
		if suppressed[i] {
			continue // 跳过 ToolDispatch 事件
		}
		t, ok := a.tools.Get(c.Name)
		a.sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{
			ID:       c.ID,
			Name:     c.Name,
			Args:     c.Arguments,
			ReadOnly: ok && t.ReadOnly(),
		}})
	}

	results := make([]string, len(calls))
	outcomes := make([]toolOutcome, len(calls))
	// V5.13 fix: 预填充被抑制调用的结果
	for i := range calls {
		if suppressed[i] {
			results[i] = "suppressed: duplicate tool call (param storm breaker)"
			outcomes[i] = toolOutcome{output: results[i], errMsg: "suppressed by param storm breaker"}
		}
	}
	run := func(i int) {
		if suppressed[i] {
			return
		}
		// V4.2: skip calls already pre-executed during stream()
		a.preMu.Lock()
		pre, hasPre := a.preOutcomes[calls[i].ID]
		a.preMu.Unlock()
		if hasPre {
			outcomes[i] = pre
			results[i] = pre.output
			return
		}
		defer func() {
			if r := recover(); r != nil {
				results[i] = fmt.Sprintf("tool panic: %v", r)
				outcomes[i] = toolOutcome{output: results[i], errMsg: fmt.Sprintf("panic: %v", r)}
			}
		}()
		outcomes[i] = a.executeOne(ctx, calls[i])
		results[i] = outcomes[i].output

		// V7.4: learn from tool errors across sessions
		if a.patternExtractor != nil {
			if p := a.patternExtractor.Extract(calls[i].Name, results[i]); p != nil {
				a.patternExtractor.SaveStore()
			}
		}
	}

	for _, batch := range partitionToolCalls(a.tools, calls) {
		if batch.parallel && batch.end-batch.start > 1 {
			runParallel(batch.start, batch.end, run)
			// After a parallel batch, check if context was cancelled before
			// proceeding to next batch — stop button during a parallel batch
			// should prevent subsequent serial calls from running.
			select {
			case <-ctx.Done():
				goto drain
			default:
			}
			continue
		}
		for i := batch.start; i < batch.end; i++ {
			// Fast path: if context is already cancelled (stop button), skip
			// remaining tool calls in this batch instead of blocking on them.
			select {
			case <-ctx.Done():
				goto drain
			default:
			}
			run(i)
		}
	}
drain:

	for i, c := range calls {
		o := outcomes[i]
		t, ok := a.tools.Get(c.Name)
		a.sink.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{
			ID:          c.ID,
			Name:        c.Name,
			Args:        c.Arguments,
			Output:      o.output,
			Err:         o.errMsg,
			Recoverable: o.recoverable,
			ReadOnly:    ok && t.ReadOnly(),
			Truncated:   o.truncated,
		}})
		if o.truncated && o.truncMsg != "" {
			a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Text: o.truncMsg})
		}
	}
	// Phase 2 DSpark: 批次一致性检查 — 检测同一批次中读失败→写失败
	// 的因果链，提前阻止注定失败的操作或注入一致性警告。
	a.postBatchCoherenceCheck(calls, results)
	a.applyStormBreaker(calls, outcomes, results)
	return results
}

type toolCallBatch struct {
	start    int
	end      int
	parallel bool
}

// partitionToolCalls groups consecutive non-conflicting tools into parallel
// batches. Two tools conflict when they share the same conflict key (see
// getConflictKey). Tools with a global conflict key (prefix "!") always form
// their own single-call serial batch. Order is fully preserved: the partition
// is a contiguous sweep that never reorders calls.
//
// This unified algorithm replaces the earlier two-phase read/write split. The
// old split forced a serial barrier between every reader→writer and writer→reader
// transition even when the tools targeted different files. Now a reader and a
// writer targeting different files comfortably coexist in one parallel batch,
// reducing turn latency by 30–50% on typical multi-tool turns.
func partitionToolCalls(r *tool.Registry, calls []provider.ToolCall) []toolCallBatch {
	var batches []toolCallBatch
	for i := 0; i < len(calls); {
		key := getConflictKey(calls[i])
		hasGlobal := key == "" || key[0] == '!' // batch contains a globally-conflicting tool
		used := map[string]bool{key: true}
		start := i
		i++
		for i < len(calls) {
			k := getConflictKey(calls[i])
			if hasGlobal || k == "" || k[0] == '!' || used[k] {
				break
			}
			used[k] = true
			i++
		}
		batches = append(batches, toolCallBatch{
			start:    start,
			end:      i,
			parallel: i-start > 1 && !hasGlobal,
		})
	}
	return batches
}

// V6.0: getConflictKey returns a conflict key for a tool call.
// Two calls with the same key cannot run in parallel.
// Prefix ! marks global conflict keys (always serial).
func getConflictKey(call provider.ToolCall) string {
	switch call.Name {
	case "task", "explore", "research", "review", "security_review",
		"run_skill", "install_skill":
		return "!spawn"
	case "complete_step", "todo_write":
		return "!ledger"
	case "edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol":
		path := extractFilePath(call.Name, call.Arguments)
		if path != "" {
			return "file:" + path
		}
		return "!write"
	case "bash", "bash_output":
		return "!bash"
	case "read_file":
		path := extractFilePath(call.Name, call.Arguments)
		if path != "" {
			return "read:" + path
		}
		return ""
	default:
		return ""
	}
}

// V6.0: extractArgsPath extracts the target file path from tool call arguments.

func runParallel(start, end int, run func(int)) {
	const maxParallel = 8
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for i := start; i < end; i++ {
		i := i
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			run(i)
		}()
	}
	wg.Wait()
}

// stormBreakThreshold is how many times in a row the same tool may fail the same
// way before the loop stops echoing the raw error back and instead returns a
// directive to change approach. Two natural self-corrections are healthy; the
// third identical failure is a death-spiral �� the dominant case being a tool call
// whose arguments are truncated at the output-token ceiling, which the model then
// re-emits (re-worded but still over-long), truncating the same way again.
const stormBreakThreshold = 3

// applyStormBreaker detects a run of identically-failing turns and, past the
// threshold, rewrites the model-facing result (results[0]) into a directive to
// change approach. It keys on each call's (tool, error) �� not its args �� because a
// stuck model reworks the arguments cosmetically while failing identically (see
// the stormSig field doc). A turn is a fixation candidate only when every one of
// its calls errored and none was merely blocked by plan mode / permissions (those
// carry a clear, distinct message the model can already act on). Any success, any
// block, or a different batch shape is varied work, so it resets the counter. This
// covers both the single-call spiral and a repeated multi-call batch. The hard
// maxSteps guard remains the ultimate backstop; this just keeps the loop from
// burning that whole budget bouncing off the same failure.
func (a *AgentRunner) applyStormBreaker(calls []provider.ToolCall, outcomes []toolOutcome, results []string) {
	sig, ok := batchStormSignature(calls, outcomes)
	if !ok {
		a.storm.Sig, a.storm.Count = "", 0
		return
	}
	if sig != a.storm.Sig {
		a.storm.Sig, a.storm.Count = sig, 1
		return
	}
	a.storm.Count++
	if a.storm.Count < stormBreakThreshold {
		return
	}

	// Phase 4: analyze the failure pattern and inject corrective context
	// as a user-role message (turn tail) �� never touching the cache-stable prefix.
	det := NewDetector()
	if report := det.Analyze(a.storm.Sig, outcomes); report != nil {
		hintMsg := "[System note: " + report.Hint + "]"
		a.session.Add(provider.Message{Role: provider.RoleUser, Content: hintMsg})
	}

	subject := fmt.Sprintf("%q", calls[0].Name)
	short := calls[0].Name
	if len(calls) > 1 {
		subject = fmt.Sprintf("this batch of %d tool calls", len(calls))
		short = fmt.Sprintf("a batch of %d calls", len(calls))
	}
	guardMsg := fmt.Sprintf(
		"\n\n[loop guard] %s has now failed %d times in a row with the same error. Re-sending it �� even with the wording changed �� will not help: the calls keep failing the same way. Change approach: if an argument is being truncated, write less in one call and split the work into several smaller calls; otherwise fix the arguments, use a different tool, or explain the blocker in your final answer.",
		subject, a.storm.Count)
	for i := range results {
		results[i] = outcomes[i].output + guardMsg
	}
	a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: fmt.Sprintf(
		"loop guard: %s failed %d�� the same way �� nudging the model to change approach",
		short, a.storm.Count)})
}

// batchStormSignature returns a per-turn fixation signature �� each call's
// (name, error) in order �� and ok=true only when every call errored and none was
// merely blocked. ok=false (any success or block) means the turn made varied
// progress, so the caller resets the counter. Keying on the error rather than the
// args is deliberate: a stuck model reworks the arguments while failing the same
// way, so identical-args matching would miss the loop.
func batchStormSignature(calls []provider.ToolCall, outcomes []toolOutcome) (string, bool) {
	if len(calls) == 0 {
		return "", false
	}
	var sb strings.Builder
	for i := range calls {
		if outcomes[i].errMsg == "" || outcomes[i].blocked {
			return "", false
		}
		sb.WriteString(calls[i].Name)
		sb.WriteByte(0)
		sb.WriteString(outcomes[i].errMsg)
		sb.WriteByte(0)
	}
	return sb.String(), true
}

// toolOutcome is one tool call's result, split into the model-facing output and
// the display-facing notice bits. errMsg is the short failure reason (empty on
// success) �� a refused call, an unknown tool, or an execution error �� so a sink
// renders the result as failed ("? name <errMsg>" / a red card) instead of OK;
// blocked narrows that to a refusal (plan mode / permission). truncMsg is set
// (without the "�� " prefix) when the output was head+tailed.
type toolOutcome struct {
	output    string
	blocked   bool
	errMsg    string
	recoverable bool // agent can fix this on next turn (bad args, wrong file, etc.)
	truncated bool
	truncMsg  string
}

// executeOne runs a single tool call. It is pure with respect to the event sink
// �� the caller emits ToolDispatch/ToolResult �� so it is safe to invoke from
// parallel goroutines.
