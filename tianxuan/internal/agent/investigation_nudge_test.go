package agent

import (
	"testing"

	"tianxuan/internal/provider"
)

// 子代理并行优先的宿主信号：主 agent 在一轮内做大量纯调查（read_file /
// grep / glob，无写工具）时，注入"改用 explore 子代理"的引导——避免
// 调查中间信息永久堆积在主上下文。

func investigationCall(name string) provider.ToolCall {
	return provider.ToolCall{Name: name, Arguments: "{}"}
}

func TestInvestigationNudge_TriggersOnHeavyPureSurvey(t *testing.T) {
	a := &AgentRunner{session: NewSession("sys")}
	var calls []provider.ToolCall
	for i := 0; i < InvestigationNudgeThreshold; i++ {
		calls = append(calls, investigationCall("read_file"))
	}

	if !a.maybeNudgeInvestigation(calls) {
		t.Fatal("heavy pure-survey round must trigger the subagent nudge")
	}
	if !sessionContains(a.session, investigationNudge) {
		t.Fatal("nudge message must be injected into the session")
	}
}

func TestInvestigationNudge_UnderThresholdSilent(t *testing.T) {
	a := &AgentRunner{session: NewSession("sys")}
	var calls []provider.ToolCall
	for i := 0; i < InvestigationNudgeThreshold-1; i++ {
		calls = append(calls, investigationCall("grep"))
	}
	if a.maybeNudgeInvestigation(calls) {
		t.Fatal("below-threshold survey must not nudge")
	}
}

func TestInvestigationNudge_WriteToolSkips(t *testing.T) {
	// 执行中读文件（伴随写工具）不算纯调查，不 nudge
	a := &AgentRunner{session: NewSession("sys")}
	var calls []provider.ToolCall
	for i := 0; i < InvestigationNudgeThreshold; i++ {
		calls = append(calls, investigationCall("read_file"))
	}
	calls = append(calls, investigationCall("edit_file"))
	if a.maybeNudgeInvestigation(calls) {
		t.Fatal("reads mixed with writes are execution, not investigation")
	}
}

func TestInvestigationNudge_CappedPerTurn(t *testing.T) {
	a := &AgentRunner{session: NewSession("sys")}
	// 触发 cap 次后不再注入
	for round := 0; round < InvestigationNudgeCap+1; round++ {
		var calls []provider.ToolCall
		for i := 0; i < InvestigationNudgeThreshold; i++ {
			calls = append(calls, investigationCall("read_file"))
		}
		a.maybeNudgeInvestigation(calls)
	}
	a.investigationNudgeCount = InvestigationNudgeCap // 已到上限
	var calls []provider.ToolCall
	for i := 0; i < InvestigationNudgeThreshold; i++ {
		calls = append(calls, investigationCall("read_file"))
	}
	if a.maybeNudgeInvestigation(calls) {
		t.Fatal("nudge must be capped per turn")
	}
}
