// Package crash captures Go panics, writes a breadcrumb trail to a local log
// file, and optionally re-raises so the process exits with a core dump. It is
// the last line of defence — every panic should be a bug, not a user-facing
// crash with no forensic trail.
//
// Usage in main.go:
//
//	defer crash.Handle()
//
// Or with a custom log directory:
//
//	defer crash.Handle(crash.WithLogDir("/tmp/tianxuan-crashes"))
package crash

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
)

// DefaultLogDir is the directory where crash logs are written. It may be
// overridden by WithLogDir or by the TIANXUAN_CRASH_LOG_DIR env var.
const DefaultLogDir = ".tianxuan"

// Options configures the crash handler.
type Options struct {
	LogDir string
	// ExitOnPanic controls whether Handle re-panics after logging so the
	// process exits (true, default) or swallows the panic for cleanup.
	ExitOnPanic bool
	// Stderr is the writer for the "panic written to file" message.
	// When nil, os.Stderr is used.
	Stderr *os.File
}

// Option is a functional option for New.
type Option func(*Options)

// WithLogDir overrides the default crash log directory.
func WithLogDir(dir string) Option {
	return func(o *Options) {
		o.LogDir = dir
	}
}

// WithExitOnPanic controls whether Handle re-panics after logging.
func WithExitOnPanic(v bool) Option {
	return func(o *Options) {
		o.ExitOnPanic = v
	}
}

// Handle recovers from a panic, writes a crash dump to a log file, and
// re-panics (by default) so the process still exits nonzero.
//
// Use as the first defer in main:
//
//	func main() {
//	    defer crash.Handle()
//	    ...
//	}
func Handle(opts ...Option) {
	options := &Options{
		LogDir:      DefaultLogDir,
		ExitOnPanic: true,
		Stderr:      os.Stderr,
	}
	for _, o := range opts {
		o(options)
	}
	if r := recover(); r != nil {
		writeCrashLog(options, r, debug.Stack())
	}
}

// Recover is a lightweight variant that logs the panic to stderr only (no file)
// and does NOT re-panic. Use it in goroutine defers where a full crash log is
// disproportionate and goroutine death should be silent.
//
//	go func() {
//	    defer crash.Recover("background-indexer")
//	    ...
//	}()
func Recover(goroutineLabel string) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		msg := fmt.Sprintf("=== RECOVERED panic in goroutine [%s] ===\n%v\n\n%s\n",
			goroutineLabel, r, stack)
		fmt.Fprint(os.Stderr, msg)
	}
}

func writeCrashLog(opts *Options, r any, stack []byte) {
	ts := time.Now().UTC().Format("20060102T150405.000000Z")
	hostname, _ := os.Hostname()
	wd, _ := os.Getwd()

	var b strings.Builder
	b.WriteString("=" + strings.Repeat("=", 70) + "\n")
	b.WriteString(fmt.Sprintf("  Tianxuan Crash Report — %s\n", ts))
	b.WriteString("=" + strings.Repeat("=", 70) + "\n\n")
	b.WriteString(fmt.Sprintf("Host:     %s\n", hostname))
	b.WriteString(fmt.Sprintf("CWD:      %s\n", wd))
	b.WriteString(fmt.Sprintf("PID:      %d\n", os.Getpid()))
	b.WriteString(fmt.Sprintf("Args:     %s\n", strings.Join(os.Args, " ")))
	if info, ok := debug.ReadBuildInfo(); ok {
		b.WriteString(fmt.Sprintf("GoVersion: %s\n", info.GoVersion))
		b.WriteString(fmt.Sprintf("BuildInfo:\n"))
		b.WriteString(fmt.Sprintf("  Module:  %s\n", info.Main.Path))
		b.WriteString(fmt.Sprintf("  Version: %s\n", info.Main.Version))
	}
	b.WriteString("\n--- Panic ---\n")
	b.WriteString(fmt.Sprintf("%+v\n\n", r))

	b.WriteString("--- Stack Trace ---\n")
	b.Write(stack)
	b.WriteByte('\n')

	b.WriteString("=" + strings.Repeat("=", 70) + "\n")

	// Try to write to a structured crash dump file first
	logDir := opts.LogDir
	if e := os.Getenv("TIANXUAN_CRASH_LOG_DIR"); e != "" {
		logDir = e
	}
	logFile := filepath.Join(logDir, fmt.Sprintf("crash-%s-%d.log", ts, os.Getpid()))
	if err := os.MkdirAll(logDir, 0o755); err == nil {
		if writeErr := os.WriteFile(logFile, []byte(b.String()), 0o644); writeErr == nil {
			fmt.Fprintf(opts.Stderr, "\n⚠ tianxuan panic: crash log written to %s\n", logFile)
			fmt.Fprintln(opts.Stderr, string(stack[:min(len(stack), 500)]))
			_ = opts.Stderr.Sync()
			if opts.ExitOnPanic {
				panic(r)
			}
			return
		}
	}
	// Fallback: just dump to stderr
	fmt.Fprintln(opts.Stderr, b.String())
	if opts.ExitOnPanic {
		panic(r)
	}
}
