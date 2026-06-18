package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// planModeSafeBashCommands lists bash command prefixes safe in plan mode.
// Matched as a prefix against the trimmed, lowercased command with word-boundary.
// V8.0.3: Reasonix-inspired plan mode bash safety.
var planModeSafeBashCommands = []string{
	"git status", "git diff", "git log", "git show",
	"git ls-files", "git grep", "git blame",
	"ls", "cat", "grep", "find", "head", "tail", "pwd",
	"echo", "wc", "which", "type", "uname", "hostname",
	"go version", "go list", "go doc", "go vet", "go test",
	"node -v", "npm list", "python --version", "python3 --version",
}

// shellMetachars that indicate chaining or substitution (NOT redirection —
// redirection is handled by hasShellRedirect with quote-safe parsing).
// Note: '&' is NOT in this list — '&&' is caught as a prefix, and '2>&1' is safe.
var shellMetachars = []string{"&&", "||", "$(", "\x60", ";", "|"}

// planModeFindWriteArgs: dangerous find args that can write/exec.
var planModeFindWriteArgs = map[string]bool{
	"-delete": true, "-exec": true, "-execdir": true,
	"-ok": true, "-okdir": true, "-fprint": true,
	"-fprintf": true, "-fls": true,
}

// planModeGoDangerousArgs: go subcommand args that can modify code.
var planModeGoDangerousArgs = map[string]bool{
	"-fix": true, "-mod": true, "-modfile": true, "-toolexec": true,
}

// planBashCheck returns "" if the bash command is safe in plan mode,
// or a block reason string if it should be denied.
func planBashCheck(args json.RawMessage) string {
	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &in); err != nil || in.Command == "" {
		return ""
	}
	cmd := strings.TrimSpace(in.Command)
	lower := strings.ToLower(cmd)

	// Shell metacharacters block chaining/substitution.
	for _, m := range shellMetachars {
		if strings.Contains(cmd, m) {
			return fmt.Sprintf("bash with %q is unsafe in plan mode — use native tools instead", m)
		}
	}

	// File redirect detection (quote-safe, allows 2> stderr).
	if reason := hasShellRedirect(cmd); reason != "" {
		return reason
	}

	// Safe prefix whitelist with word-boundary check.
	for _, safe := range planModeSafeBashCommands {
		if strings.HasPrefix(lower, safe) {
			rest := lower[len(safe):]
			if rest == "" || rest[0] == ' ' {
				// Check dangerous args for find/go commands.
				if reason := checkDangerousArgs(safe, rest); reason != "" {
					return reason
				}
				return "" // allowed
			}
		}
	}

	return fmt.Sprintf("bash %q is not in the plan-mode safe list — use native tools or approve the plan first", cmd)
}

// hasShellRedirect checks for unquoted write redirects. Returns "" if safe,
// or a block reason. Allows 2> (stderr redirect) — V8.0.4 fix.
func hasShellRedirect(cmd string) string {
	var quote rune
	var prev rune
	for _, r := range cmd {
		if quote != 0 {
			if r == quote { quote = 0 }
			prev = r
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			prev = r
			continue
		}
		if r == '>' {
			if prev == '2' {
				prev = r
				continue // 2> is stderr redirect, safe
			}
			return "bash with file redirect (>) is unsafe in plan mode — use write_file after plan approval"
		}
		prev = r
	}
	return ""
}

// checkDangerousArgs returns a block reason if the command (already whitelisted
// by prefix) contains dangerous sub-args like find -delete or go -fix.
func checkDangerousArgs(safePrefix, rest string) string {
	switch {
	case strings.HasPrefix(safePrefix, "find"):
		for arg := range planModeFindWriteArgs {
			if strings.Contains(rest, " "+arg) || strings.HasPrefix(rest, arg+" ") {
				return fmt.Sprintf("find %s is unsafe in plan mode — use native tools instead", arg)
			}
		}
	case strings.HasPrefix(safePrefix, "go "):
		for arg := range planModeGoDangerousArgs {
			if strings.Contains(rest, " "+arg) || strings.HasPrefix(rest, arg+" ") {
				return fmt.Sprintf("go %s is unsafe in plan mode — use native tools instead", arg)
			}
		}
	}
	return ""
}
