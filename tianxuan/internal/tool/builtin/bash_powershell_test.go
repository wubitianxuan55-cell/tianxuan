package builtin

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"tianxuan/internal/sandbox"
)

func powershellPath(t *testing.T) string {
	t.Helper()
	for _, n := range []string{"pwsh", "powershell"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	t.Skip("no PowerShell on PATH")
	return ""
}

func runPS(t *testing.T, command string) (string, error) {
	t.Helper()
	b := bash{shell: sandbox.Shell{Kind: sandbox.ShellPowerShell, Path: powershellPath(t)}}
	args, _ := json.Marshal(map[string]string{"command": command})
	return b.Execute(context.Background(), args)
}

func TestBashPowerShellRunsNativeCommand(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("powershell e2e is windows-only")
	}
	out, err := runPS(t, "Write-Output tianxuan-ok")
	if err != nil {
		t.Fatalf("powershell command failed: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "tianxuan-ok") {
		t.Fatalf("output = %q, want it to contain tianxuan-ok", out)
	}
}

func TestBashPowerShellSurfacesNonZeroExit(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("powershell e2e is windows-only")
	}
	if _, err := runPS(t, "exit 3"); err == nil {
		t.Fatal("non-zero exit should surface as an error")
	}
}

func TestBashPowerShellRejectsChaining(t *testing.T) {
	b := bash{shell: sandbox.Shell{Kind: sandbox.ShellPowerShell, Path: "powershell"}}
	for _, cmd := range []string{"echo a && echo b", "echo a || echo b"} {
		args, _ := json.Marshal(map[string]string{"command": cmd})
		out, err := b.Execute(context.Background(), args)
		if err == nil {
			t.Errorf("%q should be rejected on powershell, got out=%q", cmd, out)
		} else if !strings.Contains(err.Error(), "PowerShell") {
			t.Errorf("%q error should explain PowerShell, got %v", cmd, err)
		}
	}
}

func TestBashPowerShellAllowsQuotedOperator(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("runs a real powershell command")
	}
	// "&&" inside a string literal is data, not chaining — must not be rejected.
	out, err := runPS(t, `Write-Output "a && b"`)
	if err != nil {
		t.Fatalf("quoted && should run: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "a && b") {
		t.Fatalf("output = %q", out)
	}
}

func TestBashPwshAllowsChaining(t *testing.T) {
	// pwsh (PowerShell 7+) parses && — the guard must not block it.
	b := bash{shell: sandbox.Shell{Kind: sandbox.ShellPowerShell, Path: "pwsh"}}
	args, _ := json.Marshal(map[string]string{"command": "echo a && echo b"})
	_, err := b.Execute(context.Background(), args)
	if err != nil && strings.Contains(err.Error(), "does not parse") {
		t.Errorf("pwsh should not be blocked by the chaining guard: %v", err)
	}
}

func TestBashPowerShellOutputIsUTF8(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("powershell e2e is windows-only")
	}
	out, err := runPS(t, "Write-Output 'AB-中文-CD'")
	if err != nil {
		t.Fatalf("command failed: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "中文") {
		t.Fatalf("non-ASCII output mojibake — got %q (want it to contain 中文)", out)
	}
}

func TestBashDescriptionReflectsShell(t *testing.T) {
	ps := bash{shell: sandbox.Shell{Kind: sandbox.ShellPowerShell, Path: "powershell"}}
	if !strings.Contains(ps.Description(), "PowerShell") {
		t.Errorf("powershell description should warn about PowerShell: %q", ps.Description())
	}
	sh := bash{shell: sandbox.Shell{Kind: sandbox.ShellBash, Path: "bash"}}
	if strings.Contains(sh.Description(), "PowerShell") {
		t.Errorf("bash description should not mention PowerShell: %q", sh.Description())
	}
}

func TestWrapLauncherCommand(t *testing.T) {
	tests := []struct {
		cmd    string
		want   string
		wantOk bool
	}{
		// start without /b → wrapped
		{"start notepad.exe", `cmd /c start "" "notepad.exe"`, true},
		{"start myapp.exe", `cmd /c start "" "myapp.exe"`, true},
		// start /b → NOT wrapped (background, same window)
		{"start /b myapp.exe", "", false},
		{"start /B server.exe", "", false},
		// Start-Process → wrapped
		{"Start-Process notepad", `cmd /c start "" "notepad"`, true},
		// npm / wails / go → wrapped
		{"npm start", `cmd /c start "" "npm start"`, true},
		{"npm run dev", `cmd /c start "" "npm run dev"`, true},
		{"wails dev", `cmd /c start "" "wails dev"`, true},
		{"go run ./cmd/server", `cmd /c start "" "go run ./cmd/server"`, true},
		// normal commands → NOT wrapped
		{"echo hello", "", false},
		{"go build ./...", "", false},
		{"git status", "", false},
	}
	for _, tt := range tests {
		got, ok := wrapLauncherCommand(tt.cmd)
		if ok != tt.wantOk {
			t.Errorf("wrapLauncherCommand(%q) ok = %v, want %v", tt.cmd, ok, tt.wantOk)
		}
		if tt.wantOk && got != tt.want {
			t.Errorf("wrapLauncherCommand(%q) =\n  got:  %q\n  want: %q", tt.cmd, got, tt.want)
		}
	}
}
