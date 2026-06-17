package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"tianxuan/internal/jobs"
	"tianxuan/internal/sandbox"
	"tianxuan/internal/tool"
)

const bashTimeout = 120 * time.Second

func init() { tool.RegisterBuiltin(bash{}) }

// bash runs a shell command with a timeout to avoid hangs. sb, when it enforces,
// wraps the command in an OS sandbox; the zero value registered at init runs
// unconfined and is overridden per run by ConfineBash. shell is the resolved
// interpreter (real bash, or PowerShell on a Windows host without bash); the
// zero value resolves lazily. workDir, when non-empty, is the directory the
// command runs in (cmd.Dir); empty uses the process cwd.
type bash struct {
	sb      sandbox.Spec
	shell   sandbox.Shell
	workDir string
}

func (bash) Name() string { return "bash" }

func (b bash) Description() string {
	if b.resolved().Kind == sandbox.ShellPowerShell {
		return "Execute a command in the shell and return combined stdout/stderr. " +
			"NOTE: bash is not available on this host — commands run under Windows PowerShell, " +
			"so write PowerShell syntax (e.g. $null not /dev/null; ';' or separate calls, not '&&'; " +
			"Get-ChildItem/Select-String, not ls/grep). Use for builds, tests, git, etc."
	}
	return "Execute a command in the shell and return combined stdout/stderr. Use for builds, tests, git, etc."
}

// resolved returns the bound shell, resolving lazily for the zero-value instance
// (e.g. a registry that never went through ConfineBash).
func (b bash) resolved() sandbox.Shell {
	if b.shell.Path != "" {
		return b.shell
	}
	return sandbox.ResolveShell()
}

func (bash) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Shell command to execute"},"run_in_background":{"type":"boolean","description":"Run detached: returns a job id immediately and keeps running across turns (no timeout). Read new output with bash_output, wait for it with wait, stop it with kill_shell. Use for long-running commands like servers, watchers, or builds you don't need to block on."}},"required":["command"]}`)
}

// ReadOnly is false: bash's effect cannot be inferred from args (rm, curl,
// git commit, etc. are all reachable). Conservative even when a particular
// command happens to be read-only — the agent batch decision can't tell.
func (bash) ReadOnly() bool { return false }

func (bash) CompactDescription() string { return compactDesc["bash"] }
func (bash) CompactSchema() json.RawMessage   { return compactSchema["bash"] }

func (b bash) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Command         string `json:"command"`
		RunInBackground bool   `json:"run_in_background"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.Command == "" {
		return "", fmt.Errorf("command is required")
	}

	sh := b.resolved()
	if !sh.SupportsChaining() && (hasUnquotedSeq(p.Command, "&&") || hasUnquotedSeq(p.Command, "||")) {
		return "", fmt.Errorf("this shell is Windows PowerShell, which does not parse '&&' or '||'. " +
			"Sequence with ';' (both run regardless of the first's result), use 'if ($?) { ... }' for " +
			"conditional chaining, or issue the commands as separate calls")
	}

	// Wrap in the OS sandbox when configured; otherwise argv is just the shell.
	argv, _ := sandbox.Command(b.sb, sh, p.Command)

	if p.RunInBackground {
		jm, ok := jobs.FromContext(ctx)
		if !ok {
			return "", fmt.Errorf("background execution is not available in this context")
		}
		workDir := b.workDir
		// The job runs under the manager's session context (no 120s timeout), so it
		// survives this turn; its combined output streams to the job buffer.
		job := jm.Start("bash", commandPreview(p.Command), func(jobCtx context.Context, out io.Writer) (string, error) {
			cmd := exec.CommandContext(jobCtx, argv[0], argv[1:]...)
			hideBashWindow(cmd) // Windows: 防止弹出 cmd 黑框
			cmd.Dir = workDir
			cmd.Stdout = out
			cmd.Stderr = out
			if err := cmd.Start(); err != nil {
				return "", err
			}
			// Record PID so kill_shell can fall back to taskkill /T on Windows.
			if id, ok := jobs.JobIDFromContext(jobCtx); ok {
				jm.SetPid(id, cmd.Process.Pid)
			}
			return "", cmd.Wait()
		})
		return fmt.Sprintf("Started background job %q. It keeps running across turns; read new output with bash_output(job_id=%q), wait for it with wait, or stop it with kill_shell(job_id=%q).", job.ID, job.ID, job.ID), nil
	}

	ctx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	hideBashWindow(cmd) // Windows: 防止弹出 cmd 黑框
	cmd.Dir = b.workDir // "" lets exec use the process working directory
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Start()
	if err == nil {
		// V7.5: 监听 ctx 取消——不等 cmd.Wait()，卡死时立即强杀进程树。
		// cmd.Wait() 可能永久阻塞（进程卡死不响应信号），此时 killProcessTree 永远执行不到。
		go func() {
			<-ctx.Done()
			killProcessTree(cmd)
		}()

		// Try Windows Job Object for reliable process-tree cleanup.
		// When the job handle closes (defer), Windows kills all child/grandchild
		// processes recursively — even on timeout or abrupt cancel.
		job, jobErr := assignToJobObject(cmd)
		if jobErr == nil {
			defer syscall.CloseHandle(job)
		}
		err = cmd.Wait()
		if jobErr != nil {
			// Job Object failed (e.g. sandbox restriction); fall back to taskkill.
			killProcessTree(cmd)
		}
	}
	out := buf.String()

	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("command timed out (> %s)", bashTimeout)
	}
	if err != nil {
		// Non-zero exit: feed output and error back so the model can self-correct.
		out = appendTestSummary(out)
		return out, fmt.Errorf("command exited: %w", err)
	}
	return appendTestSummary(out), nil
}

// appendTestSummary scans command output for Go test failures and appends a
// structured summary. The model sees this inline and can fix failures without
// parsing raw test output.
func appendTestSummary(out string) string {
	// Match Go test failure pattern: --- FAIL: TestXxx (0.00s)
	// or: FAIL: TestXxx (0.00s) / panic: test timed out
	type fail struct {
		name string
		file string
		line string
	}
	var fails []fail
	seenName := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "--- FAIL:") && !strings.HasPrefix(line, "FAIL\t") {
			continue
		}
		name := ""
		if strings.HasPrefix(line, "--- FAIL:") {
			name = strings.TrimPrefix(line, "--- FAIL:")
		} else {
			// "FAIL\t./pkg [build failed]" or "FAIL\t./pkg\t0.123s"
			name = strings.TrimPrefix(line, "FAIL")
		}
		name = strings.TrimSpace(name)
		if idx := strings.Index(name, " ("); idx >= 0 {
			name = name[:idx]
		}
		if idx := strings.Index(name, "\t"); idx >= 0 {
			name = name[:idx]
		}
		name = strings.TrimSpace(name)
		if name == "" || seenName[name] {
			continue
		}
		seenName[name] = true
		fails = append(fails, fail{name: name})
	}

	if len(fails) == 0 {
		return out
	}

	// Extract file:line from trace lines (e.g. "    foo_test.go:42: ...")
	for i, f := range fails {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, f.name) && strings.Contains(line, ".go:") {
				// Try to find "file_test.go:42" pattern
				rest := line
				for {
					idx := strings.Index(rest, ".go:")
					if idx < 0 {
						break
					}
					start := idx
					for start > 0 && rest[start-1] != ' ' && rest[start-1] != '\t' && rest[start-1] != '/' {
						start--
					}
					end := idx + 3
					for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
						end++
					}
					if end > idx+3 {
						fails[i].file = rest[start : idx+3]
						fails[i].line = rest[idx+4 : end]
						break
					}
					rest = rest[end:]
				}
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(out)
	sb.WriteString("\n\n[Test Failure Summary]\n")
	sb.WriteString(fmt.Sprintf("Failed tests: %d\n", len(fails)))
	for _, f := range fails {
		sb.WriteString(fmt.Sprintf("  - %s", f.name))
		if f.file != "" {
			sb.WriteString(fmt.Sprintf(" (%s:%s)", f.file, f.line))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// hasUnquotedSeq reports whether seq appears in s outside any single- or
// double-quoted span, so a literal "a && b" string argument doesn't trip the
// PowerShell chaining guard.
func hasUnquotedSeq(s, seq string) bool {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if strings.HasPrefix(s[i:], seq) {
			return true
		}
	}
	return false
}

// commandPreview is a short single-line label for a background bash job, surfaced
// in the status bar and completion notices.
func commandPreview(cmd string) string {
	cmd = strings.TrimSpace(strings.ReplaceAll(cmd, "\n", " "))
	const max = 48
	r := []rune(cmd)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return cmd
}

// killProcessTree 在命令执行完毕后清理 shell 可能残留的子进程树。
// Windows 上 shell 内部的 & 后台进程不会随 shell 退出而终止，
// taskkill /T 递归终止整个进程树避免孤儿进程和 wait 死锁。
func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if runtime.GOOS != "windows" {
		return
	}
	killCmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	killCmd.Stdout = io.Discard
	killCmd.Stderr = io.Discard
	_ = killCmd.Run() // 忽略错误（进程可能已正常退出）
}
