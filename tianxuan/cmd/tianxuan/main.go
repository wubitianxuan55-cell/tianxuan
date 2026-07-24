// Command tianxuan is a config- and plugin-driven coding agent CLI.
package main

import (
	"os"

	"tianxuan/internal/cli"
	"tianxuan/internal/crash"

	// Blank imports wire compile-time built-ins into their registries.
	_ "tianxuan/internal/provider/anthropic"
	_ "tianxuan/internal/provider/openai"
	_ "tianxuan/internal/provider/xai"
	_ "tianxuan/internal/tool/builtin"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	defer crash.Handle()
	os.Exit(cli.Run(os.Args[1:], version))
}
