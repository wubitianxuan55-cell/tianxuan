package boot

import (
	"context"
	"io"
	"os"

	"tianxuan/internal/codegraph"
	"tianxuan/internal/config"
	"tianxuan/internal/event"
	"tianxuan/internal/lsp"
	"tianxuan/internal/plugin"
	"tianxuan/internal/tool"
)

// pluginsOut carries the artifacts from starting plugins and LSP.
type pluginsOut struct {
	host    *plugin.Host
	lspMgr  *lsp.Manager
	cleanup func()
}

// startPlugins initialises CodeGraph (if enabled), Context7 (if key set),
// configured MCP servers, and LSP tools. It returns a cleanup function that
// shuts down all spawned subprocesses.
func startPlugins(ctx context.Context, cfg *config.Config, reg *tool.Registry, sink event.Sink, stderrPath io.Writer) *pluginsOut {
	out := &pluginsOut{}
	pluginHost := plugin.NewHost()
	specs := PluginSpecs(cfg.AutoStartPlugins())

	if cfg.Codegraph.Enabled {
		bin, ok := codegraph.Resolve(cfg.Codegraph.Path)
		switch {
		case ok:
			cwd, _ := os.Getwd()
			if err := codegraph.EnsureInit(ctx, bin, cwd); err != nil {
				sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
					Text: "codegraph: init failed (" + err.Error() + ") — symbol-graph tools disabled this session"})
			}
			specs = append(specs, plugin.Spec{Name: "codegraph", Command: bin, Args: []string{"serve", "--mcp"}, Dir: cwd})
		case cfg.Codegraph.AutoInstall:
			notify := func(msg string) { sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Text: msg}) }
			notify("codegraph: fetching code-intelligence runtime in the background (one-time) — symbol-graph tools available next session")
			go func() {
				if _, err := codegraph.Install(context.WithoutCancel(ctx), nil); err != nil {
					notify("codegraph: install failed (" + err.Error() + ") — using grep/glob; retries next session")
				} else {
					notify("codegraph: installed — symbol-graph tools available next session")
				}
			}()
		default:
			sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
				Text: "codegraph: not installed — run `tianxuan codegraph install` to enable symbol-graph tools"})
		}
	}

	if key := os.Getenv("CONTEXT7_API_KEY"); key != "" {
		specs = append(specs, plugin.Spec{
			Name:    "context7",
			Type:    "http",
			URL:     "https://mcp.context7.com/mcp",
			Headers: map[string]string{"Authorization": "Bearer " + key},
		})
	}
	if len(specs) > 0 {
		if stderrPath != nil {
			for i := range specs {
				specs[i].Stderr = stderrPath
			}
		}
		host, ptools := plugin.StartAvailable(ctx, specs)
		pluginHost = host
		for _, t := range ptools {
			reg.Add(t)
		}
		if text, ok := MCPStartupNotice(host.Failures()); ok {
			sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: text})
		}
	}
	out.host = pluginHost
	out.cleanup = pluginHost.Close

	if cfg.LSP.Enabled {
		cwd, _ := os.Getwd()
		out.lspMgr = lsp.NewManager(cwd, LSPSpecs(cfg.LSP))
		for _, t := range lsp.Tools(out.lspMgr) {
			reg.Add(t)
		}
		prev := out.cleanup
		out.cleanup = func() { prev(); out.lspMgr.Close() }
	}
	return out
}
