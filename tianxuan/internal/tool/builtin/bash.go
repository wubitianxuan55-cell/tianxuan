package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

func init() { tool.RegisterBuiltin(bash{}) }

// bash runs a shell command with a timeout to avoid hangs. sb, when it enforces,
// wraps the command in an OS sandbox; the zero value registered at init runs
// unconfined and is overridden per run by ConfineBash. shell is the resolved
// interpreter (real bash, or PowerShell on a Windows host without bash); the
// zero value resolves lazily. workDir, when non-empty, is the directory the
// command runs in (cmd.Dir); empty uses the process cwd.
// timeout is the host-injected foreground cap (aligned with Reasonix): >0 caps
// the foreground command; <=0 means no tool-local cap, only parent-context
// cancellation kills the process tree. It is never read from model arguments —
// the model cannot change the timeout, and unknown timeout fields are rejected.
type bash struct {
	sb      sandbox.Spec
	shell   sandbox.Shell
	workDir string
	timeout time.Duration
	// env, when non-empty, is injected into every executed command's
	// environment (e.g. an augmented PATH). It replaces the inherited PATH
	// rather than appending to it — see commandEnv.
	env []string
}

func (bash) Name() string { return "bash" }

func (b bash) Description() string {
	if b.resolved().Kind == sandbox.ShellPowerShell {
		return "Execute a command in the shell and return combined stdout/stderr. " +
			"NOTE: bash is not available on this host — commands run under Windows PowerShell, " +
			"so write PowerShell syntax (e.g. $null not /dev/null; ';' or separate calls, not '&&'; " +
			"Get-ChildItem/Select-String, not ls/grep). " +
			"The foreground timeout is set by the host (default 120s) and cannot be changed via arguments: " +
			"timeout/timeout_seconds/description/cwd are NOT valid fields and will be rejected. " +
			"For long-running servers/tunnels/watchers, " +
			"you MUST use run_in_background=true — otherwise the process will be killed. " +
			"Windows safety rules: do not compose destructive file commands across shells " +
			"(e.g. enumerate paths in PowerShell then delete via cmd /c) — use one shell end-to-end, " +
			"prefer native Remove-Item / Move-Item with -LiteralPath, avoid string-built commands for file ops. " +
			"Before any recursive delete or move, verify the resolved absolute target paths stay within " +
			"the workspace or an explicitly named target directory. " +
			"When using Start-Process for a background helper/service, pass -WindowStyle Hidden " +
			"unless the user explicitly asked for a visible window."
	}
	return "Execute a shell command with a 2-minute timeout. " +
		"For long-running servers, tunnels, watchers, or daemons, you MUST use run_in_background=true " +
		"to avoid blocking. If you forget, the command will be killed after 2 minutes. " +
		"Set output_format=json for structured {ok, exit_code, duration_ms, stdout, stderr}. " +
		"The foreground timeout is set by the host (default 120s) and cannot be changed via arguments: " +
		"timeout/timeout_seconds/description/cwd are NOT valid fields and will be rejected."
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
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Shell command to execute"},"run_in_background":{"type":"boolean","description":"Run detached: returns a job id immediately and keeps running across turns. Use for persistent servers (dev/watch/serve/start), ngrok tunnels, docker compose up, and any command that does not exit on its own."},"output_format":{"type":"string","enum":["plain","json"],"description":"plain (default) returns raw merged output. json returns structured {ok, exit_code, duration_ms, stdout, stderr, command} with separated stdout/stderr fields."},"interactive":{"type":"boolean","description":"Give the background job an interactive stdin pipe so write_stdin can deliver input mid-run (REPLs, interactive CLIs, debuggers). Only meaningful with run_in_background=true. Default false."}},"required":["command"]}`)
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
		Interactive     bool   `json:"interactive"`
	}
	if err := decodeStrictArgs(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w (valid fields: command, run_in_background, output_format)", err)
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

	// P5: 高危环境变量主动预警。数据库类命令 + 用户级 DATABASE_URL 与项目
	// .env 不一致时,在输出/JSON 中注入警告,不依赖记忆兜底。
	envWarn := envWarningForCommand(p.Command, b.workDir)

	// Service-command auto-background must be judged on the ORIGINAL command:
	// adaptPowerShellCommand rewrites bare npm to npm.cmd, which would break
	// the service markers below.
	autoBackground := false
	if !p.RunInBackground && isServiceCommand(p.Command) {
		if _, ok := jobs.FromContext(ctx); ok {
			p.RunInBackground = true
			autoBackground = true
		}
	}

	// P1 Windows/PowerShell adaptation: translate bash-isms the model habitually
	// writes (heredocs, bare npm shims) and inject git's cmd dir when git is not
	// on PATH. Bash shells skip this entirely - the model's POSIX syntax works
	// as-is there.
	if sh.Kind == sandbox.ShellPowerShell {
		gitCmdDir := ""
		if _, err := exec.LookPath("git"); err != nil {
			gitCmdDir = findGitCmdDir()
		}
		adapted, aerr := adaptPowerShellCommand(p.Command, gitCmdDir)
		if aerr != nil {
			return "", aerr
		}
		p.Command = adapted
	}

	// Wrap in the OS sandbox when configured; otherwise argv is just the shell.
	argv, _ := sandbox.Command(b.sb, sh, p.Command)

	// V10.127: 服务类命令防呆——前台启动服务器/监听进程会阻塞 turn 直到
	// 120s 超时，且超时后服务进程可能仍持有输出管道写端，使 cmd.Wait()
	// 永久阻塞（整个进程卡死、只能重启）。检测到服务类命令且未显式
	// run_in_background 时自动转后台：立即返回 job id，kill_shell 可随时停。

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
			cmd.Env = b.commandEnv()
			cmd.Stdout = out
			cmd.Stderr = out
			// V10.174: interactive mode (distilled from codex write_stdin) —
			// the job gets an stdin pipe so write_stdin can deliver input
			// mid-run (REPLs, interactive CLIs, debuggers). Non-interactive
			// jobs keep stdin at the process default (no pipe), so a command
			// that never reads stdin can't wedge on a never-drained pipe.
			if p.Interactive {
				pr, pw := io.Pipe()
				cmd.Stdin = pr
				if id, ok := jobs.JobIDFromContext(jobCtx); ok {
					jm.SetStdin(id, pw)
				}
			}
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
		hint := ""
		if autoBackground {
			hint = "（服务类命令已自动转入后台——前台执行会阻塞到超时且可能卡死；用 bash_output 读输出、kill_shell 停止）"
		}
		return fmt.Sprintf("Started background job %q. It keeps running across turns; read new output with bash_output(job_id=%q), wait for it with wait, or stop it with kill_shell(job_id=%q).%s", job.ID, job.ID, job.ID, hint), nil
	}

	start := time.Now()
	timeout := b.foregroundTimeout()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	hideBashWindow(cmd) // Windows: 防止弹出 cmd 黑框
	cmd.Dir = b.workDir // "" lets exec use the process working directory
	cmd.Env = b.commandEnv()

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
		// V10.127: Wait 永不永久阻塞——命令派生的进程可能持有 stdout/stderr
		// 管道写端（服务进程），Go 的 Wait 要等所有写端关闭（EOF）才返回；
		// 超时/取消时立即返回，进程树由上面的 kill goroutine 清理。
		waitCh := make(chan error, 1)
		go func() { waitCh <- cmd.Wait() }()
		select {
		case err = <-waitCh:
		case <-ctx.Done():
			err = ctx.Err()
		}
		if jobErr != nil && ctx.Err() == nil {
			// Job Object failed (e.g. sandbox restriction); fall back to taskkill.
			killProcessTree(cmd)
		}
	}

	if ctx.Err() != nil {
		// 超时/取消：Wait 已提前返回，但 copy goroutine 可能仍在写输出 buffer
		// （服务进程持有管道写端）——不读取 buffer，避免数据竞争。
		if timeout <= 0 {
			return "", fmt.Errorf("command cancelled: %w", ctx.Err())
		}
		return "", fmt.Errorf("command timed out (> %s); process tree kill in progress", timeout)
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
		if envWarn != "" {
			result["warning"] = envWarn
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
	rawOut := stdoutBuf.String()
	rawLines := lineCount(rawOut)
	const plainMaxBytes = 48 * 1024
	out, truncated := truncateStream(rawOut, plainMaxBytes)
	out = prefixWarning(out, envWarn)

	if err != nil {
		// Non-zero exit: feed output and error back so the model can
		// self-correct. V10.169: prepend a codex-style structured header
		// (Exit code / Wall time / Output) so the model reads the failure
		// status without parsing the raw error string.
		var b strings.Builder
		fmt.Fprintf(&b, "Exit code: %d\n", exitCodeOf(err))
		fmt.Fprintf(&b, "Wall time: %.1f seconds\n", time.Since(start).Seconds())
		if truncated {
			fmt.Fprintf(&b, "Total output lines: %d\n", rawLines)
		}
		b.WriteString("Output:\n")
		b.WriteString(out)
		return b.String(), fmt.Errorf("command exited: %w", err)
	}
	return out, nil
}

// exitCodeOf extracts the process exit code from a command error. Non-ExitError
// failures (context cancellation, spawn errors) report -1, matching JSON mode.
func exitCodeOf(err error) int {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

// lineCount counts newline-terminated lines in s (a trailing newline does not
// add an extra empty line).
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n")
}

// foregroundTimeout returns the tool-local foreground cap; <=0 means no cap
// (the parent context remains the only cancellation source).
func (b bash) foregroundTimeout() time.Duration {
	return b.timeout
}

// commandEnv returns the environment for executed commands. nil (the zero
// value) means "inherit the process environment" — the cheapest common path.
// When env is configured, the inherited PATH entry is dropped and replaced by
// the configured one (exactly one PATH wins), so an augmented PATH never
// duplicates or leaks the original.
func (b bash) commandEnv() []string {
	if len(b.env) == 0 {
		return nil
	}
	base := os.Environ()
	out := make([]string, 0, len(base)+len(b.env))
	for _, kv := range base {
		k, _, ok := strings.Cut(kv, "=")
		if ok && strings.EqualFold(k, "PATH") {
			continue // drop inherited PATH; the injected entry replaces it
		}
		out = append(out, kv)
	}
	return append(out, b.env...)
}

// decodeStrictArgs decodes tool arguments with DisallowUnknownFields so a
// model-supplied schema-unknown field (timeout, description, cwd, ...) fails
// loudly instead of being silently dropped.
func decodeStrictArgs(args json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
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

// serviceCommandMarkers 识别"启动后会长时间运行/监听"的服务类命令。
// 匹配采用子串（大小写不敏感），核心服务启动器与监听/跟踪关键词带空格
// 前缀以避免误伤普通词（如 serverless、preserve、watchman）。
var serviceCommandMarkers = []string{
	"http.server", "uvicorn", "gunicorn", "ngrok", "nodemon",
	"compose up", "npm run dev", "npm run start", "npm run serve", "npm run preview",
	"npm start", "pnpm dev", "pnpm start", "pnpm run dev", "pnpm run start",
	"yarn dev", "yarn start", "yarn serve", "cargo run", "cargo watch",
	"go run",
	"tail -f", "smee", "cloudflared", "frpc", "ssh -R", "ssh -L", "adb reverse",
	" serve", " server", " listen", " daemon", " watch",
}

// isServiceCommand 报告命令是否属于服务类（启动服务器/监听/持续跟踪），
// 这类命令前台执行会阻塞 turn，应自动转后台。
func isServiceCommand(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	for _, m := range serviceCommandMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
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

// ---- Windows/PowerShell command adaptation (P1) ----

var (
	heredocMarkerRE   = regexp.MustCompile(`<<-?['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?`)
	heredocRedirectRE = regexp.MustCompile(`(>>|>)\s*("[^"]*"|'[^']*'|\S+)`)
	cmdShimRE         = regexp.MustCompile(`(^|[;&|(){}\r\n]+\s*)\b(npm|npx|pnpm|pnpx|pn)(\s|$)`)
	// stdinHeredocCmds 是可安全接收 stdin 管道 heredoc 的命令（python -、
	// node -、npx tsx - 等）。白名单之外的命令无法可靠翻译，大声失败并提示。
	stdinHeredocCmds = map[string]bool{
		"python": true, "python3": true, "py": true,
		"node": true, "tsx": true, "deno": true, "bun": true,
		"ruby": true, "perl": true, "php": true,
		"npx": true, "pnpm": true, "npm": true,
	}
)

// adaptPowerShellCommand rewrites bash-isms that Windows PowerShell 5.1 cannot
// parse, so the model's POSIX habits stop failing on a Windows host:
//   - <<heredoc blocks become PowerShell here-strings: cat > file / >> file
//     become [IO.File]::WriteAllText/AppendAllText; any other command (python -,
//     node -, npx tsx -) gets the heredoc piped to its stdin.
//   - bare npm/npx/pnpm/pnpx at command positions gain a .cmd suffix, forcing
//     PowerShell past its .ps1-first lookup that hits the execution policy.
//   - gitCmdDir, when non-empty, is prepended to $env:Path so git works even
//     when it is not on PATH (PortableGit / per-user installs).
func adaptPowerShellCommand(cmd, gitCmdDir string) (string, error) {
	var err error
	cmd, err = adaptHeredoc(cmd)
	if err != nil {
		return "", err
	}
	cmd = adaptCmdShims(cmd)
	if gitCmdDir != "" {
		cmd = "$env:Path='" + psSingleQuote(gitCmdDir) + ";' + $env:Path; " + cmd
	}
	return cmd, nil
}

// adaptHeredoc translates the first heredoc block in cmd, returning the adapted
// command. Commands without a heredoc pass through unchanged.
func adaptHeredoc(cmd string) (string, error) {
	prefix, body, redirect, rest, ok := splitHeredoc(cmd)
	if !ok {
		return cmd, nil
	}
	psBody := "@'\n" + body + "\n'@"
	name := heredocCommandName(prefix)
	switch {
	case name == "cat" && redirect != "":
		method := "WriteAllText"
		if strings.HasPrefix(redirect, ">>") {
			method = "AppendAllText"
		}
		path := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(redirect, ">>"), ">"))
		return fmt.Sprintf("[System.IO.File]::%s('%s', %s)%s", method, psSingleQuote(path), psBody, heredocTail(rest)), nil
	case name == "cat":
		return psBody + heredocTail(rest), nil
	case name != "" && stdinHeredocCmds[name]:
		return psBody + " | " + prefix + heredocTail(rest), nil
	default:
		return "", fmt.Errorf("cannot translate heredoc command %q to PowerShell; use write_file or a PowerShell here-string", prefix)
	}
}

// splitHeredoc extracts the first <<marker heredoc block from cmd: the command
// prefix before it, the body lines, any > / >> redirect on the opening line,
// and the remainder after the closing marker.
func splitHeredoc(cmd string) (prefix, body, redirect, rest string, ok bool) {
	idx := strings.Index(cmd, "<<")
	if idx < 0 {
		return "", "", "", "", false
	}
	m := heredocMarkerRE.FindStringSubmatchIndex(cmd[idx:])
	if m == nil {
		return "", "", "", "", false
	}
	marker := cmd[idx+m[2] : idx+m[3]]
	lineEnd := strings.IndexByte(cmd[idx:], '\n')
	if lineEnd < 0 {
		return "", "", "", "", false // unterminated heredoc
	}
	head := cmd[idx : idx+lineEnd]
	if r := heredocRedirectRE.FindStringSubmatch(head); r != nil {
		redirect = strings.TrimSpace(r[1] + " " + strings.Trim(r[2], `"'`))
	}
	bodyLines := []string{}
	found := false
	restLines := strings.Split(cmd[idx+lineEnd+1:], "\n")
	for i, ln := range restLines {
		if strings.TrimLeft(ln, "\t") == marker {
			rest = strings.Join(restLines[i+1:], "\n")
			found = true
			break
		}
		bodyLines = append(bodyLines, ln)
	}
	if !found {
		return "", "", "", "", false
	}
	return strings.TrimSpace(cmd[:idx]), strings.Join(bodyLines, "\n"), redirect, rest, true
}

// heredocCommandName extracts the bare command name (path and quotes stripped)
// from a heredoc command prefix, e.g. "npx tsx -" → "npx".
func heredocCommandName(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if i := strings.IndexAny(prefix, " \t"); i >= 0 {
		prefix = prefix[:i]
	}
	prefix = strings.Trim(prefix, `"'`)
	if i := strings.LastIndexAny(prefix, `/\`); i >= 0 {
		prefix = prefix[i+1:]
	}
	return prefix
}

// heredocTail appends the post-heredoc remainder on its own line, or nothing.
func heredocTail(rest string) string {
	if rest == "" {
		return ""
	}
	return "\n" + rest
}

// adaptCmdShims forces bare npm/npx/pnpm/pnpx/pn at command positions to their
// .cmd shims. PowerShell 5.1 prefers the .ps1 sibling, whose execution the
// default execution policy blocks; the .cmd suffix bypasses that lookup.
func adaptCmdShims(cmd string) string {
	return cmdShimRE.ReplaceAllString(cmd, "${1}${2}.cmd${3}")
}

// psSingleQuote escapes a string for a PowerShell single-quoted literal.
func psSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// findGitCmdDir returns the cmd directory of the first git install found in the
// usual Windows locations, or "" when git is already on PATH or nowhere found.
// Callers only use it after LookPath("git") fails.
func findGitCmdDir() string {
	var roots []string
	for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"} {
		if v := os.Getenv(env); v != "" {
			roots = append(roots, v)
		}
	}
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		roots = append(roots, filepath.Join(v, "Programs"))
		roots = append(roots, filepath.Join(v, "Programs", "Git"))
	}
	var candidates []string
	for _, r := range roots {
		candidates = append(candidates,
			filepath.Join(r, "Git", "cmd"),
			filepath.Join(r, "PortableGit", "cmd"),
		)
	}
	for _, d := range candidates {
		if fileExistsPath(filepath.Join(d, "git.exe")) {
			return d
		}
	}
	return ""
}

func fileExistsPath(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// ---- P5: high-risk environment variable warning ----

// sensitiveEnvCommandKeywords 是涉及数据库连接的命令关键词;命中才做预警,
// 避免无关命令的噪音。
var sensitiveEnvCommandKeywords = []string{
	"prisma", "database_url", "psql", "mysql", "migrate", "db push",
}

// envWarningForCommand 检查数据库类命令是否会在用户级高危环境变量与项目
// .env 不一致的情况下执行。返回预警文本;无风险返回空串。
// 主动检查项:不依赖记忆兜底,每次执行前判定(纯函数,无副作用)。
func envWarningForCommand(cmd, workDir string) string {
	lower := strings.ToLower(cmd)
	hit := false
	for _, kw := range sensitiveEnvCommandKeywords {
		if strings.Contains(lower, kw) {
			hit = true
			break
		}
	}
	if !hit {
		return ""
	}
	userVal := os.Getenv("DATABASE_URL")
	if userVal == "" {
		return ""
	}
	projVal := readDotEnvValue(workDir, "DATABASE_URL")
	if projVal == "" || projVal == userVal {
		return ""
	}
	return "[env-warning] 用户级 DATABASE_URL 与项目 .env 不一致(项目 .env: " + projVal +
		") — 命令可能连错库;确认数据库目标后再执行"
}

// readDotEnvValue 从 workDir/.env(workDir 为空则用进程 cwd)读取 KEY=VALUE,
// 支持双引号/单引号包裹。文件缺失或键不存在返回空串。
func readDotEnvValue(workDir, key string) string {
	dir := workDir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	b, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// prefixWarning 把预警拼到命令输出最前,保证模型第一眼看到。
func prefixWarning(out, warn string) string {
	if warn == "" {
		return out
	}
	if out == "" {
		return warn
	}
	return warn + "\n" + out
}
