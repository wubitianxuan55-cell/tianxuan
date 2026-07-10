package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tianxuan/internal/config"
	"tianxuan/internal/provider"
	"tianxuan/internal/schedule"
)

// ScheduleView is the frontend-friendly representation of a schedule.
type ScheduleView struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Prompt    string            `json:"prompt"`
	Frequency string            `json:"frequency"`
	Time      string            `json:"time"`
	DayOfWeek int               `json:"dayOfWeek"`
	WorkDir   string            `json:"workDir"`
	Env       map[string]string `json:"env,omitempty"`
	Enabled   bool              `json:"enabled"`
	CreatedAt int64             `json:"createdAt"`
	LastRunAt int64             `json:"lastRunAt"`
	Scope     string            `json:"scope"`
}

// ResultView is the frontend-friendly execution result.
type ResultView struct {
	ID          string `json:"id"`
	ScheduleID  string `json:"scheduleId"`
	ExecutedAt  int64  `json:"executedAt"`
	Success     bool   `json:"success"`
	Summary     string `json:"summary"`
	SessionFile string `json:"sessionFile"`
	Duration    int64  `json:"duration"`
}

func toScheduleView(s schedule.Schedule) ScheduleView {
	return ScheduleView{
		ID: s.ID, Name: s.Name, Prompt: s.Prompt,
		Frequency: string(s.Frequency), Time: s.Time, DayOfWeek: s.DayOfWeek,
		WorkDir: s.WorkDir, Env: s.Env, Enabled: s.Enabled,
		CreatedAt: s.CreatedAt, LastRunAt: s.LastRunAt, Scope: s.Scope,
	}
}

func toResultView(r schedule.ScheduleResult) ResultView {
	return ResultView{
		ID: r.ID, ScheduleID: r.ScheduleID, ExecutedAt: r.ExecutedAt,
		Success: r.Success, Summary: r.Summary, SessionFile: r.SessionFile,
		Duration: r.Duration,
	}
}

// GetSchedules returns all schedules (global + workspace).
func (a *App) GetSchedules() ([]ScheduleView, error) {
	a.mu.RLock()
	sc := a.scheduler
	a.mu.RUnlock()
	if sc == nil {
		return nil, nil
	}
	list := sc.ListSchedules()
	out := make([]ScheduleView, len(list))
	for i, s := range list {
		out[i] = toScheduleView(s)
	}
	return out, nil
}

// CreateSchedule adds a new schedule.
func (a *App) CreateSchedule(v ScheduleView) (ScheduleView, error) {
	a.mu.RLock()
	sc := a.scheduler
	a.mu.RUnlock()
	if sc == nil {
		return v, fmt.Errorf("scheduler not ready")
	}
	s := schedule.Schedule{
		ID:        schedule.NewID(),
		Name:      v.Name,
		Prompt:    v.Prompt,
		Frequency: schedule.Frequency(v.Frequency),
		Time:      v.Time,
		DayOfWeek: v.DayOfWeek,
		WorkDir:   v.WorkDir,
		Env:       v.Env,
		Enabled:   v.Enabled,
		Scope:     v.Scope,
		CreatedAt: time.Now().Unix(),
	}
	if err := sc.AddSchedule(s); err != nil {
		return v, err
	}
	return toScheduleView(s), nil
}

// UpdateSchedule modifies an existing schedule.
func (a *App) UpdateSchedule(v ScheduleView) error {
	a.mu.RLock()
	sc := a.scheduler
	a.mu.RUnlock()
	if sc == nil {
		return fmt.Errorf("scheduler not ready")
	}
	if err := sc.RemoveSchedule(v.ID); err != nil {
		return err
	}
	s := schedule.Schedule{
		ID: v.ID, Name: v.Name, Prompt: v.Prompt,
		Frequency: schedule.Frequency(v.Frequency), Time: v.Time, DayOfWeek: v.DayOfWeek,
		WorkDir: v.WorkDir, Env: v.Env, Enabled: v.Enabled,
		Scope: v.Scope, CreatedAt: v.CreatedAt, LastRunAt: v.LastRunAt,
	}
	return sc.AddSchedule(s)
}

// DeleteSchedule removes a schedule and its results.
func (a *App) DeleteSchedule(id string) error {
	a.mu.RLock()
	sc := a.scheduler
	a.mu.RUnlock()
	if sc == nil {
		return nil
	}
	return sc.RemoveSchedule(id)
}

// ToggleSchedule enables/disables a schedule.
func (a *App) ToggleSchedule(id string, enabled bool) error {
	a.mu.RLock()
	sc := a.scheduler
	a.mu.RUnlock()
	if sc == nil {
		return fmt.Errorf("scheduler not ready")
	}
	return sc.ToggleSchedule(id, enabled)
}

// RunScheduleNow manually triggers a schedule immediately.
func (a *App) RunScheduleNow(id string) (ResultView, error) {
	a.mu.RLock()
	sc := a.scheduler
	a.mu.RUnlock()
	if sc == nil {
		return ResultView{}, fmt.Errorf("scheduler not ready")
	}
	result, err := sc.RunNow(id)
	if err != nil {
		return ResultView{}, err
	}
	return toResultView(result), nil
}

// GetResults returns execution results for a schedule.
func (a *App) GetResults(scheduleID string) ([]ResultView, error) {
	a.mu.RLock()
	sc := a.scheduler
	a.mu.RUnlock()
	if sc == nil {
		return nil, nil
	}
	results, err := sc.ListResults(scheduleID)
	if err != nil {
		return nil, err
	}
	out := make([]ResultView, len(results))
	for i, r := range results {
		out[i] = toResultView(r)
	}
	return out, nil
}

const refineScheduleSystemPrompt = `你是一个任务规划专家。用户会给你一句简短的定时任务描述，你需要将其扩展为一份结构化、可执行的 AI 编程助手任务指令。

输出要求：
1. 任务目标：一句话明确要达成什么
2. 执行步骤：3~8 个具体步骤，每步包含要使用的工具（如 grep/bash/read_file/write_file/edit_file 等）和预期产出
3. 成功标准：任务完成应满足什么条件
4. 输出格式：结果应如何呈现（如摘要/报告/修复记录等）

规则：
- 只输出任务的完整规划内容，不要加"好的""明白了"等开场白
- 不要用 markdown 代码块包裹输出
- 使用中文`

// RefineSchedulePrompt expands a rough task description into a detailed,
// structured prompt suitable for scheduled execution. It calls the executor
// model once with the planning system prompt and returns the result.
func (a *App) RefineSchedulePrompt(prompt string) (string, error) {
	a.mu.RLock()
	ctrl := a.ctrl
	a.mu.RUnlock()
	if ctrl == nil {
		return "", fmt.Errorf("controller not ready")
	}
	executor := ctrl.Executor()
	if executor == nil {
		return "", fmt.Errorf("executor not ready")
	}
	prov := executor.Provider()
	if prov == nil {
		return "", fmt.Errorf("provider not ready")
	}

	req := provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: refineScheduleSystemPrompt},
			{Role: provider.RoleUser, Content: prompt},
		},
		MaxTokens:   2048,
		Temperature: 0.3,
	}

	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	ch, err := prov.Stream(ctx, req)
	if err != nil {
		return "", fmt.Errorf("stream: %w", err)
	}

	var sb strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			return "", fmt.Errorf("chunk error: %w", chunk.Err)
		}
		if chunk.Type == provider.ChunkText {
			sb.WriteString(chunk.Text)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// startScheduler bootstraps the schedule system after the controller is ready.
// Called from buildController after boot.Build() succeeds.
func (a *App) startScheduler() {
	a.mu.RLock()
	ctrl := a.ctrl
	a.mu.RUnlock()
	if ctrl == nil {
		return
	}
	executor := ctrl.Executor()
	if executor == nil {
		return
	}

	globalDir := scheduleDir()
	globalStore := schedule.NewStore(globalDir)
	var wsStore *schedule.Store
	if cwd, err := os.Getwd(); err == nil {
		wsDir := filepath.Join(cwd, ".tianxuan")
		wsStore = schedule.NewStore(wsDir)
	}

	runner := &schedule.ExecRunner{
		ExecProv:   executor.Provider(),
		ToolReg:    executor.Registry(),
		SystemMsg:  ctrl.SystemPrompt(),
		Sink:       a.sink,
		MaxSteps:   30,
		Gate:       nil, // auto-allow all tools for scheduled tasks
		ContextWin: executor.ContextWindow(),
		ArchiveDir: config.ArchiveDir(),
	}

	a.mu.Lock()
	a.scheduler = schedule.NewScheduler(runner, globalStore, wsStore)
	a.scheduler.SetOnChange(func() {
		runtime.EventsEmit(a.ctx, "schedule:changed")
	})
	a.mu.Unlock()

	a.goSafe("scheduler-start", func() {
		_ = a.scheduler.Start(a.ctx)
	})
}

func scheduleDir() string {
	dir := config.MemoryUserDir()
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "tianxuan-schedules")
	}
	return dir
}
