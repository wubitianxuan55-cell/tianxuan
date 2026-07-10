package main

import (
	"errors"
	"log/slog"
	"syscall"
)

// Sanitized session-file errors surfaced to the UI. They intentionally carry no
// path so raw OS error text never reaches the frontend.
// Ported from DeepSeek-Reasonix V1.17.10.
var (
	errSessionFileLocked       = errors.New("a session file is temporarily locked by another program (often antivirus or sync tools) — wait a moment and retry")
	errSessionFileAccessDenied = errors.New("access to a session file was denied — close programs that may be using it or check folder permissions, then retry")
	errSessionDiskFull         = errors.New("not enough disk space to finish the operation — free some space and retry")
)

// friendlySessionFileError rewrites raw OS-level filesystem errors from session
// file operations into the actionable, path-free errors above. The original
// error is logged so diagnostics keep the path and errno. Unrecognized errors
// pass through unchanged.
func friendlySessionFileError(err error) error {
	if err == nil {
		return nil
	}
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return err
	}
	var friendly error
	switch {
	case isFileInUseErrno(errno):
		friendly = errSessionFileLocked
	case isAccessDeniedErrno(errno):
		friendly = errSessionFileAccessDenied
	case isDiskFullErrno(errno):
		friendly = errSessionDiskFull
	default:
		return err
	}
	slog.Warn("desktop: session file operation blocked", "err", err)
	return friendly
}
