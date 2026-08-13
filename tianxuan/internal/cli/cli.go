// Package cli implements tianxuan's command-line entry: subcommand routing, flag
// parsing, assembly from config, and exit codes. The core is config-driven —
// providers and tools are resolved from configuration, not hardcoded.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"tianxuan/internal/agent"
	"tianxuan/internal/boot"
	"tianxuan/internal/config"
	"tianxuan/internal/control"
	"tianxuan/internal/event"
	"tianxuan/internal/i18n"
	"tianxuan/internal/serve"

	"golang.org/x/term"
)

// Run is the CLI entry point; it returns a process exit code.
func Run(args []string, version string) int {
	// Pick the UI language up front so even pre-config paths (the first-run
	// welcome banner) come through localized. Env-only first; if a config
	// exists and pins a language, that wins.
	i18n.DetectLanguage("")
	if cfg, err := config.Load(); err == nil && cfg.Language != "" {
		i18n.DetectLanguage(cfg.Language)
	}

	if len(args) == 0 {
		return welcome(version)
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "run":
		return runAgent(rest)
	case "chat":
		return chatREPL(rest)
	case "serve":
		return runServe(rest)
	case "setup":
		return setupConfig(rest)
	case "init":
		// Project memory (AGENTS.md) is model-generated in-session — `/init` runs
		// the codebase analysis. This CLI entry just points there (and to `setup`
		// for config), so `tianxuan init` isn't a dead end.
		return initHint()
	case "acp":
		return acpCommand(rest, version)
	case "mcp":
		return mcpCommand(rest)
	case "codegraph":
		return codegraphCommand(rest)
	case "tools":
		return toolsCommand(rest)
	case "update":
		return updateCommand(rest, version)
	case "doctor":
		return doctorCommand(rest)
	case "version", "--version", "-v":
		fmt.Println("tianxuan", version)
		return 0
	case "help", "--help", "-h":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, i18n.M.UnknownCommandFmt+"\n\n", cmd)
		usage()
		return 2
	}
}

// setup builds a ready-to-drive Controller from config via boot.Build. It is a
// thin adapter kept so the subcommands below read the same as before; the actual
// assembly (model resolution, tool registry, permission gate, two-model
// planner host) lives in internal/boot, shared with the desktop frontend.
// requireKey forces the executor's API key to be present (used by run); chat
// passes false so the session UI is reachable before a key is set. sink receives
// the agent's typed event stream — runAgent passes a TextSink that renders to
// stdout, the TUI passes an event-channel sink so events become tea.Msgs.
func setup(ctx context.Context, modelName string, maxStepsOverride int, requireKey bool, sink event.Sink) (*control.Controller, error) {
	return boot.Build(ctx, boot.Options{
		Model:      modelName,
		MaxSteps:   maxStepsOverride,
		RequireKey: requireKey,
		Sink:       sink,
	})
}

// setupQuiet is like setup but suppresses plugin subprocess stderr output.
// Used during model switch inside a bubbletea session to prevent plugin logs
// from corrupting the TUI's terminal raw mode.
func setupQuiet(ctx context.Context, modelName string, maxStepsOverride int, requireKey bool, sink event.Sink) (*control.Controller, error) {
	return boot.Build(ctx, boot.Options{
		Model:      modelName,
		MaxSteps:   maxStepsOverride,
		RequireKey: requireKey,
		Sink:       sink,
		Stderr:     io.Discard,
	})
}

func runAgent(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	model := fs.String("model", "", "provider name (default: config default_model)")
	maxSteps := fs.Int("max-steps", 0, "max tool-call rounds (0 = use config/default)")
	showThinking := fs.Bool("show-thinking", false, "show thinking text instead of the collapsed thinking marker")
	metricsPath := fs.String("metrics", "", "write a JSON token/cache/cost summary of the run to this path")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if prompt == "" {
		prompt = readStdin()
	}
	if prompt == "" {
		fmt.Fprintln(os.Stderr, i18n.M.UsageRunHint)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Live run: render the agent's event stream to stdout. Markdown post-stream
	// redraw (cursor moves) is enabled only on a TTY; piped / captured output
	// keeps the raw stream.
	var renderer agent.Renderer
	termW := 80
	if isTTY(os.Stdout) {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
			termW = w
		}
		renderer = newMarkdownRenderer(termW)
	}
	textSink := agent.NewTextSink(os.Stdout, renderer, termW)
	textSink.SetShowReasoning(*showThinking)
	var sink event.Sink = textSink
	var metrics *metricsSink
	if *metricsPath != "" {
		metrics = &metricsSink{inner: textSink}
		sink = metrics
	}
	ctrl, err := setup(ctx, *model, *maxSteps, true, sink)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	defer ctrl.Close()

	runErr := ctrl.Run(ctx, prompt)
	if metrics != nil {
		if err := writeMetrics(*metricsPath, metrics.m); err != nil {
			fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		}
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "\n"+i18n.M.ErrorPrefix, runErr)
		return 1
	}
	return 0
}

// runServe exposes the controller over HTTP+SSE: events stream to the browser,
// commands arrive as JSON POSTs. The Broadcaster is the controller's event sink,
// so the same typed stream the chat TUI consumes reaches web clients — the
// transport-agnostic controller driven by a second frontend.
func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	model := fs.String("model", "", "provider name (default: config default_model)")
	maxSteps := fs.Int("max-steps", 0, "max tool-call rounds (0 = use config/default)")
	addr := fs.String("addr", "127.0.0.1:8787", "listen address")
	resume := fs.String("resume", "", "resume a saved session file")
	tokenFlag := fs.String("token", "", "auth token (auto-generated when empty and --public is set)")
	public := fs.Bool("public", false, "bind 0.0.0.0 (remote access) — enables token auth + CORS")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Resolve token: explicit > auto-generate when public > empty (no auth).
	token := *tokenFlag
	if token == "" && *public {
		var err error
		token, err = serve.GenerateToken()
		if err != nil {
			fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "failed to generate token:", err)
			return 1
		}
	}

	// Resolve bind address: --public forces 0.0.0.0 unless explicitly set.
	bindAddr := *addr
	if *public && *addr == "127.0.0.1:8787" {
		bindAddr = "0.0.0.0:8787"
	}

	ctx := context.Background()
	bc := serve.NewBroadcaster()
	ctrl, err := setup(ctx, *model, *maxSteps, true, bc)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	defer ctrl.Close()

	// Auto-save target: reuse the resumed file, else a fresh one — same as chat.
	if *resume != "" {
		loaded, err := agent.LoadSession(*resume)
		if err != nil {
			fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
			return 1
		}
		ctrl.Resume(loaded, *resume)
	} else if ctrl.SessionDir() != "" {
		ctrl.SetSessionPath(agent.NewSessionPath(ctrl.SessionDir(), ctrl.Label()))
	}

	// Print connection info.
	fmt.Printf("tianxuan serve — %s\n", ctrl.Label())
	if *public {
		fmt.Printf("  🔓 Public mode — anyone with the token can control tianxuan\n")
		fmt.Printf("  🔑 Token: %s\n", token)
		fmt.Printf("  💻 Web UI:  http://%s/?token=%s\n", bindAddr, token)
	} else {
		fmt.Printf("  💻 Web UI:  http://%s\n", bindAddr)
	}

	// Use graceful shutdown so SIGINT/SIGTERM drain active connections.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	rebuildFn := func() (*control.Controller, error) {
		return setup(context.Background(), *model, *maxSteps, true, bc)
	}
	srv := serve.New(ctrl, bc).
		WithRebuild(rebuildFn, *model, *maxSteps).
		WithToken(token).
		WithPublic(*public)
	if err := srv.RunGraceful(ctx, bindAddr); err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	return 0
}

// initHint handles `tianxuan init`. Unlike a config scaffold, project memory is
// model-generated by analyzing the codebase, so it lives as the in-session
// `/init` skill rather than a CLI command. This entry just points the user there
// (and to `tianxuan setup` for config) so the verb isn't a dead end.
func initHint() int {
	fmt.Println(i18n.M.InitHint)
	return 0
}

// readStdin reads piped input if present; an interactive terminal yields "".
func readStdin() string {
	stat, err := os.Stdin.Stat()
	if err != nil || stat.Mode()&os.ModeCharDevice != 0 {
		return ""
	}
	data, _ := io.ReadAll(os.Stdin)
	return strings.TrimSpace(string(data))
}

// welcome is the zero-arg landing screen: it reports config and key readiness,
// then guides the user to the next concrete step.
func welcome(version string) int {
	src := config.SourcePath()

	// First run on an interactive terminal: actively guide setup rather than
	// printing a static screen and exiting. interactiveSetup owns the language
	// prompt and welcome banner so every prompt the user sees is already
	// localized to their choice.
	if src == "" && isInteractive() {
		if rc := interactiveSetup("tianxuan.toml"); rc != 0 {
			return rc
		}
		// Config just written; reload so .env (and any pinned language) is
		// picked up. If the chosen provider's key is ready, drop into chat.
		if cfg, err := config.Load(); err == nil && cfg.Validate(cfg.DefaultModel) == nil {
			if cfg.Language != "" {
				i18n.DetectLanguage(cfg.Language)
			}
			fmt.Printf("\n"+i18n.M.StartingChatFmt+"\n\n", bold("tianxuan chat"))
			return chatREPL(nil)
		}
		fmt.Println("\n" + i18n.M.SetKeyHint)
		return 0
	}

	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		cfg = config.Default()
	}

	// tianxuan.toml exists and parses on a terminal: go into chat. If any enabled
	// provider's key isn't set yet, re-run the wizard's key-entry step inline
	// — first run already chose language and providers, so we don't re-ask
	// those. Skipping the prompts is still fine; the chat banner falls back to
	// a one-line warning.
	if src != "" && cfgErr == nil && isInteractive() {
		if rc := promptMissingKeys(cfg); rc != 0 {
			return rc
		}
		return chatREPL(nil)
	}

	var b strings.Builder
	b.WriteString(boxed([]string{
		accent("◆") + " " + bold("tianxuan") + "  " + dim(version),
		dim(i18n.M.Subtitle),
	}))

	switch {
	case src == "":
		fmt.Fprintf(&b, "\n  %s %s\n", padRight(i18n.M.ConfigLabel, 8), dim(i18n.M.ConfigNotFound))
	case cfgErr != nil:
		fmt.Fprintf(&b, "\n  %s %s\n", padRight(i18n.M.ConfigLabel, 8), yellow(fmt.Sprintf(i18n.M.ConfigErrorFmt, src, cfgErr)))
	default:
		fmt.Fprintf(&b, "\n  %s %s\n", padRight(i18n.M.ConfigLabel, 8), src)
	}

	ready := 0
	for i, p := range cfg.Providers {
		label := i18n.M.ModelsLabel
		if i > 0 {
			label = ""
		}
		dot, status := yellow("●"), dim(i18n.M.NoKey)
		if p.APIKey() != "" {
			dot, status = green("●"), green(i18n.M.Ready)
			ready++
		}
		fmt.Fprintf(&b, "  %s %s %s%s\n", padRight(label, 8), dot, padRight(p.Name, 16), status)
	}

	fmt.Fprintf(&b, "\n  %s %s\n", accent("▌"), bold(i18n.M.GetStarted))
	n := 1
	step := func(cmd, desc string) {
		fmt.Fprintf(&b, "    %s  %s %s\n", accent(fmt.Sprint(n)), padRight(cmd, 16), dim(desc))
		n++
	}
	if src == "" {
		step("tianxuan setup", i18n.M.StepScaffold)
	}
	if ready == 0 {
		step(i18n.M.StepSetKey, i18n.M.StepSetKeyHint)
	}
	step("tianxuan chat", i18n.M.StepChatDesc)
	step(`tianxuan run "task"`, i18n.M.StepRunDesc)

	fmt.Fprintf(&b, "\n  %s\n", dim(i18n.M.HelpFooter))

	fmt.Print(b.String())
	return 0
}

func usage() {
	fmt.Print(i18n.M.UsageBody)
}
