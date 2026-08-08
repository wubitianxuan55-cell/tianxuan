package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"tianxuan/internal/event"
	"tianxuan/internal/planmode"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// scriptedHostProvider 按调用序号重放预设 chunk 序列，并记录每次请求的
// 消息（用于断言 Marker 注入与执行轮确认）。单模型规划模式下规划轮与
// 执行轮共用同一个 provider —— 这正是单模型架构的测试形态。
type scriptedHostProvider struct {
	turns  [][]provider.Chunk
	call   int
	bodies [][]provider.Message
}

func (s *scriptedHostProvider) Name() string { return "solo" }

func (s *scriptedHostProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	s.bodies = append(s.bodies, req.Messages)
	i := s.call
	if i >= len(s.turns) {
		i = len(s.turns) - 1
	}
	s.call++
	ch := make(chan provider.Chunk, len(s.turns[i]))
	for _, c := range s.turns[i] {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func textTurn(text string) []provider.Chunk {
	return []provider.Chunk{
		{Type: provider.ChunkText, Text: text},
		{Type: provider.ChunkDone},
	}
}

func signTurn(step string) []provider.Chunk {
	return []provider.Chunk{
		toolCallChunk("c1", "complete_step", fmt.Sprintf(
			`{"step":%q,"result":"ok","evidence":[{"kind":"verification","summary":"go test passed","command":"go test ./..."}]}`, step)),
		{Type: provider.ChunkDone},
	}
}

// failTurn 用空证据调用 complete_step —— 工具校验失败，步骤记为 error。
func failTurn(step string) []provider.Chunk {
	return []provider.Chunk{
		toolCallChunk("c1", "complete_step", fmt.Sprintf(
			`{"step":%q,"result":"ok","evidence":[]}`, step)),
		{Type: provider.ChunkDone},
	}
}

func newPlanHostFromProvider(t *testing.T, prov provider.Provider, autoPlan string) (*PlannerHost, *Session, *AgentRunner) {
	t.Helper()
	reg := tool.NewRegistry()
	if cs, ok := tool.LookupBuiltin("complete_step"); ok {
		reg.Add(cs)
	}
	sess := NewSession("solo L1+L2")
	exec := New(prov, reg, sess, Options{
		MaxSteps:    10,
		Temperature: 0,
	}, event.Discard)
	h := NewPlannerHost(exec, autoPlan, event.Discard)
	return h, sess, exec
}

// firstUserContent 返回某次请求中最后一条 user 消息文本。
func firstUserContent(msgs []provider.Message) string {
	for _, m := range msgs {
		if m.Role == provider.RoleUser {
			return m.Content
		}
	}
	return ""
}

func lastUserContent(msgs []provider.Message) string {
	var last string
	for _, m := range msgs {
		if m.Role == provider.RoleUser {
			last = m.Content
		}
	}
	return last
}

func anyBodyContainsMarker(p *scriptedHostProvider) bool {
	for _, msgs := range p.bodies {
		if strings.Contains(firstUserContent(msgs), planmode.Marker) {
			return true
		}
	}
	return false
}

// 默认（规划模式关闭）：复杂任务直接执行，不注入 Marker。
func TestPlannerHost_DefaultDirectExec(t *testing.T) {
	prov := &scriptedHostProvider{turns: [][]provider.Chunk{
		signTurn("步骤 1：改 a.go"),
		textTurn("done."),
	}}
	h, _, _ := newPlanHostFromProvider(t, prov, "off")
	if _, err := h.Run(context.Background(), "重构认证模块并更新多个文件"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if anyBodyContainsMarker(prov) {
		t.Fatal("default mode must not inject plan-mode marker")
	}
}

// 规划模式开启 + 复杂任务：先规划轮（Marker 注入），再执行轮（已确认执行）。
func TestPlannerHost_PlannedFlow(t *testing.T) {
	prov := &scriptedHostProvider{turns: [][]provider.Chunk{
		textTurn("<!--plan-->\n步骤 1：改 a.go"),
		signTurn("步骤 1：改 a.go"),
		textTurn("done."),
		textTurn("done."),
	}}
	h, _, _ := newPlanHostFromProvider(t, prov, "ask")
	res, err := h.Run(context.Background(), "重构认证模块并更新多个文件")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.bodies) != 4 {
		t.Fatalf("expected plan + execute calls, got %d", len(prov.bodies))
	}
	if !strings.Contains(firstUserContent(prov.bodies[0]), planmode.Marker) {
		t.Fatal("planning round must inject plan-mode marker")
	}
	if !strings.Contains(lastUserContent(prov.bodies[1]), "已确认执行以下计划") {
		t.Fatalf("execution round must confirm the plan, got: %s", lastUserContent(prov.bodies[1]))
	}
	if res == nil || !strings.Contains(res.Plan, "步骤 1") {
		t.Fatalf("TurnResult must carry the plan, got %+v", res)
	}
}

// 规划轮直接回答（无 <!--plan-->）：不派发执行轮。
func TestPlannerHost_DirectAnswer(t *testing.T) {
	prov := &scriptedHostProvider{turns: [][]provider.Chunk{
		textTurn("这是一个问答回复，不是计划。"),
	}}
	h, _, _ := newPlanHostFromProvider(t, prov, "ask")
	res, err := h.Run(context.Background(), "重构认证模块并更新多个文件")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.bodies) != 1 {
		t.Fatalf("direct answer must not trigger execution, got %d calls", len(prov.bodies))
	}
	if res == nil || !res.Success {
		t.Fatalf("direct answer should succeed without execution, got %+v", res)
	}
}

// 规划模式开启但简单任务（RouteExecOnly）：直接执行，不走规划轮。
func TestPlannerHost_SimpleTaskDirect(t *testing.T) {
	prov := &scriptedHostProvider{turns: [][]provider.Chunk{
		signTurn("修复 readme 错别字"),
		textTurn("done."),
	}}
	h, _, _ := newPlanHostFromProvider(t, prov, "ask")
	if _, err := h.Run(context.Background(), "修复 readme 里的错别字"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if anyBodyContainsMarker(prov) {
		t.Fatal("simple task must not inject plan-mode marker")
	}
}

// "!" 前缀：跳过规划直接执行。
func TestPlannerHost_BangPrefix(t *testing.T) {
	prov := &scriptedHostProvider{turns: [][]provider.Chunk{
		signTurn("构建项目"),
		textTurn("done."),
	}}
	h, _, _ := newPlanHostFromProvider(t, prov, "ask")
	if _, err := h.Run(context.Background(), "! 构建项目"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if anyBodyContainsMarker(prov) {
		t.Fatal("bang prefix must not inject plan-mode marker")
	}
	if first := firstUserContent(prov.bodies[0]); strings.Contains(first, "!") {
		t.Fatalf("bang marker must not leak to the executor, got: %s", first)
	}
}

// headless（asker=nil）：计划自动确认后执行。
func TestPlannerHost_HeadlessAutoConfirm(t *testing.T) {
	prov := &scriptedHostProvider{turns: [][]provider.Chunk{
		textTurn("<!--plan-->\n步骤 1：改 a.go"),
		signTurn("步骤 1：改 a.go"),
		textTurn("done."),
		textTurn("done."),
	}}
	h, _, _ := newPlanHostFromProvider(t, prov, "ask")
	if h.asker != nil {
		t.Fatal("headless host must have nil asker")
	}
	if _, err := h.Run(context.Background(), "重构认证模块并更新多个文件"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.bodies) != 4 {
		t.Fatalf("headless should auto-confirm, got %d calls", len(prov.bodies))
	}
}

// 交互确认：用户"按意见修改"→ 重新规划 → 确认执行。
type scriptedAsker struct {
	answers []scriptedAnswer
	idx     int
}

type scriptedAnswer struct {
	selected string
	extra    []string
}

func (a *scriptedAsker) Ask(_ context.Context, _ []event.AskQuestion) ([]event.AskAnswer, error) {
	if a.idx >= len(a.answers) {
		return nil, nil
	}
	ans := a.answers[a.idx]
	a.idx++
	return []event.AskAnswer{{Selected: append([]string{ans.selected}, ans.extra...)}}, nil
}

func TestPlannerHost_ReviseLoop(t *testing.T) {
	prov := &scriptedHostProvider{turns: [][]provider.Chunk{
		textTurn("<!--plan-->\n步骤 1：改 a.go"),
		textTurn("<!--plan-->\n步骤 1：改 a.go\n步骤 2：补测试"),
		signTurn("步骤 2：补测试"),
		textTurn("done."),
		textTurn("done."),
	}}
	h, _, _ := newPlanHostFromProvider(t, prov, "ask")
	h.SetAsker(&scriptedAsker{answers: []scriptedAnswer{
		{selected: "按用户意见修改计划", extra: []string{"需要补测试"}},
		{selected: "提交执行"},
	}})
	res, err := h.Run(context.Background(), "重构认证模块并更新多个文件")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.bodies) != 5 {
		t.Fatalf("expected replan + execute, got %d calls", len(prov.bodies))
	}
	if !strings.Contains(lastUserContent(prov.bodies[1]), "需要补测试") {
		t.Fatalf("revise feedback must reach the planner, got: %s", lastUserContent(prov.bodies[1]))
	}
	if res == nil || !strings.Contains(res.Plan, "步骤 2") {
		t.Fatalf("final plan must carry the revision, got %+v", res)
	}
}

// 交互确认：用户取消 → 返回错误，不执行。
func TestPlannerHost_Cancel(t *testing.T) {
	prov := &scriptedHostProvider{turns: [][]provider.Chunk{
		textTurn("<!--plan-->\n步骤 1：改 a.go"),
	}}
	h, _, _ := newPlanHostFromProvider(t, prov, "ask")
	h.SetAsker(&scriptedAsker{answers: []scriptedAnswer{{selected: "取消"}}})
	if _, err := h.Run(context.Background(), "重构认证模块并更新多个文件"); err == nil {
		t.Fatal("cancel must return an error")
	}
	if len(prov.bodies) != 1 {
		t.Fatalf("cancel must stop after planning, got %d calls", len(prov.bodies))
	}
}

// 自动修正循环：执行轮有失败步骤 → 生成修正计划并重新执行。
func TestPlannerHost_FixLoop(t *testing.T) {
	plan := "<!--plan-->\n步骤 1：改 a.go\n步骤 2：补测试"
	prov := &scriptedHostProvider{turns: [][]provider.Chunk{
		textTurn(plan),
		{
			toolCallChunk("c1", "complete_step", fmt.Sprintf(
				`{"step":%q,"result":"ok","evidence":[{"kind":"verification","summary":"go test passed","command":"go test ./..."}]}`, "步骤 1：改 a.go")),
			toolCallChunk("c2", "complete_step", fmt.Sprintf(
				`{"step":%q,"result":"ok","evidence":[]}`, "步骤 2：补测试")),
			{Type: provider.ChunkDone},
		},
		textTurn("done."),
		textTurn("done."),
		signTurn("步骤 2：补测试"),
		textTurn("fixed."),
		textTurn("fixed."),
	}}
	h, _, _ := newPlanHostFromProvider(t, prov, "ask")
	res, err := h.Run(context.Background(), "重构认证模块并更新多个文件")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.bodies) != 7 {
		t.Fatalf("expected plan + execute + fix, got %d calls", len(prov.bodies))
	}
	if !strings.Contains(lastUserContent(prov.bodies[4]), "修正") {
		t.Fatalf("fix round must carry a fix prompt, got: %s", lastUserContent(prov.bodies[4]))
	}
	if res == nil || len(res.StepResults) == 0 {
		t.Fatalf("final result must carry step results, got %+v", res)
	}
}
