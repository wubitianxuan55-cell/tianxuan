// Package proc provides cross-platform process utilities for the desktop app.
// (Design adopted from DeepSeek-Reasonix-V1.12)
package proc

import "os/exec"

// HideWindow stops a child process from flashing a console window on Windows.
// On non-Windows platforms this is a no-op.
func HideWindow(cmd *exec.Cmd) {
	hideWindow(cmd)
}
