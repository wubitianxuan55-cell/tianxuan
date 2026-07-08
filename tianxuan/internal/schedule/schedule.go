// Package schedule provides a desktop-session-local scheduler for timed AI agent
// tasks. It persists schedules and results to JSON sidecar files and fires the
// executor AgentRunner directly — no Hermes planner — for each due task.
package schedule

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Frequency is a preset schedule mode.
type Frequency string

const (
	Hourly Frequency = "hourly"
	Daily  Frequency = "daily"
	Weekly Frequency = "weekly"
)

// Schedule is one timed task definition.
type Schedule struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Prompt    string            `json:"prompt"`
	Frequency Frequency         `json:"frequency"`
	Time      string            `json:"time"`      // "HH:MM" e.g. "08:00"; empty for Hourly
	DayOfWeek int               `json:"dayOfWeek"` // 0=Sun … 6=Sat; -1 when not Weekly
	WorkDir   string            `json:"workDir"`
	Env       map[string]string `json:"env,omitempty"`
	Enabled   bool              `json:"enabled"`
	CreatedAt int64             `json:"createdAt"`
	LastRunAt int64             `json:"lastRunAt"` // 0 = never
	Scope     string            `json:"scope"`     // "global" | "workspace"
}

// ScheduleResult records one execution outcome.
type ScheduleResult struct {
	ID          string `json:"id"`
	ScheduleID  string `json:"scheduleId"`
	ExecutedAt  int64  `json:"executedAt"`
	Success     bool   `json:"success"`
	Summary     string `json:"summary"`     // one-line AI-generated summary
	SessionFile string `json:"sessionFile"` // relative path to JSONL archive
	Duration    int64  `json:"duration"`    // milliseconds
}

// NewID generates a short hex ID.
func NewID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Due reports whether the schedule is due at the given time.
func (s *Schedule) Due(now time.Time) bool {
	if !s.Enabled {
		return false
	}
	var h, m int
	if s.Time != "" {
		if _, err := fmt.Sscanf(s.Time, "%d:%d", &h, &m); err != nil {
			return false
		}
	}
	last := time.Unix(s.LastRunAt, 0)
	switch s.Frequency {
	case Hourly:
		if now.Minute() != 0 {
			return false
		}
		return last.IsZero() || now.Sub(last) >= time.Hour
	case Daily:
		if now.Hour() != h || now.Minute() != m {
			return false
		}
		return last.IsZero() || now.Sub(last) >= 23*time.Hour
	case Weekly:
		if now.Weekday() != time.Weekday(s.DayOfWeek) || now.Hour() != h || now.Minute() != m {
			return false
		}
		return last.IsZero() || now.Sub(last) >= 6*24*time.Hour
	}
	return false
}
