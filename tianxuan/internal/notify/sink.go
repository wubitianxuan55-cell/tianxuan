package notify

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"tianxuan/internal/event"
)

// TurnDoneSink wraps an event.Sink and emits a desktop notification when a
// turn completes. Use it to alert the user when the agent finishes a long
// background task.
type TurnDoneSink struct {
	inner event.Sink
	// MinDuration suppresses notifications for turns shorter than this.
	// Default 5 seconds — quick turns don't need a popup.
	MinDuration time.Duration
	startedAt   time.Time
	turnActive  bool
}

// NewTurnDoneSink creates a notification sink wrapping inner. Notifications
// fire only when a turn runs longer than minDur (default 5s when 0).
func NewTurnDoneSink(inner event.Sink, minDur time.Duration) *TurnDoneSink {
	if minDur <= 0 {
		minDur = 5 * time.Second
	}
	return &TurnDoneSink{
		inner:       inner,
		MinDuration: minDur,
	}
}

func (s *TurnDoneSink) Emit(e event.Event) {
	switch e.Kind {
	case event.TurnStarted:
		s.startedAt = time.Now()
		s.turnActive = true
	case event.TurnDone:
		if s.turnActive && time.Since(s.startedAt) > s.MinDuration {
			title := "tianxuan"
			body := "Turn completed"
			if e.Err != nil {
				body = fmt.Sprintf("Turn failed: %s", e.Err)
			}
			_ = Send(title, body)
		}
		s.turnActive = false
	}
	s.inner.Emit(e)
}

// Supported reports whether desktop notifications are available on the current
// platform (have a known mechanism and the required binary is on PATH).
func Supported() bool {
	switch runtime.GOOS {
	case "darwin":
		return hasBinary("osascript")
	case "linux":
		return hasBinary("notify-send")
	case "windows":
		return hasBinary("powershell")
	}
	return false
}

func hasBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// execLookPath is overridable in tests.
var execLookPath = exec.LookPath
