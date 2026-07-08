package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tianxuan/internal/config"
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

	go func() {
		_ = a.scheduler.Start(a.ctx)
	}()
}

func scheduleDir() string {
	dir := config.MemoryUserDir()
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "tianxuan-schedules")
	}
	return dir
}
