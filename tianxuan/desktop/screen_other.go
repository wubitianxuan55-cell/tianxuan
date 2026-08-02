//go:build !windows

package main

import "errors"

// CaptureScreen 仅 Windows 支持；其他平台大声报错而非静默返回空图。
func (a *App) CaptureScreen() (string, error) {
	return "", errors.New("capture screen is only supported on Windows")
}
