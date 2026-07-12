package control

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"tianxuan/internal/config"
	"tianxuan/internal/crash"
	"tianxuan/internal/memory"
)

func (c *Controller) Submit(input string) {
	trimmed := strings.TrimSpace(input)
	switch {
	case trimmed == "/compact" || strings.HasPrefix(trimmed, "/compact "):
		focus := strings.TrimSpace(strings.TrimPrefix(trimmed, "/compact"))
		go func() {
			defer crash.Recover("ctrl-compact")
			if c.Running() {
				c.notice("cannot compact while a turn is running")
				return
			}
			if err := c.Compact(context.Background(), focus); err != nil {
				c.notice("compaction failed: " + err.Error())
			} else {
				c.notice("compacted")
				if err := c.Snapshot(); err != nil {
					slog.Warn("controller: snapshot after compact", "err", err)
				}
			}
		}()
	case strings.HasPrefix(trimmed, "/dream"):
		sub := strings.TrimSpace(strings.TrimPrefix(trimmed, "/dream"))
		go func() {
			defer crash.Recover("ctrl-dream")
			if c.Running() {
				c.notice("cannot dream while a turn is running")
				return
			}
			switch {
			case sub == "scan" || strings.HasPrefix(sub, "scan "):
				dir := c.sessionDir
				if dir == "" {
					dir = config.ArchiveDir()
				}
				text := c.dreamText(dir)
				c.notice(text)
			case sub == "extract" || strings.HasPrefix(sub, "extract "):
				if err := c.Dream(context.Background()); err != nil {
					c.notice("dream extract failed: " + err.Error())
				} else {
					c.notice("dream: knowledge extracted to memory")
					if err := c.Snapshot(); err != nil {
						slog.Warn("controller: snapshot after dream extract", "err", err)
					}
				}
			default:
				// /dream (no args) → show dream analysis
				dir := c.sessionDir
				if dir == "" {
					dir = config.ArchiveDir()
				}
				text := c.dreamText(dir)
				c.notice(text)
				c.notice("Use /dream extract to write learnings to memory, or /dream scan for details")
			}
		}()
	case trimmed == "/memories" || strings.HasPrefix(trimmed, "/memories "):
		query := strings.TrimSpace(strings.TrimPrefix(trimmed, "/memories"))
		if mem := c.Memory(); mem != nil && mem.Search != nil {
			if query == "" {
				query = "project code feature fix"
			}
			matches := mem.Search.Search(query)
			if len(matches) == 0 {
				c.notice("no memories found")
				return
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("memories (%d found):\n", len(matches)))
			limit := 8
			if len(matches) < limit { limit = len(matches) }
			for i, m := range matches[:limit] {
				sb.WriteString(fmt.Sprintf("  %d. [%.0f] %s\n", i+1, m.Score, m.Name))
			}
			if len(matches) > limit {
				sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(matches)-limit))
			}
			c.notice(sb.String())
		} else {
			c.notice("memory search not available")
		}
		return
	case strings.HasPrefix(trimmed, "/goal "):
		goal := strings.TrimSpace(strings.TrimPrefix(trimmed, "/goal"))
		if goal == "" {
			c.notice("usage: /goal <description> — sets the stopping condition")
			return
		}
		c.SetGoal(goal)
		c.notice("goal set: " + goal)
		return
	case strings.HasPrefix(trimmed, "/perm") || strings.HasPrefix(trimmed, "/perm "):
		level := strings.TrimSpace(strings.TrimPrefix(trimmed, "/perm"))
		switch level {
		case "ask", "a":
			c.SetPermLevel("ask")
			c.notice("权限级别: 询问 (写入前需确认)")
		case "auto", "w":
			c.SetPermLevel("auto")
			c.notice("权限级别: 自动 (写入无需确认)")
		case "yolo", "y":
			c.SetPermLevel("yolo")
			c.notice("权限级别: YOLO (跳过所有确认)")
		case "":
			c.notice("当前权限: " + c.PermLevel() + " | 用法: /perm ask|auto|yolo (或 a/w/y)")
		default:
			c.notice("未知权限级别: " + level + " — 使用 ask/auto/yolo (或 a/w/y)")
		}
		return
	case strings.HasPrefix(trimmed, "/distill"):
		sub := strings.TrimSpace(strings.TrimPrefix(trimmed, "/distill"))
		go func() {
			defer crash.Recover("ctrl-distill")
			if c.Running() {
				c.notice("cannot distill while a turn is running")
				return
			}
			switch {
			case sub == "scan" || strings.HasPrefix(sub, "scan "):
				dir := c.sessionDir
				if dir == "" {
					dir = config.ArchiveDir()
				}
				text := c.distillText(dir)
				c.notice(text)
			case sub == "create" || strings.HasPrefix(sub, "create "):
				// Generate skill templates from detected patterns
				patterns := c.detectPatterns()
				if len(patterns) == 0 {
					c.notice("distill: no patterns to create skills from")
					return
				}
				created := c.createSkillTemplates(patterns)
				c.notice("distill: " + fmt.Sprintf("%d", created) + " skill templates created in .tianxuan/skills/")
			default:
				if err := c.Distill(context.Background()); err != nil {
					c.notice("distill failed: " + err.Error())
				} else {
					c.notice("distill complete — patterns saved to memory")
					c.notice("Use /distill create to generate skill templates, or /distill scan for details")
				}
			}
		}()
	case trimmed == "/new":
		go func() {
			defer crash.Recover("ctrl-new-session")
			if c.Running() {
				c.notice("cannot start new session while a turn is running")
				return
			}
			if err := c.NewSession(); err != nil {
				c.notice("new session failed: " + err.Error())
			} else {
				c.notice("new session")
			}
		}()
	case strings.HasPrefix(trimmed, "#"):
		// "#<note>" quick-adds a memory line — same shortcut as the chat TUI, so
		// the desktop and HTTP frontends (which route raw input through Submit)
		// get it for free. It never starts a model turn.
		note := strings.TrimSpace(trimmed[1:])
		if note == "" {
			c.notice("nothing to remember")
			return
		}
		if path, err := c.QuickAdd(memory.ScopeProject, note); err != nil {
			c.notice("memory: " + err.Error())
		} else {
			c.notice("remembered → " + path)
		}
	case strings.HasPrefix(trimmed, "/mcp__"):
		c.runGuarded(func(ctx context.Context) error {
			sent, found, err := c.MCPPrompt(ctx, trimmed)
			if err != nil {
				return err
			}
			if !found {
				c.notice("unknown command: " + trimmed)
				return nil
			}
			return c.runTurnWithRaw(ctx, sent, sent)
		})
	case strings.HasPrefix(trimmed, "/"):
		// Read-only management verbs (/model /memory /skill /hooks /mcp) emit a
		// listing Notice, so Submit-based frontends (desktop, HTTP) get them with
		// no extra wiring. (The chat TUI handles these itself with richer output.)
		fields := strings.Fields(trimmed)
		switch fields[0] {
		case "/tree":
			c.notice(c.BranchTreeText())
			return
		case "/branch":
			args := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
			if turn, name, fromTurn, err := ParseBranchTarget(args); err != nil {
				c.notice(err.Error())
			} else if fromTurn {
				if _, err := c.ForkNamed(turn-1, name); err != nil {
					c.notice(err.Error())
				}
			} else {
				if _, err := c.Branch(name); err != nil {
					c.notice(err.Error())
				}
			}
			return
		case "/switch":
			ref := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
			if _, err := c.SwitchBranch(ref); err != nil {
				c.notice(err.Error())
			}
			return
		case "/undo":
			args := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
			n := 1
			if args != "" {
				if parsed, err := strconv.Atoi(args); err == nil && parsed > 0 {
					n = parsed
				}
			}
			checkpoints := c.Checkpoints()
			if len(checkpoints) == 0 {
				c.notice("undo: nothing to undo (no checkpoints)")
				return
			}
			target := len(checkpoints) - n
			if target < 0 {
				target = 0
			}
			if target >= len(checkpoints) {
				c.notice(fmt.Sprintf("undo: only %d turn(s) available", len(checkpoints)))
				return
			}
			turn := checkpoints[target].Turn
			if err := c.Rewind(turn, RewindBoth); err != nil {
				c.notice("undo failed: " + err.Error())
				return
			}
			c.notice(fmt.Sprintf("undo: rewound to turn %d (%d turn(s))", turn, n))
			return
		}
		if c.managementNotice(trimmed) {
			return
		}
		// A custom command wins over a skill of the same name; both resolve to a
		// turn. (Built-in slash verbs like /compact are handled above.)
		if sent, ok := c.CustomCommand(trimmed); ok {
			c.runGuarded(func(ctx context.Context) error {
				return c.runTurnWithRaw(ctx, sent, sent)
			})
			return
		}
		if sent, ok := c.RunSkill(trimmed); ok {
			c.runGuarded(func(ctx context.Context) error {
				return c.runTurnWithRaw(ctx, sent, sent)
			})
			return
		}
		c.notice("unknown command: " + trimmed)
	default:
		c.runGuarded(func(ctx context.Context) error {
			block, errs := c.ResolveRefs(ctx, input)
			for _, e := range errs {
				c.notice(e)
			}
			sent := input
			if block != "" {
				sent = "Referenced context:\n\n" + block + "\n\n" + input
			}
			return c.runTurnWithRaw(ctx, sent, input)
		})
	}
}

// notice emits an informational Notice event.
