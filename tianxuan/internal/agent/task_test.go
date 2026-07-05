package agent

import (
	"context"
	"strings"
	"testing"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

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
