package hook

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// processAlive reports whether the process with the given PID is running.
func processAlive(t *testing.T, pid string) bool {
	t.Helper()
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %s", pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	// tasklist CSV output contains the PID column when the process exists.
	return strings.Contains(string(out), fmt.Sprintf("\"%s\"", pid))
}

// TestDefaultSpawnerKillsGrandchildOnTimeout is the regression test for the
// orphaned-grandchild gap: when a hook command times out, the direct child
// (the shell) is killed but a grandchild the hook spawned (e.g. a lint tool
// chain) used to survive, holding file locks / ports / pipes. The fix routes
// DefaultSpawner through proc.StartTracked + KillTracked so the whole tree is
// reaped (Windows Job Object + taskkill /T fallback).
func TestDefaultSpawnerKillsGrandchildOnTimeout(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("spawns a Windows PowerShell grandchild")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")
	// The hook spawns a hidden grandchild (Start-Sleep 60), records its PID,
	// then sleeps in the foreground long enough to hit the timeout. The
	// grandchild must NOT survive the timeout. A .ps1 file avoids cmd /c
	// quote-escaping of the PowerShell $variable syntax.
	script := filepath.Join(dir, "spawn.ps1")
	os.WriteFile(script, []byte(
		"param([string]$pidFile)\n"+
			"$p = Start-Process -WindowStyle Hidden -PassThru powershell -ArgumentList '-NoProfile','-Command','Start-Sleep 60'\n"+
			"$p.Id | Out-File -Encoding ascii $pidFile\n"+
			"Start-Sleep 30\n",
	), 0644)
	cmd := fmt.Sprintf("powershell -NoProfile -ExecutionPolicy Bypass -File %s %s", script, pidFile)

	r := DefaultSpawner(context.Background(), SpawnInput{Command: cmd, Timeout: 1200 * time.Millisecond})
	if !r.TimedOut {
		t.Fatalf("expected timeout, got %+v", r)
	}
	// Give the tree-kill a moment to reap the grandchild.
	deadline := time.Now().Add(3 * time.Second)
	var pid string
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(pidFile)
		if err == nil {
			pid = strings.TrimSpace(string(b))
			if pid != "" {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pid == "" {
		t.Skipf("grandchild pid file %s never appeared (hook startup raced timeout); cannot assert", pidFile)
	}
	if processAlive(t, pid) {
		t.Errorf("grandchild pid %s survived hook timeout — process tree was not reaped", pid)
	}
}
