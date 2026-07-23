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
	"unicode/utf8"

	"tianxuan/internal/crash"
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
			"Get-ChildItem/Select-String, not ls/grep). " +
			"Commands time out after 2 minutes. For long-running servers/tunnels/watchers, " +
			"you MUST use run_in_background=true — otherwise the process will be killed."
	}
	return "Execute a shell command with a 2-minute timeout. " +
		"For long-running servers, tunnels, watchers, or daemons, you MUST use run_in_background=true " +
		"to avoid blocking. If you forget, the command will be killed after 2 minutes. " +
		"Set output_format=json for structured {ok, exit_code, duration_ms, stdout, stderr}."
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
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Shell command to execute"},"run_in_background":{"type":"boolean","description":"Run detached: returns a job id immediately and keeps running across turns. Use for persistent servers (dev/watch/serve/start), ngrok tunnels, docker compose up, and any command that does not exit on its own."},"output_format":{"type":"string","enum":["plain","json"],"description":"plain (default) returns raw merged output. json returns structured {ok, exit_code, duration_ms, stdout, stderr, command} with separated stdout/stderr fields."}},"required":["command"]}`)
}

// ReadOnly is false: bash's effect cannot be inferred from args (rm, curl,
// git commit, etc. are all reachable). Conservative even when a particular
// command happens to be read-only — the agent batch decision can't tell.
func (bash) ReadOnly() bool { return false }
func (bash) Kind() tool.ToolKind { return tool.KindExecute }

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
				defer crash.Recover("bash-bg-kill")
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
		// V10.x: ctx 超时/取消时，先关闭 Job Object handle 触发内核 KILL_ON_CLOSE
		// 强制递归杀整个进程树，再 fallback taskkill。Job Object 关闭后 cmd.Wait()
		// 瞬间返回，不会像之前那样永久阻塞。
		var jobHandle syscall.Handle
		go func() {
			defer crash.Recover("bash-fg-kill")
			<-ctx.Done()
			if jobHandle != 0 {
				syscall.CloseHandle(jobHandle)
			}
			killProcessTree(cmd)
		}()

		// Try Windows Job Object for reliable process-tree cleanup.
		// When the job handle closes (defer or goroutine above), Windows kills all
		// child/grandchild processes recursively.
		var jobErr error
		jobHandle, jobErr = assignToJobObject(cmd)
		if jobErr == nil {
			defer syscall.CloseHandle(jobHandle)
		}
		err = cmd.Wait()
		if jobErr != nil {
			// Job Object failed (e.g. sandbox restriction); fall back to taskkill.
			killProcessTree(cmd)
		}
	}

	// JSON output mode: return structured result with separated stdout/stderr.
	// Apply truncation to prevent large outputs from blowing up context window
	// (V10.12: previously JSON mode had NO truncation, risking massive blobs).
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
		stdoutStr := strings.TrimSpace(stdoutBuf.String())
		stderrStr := strings.TrimSpace(stderrBuf.String())

		// Truncate each stream independently to ~24KB each (half of plain-mode 48KB).
		// Keeping both streams available is more useful than one large merged output.
		const jsonStreamMaxBytes = 24 * 1024
		stdoutStr, stdoutTrunc := truncateStream(stdoutStr, jsonStreamMaxBytes)
		stderrStr, stderrTrunc := truncateStream(stderrStr, jsonStreamMaxBytes)

		var buf2 bytes.Buffer
		enc := json.NewEncoder(&buf2)
		enc.SetEscapeHTML(false)
		result := map[string]any{
			"ok":          ok,
			"exit_code":   exitCode,
			"duration_ms": time.Since(start).Milliseconds(),
			"stdout":      stdoutStr,
			"stderr":      stderrStr,
			"command":     p.Command,
		}
		if stdoutTrunc {
			result["stdout_truncated"] = true
		}
		if stderrTrunc {
			result["stderr_truncated"] = true
		}
		_ = enc.Encode(result)
		return strings.TrimSpace(buf2.String()), nil
	}

	// Plain mode: merged output — apply same truncation as JSON mode for safety.
	out := stdoutBuf.String()
	const plainMaxBytes = 48 * 1024
	out, _ = truncateStream(out, plainMaxBytes)

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

// truncateStream applies head+tail truncation to a command output stream.
// Keeps the first N bytes and last N bytes, eliding the middle. Returns the
// truncated string and a boolean indicating whether truncation occurred.
// Uses simple byte-length truncation (not line-aware) for predictable sizing.
func truncateStream(s string, maxBytes int) (string, bool) {
	if len(s) <= maxBytes {
		return s, false
	}
	// ceil division: (maxBytes+1)/2 so an odd maxBytes doesn't lose a byte
	half := (maxBytes + 1) / 2
	// Adjust half to a valid UTF-8 boundary so we don't split multi-byte runes.
	for half > 0 && half < len(s) && !utf8.RuneStart(s[half]) {
		half--
	}
	head := s[:half]
	tailStart := len(s) - half
	if tailStart <= half {
		tailStart = half // prevent head/tail overlap when just barely over maxBytes
	}
	// Adjust tailStart to a valid UTF-8 boundary.
	for tailStart < len(s) && !utf8.RuneStart(s[tailStart]) {
		tailStart++
	}
	tail := s[tailStart:]
	result := head + fmt.Sprintf("\n... (%d bytes elided) ...\n", len(s)-maxBytes) + tail
	// If truncation hint makes the result longer than the original (input just
	// barely over maxBytes), return the original — truncation would be harmful.
	if len(result) >= len(s) {
		return s, false
	}
	return result, true
}
