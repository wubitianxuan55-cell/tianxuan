# Tianxuan · Core

<p align="center"><strong>A minimal AI coding assistant optimized for DeepSeek</strong> — single Go binary, CLI + desktop.</p>

## What is this?

Tianxuan is an AI coding assistant whose core idea is to design the message
structure around DeepSeek's prefix-cache mechanism, keeping long-session token
costs extremely low (measured cache-hit rate routinely 90%+).

- **CLI** — `tianxuan chat` or `tianxuan run "task"` in the terminal
- **Desktop** — a Wails-shelled GUI with a system tray (click X to hide, tray menu to quit)

## Quick start

```bash
# Build the CLI
go build -o tianxuan.exe ./cmd/tianxuan/

# Configure the API key
export DEEPSEEK_API_KEY=sk-...

# Use it
./tianxuan.exe chat          # interactive conversation
./tianxuan.exe run "task"    # one-shot execution
```

## Core design

| Layer | Content | Cache strategy |
|-------|---------|----------------|
| **L1 Identity** | System prompt (~300 tok) | SHA-256 verified, immutable |
| **L2 Runtime** | Project/language/environment (~100 tok) | Locked on first turn |
| **L3 Skills** | Compact tool descriptors (~1200 tok) | 100% hit, no extra cost |
| **L4 Flow** | Conversation history | Three-dimensional compression (HistoryHygiene) |

> Cache is the lifeline: any byte change in L1 breaks the whole prefix → full
> cache miss → ~2.5x cost. All changes must pass the cache-safety check.

## Main features

- **30+ built-in tools** — file read/write, bash, git, LSP, web search, MCP client
- **Plan mode** — complex tasks generate a read-only plan first, executed after approval
- **Permission sandbox** — allow/ask/deny levels with project-scoped write limits
- **MCP plugin** — stdio + Streamable HTTP, compatible with Claude Code `.mcp.json`
- **Session persistence** — session forks, checkpoint rewind, cross-session resume
- **Dual-model collaboration** — executor + planner with separate cache-stable sessions
- **System tray** — desktop app hides to tray on close

## Repository layout

```
cmd/tianxuan/       → CLI entry point
internal/           → core packages (agent/cache/context/control/tool/lsp/…)
desktop/            → desktop app (Wails + React, separate Go module)
scripts/            → release/build/cache-guard scripts
docs/               → engineering specs and migration guide
_archive/           → historical architecture documents
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow.

## Version

Current **V10.177.0** — see [CHANGELOG.md](CHANGELOG.md).

## License

MIT — see [LICENSE](LICENSE).
