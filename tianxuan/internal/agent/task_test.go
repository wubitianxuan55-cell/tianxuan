package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// loopingProvider keeps requesting a tool call so an uncapped sub-agent never
// finishes — the regression fixture for the default max_steps cap.
type loopingProvider struct {
	name  string
	calls int32
}

func (l *loopingProvider) Name() string { return l.name }

func (l *loopingProvider) Stream(_ context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	n := atomic.AddInt32(&l.calls, 1)
	ch := make(chan provider.Chunk, 1)
	if n <= 40 {
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
			ID: fmt.Sprintf("call-%d", n), Name: "bash", Arguments: `{"command":"echo hi"}`,
		}}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "finally done"}
	}
	close(ch)
	return ch, nil
}

// TestTaskDefaultMaxStepsCapsRunawayLoop verifies a sub-agent that keeps
// requesting tools is capped at DefaultSubagentMaxSteps instead of running
// forever: an unlimited sub-agent (V10.53: 0 = unlimited) blocks the parent
// task tool indefinitely.
func TestTaskDefaultMaxStepsCapsRunawayLoop(t *testing.T) {
	sub := &loopingProvider{name: "loop"}
	parentReg := tool.NewRegistry()
	parentReg.Add(fakeTool{name: "bash", readOnly: false})
	task := NewTaskTool(sub, nil, parentReg, 20, 0, 0.0, "", "sys", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := task.Execute(ctx, []byte(`{"prompt":"keep looping"}`))
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("sub-agent hung until the test timeout instead of hitting the default cap")
	}
	if got := atomic.LoadInt32(&sub.calls); got > DefaultSubagentMaxSteps+2 {
		t.Errorf("sub-agent ran %d rounds, want capped near %d", got, DefaultSubagentMaxSteps)
	}
}

// TestParallelTasksDefaultMaxStepsCapsLoop mirrors the task cap for the
// parallel_tasks tool: each sub-task gets the same default step ceiling on top
// of its 120s timeout, so a looping sub-task cannot spin until the deadline.
func TestParallelTasksDefaultMaxStepsCapsLoop(t *testing.T) {
	sub := &loopingProvider{name: "loop"}
	parentReg := tool.NewRegistry()
	parentReg.Add(fakeTool{name: "bash", readOnly: false})
	pt := NewParallelTasksTool(sub, nil, parentReg, 0, 0, 0.0, "", "sys", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := pt.Execute(ctx, []byte(`{"tasks":[{"prompt":"loop"}]}`))
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("parallel sub-task hung until the test timeout instead of hitting the default cap")
	}
	if got := atomic.LoadInt32(&sub.calls); got > DefaultSubagentMaxSteps+2 {
		t.Errorf("parallel sub-task ran %d rounds, want capped near %d", got, DefaultSubagentMaxSteps)
	}
}

// TestTaskToolReturnsSubAgentFinalAnswer runs a task against a mock provider
// that emits a single text turn, and verifies the tool returns exactly that
// text — sub-agent intermediate state isn't supposed to leak.
func TestTaskToolReturnsSubAgentFinalAnswer(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "found 3 callers of Foo"},
		{Type: provider.ChunkDone},
	}}
	parentReg := tool.NewRegistry()
	task := NewTaskTool(sub, nil, parentReg, 20, 0, 0.0, "", "test-sys-prompt", nil)

	out, err := task.Execute(context.Background(), []byte(`{"prompt":"find callers of Foo"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "found 3 callers of Foo") {
		t.Errorf("got %q, want sub-agent final answer", out)
	}

	// The sub-agent must have received the prompt as its user message and
	// the configured system prompt at the top — proving the session was
	// fresh, not the parent's.
	if sys := sub.lastReq.Messages[0]; sys.Role != provider.RoleSystem || sys.Content != "test-sys-prompt" {
		t.Errorf("first message = %+v, want system 'test-sys-prompt'", sys)
	}
	if got := lastUser(sub.lastReq); got != "find callers of Foo" {
		t.Errorf("sub-agent user = %q, want the prompt verbatim", got)
	}
}

// TestTaskForkInheritsContext verifies inherit_context (Qwen /fork semantics)
// injects the parent conversation snapshot as the sub-agent's first user
// message, so the sub-agent starts with context instead of a blank slate while
// the task prompt stays the final user message.
func TestTaskForkInheritsContext(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "forked work done"},
		{Type: provider.ChunkDone},
	}}
	parentReg := tool.NewRegistry()
	parentReg.Add(fakeTool{name: "bash", readOnly: false})
	task := NewTaskTool(sub, nil, parentReg, 10, 0, 0.0, "", "sys", nil)
	task.SetForkContext(func() string { return "parent context: fixing the auth flow" })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := task.Execute(ctx, []byte(`{"prompt":"continue the work","inherit_context":true}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "forked work done") {
		t.Fatalf("unexpected output: %q", out)
	}
	found := false
	for _, m := range sub.lastReq.Messages {
		if strings.Contains(m.Content, forkContextHeader) && strings.Contains(m.Content, "fixing the auth flow") {
			found = true
		}
	}
	if !found {
		t.Fatalf("inherit_context did not inject the parent snapshot into the sub-agent request; last user message: %q", lastUser(sub.lastReq))
	}
	if got := lastUser(sub.lastReq); got != "continue the work" {
		t.Fatalf("task prompt should be the last user message, got %q", got)
	}
}

// TestTaskForkWithoutProviderFails verifies inherit_context without a wired
// parent context provider fails loudly instead of silently ignoring the fork.
func TestTaskForkWithoutProviderFails(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "x"},
		{Type: provider.ChunkDone},
	}}
	parentReg := tool.NewRegistry()
	parentReg.Add(fakeTool{name: "bash", readOnly: false})
	task := NewTaskTool(sub, nil, parentReg, 10, 0, 0.0, "", "sys", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := task.Execute(ctx, []byte(`{"prompt":"continue","inherit_context":true}`))
	if err == nil || !strings.Contains(err.Error(), "inherit_context") {
		t.Fatalf("expected inherit_context wiring error, got %v", err)
	}
}

// TestTaskToolFiltersTools verifies the whitelist behaviour: when the caller
// names a subset of tools, the sub-agent's registry contains exactly that set
// with subagent/skill meta-tools stripped to prevent recursive delegation.
func TestTaskToolFiltersTools(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "ok"},
		{Type: provider.ChunkDone},
	}}
	parentReg := tool.NewRegistry()
	parentReg.Add(fakeTool{name: "read_file", readOnly: true})
	parentReg.Add(fakeTool{name: "write_file", readOnly: false})
	parentReg.Add(fakeTool{name: "bash", readOnly: false})
	task := NewTaskTool(sub, nil, parentReg, 20, 0, 0.0, "", "sys", nil)
	parentReg.Add(task) // simulate the wiring in cli.setup
	parentReg.Add(fakeTool{name: "run_skill", readOnly: false})
	parentReg.Add(fakeTool{name: "research", readOnly: false})

	args := []byte(`{"prompt":"x","tools":["read_file","task","write_file","run_skill","research"]}`)
	if _, err := task.Execute(context.Background(), args); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// V6.0: 子代理 API 请求发送过滤后工具（排除 meta-tools），
	// 参数白名单 [read_file, task, write_file, run_skill, research]
	// 过滤 meta-tools 后 → [read_file, write_file]
	got := map[string]bool{}
	for _, s := range sub.lastReq.Tools {
		got[s.Name] = true
	}
	if !got["read_file"] || !got["write_file"] {
		t.Errorf("V6.0: API request tools = %v, want [read_file, write_file]", got)
	}
	if got["task"] || got["run_skill"] || got["research"] {
		t.Errorf("V6.0: meta-tools should be excluded, got %v", got)
	}
}

// TestTaskToolDefaultsToParentToolsWithoutMetaTools covers the no-whitelist
// path: the sub-agent inherits parent tools except subagent/skill meta-tools.
func TestTaskToolDefaultsToParentToolsWithoutMetaTools(t *testing.T) {
	sub := &mockProvider{name: "sub", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "ok"},
		{Type: provider.ChunkDone},
	}}
	parentReg := tool.NewRegistry()
	parentReg.Add(fakeTool{name: "read_file", readOnly: true})
	parentReg.Add(fakeTool{name: "grep", readOnly: true})
	task := NewTaskTool(sub, nil, parentReg, 20, 0, 0.0, "", "sys", nil)
	parentReg.Add(task)
	parentReg.Add(fakeTool{name: "run_skill", readOnly: false})
	parentReg.Add(fakeTool{name: "explore", readOnly: false})
	parentReg.Add(fakeTool{name: "research", readOnly: false})
	parentReg.Add(fakeTool{name: "review", readOnly: false})
	parentReg.Add(fakeTool{name: "security_review", readOnly: false})
	parentReg.Add(fakeTool{name: "remember", readOnly: false})

	if _, err := task.Execute(context.Background(), []byte(`{"prompt":"x"}`)); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// V6.0: 子代理默认继承父工具但排除 meta-tools。
	// 父工具: [read_file, grep, task, run_skill, explore, research, review, security_review, remember]
	// 排除 meta-tools 后 → [read_file, grep, remember]
	got := map[string]bool{}
	for _, s := range sub.lastReq.Tools {
		got[s.Name] = true
	}
	if !got["read_file"] || !got["grep"] || !got["remember"] {
		t.Errorf("V6.0: default sub-agent API request tools = %v, want [read_file, grep, remember]", got)
	}
	if got["task"] || got["run_skill"] || got["explore"] || got["research"] || got["review"] || got["security_review"] {
		t.Errorf("V6.0: meta-tools should be excluded, got %v", got)
	}
}

// TestTaskToolPassesPricingToSubAgent verifies the sub-agent's Usage event
// carries the parent's Pricing so cost statistics are non-zero.
func TestTaskToolPassesPricingToSubAgent(t *testing.T) {
	pricing := &provider.Pricing{CacheHit: 0.025, Input: 3, Output: 6}
	sub := &mockProvider{
		name: "sub",
		chunks: []provider.Chunk{
			{Type: provider.ChunkText, Text: "ok"},
			{Type: provider.ChunkUsage, Usage: &provider.Usage{
				PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
				CacheHitTokens: 80, CacheMissTokens: 20,
			}},
			{Type: provider.ChunkDone},
		},
	}
	sink := &testSink{}
	parentReg := tool.NewRegistry()
	task := NewTaskTool(sub, pricing, parentReg, 20, 0, 0.0, "", "sys", nil)
	parentReg.Add(task)

	ctx := withCallContext(context.Background(), "call_1", sink, nil)
	_, err := task.Execute(ctx, []byte(`{"prompt":"test pricing flow"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Find the last Usage event (sub-agent usage tagged as "subagent")
	var lastUsage *provider.Usage
	var lastPricing *provider.Pricing
	for _, e := range sink.events {
		if e.Kind == event.Usage && e.UsageSource == event.UsageSourceSubagent {
			lastUsage = e.Usage
			lastPricing = e.Pricing
		}
	}
	if lastUsage == nil {
		t.Fatal("sub-agent did not emit a Usage event with UsageSourceSubagent")
	}
	if lastPricing == nil {
		t.Fatal("sub-agent Usage event has nil Pricing — cost will be 0")
	}
	if lastPricing != pricing {
		t.Errorf("sub-agent Pricing = %+v, want parent pricing %+v", lastPricing, pricing)
	}
	cost := pricing.Cost(lastUsage)
	if cost <= 0 {
		t.Errorf("sub-agent cost = %v, want > 0", cost)
	}
	t.Logf("sub-agent cost = %v (tokens: prompt=%d completion=%d)", cost, lastUsage.PromptTokens, lastUsage.CompletionTokens)
}

// TestTaskToolSubagentPricingFallsBackToParent verifies that when subagent_model
// pricing is nil, it falls back to the parent's pricing.
func TestTaskToolSubagentPricingFallsBackToParent(t *testing.T) {
	parentPricing := &provider.Pricing{CacheHit: 0.025, Input: 3, Output: 6}
	sub := &mockProvider{
		name: "sub",
		chunks: []provider.Chunk{
			{Type: provider.ChunkText, Text: "ok"},
			{Type: provider.ChunkUsage, Usage: &provider.Usage{
				PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
			}},
			{Type: provider.ChunkDone},
		},
	}
	sub2 := &mockProvider{
		name: "sub2",
		chunks: []provider.Chunk{
			{Type: provider.ChunkText, Text: "ok"},
			{Type: provider.ChunkUsage, Usage: &provider.Usage{
				PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300,
			}},
			{Type: provider.ChunkDone},
		},
	}
	sink := &testSink{}
	parentReg := tool.NewRegistry()
	task := NewTaskTool(sub, parentPricing, parentReg, 20, 0, 0.0, "", "sys", nil)
	// Set subagent model with nil pricing — should fall back to parentPricing
	task.SetSubagentProvider(sub2, nil, 0)
	parentReg.Add(task)

	ctx := withCallContext(context.Background(), "call_1", sink, nil)
	_, err := task.Execute(ctx, []byte(`{"prompt":"test fallback"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var lastUsage *provider.Usage
	var lastPricing *provider.Pricing
	for _, e := range sink.events {
		if e.Kind == event.Usage && e.UsageSource == event.UsageSourceSubagent {
			lastUsage = e.Usage
			lastPricing = e.Pricing
		}
	}
	if lastUsage == nil {
		t.Fatal("sub-agent did not emit a Usage event")
	}
	if lastPricing == nil {
		t.Fatal("sub-agent Pricing is nil — fallback to parent pricing failed")
	}
	if lastPricing != parentPricing {
		t.Errorf("sub-agent Pricing = %+v, want parent pricing %+v", lastPricing, parentPricing)
	}
	cost := parentPricing.Cost(lastUsage)
	if cost <= 0 {
		t.Errorf("sub-agent cost = %v, want > 0", cost)
	}
	t.Logf("fallback sub-agent cost = %v", cost)
}

// testSink is a simple event sink for tests.
type testSink struct {
	events []event.Event
}

func (s *testSink) Emit(e event.Event) {
	s.events = append(s.events, e)
}
