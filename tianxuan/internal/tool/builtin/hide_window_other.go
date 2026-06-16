//go:build !windows

package builtin

import (
	"os/exec"
	"syscall"
)

// hideBashWindow 在非 Windows 平台为空操作
func hideBashWindow(cmd *exec.Cmd) {}

// assignToJobObject 在非 Windows 平台不支持，返回错误。
// 调用方应 fallback 到 killProcessTree。
func assignToJobObject(cmd *exec.Cmd) (syscall.Handle, error) {
	return 0, syscall.ENOSYS
}
