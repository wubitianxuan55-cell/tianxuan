// tray.go — system tray icon + menu for tianxuan desktop.
// Uses getlantern/systray to create a taskbar notification area icon.
// The tray provides: Show/Hide window, schedule controls, Quit.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"time"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tianxuan/internal/crash"
)

// trayIconICO is the tray icon in ICO format (multi-size: 16/32/48).
//
//go:embed tray_icon.ico
var trayIconICO []byte

var (
	quitting = false
)

// runTray initializes the system tray with a "T" icon and Show/Quit menu.
// Must be called in a goroutine from App.startup() with the Wails context.
// systray.Run is blocking; it returns when systray.Quit() is called.
func runTray(ctx context.Context, app *App) {
	systray.Run(
		func() {
			systray.SetIcon(trayIconICO)
			systray.SetTitle("tianxuan")
			systray.SetTooltip("tianxuan — AI Coding Agent")

			showItem := systray.AddMenuItem("显示 tianxuan", "恢复主窗口")
			systray.AddSeparator()

			// —— 定时任务子菜单 ——
			schedItem := systray.AddMenuItem("定时任务: --", "定时任务状态")
			pauseAllItem := schedItem.AddSubMenuItem("暂停全部", "暂停所有定时任务")
			resumeAllItem := schedItem.AddSubMenuItem("恢复全部", "恢复所有定时任务")
			systray.AddSeparator()
			// —— 定时任务子菜单结束 ——

			quitItem := systray.AddMenuItem("退出", "完全退出 tianxuan")

			// Update schedule title periodically
			go func() {
				defer crash.Recover("tray-schedule-title")
				updateScheduleTitle(schedItem, app)
				ticker := time.NewTicker(5 * time.Second)
				defer ticker.Stop()
				for range ticker.C {
					if quitting {
						return
					}
					updateScheduleTitle(schedItem, app)
				}
			}()

			go func() {
				defer crash.Recover("tray-event-loop")
				for {
					select {
					case <-showItem.ClickedCh:
						runtime.WindowShow(ctx)
						runtime.WindowUnminimise(ctx)
					case <-pauseAllItem.ClickedCh:
						toggleAllSchedules(app, false)
						updateScheduleTitle(schedItem, app)
					case <-resumeAllItem.ClickedCh:
						toggleAllSchedules(app, true)
						updateScheduleTitle(schedItem, app)
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
			slog.Info("tray: exiting, requesting graceful shutdown")
			runtime.Quit(ctx)
		},
	)
}

func updateScheduleTitle(item *systray.MenuItem, app *App) {
	if app == nil || app.scheduler == nil {
		item.SetTitle("定时任务: --")
		item.Disable()
		return
	}
	schedules := app.scheduler.ListSchedules()
	total := len(schedules)
	enabled := 0
	for _, s := range schedules {
		if s.Enabled {
			enabled++
		}
	}
	item.SetTitle(fmt.Sprintf("定时任务 (%d/%d)", enabled, total))
	if total == 0 {
		item.Disable()
	}
}

func toggleAllSchedules(app *App, enabled bool) {
	if app == nil || app.scheduler == nil {
		return
	}
	schedules := app.scheduler.ListSchedules()
	for _, s := range schedules {
		_ = app.scheduler.ToggleSchedule(s.ID, enabled)
	}
}
