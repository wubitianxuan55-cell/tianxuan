package cli

import (
	"fmt"
	"os"

	"tianxuan/internal/tool"
)

// toolsCommand exposes diagnostics about the built-in tool system. Currently
// it reports the cross-session per-tool error stats (V10.154, distilled from
// codex CLI's ToolDispatchTrace) so the host can see which tool and which
// failure mode dominate before deciding where to invest.
func toolsCommand(args []string) int {
	if len(args) > 0 && args[0] == "stats" {
		return toolStatsCommand()
	}
	fmt.Fprintln(os.Stderr, "usage: tianxuan tools <stats>")
	return 2
}

func toolStatsCommand() int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tools stats: %v\n", err)
		return 2
	}
	path := tool.DefaultStatsPath(cwd)
	stats := tool.NewStats(path)
	snap := stats.Snapshot()
	if len(snap) == 0 {
		fmt.Println("no tool errors recorded yet (stats file: " + path + ")")
		return 0
	}
	fmt.Print(stats.Report())
	return 0
}
