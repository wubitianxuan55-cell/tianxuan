package control

import (
	"context"
	"testing"
	"time"

	"tianxuan/internal/agent"
	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

type stubProvider2 struct{}

func (stubProvider2) Name() string { return "stub" }
func (stubProvider2) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	ch := make(chan provider.Chunk)
	close(ch)
	return ch, nil
}

type countingRunner struct{ runs int }

func (r *countingRunner) Run(ctx context.Context, input string) (*agent.TurnResult, error) {
	r.runs++
	return &agent.TurnResult{}, nil
}

func newSteerController(runner agent.Runner) (*Controller, *agent.Agent) {
	exec := agent.New(stubProvider2{}, tool.NewRegistry(),
		agent.NewSession("sys"), agent.Options{MaxSteps: 1}, event.Discard)
	return &Controller{runner: runner, executor: exec, sink: event.Discard}, exec
}

// TestControllerSteer_WhileRunningQueuesIntoExecutor：运行中 Steer 必须把
// 纠偏消息注入 executor 队列（不启动新 turn、不消费），下一模型步骤生效。
func TestControllerSteer_WhileRunningQueuesIntoExecutor(t *testing.T) {
	runner := &countingRunner{}
	c, exec := newSteerController(runner)
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	c.Steer("跳过测试，先修编译")

	if runner.runs != 0 {
		t.Fatalf("Steer while running must not start a new turn, got %d runs", runner.runs)
	}
	if exec.SteerConsumed() {
		t.Fatal("steer must be queued but not yet consumed")
	}
}

// TestControllerSteer_IdleFallsBackToSend：空闲时 Steer 等同 Send（启动一轮）。
func TestControllerSteer_IdleFallsBackToSend(t *testing.T) {
	runner := &countingRunner{}
	c, _ := newSteerController(runner)

	c.Steer("hello")

	waitFor(t, func() bool { return runner.runs > 0 })
	if runner.runs == 0 {
		t.Fatal("idle Steer must start a turn via Send")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
