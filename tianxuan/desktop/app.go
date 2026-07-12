package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tianxuan/internal/agent"
	"tianxuan/internal/boot"
	"tianxuan/internal/config"
	"tianxuan/internal/control"
	"tianxuan/internal/event"
	"tianxuan/internal/i18n"
	"tianxuan/internal/provider"
	"tianxuan/internal/schedule"
	"tianxuan/internal/serve"
)

// eventChannel is the Wails runtime event name the frontend subscribes to for the
// agent's typed event stream. One channel carries every event kind; the payload's
// `kind` field discriminates — the desktop analogue of the serve transport's SSE
// `data:` frames.
const eventChannel = "agent:event"

// App is the Wails-bound application object: the desktop frontend's command
// surface. Its exported methods (Submit/Cancel/Approve/…) are generated into JS
// bindings and call straight through to one transport-agnostic control.Controller
// — the same controller the chat TUI and the HTTP/SSE server drive, assembled by
// the shared internal/boot. Events flow the other way: the controller emits to an
// eventSink that forwards each one to the webview via runtime.EventsEmit.
type App struct {
	ctx  context.Context
	sink *eventSink

	// --- multi-tab support ---
	// tabs holds all open conversation tabs. The map and tabOrder are protected
	// by mu. activeTabID points to the currently selected tab.
	tabs        map[string]*WorkspaceTab
	tabOrder    []string
	activeTabID string

	// ctrl is a convenience pointer to the active tab's controller (nil until
	// the first tab is built). Legacy code reads this directly; new code should
	// prefer activeCtrlLocked() for thread safety.
	ctrl *control.Controller

	// mu protects ctrl, label, model, startupErr, ready, tabs, tabOrder,
	// activeTabID, and per-tab fields. startup() spawns a goroutine for
	// boot.Build(); all methods that touch the controller acquire the lock.
	mu          sync.RWMutex
	startupErr  string
	label       string
	model       string // active provider name (for the bottom model switcher)
	subagentLabel  string // subagent model label (from config, or empty)
	plannerLabel   string // planner model label (from config, or empty; V10.31)
	ready          bool   // true once boot.Build completes (success or failure)
	disabledMCP map[string]ServerView
	mcpOrder    []string
	scheduler   *schedule.Scheduler
}

// NewApp constructs the bound object. The controller is built later, in startup,
// once the Wails context exists.
func NewApp() *App {
	a := &App{
		sink:        &eventSink{},
		tabs:        map[string]*WorkspaceTab{},
		disabledMCP: map[string]ServerView{},
	}
	a.sink.app = a
	return a
}

// startup runs once the webview process is up, before the frontend can issue any
// bound call. It captures the Wails context (needed for EventsEmit), points the
// sink at it, then kicks off the entire initialization (workspace, config, build)
// in a background goroutine so the webview loads immediately. The frontend polls
// Meta() and sees Ready flip to true once the controller is assembled. RequireKey
// is false so a missing API key opens the window in a "set your key" state rather
// than failing to launch; a build error is surfaced through Meta instead of
// crashing the window.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.sink.ctx = ctx

	// Drain and log any crash from a prior session.
	a.flushPendingCrash()

	// 居中窗口避免被任务栏遮挡底部（状态栏 + 输入框）
	runtime.WindowCenter(ctx)

	// Everything else — workspace resolution, config loading, i18n setup, and
	// boot.Build — runs in the background so the webview appears instantly.
	// During this window Meta().Ready is false and the frontend shows a loading
	// state; bound calls are no-ops (ctrl is nil).
	a.goSafe("buildController", a.buildController)

	// Start system tray — close minimizes to tray, not exit.
	a.goSafe("runTray", func() { runTray(ctx, a) })
}

// domReady fires after the frontend has loaded and rendered. Restore saved
// window geometry and show the window (StartHidden keeps it invisible until now).
func (a *App) domReady(ctx context.Context) {
	if saved, ok := loadWindowState(); ok {
		if saved.Maximised {
			runtime.WindowMaximise(ctx)
		} else if saved.X > 0 || saved.Y > 0 {
			runtime.WindowSetPosition(ctx, saved.X, saved.Y)
		}
	}
	runtime.WindowShow(ctx)
}

// buildController runs the full initialization sequence in a background goroutine:
// workspace resolution, config loading, i18n setup, and boot.Build. On success it
// wires up the controller and flips ready; on failure it stores the error so
// Meta().StartupErr surfaces it.
func (a *App) buildController() {
	ctx := a.ctx // captured by startup before this goroutine starts

	// A GUI launch starts in "/" (read-only); move into a real, writable working
	// folder (the remembered one, else home) before anything reads/writes config,
	// .env, memory, or skills relative to cwd.
	ensureWorkspace()
	cwd, err := os.Getwd()
	if err != nil {
		cwd, _ = os.UserHomeDir()
		_ = os.Chdir(cwd)
	}
	// 持久化当前工作空间：正常启动/关闭时从未调用 saveWorkspace，
	// 导致下次启动无法恢复。现在每次 buildController 都保存当前 cwd。
	saveWorkspace(cwd)

	// Resolve the active model to its canonical "provider/model" ref up front so
	// the switcher can mark it current.
	model := ""
	if cfg, errCfg := config.Load(); errCfg == nil {
		// Drive the Go-side catalogue (i18n.M) from the configured language so the
		// backend-provided slash UI — command descriptions, sub-command hints,
		// listing notices — comes through localized, matching the frontend.
		i18n.DetectLanguage(cfg.Language)
		model = cfg.DefaultModel
		if e, ok := cfg.ResolveModel(cfg.DefaultModel); ok {
			model = e.Name + "/" + e.Model
		}
		// Resolve subagent model from config (empty when not configured).
		if subRef := strings.TrimSpace(cfg.Agent.SubagentModel); subRef != "" {
			if subEntry, ok := cfg.ResolveModel(subRef); ok {
				a.subagentLabel = subEntry.Name
			}
		}
		// Resolve planner model from config (V10.31: separate planner label for stats).
		if plannerRef := strings.TrimSpace(cfg.Agent.PlannerModel); plannerRef != "" {
			if plannerEntry, ok := cfg.ResolveModel(plannerRef); ok {
				a.plannerLabel = plannerEntry.Name
			}
		}
	}

	a.mu.Lock()
	a.model = model
	a.mu.Unlock()

	// Create the default tab. Every desktop session has at least one tab;
	// this keeps backward compatibility with all existing bound methods.
	tab := newWorkspaceTab("project", "", "新会话")
	a.mu.Lock()
	a.tabs[tab.ID] = tab
	a.tabOrder = []string{tab.ID}
	a.activeTabID = tab.ID
	a.mu.Unlock()

	ctrl, buildErr := boot.Build(ctx, boot.Options{
		Model: model, RequireKey: false, Sink: a.sink,
		SessionDir: config.WorkspaceSessionDir(cwd),
	})
	if buildErr != nil {
		a.mu.Lock()
		a.startupErr = buildErr.Error()
		tab.StartupErr = buildErr.Error()
		a.ready = true
		tab.Ready = true
		a.mu.Unlock()
		runtime.EventsEmit(ctx, "agent:ready")
		return
	}

	a.mu.Lock()
	a.ctrl = ctrl
	tab.Ctrl = ctrl
	a.label = ctrl.Label()
	tab.Label = ctrl.Label()
	a.ready = true
	tab.Ready = true
	a.mu.Unlock()

	// Desktop is interactive: route "ask" gate decisions to the frontend as
	// approval_request events, answered via Approve.
	ctrl.EnableInteractiveApproval()

	// Start the scheduler for timed tasks.
	a.startScheduler()

	// Land auto-save in a fresh session file (same as a fresh chat/serve start).
	if dir := ctrl.SessionDir(); dir != "" {
		ctrl.SetSessionPath(agent.NewSessionPath(dir, ctrl.Label()))
	}

	// V1.6: auto-resume the most recent session from this workspace.
	// ListSessions returns sessions sorted by mtime descending; the first
	// entry is the latest one. If it has content (Turns > 0), load and
	// resume it so the user picks up where they left off. Corrupt files
	// and empty sessions are skipped silently — the fresh session created
	// above is the safe fallback.
	if dir := ctrl.SessionDir(); dir != "" {
		sessions, _ := agent.ListSessions(dir)
		if len(sessions) > 0 && sessions[0].Turns > 0 {
			if loaded, err := agent.LoadSession(sessions[0].Path); err == nil {
				// Replace the loaded session's system prompt with the fresh L1.
				// Old sessions may have a different system prompt format (e.g.
				// V1.4 included Profile in L1; V1.5 keeps it in turn-tail). Using
				// the fresh L1 ensures the prefix matches across sessions for
				// DeepSeek's global cache.
				if msgs := loaded.Messages; len(msgs) > 0 && msgs[0].Role == provider.RoleSystem {
					if sys := ctrl.SystemPrompt(); sys != "" {
						msgs[0] = provider.Message{Role: provider.RoleSystem, Content: sys}
					}
				}
				ctrl.Resume(loaded, sessions[0].Path)
				slog.Info("auto-resumed session", "path", sessions[0].Path, "turns", sessions[0].Turns)
			}
		}
	}

	// Notify the frontend that the controller is ready — it re-fetches Meta,
	// ContextUsage, and History.
	runtime.EventsEmit(ctx, "agent:ready")

	// Auto-start mobile access if previously configured.
	// FIXME: re-enable after root-causing desktop crash on executor execution.
	// a.AutoStartMobileAccess()

	// Persist the default tab so a relaunch can restore it.
	a.saveTabs()
}

// shutdown snapshots every tab's conversation and stops plugin subprocesses.
func (a *App) shutdown(context.Context) {
	// Save window geometry before the webview tears down.
	a.saveWindowStateSync()
	if a.scheduler != nil {
		a.scheduler.Stop()
	}
	a.mu.RLock()
	tabs := make([]*WorkspaceTab, 0, len(a.tabs))
	for _, t := range a.tabs {
		tabs = append(tabs, t)
	}
	a.mu.RUnlock()
	for _, tab := range tabs {
		if tab.Ctrl != nil {
			_ = tab.Ctrl.Snapshot()
			tab.Ctrl.Close()
		}
	}
}

// eventSink is the controller's event.Sink in desktop mode: it forwards every
// agent event to the webview as one runtime event, JSON-shaped by toWire. It is a
// type distinct from App so App's bound method set stays the clean command surface
// — Emit must not be exposed to JS. Emit runs on the agent goroutine;
// runtime.EventsEmit is goroutine-safe, and the ctx guard covers the brief window
// before startup assigns it.
type eventSink struct {
	ctx context.Context
	app *App // optional back-reference for tab ID injection
	bc  *serve.Broadcaster // optional: mobile SSE broadcaster (set by StartMobileAccess)
}

func (s *eventSink) Emit(e event.Event) {
	if s.ctx == nil {
		return
	}
	// Inject the active tab ID so the frontend can route events. When no tabs
	// exist yet (during initial boot), omit the field.
	tabID := ""
	if s.app != nil {
		s.app.mu.RLock()
		tabID = s.app.activeTabID
		s.app.mu.RUnlock()
	}
	if tabID != "" {
		runtime.EventsEmit(s.ctx, eventChannel, toWireTab(e, tabID))
	} else {
		runtime.EventsEmit(s.ctx, eventChannel, toWire(e))
	}
	// Also forward to the mobile SSE broadcaster if active.
	if s.bc != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("desktop: eventSink mobile broadcast panic", "panic", r)
				}
			}()
			s.bc.Emit(e)
		}()
	}
}

// --- tab helper methods ---

// activeCtrlLocked returns the active tab's controller. Must be called with
// mu held (read or write). Falls back to a.ctrl for backward compatibility.
func (a *App) activeCtrlLocked() *control.Controller {
	if a.activeTabID != "" {
		if tab := a.tabs[a.activeTabID]; tab != nil && tab.Ctrl != nil {
			return tab.Ctrl
		}
	}
	return a.ctrl
}

// activeTabLocked returns the active tab. Must be called with mu held.
func (a *App) activeTabLocked() *WorkspaceTab {
	if a.activeTabID != "" {
		return a.tabs[a.activeTabID]
	}
	return nil
}

// TabMeta returns metadata for every open tab, for the frontend tab bar.
func (a *App) TabMeta() []TabMeta {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]TabMeta, 0, len(a.tabOrder))
	for _, id := range a.tabOrder {
		if tab := a.tabs[id]; tab != nil && !tab.closing {
			out = append(out, tab.tabMeta())
		}
	}
	return out
}

// SelectTab switches the active tab. The empty string selects the first tab.
func (a *App) SelectTab(tabID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if tabID == "" || a.tabs[tabID] == nil {
		if len(a.tabOrder) > 0 {
			tabID = a.tabOrder[0]
		} else {
			return
		}
	}
	a.activeTabID = tabID
	if tab := a.tabs[tabID]; tab != nil && tab.Ctrl != nil {
		a.ctrl = tab.Ctrl
		a.label = tab.Label
		a.model = tab.model
	}
}

// ctrlByTabID returns the controller for the given tab. An empty tabID resolves
// to the active tab's controller. Returns nil when no controller is available.
func (a *App) ctrlByTabID(tabID string) *control.Controller {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if tabID == "" {
		return a.activeCtrlLocked()
	}
	if tab := a.tabs[tabID]; tab != nil {
		return tab.Ctrl
	}
	return nil
}

// tabByIDLocked returns the tab for the given ID. Must be called with mu held.
// An empty tabID resolves to the active tab.
func (a *App) tabByIDLocked(tabID string) *WorkspaceTab {
	if tabID == "" {
		return a.activeTabLocked()
	}
	return a.tabs[tabID]
}
