//go:build windows

package main

import "golang.org/x/sys/windows"

func init() {
	// Suppress Windows critical-error and GPFault dialogs that would
	// block the process until a human clicks OK. Without this, any
	// exec.Command that references a missing executable — a misconfigured
	// MCP server, a stale LSP binary path, or a shell probe — triggers a
	// system-modal "Windows cannot find 'xxx'" popup that freezes the
	// agent until the user manually dismisses it.
	//
	// SEM_FAILCRITICALERRORS: the OS returns the error to the process
	//   instead of showing a critical-error message box.
	// SEM_NOGPFAULTERRORBOX:   the OS does not display the
	//   general-protection-fault message box (32-bit only; harmless on
	//   x64 — kept for consistency with standard practice).
	windows.SetErrorMode(windows.SEM_FAILCRITICALERRORS | windows.SEM_NOGPFAULTERRORBOX)
}
