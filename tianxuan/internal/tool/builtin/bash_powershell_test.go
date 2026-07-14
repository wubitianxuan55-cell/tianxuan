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
	ps := sandbox.Shell{Kind: sandbox.ShellPowerShell}
	bash := sandbox.Shell{Kind: sandbox.ShellBash}

	tests := []struct {
		cmd    string
		sh     sandbox.Shell
		want   string
		wantOk bool
	}{
		// ── PowerShell path ──
		// start without /b → already non-blocking, no wrap needed
		{"start notepad.exe", ps, "", false},
		{"start myapp.exe", ps, "", false},
		// start /b → NOT a launcher (background, same window)
		{"start /b myapp.exe", ps, "", false},
		{"start /B server.exe", ps, "", false},
		// Start-Process → already non-blocking, no wrap needed
		{"Start-Process notepad", ps, "", false},
		// npm / wails / go → wrapped via cmd /c start
		{"npm start", ps, `cmd /c start "" "npm start"`, true},
		{"npm run dev", ps, `cmd /c start "" "npm run dev"`, true},
		{"wails dev", ps, `cmd /c start "" "wails dev"`, true},
		{"go run ./cmd/server", ps, `cmd /c start "" "go run ./cmd/server"`, true},
		// normal commands → NOT wrapped
		{"echo hello", ps, "", false},
		{"go build ./...", ps, "", false},
		{"git status", ps, "", false},

		// ── Bash path ──
		// launcher commands → " &" appended
		{"npm start", bash, "npm start &", true},
		{"npm run dev", bash, "npm run dev &", true},
		{"wails dev", bash, "wails dev &", true},
		{"go run ./cmd/server", bash, "go run ./cmd/server &", true},
		{"ngrok http 8080", bash, "ngrok http 8080 &", true},
		// keyword-based detection — "server" keyword
		{"python server.py", bash, "python server.py &", true},
		{"node server.js", bash, "node server.js &", true},
		{"./bin/my-server --port 8080", bash, "./bin/my-server --port 8080 &", true},
		// keyword-based detection — "daemon" keyword
		{"mongod --daemon", bash, "mongod --daemon &", true},
		// keyword-based detection — "listen" keyword
		{"./app --listen :8080", bash, "./app --listen :8080 &", true},
		// keyword-based detection — "runserver" keyword
		{"python manage.py runserver", bash, "python manage.py runserver &", true},
		// PowerShell path — keyword-based
		{"python server.py", ps, `cmd /c start "" "python server.py"`, true},
		{"node server.js", ps, `cmd /c start "" "node server.js"`, true},
		// negative: "server" as part of a normal word should NOT match
		{"echo observer", bash, "", false},
		// negative: non-launcher commands should still NOT match
		{"pip install requests", bash, "", false},
		{"go test ./...", bash, "", false},
		// normal commands → NOT wrapped
		{"echo hello", bash, "", false},
		{"go build ./...", bash, "", false},
		{"git status", bash, "", false},
	}
	for _, tt := range tests {
		got, ok := wrapLauncherCommand(tt.cmd, tt.sh)
		if ok != tt.wantOk {
			t.Errorf("wrapLauncherCommand(%q, %s) ok = %v, want %v", tt.cmd, tt.sh.Kind, ok, tt.wantOk)
		}
		if tt.wantOk && got != tt.want {
			t.Errorf("wrapLauncherCommand(%q, %s) =\n  got:  %q\n  want: %q", tt.cmd, tt.sh.Kind, got, tt.want)
		}
	}
}
