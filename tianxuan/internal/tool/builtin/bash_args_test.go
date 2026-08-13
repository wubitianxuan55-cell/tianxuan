package builtin

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"tianxuan/internal/sandbox"
)

// longSleepCommand is a command that runs for ~2s, used to prove a foreground
// timeout actually fired (any host shell).
func longSleepCommand(sh sandbox.Shell) string {
	if sh.Kind == sandbox.ShellPowerShell {
		return "Start-Sleep -Seconds 2"
	}
	return "sleep 2"
}

// TestBashConfiguredTimeoutApplies locks the host-injected timeout (aligned
// with Reasonix): b.timeout > 0 caps the foreground command.
func TestBashConfiguredTimeoutApplies(t *testing.T) {
	sh := resolvedTestShell(t)
	b := bash{shell: sh, timeout: 150 * time.Millisecond}

	start := time.Now()
	_, err := b.Execute(context.Background(), argsJSON(t, map[string]any{"command": longSleepCommand(sh)}))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("configured timeout returned too slowly: %v", elapsed)
	}
}

// TestBashZeroTimeoutNoLocalCap locks timeout=0 semantics: no tool-local cap,
// only the parent context can cancel (aligned with Reasonix).
func TestBashZeroTimeoutNoLocalCap(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real shell; skipped under -short")
	}
	sh := resolvedTestShell(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := "sleep 1; printf done"
	if sh.Kind == sandbox.ShellPowerShell {
		cmd = "Start-Sleep -Seconds 1; Write-Output done"
	}
	out, err := (bash{shell: sh, timeout: 0}).Execute(ctx, argsJSON(t, map[string]any{"command": cmd}))
	if err != nil {
		t.Fatalf("zero-timeout foreground command failed: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "done") {
		t.Fatalf("output = %q, want done", out)
	}
}

// TestBashRejectsUnknownArgs locks strict argument parsing: schema-unknown
// fields (timeout, timeout_seconds, description, cwd, ...) must fail loudly
// instead of being silently dropped, and the error must name valid fields.
func TestBashRejectsUnknownArgs(t *testing.T) {
	b := bash{shell: resolvedTestShell(t)}
	for _, args := range []map[string]any{
		{"command": "echo hi", "timeout": 30},
		{"command": "echo hi", "timeout_seconds": 30},
		{"command": "echo hi", "timeout_ms": 5000},
		{"command": "echo hi", "description": "run tests"},
		{"command": "echo hi", "cwd": "/tmp"},
	} {
		_, err := b.Execute(context.Background(), argsJSON(t, args))
		if err == nil {
			t.Fatalf("args %v should be rejected", args)
		}
		if !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("args %v err = %v, want unknown-field error", args, err)
		}
		if !strings.Contains(err.Error(), "command") {
			t.Fatalf("args %v err = %v, want valid-fields hint", args, err)
		}
	}
}

// TestConfineBashInjectsTimeout locks ConfineBash's variadic timeout binding:
// the composed bash tool actually applies the host-configured cap.
func TestConfineBashInjectsTimeout(t *testing.T) {
	sh := resolvedTestShell(t)
	confined := ConfineBash(sandbox.Spec{}, WithBashTimeout(150*time.Millisecond))

	start := time.Now()
	_, err := confined.Execute(context.Background(), argsJSON(t, map[string]any{"command": longSleepCommand(sh)}))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected injected timeout to fire, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("injected timeout returned too slowly: %v", elapsed)
	}
}

// TestBashCommandEnvInjectsPath locks the environment-injection contract: a
// configured PATH entry replaces the inherited PATH (nothing leaks, nothing
// duplicated), so the model never has to probe for go/node locations.
func TestBashCommandEnvInjectsPath(t *testing.T) {
	b := bash{env: []string{"PATH=" + string(os.PathListSeparator) + "C:\\tools"}}
	env := b.commandEnv()
	if len(env) == 0 {
		t.Fatal("commandEnv must return the merged environment when env is set")
	}
	gotPath := ""
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if ok && strings.EqualFold(k, "PATH") {
			gotPath = v
		}
	}
	if !strings.Contains(gotPath, "C:\\tools") {
		t.Fatalf("commandEnv PATH = %q, want injected tools dir", gotPath)
	}
	// The inherited PATH must not survive separately: exactly one PATH entry.
	pathCount := 0
	for _, kv := range env {
		k, _, ok := strings.Cut(kv, "=")
		if ok && strings.EqualFold(k, "PATH") {
			pathCount++
		}
	}
	if pathCount != 1 {
		t.Fatalf("expected exactly one PATH entry, got %d", pathCount)
	}
}

// TestBashCommandEnvNilWhenUnset: the zero-value bash keeps inheriting the
// process environment (nil Env) — the cheapest common path.
func TestBashCommandEnvNilWhenUnset(t *testing.T) {
	var b bash
	if env := b.commandEnv(); env != nil {
		t.Fatalf("unset env must stay nil (inherit), got %v", env)
	}
}
