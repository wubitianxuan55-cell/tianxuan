// scheduler.go — ticker-based scheduling loop.

package schedule

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Runner executes a schedule's prompt via the executor AgentRunner and returns
// the result. The Scheduler calls this in a goroutine; the implementation must be
// safe for concurrent invocation.
type Runner interface {
	Run(ctx context.Context, s Schedule) (ScheduleResult, error)
}

// Scheduler checks schedules on a 1-second ticker and fires due tasks through
// the Runner. All public methods are safe for concurrent use.
type Scheduler struct {
	mu     sync.Mutex
	runner Runner
	global *Store
	ws     *Store

	schedules []Schedule
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	running   map[string]bool
	onChange  func()
}

// NewScheduler creates a Scheduler. global store must be non-nil; ws may be nil.
func NewScheduler(runner Runner, global, ws *Store) *Scheduler {
	return &Scheduler{
		runner:  runner,
		global:  global,
		ws:      ws,
		running: map[string]bool{},
	}
}

// Start begins the ticker loop. Blocks until ctx is cancelled.
func (sc *Scheduler) Start(ctx context.Context) error {
	ctx, sc.cancel = context.WithCancel(ctx)
	defer sc.cancel()

	if err := sc.reload(); err != nil {
		return fmt.Errorf("scheduler: initial load: %w", err)
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case t := <-ticker.C:
			sc.checkAndFire(t)
		}
	}
}

// Stop cancels the scheduler loop and waits for in-flight executions.
func (sc *Scheduler) Stop() {
	if sc.cancel != nil {
		sc.cancel()
	}
	sc.wg.Wait()
}

func (sc *Scheduler) checkAndFire(now time.Time) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	for i := range sc.schedules {
		s := &sc.schedules[i]
		if !s.Due(now) {
			continue
		}
		if sc.running[s.ID] {
			continue
		}
		sc.running[s.ID] = true
		sched := *s
		sc.wg.Add(1)
		go func() {
			defer sc.wg.Done()
			sc.fire(sched)
		}()
	}
}

func (sc *Scheduler) fire(s Schedule) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	slog.Info("scheduler: firing", "id", s.ID, "name", s.Name)
	result, err := sc.runner.Run(ctx, s)

	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.running[s.ID] = false

	if err != nil {
		result = ScheduleResult{
			ID:         NewID(),
			ScheduleID: s.ID,
			ExecutedAt: time.Now().Unix(),
			Success:    false,
			Summary:    err.Error(),
		}
	}
	if werr := sc.global.SaveResult(result); werr != nil {
		slog.Error("scheduler: save result", "err", werr)
	}

	for i := range sc.schedules {
		if sc.schedules[i].ID == s.ID {
			sc.schedules[i].LastRunAt = result.ExecutedAt
			break
		}
	}
	targetStore := sc.global
	if s.Scope == "workspace" && sc.ws != nil {
		targetStore = sc.ws
	}
	_ = targetStore.Save(sc.schedulesForStore(s.Scope))

	if sc.onChange != nil {
		sc.onChange()
	}
	status := "ok"
	if !result.Success {
		status = "failed"
	}
	slog.Info("scheduler: fired", "id", s.ID, "status", status, "dur_ms", result.Duration)
}

func (sc *Scheduler) schedulesForStore(scope string) []Schedule {
	var out []Schedule
	for _, s := range sc.schedules {
		if s.Scope == scope {
			out = append(out, s)
		}
	}
	return out
}

// SetOnChange registers a callback invoked after every schedule fire or state
// change. The callback is called with the scheduler lock held; keep it short.
func (sc *Scheduler) SetOnChange(fn func()) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.onChange = fn
}

// ReloadWorkspace replaces the workspace store and re-reads all schedules.
func (sc *Scheduler) ReloadWorkspace(ws *Store) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.ws = ws
	sc.reloadLocked()
}

func (sc *Scheduler) reload() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.reloadLocked()
}

func (sc *Scheduler) reloadLocked() error {
	globalSchedules, err := sc.global.Load()
	if err != nil {
		return err
	}
	for i := range globalSchedules {
		globalSchedules[i].Scope = "global"
	}
	all := globalSchedules
	if sc.ws != nil {
		wsSchedules, err := sc.ws.Load()
		if err != nil {
			return err
		}
		for i := range wsSchedules {
			wsSchedules[i].Scope = "workspace"
		}
		all = append(all, wsSchedules...)
	}
	sc.schedules = all
	return nil
}

// AddSchedule persists a new schedule and reloads.
func (sc *Scheduler) AddSchedule(s Schedule) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	target := sc.global
	if s.Scope == "workspace" && sc.ws != nil {
		target = sc.ws
	}
	existing, _ := target.Load()
	existing = append(existing, s)
	if err := target.Save(existing); err != nil {
		return err
	}
	return sc.reloadLocked()
}

// RemoveSchedule deletes a schedule by ID.
func (sc *Scheduler) RemoveSchedule(id string) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for i, s := range sc.schedules {
		if s.ID == id {
			target := sc.global
			if s.Scope == "workspace" && sc.ws != nil {
				target = sc.ws
			}
			existing, _ := target.Load()
			for j, es := range existing {
				if es.ID == id {
					existing = append(existing[:j], existing[j+1:]...)
					break
				}
			}
			if err := target.Save(existing); err != nil {
				return err
			}
			sc.schedules = append(sc.schedules[:i], sc.schedules[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("schedule %q not found", id)
}

// ToggleSchedule enables or disables a schedule.
func (sc *Scheduler) ToggleSchedule(id string, enabled bool) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for i := range sc.schedules {
		if sc.schedules[i].ID == id {
			s := sc.schedules[i]
			target := sc.global
			if s.Scope == "workspace" && sc.ws != nil {
				target = sc.ws
			}
			existing, _ := target.Load()
			for j := range existing {
				if existing[j].ID == id {
					existing[j].Enabled = enabled
					break
				}
			}
			if err := target.Save(existing); err != nil {
				return err
			}
			sc.schedules[i].Enabled = enabled
			return nil
		}
	}
	return fmt.Errorf("schedule %q not found", id)
}

// ListSchedules returns a copy of all loaded schedules.
func (sc *Scheduler) ListSchedules() []Schedule {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	out := make([]Schedule, len(sc.schedules))
	copy(out, sc.schedules)
	return out
}

// ListResults returns execution results for a given schedule ID, newest first.
func (sc *Scheduler) ListResults(scheduleID string) ([]ScheduleResult, error) {
	m, err := sc.global.LoadResults()
	if err != nil {
		return nil, err
	}
	if results, ok := m[scheduleID]; ok {
		return results, nil
	}
	return nil, nil
}

// RunNow immediately runs a schedule by ID and records the result.
func (sc *Scheduler) RunNow(id string) (ScheduleResult, error) {
	sc.mu.Lock()
	var sched Schedule
	found := false
	for _, s := range sc.schedules {
		if s.ID == id {
			sched = s
			found = true
			break
		}
	}
	sc.mu.Unlock()
	if !found {
		return ScheduleResult{}, fmt.Errorf("schedule %q not found", id)
	}
	result, err := sc.runner.Run(context.Background(), sched)
	if err != nil {
		result = ScheduleResult{
			ID:         NewID(),
			ScheduleID: id,
			ExecutedAt: time.Now().Unix(),
			Success:    false,
			Summary:    err.Error(),
		}
	}
	sc.global.SaveResult(result)
	return result, nil
}
