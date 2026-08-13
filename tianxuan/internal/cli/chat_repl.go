package cli

import (
	"charm.land/bubbletea/v2"
	"context"
	"flag"
	"fmt"
	"golang.org/x/term"
	"os"
	"tianxuan/internal/agent"
	"tianxuan/internal/config"
	"tianxuan/internal/control"
	"tianxuan/internal/event"
	"tianxuan/internal/i18n"
	"tianxuan/internal/notify"
	"tianxuan/internal/provider"
	"time"
)

// chatREPL is an interactive session: a single persistent agent/session and a
// prompt loop that keeps conversation context across turns. Exit with
// 'exit'/'quit' or Ctrl-D.
func chatREPL(args []string) int {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	model := fs.String("model", "", "provider name (default: config default_model)")
	maxSteps := fs.Int("max-steps", 0, "max tool-call rounds (0 = use config/default)")
	cont := fs.Bool("continue", false, "resume the most recent saved session")
	fs.BoolVar(cont, "c", false, "shorthand for --continue")
	resume := fs.Bool("resume", false, "list saved sessions and pick one to resume")
	yolo := fs.Bool("dangerously-skip-permissions", false, "YOLO: auto-approve every tool call this session (deny rules still apply)")
	fs.BoolVar(yolo, "yolo", false, "alias for --dangerously-skip-permissions")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Decide whether we're starting fresh or resuming. --resume opens an
	// interactive picker; --continue / -c jumps straight into the newest.
	var resumePath string
	switch {
	case *resume:
		path, rc := pickSessionToResume()
		if rc != 0 {
			return rc
		}
		resumePath = path
	case *cont:
		sessions, err := agent.ListSessions(config.SessionDir())
		if err != nil {
			fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
			return 1
		}
		if len(sessions) == 0 {
			fmt.Fprintln(os.Stderr, i18n.M.NoSessionToResume)
			return 1
		}
		resumePath = sessions[0].Path
	}

	ctx := context.Background()

	// Plumb the controller's typed event stream through a channel so each event
	// can become a tea.Msg inside the TUI's update loop. Buffered generously:
	// streaming bursts (tool results, long answers) shouldn't backpressure the
	// agent goroutine.
	eventCh := make(chan event.Event, 1024)

	sink := event.Sink(&eventSink{ch: eventCh})

	// Wire desktop notifications on turn completion when enabled in config.
	if cfg, nerr := config.Load(); nerr == nil && cfg.Notify.Enabled {
		sink = notify.NewTurnDoneSink(sink, time.Duration(cfg.Notify.MinDuration)*time.Second)
	}

	ctrl, err := setup(ctx, *model, *maxSteps, false, sink)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}

	// Decide where this conversation's auto-save lands. A resume reuses the
	// file so closing/reopening keeps appending to the same history; a fresh
	// session lands in a new file stamped with the model name.
	if resumePath != "" {
		if loaded, err := agent.LoadSession(resumePath); err != nil {
			fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
			return 1
		} else {
			ctrl.Resume(loaded, resumePath)
		}
	} else if ctrl.SessionDir() != "" {
		ctrl.SetSessionPath(agent.NewSessionPath(ctrl.SessionDir(), ctrl.Label()))
	}

	// Surface a missing-key warning inside the TUI banner so the first message
	// failing is at least pre-announced; the user can still enter chat.
	missing := ""
	if cfg, loadErr := config.Load(); loadErr == nil {
		name := *model
		if name == "" {
			name = cfg.DefaultModel
		}
		if vErr := cfg.Validate(name); vErr != nil {
			missing = vErr.Error()
		}
	}

	// Initial terminal width — the TUI re-flows on every WindowSizeMsg so
	// this is just a starting estimate before the first resize event lands.
	termW := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		termW = w
	}

	// Route "ask" decisions to the TUI: the controller emits an ApprovalRequest
	// event and blocks until the user answers via ctrl.Approve. Sub-agents (the
	// task tool) keep their headless gate from setup — no UI to prompt through.
	ctrl.EnableInteractiveApproval()
	if *yolo {
		ctrl.SetPermLevel("yolo")
	}

	m := newChatTUI(ctrl, missing, eventCh, termW)
	if cfg, err := config.Load(); err == nil {
		m.outputStyle = cfg.Agent.OutputStyle    // shown as the active entry in /output-style
		m.statuslineCmd = cfg.Statusline.Command // custom status-line command, "" = built-in row
	}

	// /model support: a pure builder the TUI calls to rebuild on a different
	// model (carrying the conversation). It must NOT touch the running model —
	// runModelSubcommand performs the swap on the live copy. The same stable sink
	// feeds the new controller, so events keep flowing to this TUI.
	m.buildController = func(ref string, carry []provider.Message) (*control.Controller, error) {
		c, err := setupQuiet(ctx, ref, *maxSteps, false, sink)
		if err != nil {
			return nil, err
		}
		path := ""
		if dir := c.SessionDir(); dir != "" {
			path = agent.NewSessionPath(dir, c.Label())
		}
		if len(carry) > 0 {
			c.Resume(&agent.Session{Messages: carry}, path)
		} else if path != "" {
			c.SetSessionPath(path)
		}
		c.EnableInteractiveApproval()
		if *yolo {
			c.SetPermLevel("yolo")
		}
		return c, nil
	}
	if cfg, e := config.Load(); e == nil {
		name := *model
		if name == "" {
			name = cfg.DefaultModel
		}
		if entry, ok := cfg.ResolveModel(name); ok {
			m.modelRef = entry.Name + "/" + entry.Model
		}
	}

	// No alt-screen: finalized transcript lines are committed to the terminal's
	// normal buffer (via tea.Println) so native scrollback, the wheel, and copy
	// all work — the bubbletea-managed region is just the bottom input/status.
	p := tea.NewProgram(m)
	final, runErr := p.Run()
	// Close the active controller plus any retired ones from /model switches.
	// Retired controllers were stashed rather than closed at switch time
	// because Controller.Close() runs SessionEnd hooks and kills plugin
	// subprocesses — operations that corrupt bubbletea's terminal raw mode
	// when executed while the TUI is alive.
	if fm, ok := final.(chatTUI); ok {
		for _, oc := range fm.oldControllers {
			oc.Close()
		}
		if fm.ctrl != nil {
			fm.ctrl.Close()
		} else {
			ctrl.Close()
		}
	} else {
		ctrl.Close()
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, runErr)
		return 1
	}
	return 0
}
