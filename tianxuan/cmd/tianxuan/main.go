// Command tianxuan is a config- and plugin-driven coding agent CLI.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"tianxuan/internal/cli"

	// Blank imports wire compile-time built-ins into their registries.
	_ "tianxuan/internal/provider/openai"
	_ "tianxuan/internal/tool/builtin"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "panic: %v\n%s\n", r, debug.Stack())
		}
	}()
	os.Exit(cli.Run(os.Args[1:], version))
}
