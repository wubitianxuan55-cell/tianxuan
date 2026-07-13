package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	goruntime "runtime"
	"runtime/debug"
	"strings"

	"tianxuan/internal/config"
	"tianxuan/internal/event"
)

// crash_pending.go captures Go-side panics to disk and ships them on the next
// launch. Frontend crashes are handled by the ErrorBoundary component; an
// unrecovered Go panic kills the process before the user can react, so the
// whole agent/provider/tool layer would otherwise never surface a single report.
// (Design adopted from DeepSeek-Reasonix-V1.12)

const pendingCrashFile = "crash-pending.json"

func pendingCrashPath() string {
	return filepath.Join(config.MemoryUserDir(), pendingCrashFile)
}

// recoverToPending records a panicking goroutine to the pending-crash file and
// re-raises, so the process still crashes — but the stack is shipped next launch.
func (a *App) recoverToPending(site string) {
	r := recover()
	if r == nil {
		return
	}
	writePendingCrash(site, r, debug.Stack())
	panic(r)
}

func writePendingCrash(site string, r any, stack []byte) {
	stackText := string(stack)
	msg := fmt.Sprintf("[go panic] %s: %v\n\n%s", site, r, stackText)
	report := map[string]any{
		"kind":     "crash",
		"source":   "go",
		"site":     site,
		"error":    fmt.Sprintf("%T: %v", r, r),
		"stack":    stackText,
		"message":  msg,
		"version":  version,
		"os":       goruntime.GOOS,
		"arch":     goruntime.GOARCH,
	}
	body, err := json.Marshal(report)
	if err != nil {
		return
	}
	path := pendingCrashPath()
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	_ = os.WriteFile(path, body, 0o644)
}

// goSafe runs fn in a new goroutine with crash recovery.
// Caught panics are written to crash-pending.json and re-raised, killing the
// process — use only for critical goroutines whose failure is fatal
// (buildController, tray, scheduler).
func (a *App) goSafe(site string, fn func()) {
	go func() {
		defer a.recoverToPending(site)
		fn()
	}()
}

// goSafeQuiet runs fn in a new goroutine with crash recovery that does NOT
// re-panic. Panics are logged via slog and written to crash-pending.json for
// diagnostics, but the process stays alive. Use for auxiliary goroutines
// (mobile HTTP server, ngrok polling, etc.) whose failure should not kill the app.
func (a *App) goSafeQuiet(site string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("desktop: goroutine panic (non-fatal)", "site", site, "panic", r)
				writePendingCrash(site, r, debug.Stack())
			}
		}()
		fn()
	}()
}

// flushPendingCrash drains a Go panic captured on a prior run and logs it, then
// clears it. Called at startup so a crash never sits on disk unnoticed.
func (a *App) flushPendingCrash() {
	if version == "dev" {
		return
	}
	path := pendingCrashPath()
	body, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = os.Remove(path)
	// Log and surface to the user so they know what happened.
	slog.Error("desktop: recovered crash from prior session", "path", path)
	bodyStr := string(body)
	println("[crash] recovered from prior session:", bodyStr)
	// If we have a sink, emit a notice so the user sees it in the UI.
	if a.sink != nil {
		a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
			Text: "检测到上一次会话崩溃。截取部分堆栈：" + crashSummary(bodyStr)})
	}
}

// crashSummary extracts the first meaningful line from a crash dump.
func crashSummary(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "goroutine ") {
			continue
		}
		if strings.Contains(t, "panic(") || strings.Contains(t, ".go:") {
			if len(t) > 200 { t = t[:197] + "..." }
			return t
		}
	}
	return "(无法解析崩溃堆栈)"
}
