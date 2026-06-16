//go:build !windows

package plugin

import "os/exec"

// hideProcessWindow is a no-op on non-Windows platforms
func hideProcessWindow(_ *exec.Cmd) {}
