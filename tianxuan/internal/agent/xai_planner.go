package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"tianxuan/internal/event"
	"tianxuan/internal/planmode"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// XAIPlanner 是 XAI（Grok）规划者的独立实现。
//
// 设计思想（借鉴 Claude Code Plan Mode）：
//   - 不强制阶段划分——模型在 thinking 中自然完成探索→分析→规划
//   - 所有只读工具始终可用，模型自主决定何时探索、何时输出计划
//   - 一个高质量的 planning prompt 引导行为（和 DeepSeek 共用 HermesPrompt）
//   - 分层上下文管理 + 内容感知压缩控制 token 用量
//
// 与 DeepSeek AgentRunner 完全隔离：
//   - 分层上下文（xaiContext），不使用 TCCA 四域缓存
//   - 无前缀缓存守卫——XAI 月费制
//   - 结构化压缩：不丢弃消息，压缩保留
//   - 对外接口完全兼容 Planner
type XAIPlanner struct {
	prov    provider.Provider
	tools   *tool.Registry
	session *Session
	sessMu  sync.Mutex

	maxSteps    int
	temperature float64
	pricing     *provider.Pricing

	sink  event.Sink
	asker Asker

	ctxWindow int

	planMode       bool
	planModePolicy planmode.Policy

	lastUsage provider.Usage
}

// NewXAIPlanner 创建 XAI 规划者。
func NewXAIPlanner(prov provider.Provider, tools *tool.Registry, session *Session,
	maxSteps int, temperature float64, pricing *provider.Pricing,
	ctxWindow int, archiveDir string, sink event.Sink) *XAIPlanner {

	if session == nil {
		session = NewSession("")
	}
	return &XAIPlanner{
		prov:        prov,
		tools:       tools,
		session:     session,
		maxSteps:    maxSteps,
		temperature: temperature,
		pricing:     pricing,
		sink:        sink,
		ctxWindow:   ctxWindow,
	}
}

// Run 执行规划。自然的对话循环——模型自主探索代码库，然后在合适的时机输出计划。
func (p *XAIPlanner) Run(ctx context.Context, input string) (*TurnResult, error) {
	xctx := p.initContext(input)

	maxSteps := p.maxSteps
	if maxSteps <= 0 {
		maxSteps = 12
	}

	var toolErrors []string
	var lastText string

	for step := 0; step < maxSteps; step++ {
		// 首轮传完整任务，后续不追加 user 消息（模型从工具结果自然继续）
		taskPrompt := input
		if step > 0 {
			taskPrompt = ""
		}
		msgs := xctx.BuildMessages(taskPrompt)

		text, calls, toolResults, toolErrMsgs, usage, err := p.streamOnce(ctx, msgs)
		if err != nil {
			return nil, fmt.Errorf("xai planner: %w", err)
		}
		if usage != nil {
			p.lastUsage = *usage
		}

		lastText = text

		// 保存 assistant 回复（含 tool_calls 保持 API 规范）
		xctx.AddHistory(provider.Message{
			Role:      provider.RoleAssistant,
			Content:   text,
			ToolCalls: calls,
		})

		// 无工具调用 → 模型已给出最终计划
		if len(calls) == 0 {
			break
		}

		// 工具已在 streamOnce 中即时执行，这里只需注入结果到上下文
		for i, tc := range calls {
			result := ""
			if i < len(toolResults) {
				result = toolResults[i]
			}
			xctx.AddHistory(provider.Message{
				Role:       provider.RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
				Name:       tc.Name,
			})
			xctx.AddDiscovery(tc.Name, result)

			if i < len(toolErrMsgs) && toolErrMsgs[i] != "" {
				toolErrors = append(toolErrors, fmt.Sprintf("%s: %s", tc.Name, toolErrMsgs[i]))
			}
		}

		// 上下文压缩检查
		if usage != nil && p.ctxWindow > 0 {
			p.maybeCompact(xctx, usage.PromptTokens)
		}
	}

	p.persistToSession(input, lastText)

	return &TurnResult{
		Summary: strings.TrimSpace(lastText),
		Success: len(toolErrors) == 0,
		Errors:  toolErrors,
	}, nil
}

// initContext 从 session 初始化分层上下文。
func (p *XAIPlanner) initContext(input string) *xaiContext {
	p.sessMu.Lock()
	msgs := p.session.Snapshot()
	p.sessMu.Unlock()

	var sysMsgs []provider.Message
	for _, m := range msgs {
		if m.Role == provider.RoleSystem {
			sysMsgs = append(sysMsgs, m)
		}
	}

	xctx := newXAIContext(sysMsgs)
	xctx.SetGoal(input)

	// 恢复历史消息到 L4
	var nonSys []provider.Message
	for _, m := range msgs {
		if m.Role != provider.RoleSystem {
			nonSys = append(nonSys, m)
		}
	}
	if len(nonSys) > 8 {
		nonSys = nonSys[len(nonSys)-8:]
	}
	for _, m := range nonSys {
		xctx.AddHistory(m)
	}

	return xctx
}

// persistToSession 持久化到 session（供 Hermes 修正循环使用）。
func (p *XAIPlanner) persistToSession(input, summary string) {
	p.sessMu.Lock()
	defer p.sessMu.Unlock()
	p.session.Add(provider.Message{Role: provider.RoleUser, Content: input})
	p.session.Add(provider.Message{Role: provider.RoleAssistant, Content: summary})
}

// maybeCompact 上下文压缩。
func (p *XAIPlanner) maybeCompact(xctx *xaiContext, promptTokens int) {
	if p.ctxWindow <= 0 {
		return
	}
	if float64(promptTokens)/float64(p.ctxWindow) < 0.75 {
		return
	}
	p.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
		Text: fmt.Sprintf("上下文用量 %.0f%%，触发分层压缩", float64(promptTokens)/float64(p.ctxWindow)*100)})
	xctx.Compact()
}

// ── stream / tool ──

func (p *XAIPlanner) streamOnce(ctx context.Context, msgs []provider.Message) (
	text string, calls []provider.ToolCall, toolResults []string, toolErrMsgs []string, usage *provider.Usage, err error) {

	schemas := p.tools.Schemas()
	ch, err := p.prov.Stream(ctx, provider.Request{
		Messages:    msgs,
		Tools:       schemas,
		Temperature: p.temperature,
	})
	if err != nil {
		return "", nil, nil, nil, nil, err
	}

	var textBuf strings.Builder
	batcher := newStreamBatcher(p.sink)

	// 异步工具执行（对齐 AgentRunner pre-execute goroutine 模式）
	type toolOutcome struct {
		index  int
		result string
		errMsg string
	}
	var wg sync.WaitGroup
	outcomes := make(chan toolOutcome, 8)
	toolIndex := 0

	for chunk := range ch {
		switch chunk.Type {
		case provider.ChunkText:
			textBuf.WriteString(chunk.Text)
			batcher.AddText(chunk.Text)
		case provider.ChunkReasoning:
			batcher.AddReasoning(chunk.Text)
		case provider.ChunkToolCallStart:
			batcher.FlushNow()
			if tc := chunk.ToolCall; tc != nil {
				p.sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{
					ID: tc.ID, Name: tc.Name, ReadOnly: true, Partial: true,
				}})
			}
		case provider.ChunkToolCall:
			calls = append(calls, *chunk.ToolCall)
			if tc := chunk.ToolCall; tc != nil && tc.ID != "" {
				idx := toolIndex
				toolIndex++
				wg.Add(1)
				go func(call provider.ToolCall, i int) {
					defer wg.Done()
					r, em := p.executeTool(ctx, call)
					outcomes <- toolOutcome{index: i, result: r, errMsg: em}
				}(*tc, idx)
			}
		case provider.ChunkUsage:
			usage = chunk.Usage
			batcher.FlushNow()
			if usage != nil {
				p.sink.Emit(event.Event{Kind: event.Usage, Usage: usage, Pricing: p.pricing})
			}
		case provider.ChunkError:
			batcher.FlushAll()
			return "", nil, nil, nil, nil, chunk.Err
		}
	}
	batcher.FlushAll()

	// 等待所有异步工具执行完成，收集结果
	wg.Wait()
	close(outcomes)
	toolResults = make([]string, toolIndex)
	toolErrMsgs = make([]string, toolIndex)
	for o := range outcomes {
		toolResults[o.index] = o.result
		if o.errMsg != "" {
			toolErrMsgs[o.index] = o.errMsg
		}
	}
	return textBuf.String(), calls, toolResults, toolErrMsgs, usage, nil
}

func (p *XAIPlanner) executeTool(ctx context.Context, tc provider.ToolCall) (result, errMsg string) {
	t, ok := p.tools.Get(tc.Name)
	if !ok {
		return fmt.Sprintf("unknown tool: %s", tc.Name), fmt.Sprintf("unknown tool: %s", tc.Name)
	}
	if p.planMode {
		decision := p.planModePolicy.Decide(planmode.Call{
			Name: tc.Name, ReadOnly: t.ReadOnly(), Args: json.RawMessage(tc.Arguments),
		})
		if decision.Blocked {
			return decision.Message, decision.Message
		}
	}
	outcome, err := t.Execute(ctx, json.RawMessage(tc.Arguments))
	p.sink.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{
		ID: tc.ID, Name: tc.Name, ReadOnly: t.ReadOnly(), Output: outcome,
	}})
	if err != nil {
		return outcome, err.Error()
	}
	return outcome, ""
}

// ── Planner 接口方法 ──

func (p *XAIPlanner) SetSession(s *Session) {
	p.sessMu.Lock()
	defer p.sessMu.Unlock()
	p.session = s
}
func (p *XAIPlanner) SetAsker(a Asker)                        { p.asker = a }
func (p *XAIPlanner) SetPlanMode(v bool)                       { p.planMode = v }
func (p *XAIPlanner) SetPlanModePolicy(pol planmode.Policy)    { p.planModePolicy = pol }
func (p *XAIPlanner) CompactNow(_ context.Context, _ string) error { return nil }
func (p *XAIPlanner) LastUsage() *provider.Usage               { return &p.lastUsage }
func (p *XAIPlanner) ContextWindow() int                       { return p.ctxWindow }
func (p *XAIPlanner) ProvName() string                         { return p.prov.Name() }

// FormatFeedback 生成精简版执行反馈（XAI 专用，省 token）。
// 只传失败步骤的关键信息，不传完整 Summary。
func (p *XAIPlanner) FormatFeedback(r *TurnResult) string {
	if r == nil {
		return ""
	}
	var parts []string

	// 成功的步骤
	successCount := 0
	for _, sr := range r.StepResults {
		if sr.Status == "success" {
			successCount++
		}
	}
	if successCount > 0 {
		parts = append(parts, fmt.Sprintf("✅ %d 步骤成功", successCount))
	}

	// 失败步骤的关键信息
	for _, sr := range r.StepResults {
		if sr.Status != "success" && sr.Result != "" {
			parts = append(parts, fmt.Sprintf("❌ %s: %s", sr.Step, sr.Result))
		}
	}

	// fallback: 没有步骤结果时用错误摘要
	if len(parts) == 0 && len(r.Errors) > 0 {
		parts = append(parts, fmt.Sprintf("执行错误: %s", strings.Join(r.Errors, "; ")))
	}

	if len(parts) == 0 {
		return "执行完成"
	}
	return "[执行反馈] " + strings.Join(parts, " | ")
}
