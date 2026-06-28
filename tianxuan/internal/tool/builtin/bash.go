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

const bashTimeout = 300 * time.Second

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
	return "Execute a shell command. 5-minute timeout. For long-running commands, use run_in_background=true. Set output_format=json to get structured result with separated stdout/stderr fields."
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
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Shell command to execute"},"run_in_background":{"type":"boolean","description":"Run detached: returns a job id immediately and keeps running across turns."},"output_format":{"type":"string","enum":["plain","json"],"description":"plain (default) returns raw merged output. json returns structured {ok, exit_code, duration_ms, stdout, stderr, command} with separated stdout/stderr fields."}},"required":["command"]}`)
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
		OutputFormat    string `json:"output_format"`
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

			// V8.2: 后台任务也加上前台同款保护——jobCtx 取消时立刻强杀进程树，
			// 防止 cmd.Wait() 永久阻塞（软件启动后卡死或不正常退出）。
			go func() {
				<-jobCtx.Done()
				killProcessTree(cmd)
			}()

			// Try Windows Job Object for reliable process-tree cleanup.
			// When the job handle closes (defer), Windows kills all child/grandchild
			// processes recursively — even on kill_shell cancel or session close.
			job, jobErr := assignToJobObject(cmd)
			if jobErr == nil {
				defer syscall.CloseHandle(job)
			}
			err := cmd.Wait()
			if jobErr != nil {
				// Job Object failed (e.g. sandbox restriction); fall back to taskkill.
				killProcessTree(cmd)
			}
			return "", err
		})
		return fmt.Sprintf("Started background job %q. It keeps running across turns; read new output with bash_output(job_id=%q), wait for it with wait, or stop it with kill_shell(job_id=%q).", job.ID, job.ID, job.ID), nil
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	hideBashWindow(cmd) // Windows: 防止弹出 cmd 黑框
	cmd.Dir = b.workDir // "" lets exec use the process working directory

	// V10.5: json 模式下分离 stdout/stderr；plain 模式保持合并
	var stdoutBuf, stderrBuf bytes.Buffer
	if p.OutputFormat == "json" {
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
	} else {
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stdoutBuf // merged in plain mode
	}

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

	// JSON output mode: return structured result with separated stdout/stderr
	if p.OutputFormat == "json" {
		ok := err == nil && ctx.Err() == nil
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		var buf2 bytes.Buffer
		enc := json.NewEncoder(&buf2)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(map[string]any{
			"ok":          ok,
			"exit_code":   exitCode,
			"duration_ms": time.Since(start).Milliseconds(),
			"stdout":      strings.TrimSpace(stdoutBuf.String()),
			"stderr":      strings.TrimSpace(stderrBuf.String()),
			"command":     p.Command,
		})
		return strings.TrimSpace(buf2.String()), nil
	}

	// Plain mode: merged output
	out := stdoutBuf.String()

	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("command timed out (> %s)", bashTimeout)
	}
	if err != nil {
		// Non-zero exit: feed output and error back so the model can self-correct.
		return out, fmt.Errorf("command exited: %w", err)
	}
	return out, nil
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
