# Contributing to Tianxuan

Thank you for your interest in contributing to Tianxuan! This guide covers
everything you need to get started.

## Prerequisites

- **Go 1.25+** — the project targets Go 1.25 (`go.mod`)
- **Git** — for version control
- **Node.js + pnpm** (optional) — only if you work on the desktop app
  (`desktop/frontend/`)

## Getting started

```bash
git clone git@github.com:wubitianxuan55-cell/tianxuan.git
cd tianxuan
go build ./cmd/tianxuan    # builds the CLI binary
go test ./...              # runs the full test suite
```

## Project structure

| Directory | Purpose |
|-----------|---------|
| `cmd/tianxuan` | CLI entry point |
| `internal/agent` | Agent loop, session, coordinator |
| `internal/cli` | TUI, subcommands |
| `internal/control` | Transport-agnostic controller |
| `internal/config` | TOML configuration loading |
| `internal/tool/builtin` | Built-in tools (bash, read_file, …) |
| `internal/provider` | Model-backend abstraction |
| `internal/plugin` | MCP client (stdio + HTTP) |
| `internal/cache` | L1–L4 prefix-cache layers |
| `internal/context` | TCCA cache core |
| `internal/event` | Typed event stream |
| `internal/memory` | Persistent memory + extraction |
| `internal/skill` | Skill discovery from Markdown |
| `internal/sandbox` | OS-level sandboxing |
| `internal/serve` | HTTP/SSE server frontend |
| `internal/checkpoint` | Snapshot-based rewind |
| `desktop/` | Wails-based desktop app (separate Go module) |
| `docs/` | Engineering specs, migration guide |

## Development workflow

### Building

```bash
go build ./...            # build all packages
go build ./cmd/tianxuan   # build the CLI binary
```

### Running tests

```bash
go test ./...                           # all Go tests
go test ./internal/agent/ -v            # verbose, one package
go test ./internal/tool/builtin/ -run TestGrep  # one test
cd desktop && go test ./...             # desktop Go module tests
cd desktop/frontend && npx vitest run   # frontend tests
```

### Verification levels

Match verification to the change scope (see CHANGELOG v10.141.0):

| Change scope | Verification |
|--------------|--------------|
| Go code | `go build` + affected package tests |
| Frontend (`.ts/.tsx/.css`) | `tsc --noEmit` / frontend tests / build |
| Docs/config | content review, no forced tests |
| Cross-module refactor | full suite (`go test ./...`) |

### Code style

- `gofmt` is enforced by CI — format before committing
- Wrap errors with `fmt.Errorf("...: %w", err)`; never swallow errors silently
- Library code never calls `os.Exit` or prints to stdout/stderr
- Only `cli/` and `main/` decide exit codes and user-facing messages
- Exported identifiers must have doc comments

### Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(tool): add ** recursive pattern support
fix(cache): keep compact schema constraints in sync
test(event): add comprehensive unit tests for event package
docs: refresh README version to current release
```

## Adding a new built-in tool

1. Create `internal/tool/builtin/mytool.go`
2. Implement the `tool.Tool` interface: `Name()`, `Description()`, `Schema()`, `ReadOnly()`, `Execute()`
3. Register via `func init() { tool.RegisterBuiltin(myTool{}) }`
4. Add tests in `mytool_test.go` — write the failing test first (TDD)
5. The tool is automatically available — `main` blank-imports `builtin`

## Adding a new model provider

(For MCP tool servers see `internal/plugin` instead — that's a different layer.)

1. Create `internal/provider/myprovider/`
2. Implement `provider.Provider`: `Name()`, `Stream()`
3. Register via `func init() { provider.Register("mykind", New) }`
4. The provider is available from config with `kind = "mykind"`

## Submitting changes

1. Create a feature branch from the current release branch
2. Make your changes with tests (TDD: failing test → minimal implementation → green)
3. Ensure the matching verification level passes (see above)
4. Ensure `gofmt -l .` shows no changes
5. Open a pull request

## License

By contributing, you agree that your contributions will be licensed under the
same license as the project (MIT).
