package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"tianxuan/internal/tool"
)

// toolsCommand exposes diagnostics about the built-in tool system. Currently
// it reports the cross-session per-tool error stats (V10.154, distilled from
// codex CLI's ToolDispatchTrace) so the host can see which tool and which
// failure mode dominate before deciding where to invest, plus the per-dispatch
// JSONL trace (V10.167) for offline error-rate analysis.
func toolsCommand(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "stats":
			return toolStatsCommand()
		case "trace":
			return toolTraceCommand(args[1:])
		}
	}
	fmt.Fprintln(os.Stderr, "usage: tianxuan tools <stats|trace [-n N]>")
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

// toolTraceCommand prints the most recent N dispatch-trace lines (JSONL, one
// per tool call) in chronological order. Default 10; pass -n to change.
func toolTraceCommand(args []string) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tools trace: %v\n", err)
		return 2
	}
	n := 10
	if len(args) == 2 && args[0] == "-n" {
		if v, err := strconv.Atoi(args[1]); err == nil && v > 0 {
			n = v
		} else {
			fmt.Fprintf(os.Stderr, "tools trace: invalid -n %q (want a positive integer)\n", args[1])
			return 2
		}
	} else if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "usage: tianxuan tools trace [-n N]")
		return 2
	}

	path := tool.DefaultTracePath(cwd)
	f, err := os.Open(path)
	if err != nil {
		fmt.Println("no tool dispatches recorded yet (trace file: " + path + ")")
		return 0
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "tools trace: %v\n", err)
		return 2
	}
	if len(lines) == 0 {
		fmt.Println("no tool dispatches recorded yet (trace file: " + path + ")")
		return 0
	}

	start := len(lines) - n
	if start < 0 {
		start = 0
	}
	fmt.Printf("%d of %d dispatches (tail %d) from %s\n", len(lines)-start, len(lines), n, path)
	for _, l := range lines[start:] {
		var e tool.TraceEntry
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			fmt.Println(l)
			continue
		}
		args := e.Args
		if len(args) > 60 {
			args = args[:60] + "..."
		}
		fmt.Printf("%s %-18s %-7s %6dms %s\n",
			strings.TrimPrefix(e.Ts, "T"), e.Tool, e.Outcome, e.DurationMs, args)
	}
	return 0
}
