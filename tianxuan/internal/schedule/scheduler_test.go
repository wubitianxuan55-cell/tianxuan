package schedule

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeRunner struct {
	mu      sync.Mutex
	results []ScheduleResult
}

func (f *fakeRunner) Run(_ context.Context, s Schedule) (ScheduleResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := ScheduleResult{
		ID:         NewID(),
		ScheduleID: s.ID,
		ExecutedAt: time.Now().Unix(),
		Success:    true,
		Summary:    "ok from " + s.Name,
		Duration:   100,
	}
	f.results = append(f.results, r)
	return r, nil
}

func TestSchedulerFiresDue(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{}
	global := NewStore(dir)
	sc := NewScheduler(runner, global, nil)

	s := Schedule{
		ID:        "test-1",
		Name:      "test",
		Frequency: Hourly,
		Enabled:   true,
		CreatedAt: time.Now().Unix(),
	}
	if err := sc.AddSchedule(s); err != nil {
		t.Fatal("add:", err)
	}

	sc.checkAndFire(time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC))
	time.Sleep(50 * time.Millisecond)
	sc.wg.Wait()

	runner.mu.Lock()
	count := len(runner.results)
	runner.mu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 fire, got %d", count)
	}
}

func TestSchedulerListSchedules(t *testing.T) {
	dir := t.TempDir()
	global := NewStore(dir)
	sc := NewScheduler(nil, global, nil)
	_ = sc.AddSchedule(Schedule{ID: "a", Frequency: Daily, Time: "08:00"})
	_ = sc.AddSchedule(Schedule{ID: "b", Frequency: Weekly, Time: "09:00", DayOfWeek: 1})

	list := sc.ListSchedules()
	if len(list) != 2 {
		t.Fatalf("expected 2 schedules, got %d", len(list))
	}
}

func TestSchedulerToggle(t *testing.T) {
	dir := t.TempDir()
	global := NewStore(dir)
	sc := NewScheduler(nil, global, nil)
	_ = sc.AddSchedule(Schedule{ID: "t", Frequency: Daily, Time: "08:00", Enabled: true})
	_ = sc.ToggleSchedule("t", false)
	list := sc.ListSchedules()
	if len(list) != 1 || list[0].Enabled {
		t.Fatal("expected disabled")
	}
	_ = sc.ToggleSchedule("t", true)
	list = sc.ListSchedules()
	if !list[0].Enabled {
		t.Fatal("expected enabled")
	}
}

func TestSchedulerRemove(t *testing.T) {
	dir := t.TempDir()
	global := NewStore(dir)
	sc := NewScheduler(nil, global, nil)
	_ = sc.AddSchedule(Schedule{ID: "r", Frequency: Daily, Time: "08:00"})
	_ = sc.RemoveSchedule("r")
	if len(sc.ListSchedules()) != 0 {
		t.Fatal("expected empty after remove")
	}
}

func TestSchedulerNoFireWhenRunning(t *testing.T) {
	dir := t.TempDir()
	// Use a runner that blocks to simulate in-flight execution
	runner := &blockingRunner{block: make(chan struct{})}
	global := NewStore(dir)
	sc := NewScheduler(runner, global, nil)

	s := Schedule{ID: "b1", Name: "blocker", Frequency: Hourly, Enabled: true}
	_ = sc.AddSchedule(s)

	// First fire starts and blocks
	sc.checkAndFire(time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC))
	time.Sleep(20 * time.Millisecond)

	// Second fire should NOT start a new goroutine (already running)
	sc.checkAndFire(time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC))

	// Only 1 goroutine should be running
	if runner.callCount() != 1 {
		t.Fatalf("expected 1 call, got %d", runner.callCount())
	}

	// Unblock
	close(runner.block)
	sc.wg.Wait()
}

type blockingRunner struct {
	mu         sync.Mutex
	block      chan struct{}
	callCount_ int
}

func (b *blockingRunner) Run(_ context.Context, s Schedule) (ScheduleResult, error) {
	b.mu.Lock()
	b.callCount_++
	b.mu.Unlock()
	<-b.block
	return ScheduleResult{ID: NewID(), ScheduleID: s.ID, Success: true}, nil
}

func (b *blockingRunner) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.callCount_
}
