package control

import (
	"context"
	"testing"

	"tianxuan/internal/agent"
	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// stubProvider 是只用于构造 AgentRunner 的最小 provider（不发起真实调用）。
type stubProvider struct{}

func (stubProvider) Name() string { return "stub" }
func (stubProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	ch := make(chan provider.Chunk)
	close(ch)
	return ch, nil
}

func newPlanModeExecutor(t *testing.T) *agent.Agent {
	t.Helper()
	return agent.New(stubProvider{}, tool.NewRegistry(),
		agent.NewSession("test system"), agent.Options{MaxSteps: 1}, event.Discard)
}

// TestMaybeExitPlanMode_ApprovalExitsGate：用户选择"提交执行"后，只读
// 计划门必须退出，autoPlanActive 复位——同轮才能继续执行。
func TestMaybeExitPlanMode_ApprovalExitsGate(t *testing.T) {
	exec := newPlanModeExecutor(t)
	c := &Controller{executor: exec, autoPlanActive: true}
	exec.SetPlanMode(true)

	c.maybeExitPlanMode([]event.AskAnswer{{Selected: []string{"提交执行"}}})

	if exec.PlanMode() {
		t.Fatal("approval must exit the read-only plan gate")
	}
	if c.autoPlanActive {
		t.Fatal("autoPlanActive must reset after approval")
	}
}

// TestMaybeExitPlanMode_ReviseKeepsGate：用户要求修改/取消时，计划门保持
// 开启（模型继续只读规划，不会误写文件）。
func TestMaybeExitPlanMode_ReviseKeepsGate(t *testing.T) {
	exec := newPlanModeExecutor(t)
	c := &Controller{executor: exec, autoPlanActive: true}
	exec.SetPlanMode(true)

	for _, sel := range []string{"按意见修改", "取消"} {
		c.maybeExitPlanMode([]event.AskAnswer{{Selected: []string{sel}}})
		if !exec.PlanMode() {
			t.Fatalf("%q must keep the plan gate on", sel)
		}
		if !c.autoPlanActive {
			t.Fatalf("%q must keep autoPlanActive", sel)
		}
	}
}

// TestAutoPlanEnabled_RequiresInteractiveSolo：非交互、off、executor 缺失
// 时都不启用；Solo（runner 非 Hermes）+ ask + 交互时启用。
func TestAutoPlanEnabled_RequiresInteractiveSolo(t *testing.T) {
	exec := newPlanModeExecutor(t)

	c := &Controller{executor: exec, autoPlan: "ask", interactive: false}
	if c.autoPlanEnabled() {
		t.Fatal("non-interactive must not auto-plan")
	}
	c.interactive = true
	c.autoPlan = "off"
	if c.autoPlanEnabled() {
		t.Fatal("off must not auto-plan")
	}
	c.autoPlan = "ask"
	if !c.autoPlanEnabled() {
		t.Fatal("interactive Solo + ask should auto-plan")
	}
	c.executor = nil
	if c.autoPlanEnabled() {
		t.Fatal("nil executor must not auto-plan")
	}
}
