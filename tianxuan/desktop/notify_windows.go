//go:build windows

package main

import (
	"sync"

	"git.sr.ht/~jackmordaunt/go-toast/v2"
)

var (
	toastInitOnce sync.Once
	toastInitErr  error
)

// ensureToastAppData 注册 Windows 通知的 AUMID（注册表全局状态，进程内只做一次）。
func ensureToastAppData() error {
	toastInitOnce.Do(func() {
		toastInitErr = toast.SetAppData(toast.AppData{
			AppID: "Tianxuan",
		})
	})
	return toastInitErr
}

// Notify 显示 Windows 系统通知——Codex 长任务完成提醒的蒸馏。
// 注册失败或推送失败都大声返回错误，由前端静默忽略（通知非关键路径）。
func (a *App) Notify(title, body string) error {
	if err := ensureToastAppData(); err != nil {
		return err
	}
	n := toast.Notification{
		AppID: "Tianxuan",
		Title: title,
		Body:  body,
	}
	return n.Push()
}
