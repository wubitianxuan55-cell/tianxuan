package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tianxuan/internal/config"
)

// window_state.go saves/restores the window geometry across launches.
// The frontend calls SaveWindowState periodically and before quit;
// the Go side calls saveWindowStateSync during shutdown as a fallback.
// (Design adopted from DeepSeek-Reasonix-V1.12)

// DesktopWindowState captures the window geometry to restore across launches.
type DesktopWindowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Maximised bool `json:"maximised"`
}

func windowStatePath() string {
	return filepath.Join(config.MemoryUserDir(), "desktop-window.json")
}

// loadWindowState reads the saved window geometry. Returns false when no saved
// state exists (first launch, missing file, corrupt JSON).
func loadWindowState() (DesktopWindowState, bool) {
	path := windowStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return DesktopWindowState{}, false
	}
	var s DesktopWindowState
	if err := json.Unmarshal(data, &s); err != nil {
		return DesktopWindowState{}, false
	}
	if s.Width < 400 {
		s.Width = 0
	}
	if s.Height < 300 {
		s.Height = 0
	}
	// Migration guard: treat all-zero as missing (first launch).
	if s.Width == 0 && s.Height == 0 && s.X == 0 && s.Y == 0 {
		return DesktopWindowState{}, false
	}
	return s, true
}

// SaveWindowState persists the current window geometry. Called by the frontend.
func (a *App) SaveWindowState(state DesktopWindowState) error {
	path := windowStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// saveWindowStateSync saves the current window geometry from the Go side.
// Called during shutdown so the last-known state is persisted even if the
// frontend's beforeunload hasn't resolved yet.
//
// Known Wails v2.12.0 boundary: the shutdown callback fires while the
// underlying Win32 window is tearing down. At that point WindowGetSize /
// WindowGetPosition can hit a divide-by-zero in winc's DPI scaling (DPI
// denominator may already be zero), so we guard with recover.
func (a *App) saveWindowStateSync() {
	if a.ctx == nil {
		return
	}
	// Context already cancelled is a strong signal that shutdown is past the
	// point where the window handle is valid — bail out early.
	if a.ctx.Err() != nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("saveWindowStateSync: window already torn down, skipping geometry save",
				"panic", r)
		}
	}()
	var w, h, x, y int
	var max bool
	func() {
		defer func() { recover() }()
		w, h = runtime.WindowGetSize(a.ctx)
	}()
	func() {
		defer func() { recover() }()
		x, y = runtime.WindowGetPosition(a.ctx)
	}()
	func() {
		defer func() { recover() }()
		max = runtime.WindowIsMaximised(a.ctx)
	}()
	_ = a.SaveWindowState(DesktopWindowState{
		Width:     w,
		Height:    h,
		X:         x,
		Y:         y,
		Maximised: max,
	})
}
