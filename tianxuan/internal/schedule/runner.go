// runner.go — bridges the Scheduler to the executor AgentRunner.

package schedule

import (
	"context"
	"fmt"
	"os"
	"time"

	"tianxuan/internal/agent"
	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// ExecRunner executes schedules via an executor AgentRunner, creating a fresh
// Session for each run. It skips the Hermes planner entirely — the prompt goes
// straight to the executor.
type ExecRunner struct {
	ExecProv   provider.Provider
	ToolReg    *tool.Registry
	SystemMsg  string
	Sink       event.Sink
	MaxSteps   int
	Gate       agent.Gate // nil = auto-allow
	ContextWin int
	ArchiveDir string
}

// Run implements Runner. It:
//  1. chdir to s.WorkDir (and restores on exit)
//  2. Sets s.Env (and restores on exit)
//  3. Creates a fresh agent.Session with PlannerMode=true
//  4. Calls executor.Run with s.Prompt
//  5. Returns a ScheduleResult
func (r *ExecRunner) Run(ctx context.Context, s Schedule) (ScheduleResult, error) {
	start := time.Now()

	// Switch working directory
	origDir, _ := os.Getwd()
	if s.WorkDir != "" {
		if err := os.Chdir(s.WorkDir); err != nil {
			return ScheduleResult{}, fmt.Errorf("chdir %s: %w", s.WorkDir, err)
		}
	}
	defer func() {
		if origDir != "" {
			_ = os.Chdir(origDir)
		}
	}()

	// Set environment variables
	restoreEnv := make(map[string]string)
	for k, v := range s.Env {
		if prev, ok := os.LookupEnv(k); ok {
			restoreEnv[k] = prev
		}
		os.Setenv(k, v)
	}
	defer func() {
		for k := range s.Env {
			if prev, ok := restoreEnv[k]; ok {
				os.Setenv(k, prev)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	// Create a fresh session
	sess := agent.NewSession(r.SystemMsg)

	maxSteps := r.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 30
	}

	opts := agent.Options{
		MaxSteps:      maxSteps,
		Gate:          r.Gate,
		ContextWindow: r.ContextWin,
		PlannerMode:   true, // skip planner-specific logic
	}

	executor := agent.New(r.ExecProv, r.ToolReg, sess, opts, r.Sink)

	// emit notification: task started
	r.Sink.Emit(event.Event{
		Kind:  event.Notice,
		Level: event.LevelInfo,
		Text:  fmt.Sprintf("🔵 定时任务: %s 正在执行...", s.Name),
	})

	result, err := executor.Run(ctx, s.Prompt)

	dur := time.Since(start).Milliseconds()

	sr := ScheduleResult{
		ID:         NewID(),
		ScheduleID: s.ID,
		ExecutedAt: start.Unix(),
		Duration:   dur,
		Success:    err == nil,
	}

	if err != nil {
		sr.Summary = fmt.Sprintf("exec error: %v", err)
		r.Sink.Emit(event.Event{
			Kind:  event.Notice,
			Level: event.LevelWarn,
			Text:  fmt.Sprintf("❌ %s 失败 — %v", s.Name, err),
		})
	} else {
		if result != nil {
			sr.Summary = result.Summary
			sr.Success = result.Success
		}
		if sr.Summary == "" {
			sr.Summary = "completed"
		}
		r.Sink.Emit(event.Event{
			Kind:  event.Notice,
			Level: event.LevelInfo,
			Text:  fmt.Sprintf("✅ %s 完成 — %s", s.Name, sr.Summary),
		})
	}

	return sr, nil
}
