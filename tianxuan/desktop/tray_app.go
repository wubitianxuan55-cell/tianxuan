// tray_app.go — system tray methods bound to the Wails App.
// These are separate from app.go to keep the tray concern isolated.

package main

import (
	"context"
	"log/slog"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// beforeClose hides the window instead of quitting when the user clicks X.
// Returns false to prevent Wails from closing the window.
// Only quitting=true (set by QuitApp) allows a real close.
func (a *App) beforeClose(ctx context.Context) bool {
	if quitting {
		return true
	}
	runtime.WindowHide(ctx)
	return false
}

// QuitApp sets the quit flag and shuts down the tray. The actual session
// snapshot + controller close happens in OnShutdown, which is triggered by
// runtime.Quit(ctx) from the tray's onExit callback. This avoids double-cleanup.
// Callable from both the tray menu and the frontend.
func (a *App) QuitApp() {
	quitting = true
	slog.Info("tray: quit requested via QuitApp")
	systray.Quit()
}

// ShowWindow restores and focuses the main window from the tray.
// Callable from both the tray menu and the frontend.
func (a *App) ShowWindow() {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}
