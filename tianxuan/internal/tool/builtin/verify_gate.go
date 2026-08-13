// Package builtin — verify_gate tool.
// V10.97: 蒸馏自 headsign Phase Gate — 用 shell 命令验证工作完成，
// 替代 LLM 判断。每个 check 的退出码决定 pass/fail。
package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"tianxuan/internal/tool"
)

// verifyGateMaxBytes caps each check's echoed output. Kept below the bash
// ceiling because a gate echoes several checks in one response.
const verifyGateMaxBytes = 2000

func init() { tool.RegisterBuiltin(verifyGate{}) }

type verifyGate struct {
	roots   []string
	workDir string
}

func (verifyGate) Name() string { return "verify_gate" }

func (verifyGate) Description() string {
	return "Run deterministic shell checks to verify work is complete. Unlike asking the LLM to judge, shell exit codes decide pass/fail — the same way CI works. Use after implementing a feature or fixing a bug to confirm the work passes objective checks (tests, lint, build, etc.). Each check runs in order; the gate fails on the first non-zero exit code."
}

func (verifyGate) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "checks":{
    "type":"array",
    "minItems":1,
    "maxItems":10,
    "description":"Ordered shell checks. Each check runs a shell command; exit 0 = pass, non-zero = fail. The gate stops on the first failure.",
    "items":{
      "type":"object",
      "properties":{
        "name":{"type":"string","description":"Human-readable check name (e.g. 'unit tests', 'lint')"},
        "command":{"type":"string","description":"Shell command to run. Must be a single-line command."},
        "timeout":{"type":"integer","description":"Timeout in seconds (default 120, max 600)","minimum":1,"maximum":600}
      },
      "required":["name","command"]
    }
  }
},
"required":["checks"]
}`)
}

func (verifyGate) ReadOnly() bool      { return true }
func (verifyGate) Kind() tool.ToolKind { return tool.KindRead }

func (verifyGate) CompactDescription() string { return compactDesc["verify_gate"] }
func (verifyGate) CompactSchema() json.RawMessage {
	return compactSchema["verify_gate"]
}

func (v verifyGate) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Checks []struct {
			Name    string `json:"name"`
			Command string `json:"command"`
			Timeout int    `json:"timeout"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if len(p.Checks) == 0 {
		return "", fmt.Errorf("checks must not be empty")
	}

	var b strings.Builder
	passed := 0
	failed := 0

	for i, check := range p.Checks {
		if check.Name == "" {
			return "", fmt.Errorf("check %d: name is required", i+1)
		}
		if check.Command == "" {
			return "", fmt.Errorf("check %d: command is required", i+1)
		}
		timeout := check.Timeout
		if timeout <= 0 {
			timeout = 120
		}
		if timeout > 600 {
			timeout = 600
		}

		cctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		cmd := exec.CommandContext(cctx, resolveShell(), shellFlag(), check.Command)
		hideBashWindow(cmd)
		cmd.Dir = v.effectiveWorkDir()
		out, err := cmd.CombinedOutput()
		cancel()

		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}

		output, _ := truncateStream(strings.TrimSpace(string(out)), verifyGateMaxBytes)

		if exitCode == 0 {
			passed++
			fmt.Fprintf(&b, "✅ %s (exit 0)\n", check.Name)
			if output != "" {
				b.WriteString(output)
				b.WriteByte('\n')
			}
		} else {
			failed++
			fmt.Fprintf(&b, "❌ %s (exit %d)\n", check.Name, exitCode)
			if output != "" {
				b.WriteString(output)
				b.WriteByte('\n')
			}
			// headsign: first failure stops the gate
			break
		}
	}

	// Summary line (headsign-compatible format)
	if failed == 0 {
		fmt.Fprintf(&b, "\nGATE PASSED: %d/%d checks passed\n", passed, passed+failed)
	} else {
		fmt.Fprintf(&b, "\nGATE FAILED: %d passed, %d failed. Fix the failing check and verify again.\n", passed, failed)
	}

	return b.String(), nil
}

func (v verifyGate) effectiveWorkDir() string {
	if v.workDir != "" {
		return v.workDir
	}
	return "."
}

// resolveShell returns the shell to use for check commands.
func resolveShell() string {
	if p, err := exec.LookPath("bash"); err == nil {
		return p
	}
	if p, err := exec.LookPath("sh"); err == nil {
		return p
	}
	return "cmd"
}

// shellFlag returns the flag to pass a command to the shell.
func shellFlag() string {
	if _, err := exec.LookPath("bash"); err == nil {
		return "-c"
	}
	if _, err := exec.LookPath("sh"); err == nil {
		return "-c"
	}
	return "/c"
}
