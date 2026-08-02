//go:build !windows

package main

import "errors"

// OpenBrowserWindow 仅 Windows 支持（go-webview2 的 Win32 后端）；其他平台大声报错。
func (a *App) OpenBrowserWindow(_url string) error {
	return errors.New("browser window is only supported on Windows")
}
