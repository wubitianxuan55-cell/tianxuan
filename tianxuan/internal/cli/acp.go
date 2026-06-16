package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"tianxuan/internal/acp"
	"tianxuan/internal/agent"
	"tianxuan/internal/boot"
	"tianxuan/internal/command"
	"tianxuan/internal/config"
	"tianxuan/internal/control"
	"tianxuan/internal/event"
	"tianxuan/internal/i18n"
	"tianxuan/internal/permission"
	"tianxuan/internal/plugin"
	"tianxuan/internal/sandbox"
	"tianxuan/internal/tool"
	"tianxuan/internal/tool/builtin"
)

// acpCommand runs Tianxuan as an Agent Client Protocol agent: a stdio JSON-RPC
// server that editors and other host clients drive (initialize, session/new,
// session/prompt, session/cancel). It keeps v2 wire-compatible with the many
// tools that integrated with v1 over ACP.
//
// stdin/stdout are the JSON-RPC channel — nothing else may write to stdout, so
// all diagnostics go to stderr. Each session is assembled by acpFactory, rooted
// at the cwd the client opens.
func acpCommand(args []string, version string) int {
	fs := flag.NewFlagSet("acp", flag.ContinueOnError)
	model := fs.String("model", "", "provider name (default: config default_model)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	modelName := *model
	if modelName == "" {
		modelName = cfg.DefaultModel
	}
	// Fail fast on a missing/invalid key, with stderr (never stdout) so the wire
	// stays clean, rather than failing per-session deep inside session/new.
	if err := cfg.Validate(modelName); err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	if cfg.BashMode() == "enforce" && !sandbox.Available() {
		fmt.Fprintln(os.Stderr, "warning: bash sandbox requested but unavailable on this platform; running bash unconfined")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	factory := &acpFactory{cfg: cfg, model: modelName}
	info := acp.AgentInfo{Name: "tianxuan", Version: version}
	if err := acp.Serve(ctx, os.Stdin, os.Stdout, factory, info); err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	return 0
}

// acpFactory builds one control.Controller per ACP session. It mirrors setup()'s
// assembly, with two differences that make sessions independent: the built-in
// tools are bound to the session's cwd via builtin.Workspace (so concurrent
// sessions have separate path roots), and the client's per-session MCP servers
// are connected alongside the config's own plugins.
type acpFactory struct {
	cfg   *config.Config
	model string
}

// NewSession assembles the per-session controller. Resources (MCP subprocesses)
// are released via the controller's Cleanup, run on ctrl.Close().
func (f *acpFactory) NewSession(ctx context.Context, p acp.SessionParams) (*control.Controller, error) {
	cfg := f.cfg
	entry, ok := cfg.ResolveModel(f.model)
	if !ok {
		return nil, fmt.Errorf("unknown model %q", f.model)
	}
	execProv, err := boot.NewProvider(entry)
	if err != nil {
		return nil, err
	}
	sysPrompt, err := cfg.ResolveSystemPrompt()
	if err != nil {
		return nil, err
	}

	// Built-ins rooted at the session cwd. Writes confine to that cwd by default
	// (Workspace makes Dir the sole write root when WriteRoots is empty), which is
	// the right scope for a client that opened the session on a project; an empty
	// cwd falls back to process-cwd tools, identical to the headless run.
	reg := tool.NewRegistry()
	var writeRoots []string
	if p.Cwd != "" {
		writeRoots = []string{p.Cwd}
	}
	bashSpec := sandbox.Spec{Mode: cfg.BashMode(), WriteRoots: writeRoots, Network: cfg.Sandbox.Network}
	ws := builtin.Workspace{Dir: p.Cwd, WriteRoots: writeRoots, Bash: bashSpec}
	for _, t := range ws.Tools(cfg.Tools.Enabled...) {
		reg.Add(t)
	}

	// MCP: the config's own plugins plus the servers the client passed in
	// session/new, all connected for the session's lifetime.
	cleanup := func() {}
	var host *plugin.Host
	specs := append(boot.PluginSpecs(cfg.AutoStartPlugins()), p.MCPServers...)
	if len(specs) > 0 {
		h, ptools := plugin.StartAvailable(ctx, specs)
		host = h
		cleanup = h.Close
		for _, t := range ptools {
			reg.Add(t)
		}
		if text, ok := boot.MCPStartupNotice(h.Failures()); ok {
			p.Sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: text})
		}
	}

	maxSteps := cfg.Agent.MaxSteps
	policy := permission.New(cfg.Permissions.Mode, cfg.Permissions.Allow, cfg.Permissions.Ask, cfg.Permissions.Deny)
	headlessGate := permission.NewGate(policy, nil)
	reg.Add(agent.NewTaskTool(execProv, entry.Price, reg, maxSteps,
		entry.ContextWindow, cfg.Agent.Temperature, config.ArchiveDir(), "", headlessGate))

	executor := agent.New(execProv, reg, agent.NewSession(sysPrompt), agent.Options{
		MaxSteps:      maxSteps,
		Temperature:   cfg.Agent.Temperature,
		Pricing:       entry.Price,
		Gate:          headlessGate,
		ContextWindow: entry.ContextWindow,
		ArchiveDir:    config.ArchiveDir(),
	}, p.Sink)

	cmds, _ := command.Load(config.CommandDirs()...)

	label := entry.Model

	return control.New(control.Options{
		Runner:       executor,
		Executor:     executor,
		Sink:         p.Sink,
		Policy:       policy,
		Label:        label,
		SystemPrompt: sysPrompt,
		SessionDir:   config.SessionDir(),
		Host:         host,
		Commands:     cmds,
		Cleanup:      cleanup,
	}), nil
}
