package schedule

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const schedulesFile = "schedules.json"
const resultsFile = "schedule-results.json"
const maxResultsPerSchedule = 20

// Store manages JSON file persistence for schedules and results.
// Safe for concurrent use.
type Store struct {
	mu  sync.Mutex
	dir string
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

func (st *Store) Load() ([]Schedule, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return loadSchedules(filepath.Join(st.dir, schedulesFile))
}

func (st *Store) Save(schedules []Schedule) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	return saveJSON(filepath.Join(st.dir, schedulesFile), schedules)
}

func (st *Store) LoadResults() (map[string][]ScheduleResult, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return loadResults(filepath.Join(st.dir, resultsFile))
}

func (st *Store) SaveResult(r ScheduleResult) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	p := filepath.Join(st.dir, resultsFile)
	m, err := loadResults(p)
	if err != nil {
		return err
	}
	if m == nil {
		m = map[string][]ScheduleResult{}
	}
	results := append(m[r.ScheduleID], r)
	if len(results) > maxResultsPerSchedule {
		results = results[len(results)-maxResultsPerSchedule:]
	}
	m[r.ScheduleID] = results
	return saveJSON(p, m)
}

func loadSchedules(path string) ([]Schedule, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s []Schedule
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return s, nil
}

func loadResults(path string) (map[string][]ScheduleResult, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var m map[string][]ScheduleResult
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

func saveJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sched-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
