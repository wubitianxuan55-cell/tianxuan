//go:build windows

package plugin

import (
	"os/exec"
	"syscall"
)

// hideProcessWindow 防止子进程在 Windows 上弹出控制台窗口
func hideProcessWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}
