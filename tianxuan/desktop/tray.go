// tray.go — system tray icon + menu for tianxuan desktop.
// Uses getlantern/systray to create a taskbar notification area icon.
// The tray provides: Show/Hide window, Quit.
package main

import (
	"context"
	_ "embed"
	"log/slog"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// trayIcon16 is a 16x16 purple "T" icon in PNG format.
//
//go:embed tray_icon.png
var trayIconPNG []byte

var (
	quitting = false
)

// runTray initializes the system tray with a "T" icon and Show/Quit menu.
// Must be called in a goroutine from App.startup() with the Wails context.
// systray.Run is blocking; it returns when systray.Quit() is called.
func runTray(ctx context.Context) {
	systray.Run(
		func() {
			// onReady — runs in a new goroutine after systray initializes.
			systray.SetIcon(trayIconPNG)
			systray.SetTitle("tianxuan")
			systray.SetTooltip("tianxuan — AI Coding Agent")

			showItem := systray.AddMenuItem("显示 tianxuan", "恢复主窗口")
			systray.AddSeparator()
			quitItem := systray.AddMenuItem("退出", "完全退出 tianxuan")

			// Handle menu clicks in a separate goroutine (systray requires it).
			go func() {
				for {
					select {
					case <-showItem.ClickedCh:
						runtime.WindowShow(ctx)
						runtime.WindowUnminimise(ctx)
					case <-quitItem.ClickedCh:
						slog.Info("tray: quit requested")
						quitting = true
						systray.Quit()
						return
					}
				}
			}()
		},
		func() {
			// onExit — called when systray.Quit() is invoked or the tray is destroyed.
			// Use runtime.Quit to trigger Wails' graceful shutdown (OnShutdown →
			// snapshot + ctrl.Close) instead of os.Exit which would skip cleanup.
			slog.Info("tray: exiting, requesting graceful shutdown")
			runtime.Quit(ctx)
		},
	)
}
