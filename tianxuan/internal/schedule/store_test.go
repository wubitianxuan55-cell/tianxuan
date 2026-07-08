package schedule

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreSaveLoad(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)

	s := []Schedule{{
		ID:        "test-1",
		Name:      "daily check",
		Frequency: Daily,
		Time:      "08:00",
		Enabled:   true,
	}}

	if err := st.Save(s); err != nil {
		t.Fatal("save:", err)
	}

	got, err := st.Load()
	if err != nil {
		t.Fatal("load:", err)
	}
	if len(got) != 1 || got[0].Name != "daily check" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestStoreLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	got, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d", len(got))
	}
}

func TestStoreSaveResultTruncation(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	for i := 0; i < 25; i++ {
		r := ScheduleResult{ID: NewID(), ScheduleID: "s1", ExecutedAt: int64(i)}
		if err := st.SaveResult(r); err != nil {
			t.Fatal("save result:", err)
		}
	}
	m, err := st.LoadResults()
	if err != nil {
		t.Fatal(err)
	}
	if len(m["s1"]) != maxResultsPerSchedule {
		t.Fatalf("expected %d results after truncation, got %d", maxResultsPerSchedule, len(m["s1"]))
	}
}

func TestStoreAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, schedulesFile)
	if err := saveJSON(path, []Schedule{{ID: "pre"}}); err != nil {
		t.Fatal(err)
	}
	st := NewStore(dir)
	if err := st.Save([]Schedule{{ID: "post"}}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "post" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestDue(t *testing.T) {
	s := &Schedule{Frequency: Hourly, Enabled: true}
	now := time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC)
	if !s.Due(now) {
		t.Error("hourly should be due at minute 0 with no last run")
	}
	s.LastRunAt = now.Add(-30 * time.Minute).Unix()
	if s.Due(now) {
		t.Error("hourly should not be due if last run < 1 hour ago")
	}
}

func TestDueDaily(t *testing.T) {
	s := &Schedule{Frequency: Daily, Time: "08:00", Enabled: true}
	now := time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC)
	if !s.Due(now) {
		t.Error("daily should be due at specified time")
	}
	now = time.Date(2026, 7, 8, 8, 1, 0, 0, time.UTC)
	if s.Due(now) {
		t.Error("daily should not be due at wrong minute")
	}
}

func TestDueWeekly(t *testing.T) {
	s := &Schedule{Frequency: Weekly, Time: "09:00", DayOfWeek: 3, Enabled: true} // Wed
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC) // 2026-07-08 is a Wednesday
	if !s.Due(now) {
		t.Error("weekly should be due on correct day at correct time")
	}
	now = time.Date(2026, 7, 9, 9, 0, 0, 0, time.UTC) // Thursday
	if s.Due(now) {
		t.Error("weekly should not be due on wrong day")
	}
}

func TestDueDisabled(t *testing.T) {
	s := &Schedule{Frequency: Hourly, Enabled: false}
	now := time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC)
	if s.Due(now) {
		t.Error("disabled schedule should never be due")
	}
}

func TestDueBadTime(t *testing.T) {
	s := &Schedule{Frequency: Daily, Time: "bad", Enabled: true}
	now := time.Date(2026, 7, 8, 8, 0, 0, 0, time.UTC)
	if s.Due(now) {
		t.Error("schedule with bad time format should not be due")
	}
}
