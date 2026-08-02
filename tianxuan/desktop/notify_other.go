//go:build !windows

package main

import "errors"

// Notify 仅 Windows 支持（go-toast 基于 WinRT）；其他平台大声报错。
func (a *App) Notify(_title, _body string) error {
	return errors.New("desktop notifications are only supported on Windows")
}
