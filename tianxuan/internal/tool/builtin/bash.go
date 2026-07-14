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
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Shell command to execute"},"run_in_background":{"type":"boolean","description":"Run detached: returns a job id immediately and keeps running across turns. Use for persistent servers (dev/watch/serve/start), ngrok tunnels, docker compose up, and any command that does not exit on its own."},"output_format":{"type":"string","enum":["plain","json"],"description":"plain (default) returns raw merged output. json returns structured {ok, exit_code, duration_ms, stdout, stderr, command} with separated stdout/stderr fields."}},"required":["command"]}`)
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

// Detect launcher commands (dev servers, tunnels, watchers, etc.) and
	// background them using the shell's native mechanism so the bash tool does
	// not block on cmd.Wait(). Applies to any shell on any platform.
	if !p.RunInBackground {
		if wrapped, ok := wrapLauncherCommand(p.Command, sh); ok {
			p.Command = wrapped
		}
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
		// V7.5: 监听 ctx 取消——不等 cmd.Wait()，卡死时立即强杀进程树。
		// cmd.Wait() 可能永久阻塞（进程卡死不响应信号），此时 killProcessTree 永远执行不到。
		go func() {
			defer crash.Recover("bash-fg-kill")
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

// ── launcher command detection (Windows, any shell) ──

// launcherPrefixes are command prefixes that typically start long-running
// server, tunnel, watcher, or GUI processes. On Windows, these are wrapped via
// cmd /c start to open a new visible window and return immediately,
// preventing the bash tool from blocking on cmd.Wait().
var launcherPrefixes = []string{
	// Node.js / Bun
	"npm start", "npm run dev", "npm run serve", "npm run watch", "npm run ",
	"npx ",
	"yarn dev", "yarn start", "yarn serve", "yarn watch", "yarn ",
	"pnpm dev", "pnpm start", "pnpm serve", "pnpm watch", "pnpm ",
	"bun dev", "bun start", "bun run ",
	// Go
	"go run ",
	"air", // Go live-reload
	// Rust
	"cargo run", "cargo watch",
	// Python
	"python -m http.server", "python3 -m http.server",
	"python -m uvicorn", "python3 -m uvicorn",
	"python -m flask run", "python3 -m flask run",
	"uvicorn ", "gunicorn ", "flask run",
	// Wails / Tauri / Electron
	"wails dev", "wails serve",
	// Docker
	"docker compose up", "docker-compose up", "docker run ",
	// Tunnels / proxies
	"ngrok ", "cloudflared tunnel", "localtunnel ",
	// Java / JVM
	"java -jar ", "mvn spring-boot:run", "mvnw spring-boot:run",
	"gradle bootrun", "gradlew bootrun",
	// Elixir / Phoenix
	"iex -S mix", "mix phx.server",
	// Watchers / live-reload
	"nodemon ", "ts-node-dev ", "tsx watch",
	// .NET
	"dotnet watch", "dotnet run",
	// Make / Task runners
	"make dev", "make serve", "make watch",
	// Misc
	"vite", "webpack-dev-server", "next dev", "nuxt dev",
	"hugo server", "jekyll serve", "mkdocs serve",
	// Python env managers running servers
	"pipenv run ", "poetry run ",
}

// wrapLauncherCommand detects whether cmd starts a long-running process and
// wraps it for non-blocking execution. The strategy depends on the shell:
//   - bash: appends " &" so the shell backgrounds the command and returns.
//   - PowerShell: wraps via cmd /c start (PowerShell has no native background
//     operator; run_in_background is the preferred way for headless bg).
// Returns the (possibly wrapped) command and true if wrapping was applied.
func wrapLauncherCommand(cmd string, sh sandbox.Shell) (string, bool) {
	trimmed := strings.TrimSpace(cmd)
	lower := strings.ToLower(trimmed)

	if !isLauncher(trimmed, lower) {
		return cmd, false
	}

	// PowerShell: wrap launcher commands via cmd /c start so the process
	// launches independently and exits immediately, never blocking bash.
	// Shell will briefly flash a cmd window; start /b was tried but
	// triggers "Windows cannot find file" ShellExecute popups on commands
	// containing quoted arguments (V10.71→V10.75 regression).
	if sh.Kind == sandbox.ShellPowerShell {
		if isStartCmd(trimmed) {
			return cmd, false // already non-blocking
		}
		return fmt.Sprintf(`cmd /c start "" %s`, escapeCmdArg(cmd)), true
	}

	// Bash / POSIX shell: append " &" to background the command.
	return trimmed + " &", true
}

// isLauncher reports whether cmd looks like a long-running server/tunnel/watcher.
func isLauncher(trimmed, lower string) bool {
	// start /b is already backgrounded by the user; no wrapping needed.
	if strings.HasPrefix(lower, "start /b") || strings.HasPrefix(lower, "start /B") {
		return false
	}
	if isStartCmd(trimmed) {
		return true
	}
	for _, prefix := range launcherPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return isLauncherByKeywords(lower)
}

// isLauncherByKeywords detects commands that contain keywords strongly
// associated with long-running servers, watchers, or tunnels even when
// they don't match a known prefix. Catches ad-hoc commands like
// "node server.js", "python app.py --serve", etc.
func isLauncherByKeywords(lower string) bool {
	// Normalise common word separators so "server.py" / "bin/server" / "my-app"
	// all expose the keyword boundaries for " server " etc.
	normalised := strings.NewReplacer(".", " ", "/", " ", "\\", " ", "-", " ").Replace(lower)
	// Pad so we can match keywords at the end of the string.
	padded := normalised + " "
	for _, kw := range launcherKeywords {
		if strings.Contains(padded, kw) {
			return true
		}
	}
	return false
}

// launcherKeywords are substrings that strongly indicate a long-running process.
// Each keyword is padded with spaces so it only matches as a whole word — e.g.
// " server " matches "python server.py" (after normalising separators) but not
// "echo observer".
var launcherKeywords = []string{
	" serve ", " dev ", " watch ", " start ", " runserver ",
	" server ", " daemon ", " listen ",
	" ngrok ", " tunnel ", " proxy ",
	" start-server ", " run-server ", " serve-app ",
}

// isStartCmd reports whether cmd starts with "start " (PowerShell alias for
// Start-Process) or "Start-Process ", excluding "start /b" (background).
func isStartCmd(cmd string) bool {
	lower := strings.ToLower(cmd)
	if strings.HasPrefix(lower, "start /b") || strings.HasPrefix(lower, "start /B") {
		return false // /b = same window, no wrapping needed
	}
	return strings.HasPrefix(lower, "start ") || strings.HasPrefix(lower, "start-process ")
}

// restAfterStart returns the command part after "start " or "Start-Process ",
// stripping the optional quoted window title and Start-Process flags.
func restAfterStart(cmd string) string {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	var rest string
	if strings.HasPrefix(lower, "start-process ") {
		rest = strings.TrimSpace(cmd[len("start-process "):])
	} else {
		rest = strings.TrimSpace(cmd[len("start "):])
	}
	// Skip optional quoted window title (cmd.exe start convention)
	if strings.HasPrefix(rest, `"`) {
		if end := strings.Index(rest[1:], `"`); end >= 0 {
			rest = strings.TrimSpace(rest[end+2:])
		}
	}
	// Strip Start-Process flags that conflict with new-window wrapping
	if strings.HasPrefix(lower, "start-process ") {
		rest = stripStartProcessFlags(rest)
	}
	return rest
}

// stripStartProcessFlags removes -NoNewWindow, -WindowStyle, and -Wait flags
// from a Start-Process argument list.
func stripStartProcessFlags(cmd string) string {
	for _, flag := range []string{"-NoNewWindow", "-WindowStyle", "-Wait"} {
		for {
			idx := findFlag(cmd, flag)
			if idx < 0 {
				break
			}
			end := idx + len(flag)
			rest := strings.TrimSpace(cmd[end:])
			// If next token is a value (e.g. -WindowStyle Hidden), skip it too
			if len(rest) > 0 && rest[0] != '-' {
				if spaceIdx := strings.IndexByte(rest, ' '); spaceIdx > 0 {
					rest = rest[spaceIdx:]
				} else {
					rest = ""
				}
			}
			cmd = strings.TrimSpace(cmd[:idx] + rest)
		}
	}
	return cmd
}

func findFlag(s, flag string) int {
	return strings.Index(strings.ToLower(s), strings.ToLower(flag))
}

// escapeCmdArg wraps a command for use as a single argument to cmd /c start.
func escapeCmdArg(cmd string) string {
	escaped := strings.ReplaceAll(cmd, `"`, `""`)
	return `"` + escaped + `"`
}
