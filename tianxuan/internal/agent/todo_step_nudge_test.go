package agent

import (
	"testing"

	"tianxuan/internal/evidence"
	"tianxuan/internal/provider"
)

// Adaptive Execution 宿主信号：当前 in_progress todo 步骤连续多轮出现
// 工具失败（动作各不相同，不会命中 detectRepeatedSteps 的相同动作检测）
// 时，宿主注入"诊断根因→调整 todo→换方案"的 nudge，而不是等模型自觉。

func failCall(name string) (provider.ToolCall, string) {
	return provider.ToolCall{Name: name, Arguments: "{}"}, "error: boom"
}

func okCall(name string) (provider.ToolCall, string) {
	return provider.ToolCall{Name: name, Arguments: "{}"}, "done"
}

func newTodoNudgeAgent(step string) *AgentRunner {
	var todos []evidence.TodoItem
	if step != "" {
		todos = []evidence.TodoItem{{Content: step, Status: "in_progress"}}
	}
	return &AgentRunner{
		session:   NewSession("sys"),
		todoState: todos,
	}
}

// TestTodoStepNudge_TriggersAfterRepeatedFailures：同一 todo 步骤连续
// TodoStepFailNudgeThreshold 轮工具失败后注入 nudge。
func TestTodoStepNudge_TriggersAfterRepeatedFailures(t *testing.T) {
	a := newTodoNudgeAgent("实现缓存")

	for i := 0; i < TodoStepFailNudgeThreshold-1; i++ {
		calls, results := failCall("edit_file")
		if a.maybeNudgeStuckTodoStep([]provider.ToolCall{calls}, []string{results}) {
			t.Fatalf("round %d: nudge must not fire before threshold", i+1)
		}
	}
	calls, results := failCall("bash")
	if !a.maybeNudgeStuckTodoStep([]provider.ToolCall{calls}, []string{results}) {
		t.Fatal("nudge must fire at threshold")
	}
	if !sessionContains(a.session, todoStepStuckNudge) {
		t.Fatal("nudge message must be injected into the session")
	}
}

// TestTodoStepNudge_ResetOnStepChange：todo 步骤变化时计数重置。
func TestTodoStepNudge_ResetOnStepChange(t *testing.T) {
	a := newTodoNudgeAgent("步骤 A")
	for i := 0; i < TodoStepFailNudgeThreshold-1; i++ {
		c, r := failCall("bash")
		a.maybeNudgeStuckTodoStep([]provider.ToolCall{c}, []string{r})
	}
	// 切换到步骤 B（todo_write 更新）后连续失败应从零开始
	a.todoState = []evidence.TodoItem{{Content: "步骤 B", Status: "in_progress"}}
	for i := 0; i < TodoStepFailNudgeThreshold-1; i++ {
		c, r := failCall("bash")
		if a.maybeNudgeStuckTodoStep([]provider.ToolCall{c}, []string{r}) {
			t.Fatalf("step switch must reset the counter, fired at round %d", i+1)
		}
	}
}

// TestTodoStepNudge_SuccessBreaksStreak：无失败的一轮中断累计。
func TestTodoStepNudge_SuccessBreaksStreak(t *testing.T) {
	a := newTodoNudgeAgent("实现缓存")
	for i := 0; i < TodoStepFailNudgeThreshold-1; i++ {
		c, r := failCall("bash")
		a.maybeNudgeStuckTodoStep([]provider.ToolCall{c}, []string{r})
	}
	// 成功轮中断连续失败
	c, r := okCall("bash")
	a.maybeNudgeStuckTodoStep([]provider.ToolCall{c}, []string{r})
	// 再失败 3 轮也不到阈值（从 0 重新累计）
	for i := 0; i < TodoStepFailNudgeThreshold-1; i++ {
		c, r := failCall("bash")
		if a.maybeNudgeStuckTodoStep([]provider.ToolCall{c}, []string{r}) {
			t.Fatalf("success must break the streak, fired at round %d", i+1)
		}
	}
}

// TestTodoStepNudge_NoTodoNoNudge：没有 in_progress 步骤时永不 nudge。
func TestTodoStepNudge_NoTodoNoNudge(t *testing.T) {
	a := newTodoNudgeAgent("")
	for i := 0; i < TodoStepFailNudgeThreshold+2; i++ {
		c, r := failCall("bash")
		if a.maybeNudgeStuckTodoStep([]provider.ToolCall{c}, []string{r}) {
			t.Fatal("no in_progress todo must never nudge")
		}
	}
}
