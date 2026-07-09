package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tianxuan/internal/config"
	"tianxuan/internal/control"
	"tianxuan/internal/event"
)

// WorkspaceTab is one open conversation tab in the desktop. Each tab owns an
// independent controller so multiple projects and topics can be active
// concurrently. The zero value is not valid — use newWorkspaceTab to create.
type WorkspaceTab struct {
	ID            string             // stable random id
	Scope         string             // "project" | "global"
	WorkspaceRoot string             // project root dir (empty for global)
	TopicTitle    string             // display title
	SessionPath   string             // exact .jsonl file this tab continues
	Ctrl          *control.Controller // nil while booting / on error
	Label         string             // model label (for the tab badge)
	Ready         bool               // true once boot.Build completes
	StartupErr    string             // build error, surfaced to the frontend

	// Per-tab MCP state carried from the old App-level fields.
	DisabledMCP map[string]ServerView
	MCPOrder    []string

	// ActivityStatus is the transient project-tree status for the in-flight turn.
	ActivityStatus string

	closing   bool

	model string // active model ref (for meta)
}

// newWorkspaceTab creates a tab with a random ID and initializes the save
// condition variable.
func newWorkspaceTab(scope, workspaceRoot, topicTitle string) *WorkspaceTab {
	return &WorkspaceTab{
		ID:            randomTabID(),
		Scope:         scope,
		WorkspaceRoot: workspaceRoot,
		TopicTitle:    topicTitle,
		DisabledMCP:   map[string]ServerView{},
	}
}

func randomTabID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is nearly impossible, but panicking kills the
		// entire desktop process. Fall back to a time-based ID so the app
		// stays alive — the tab ID just needs to be unique per process.
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))[:16]
	}
	return hex.EncodeToString(b)
}

// TabMeta is a lightweight summary sent to the frontend for the tab bar.
type TabMeta struct {
	ID            string `json:"id"`
	Scope         string `json:"scope"`
	WorkspaceRoot string `json:"workspaceRoot"`
	WorkspaceName string `json:"workspaceName,omitempty"`
	Title         string `json:"title"`
	Ready         bool   `json:"ready"`
	Label         string `json:"label,omitempty"`
	ActivityStatus string `json:"activityStatus,omitempty"`
}

// tabMeta returns a lightweight summary for the frontend tab bar.
func (t *WorkspaceTab) tabMeta() TabMeta {
	if t == nil {
		return TabMeta{}
	}
	return TabMeta{
		ID:             t.ID,
		Scope:          t.Scope,
		WorkspaceRoot:  t.WorkspaceRoot,
		WorkspaceName:  filepath.Base(t.WorkspaceRoot),
		Title:          t.TopicTitle,
		Ready:          t.Ready,
		Label:          t.Label,
		ActivityStatus: t.ActivityStatus,
	}
}

// ActivityStatus tracks the in-flight turn status for the tab. Updated by the
// event sink; read by the frontend through MetaForTab.
type ActivityStatus = string

const (
	StatusThinking            ActivityStatus = "thinking"
	StatusStreaming           ActivityStatus = "streaming"
	StatusWaitingConfirmation                = "waiting_confirmation"
	StatusBackgroundJob                      = "background_job"
	StatusPaused                             = "paused"
	StatusError                              = "error"
)

// --- event routing ---

// wireEventTab is like wireEvent but includes the tabId so the frontend can
// route events to the correct per-tab reducer.
type wireEventTab struct {
	wireEvent
	TabID string `json:"tabId"`
}

// toWireTab converts an event.Event and injects the tab ID for routing.
func toWireTab(e event.Event, tabID string) wireEventTab {
	return wireEventTab{wireEvent: toWire(e), TabID: tabID}
}

// tabEventSink wraps the global eventSink and injects a tabId into every event
// so the frontend can route it to the correct tab's reducer. It implements
// event.Sink so it can be passed directly to boot.Build.
type tabEventSink struct {
	tabID string
	app   *App
	ctx   context.Context
	mu    sync.RWMutex
}

// Emit forwards the event to the webview with this tab's ID injected.
func (s *tabEventSink) Emit(e event.Event) {
	s.mu.RLock()
	ctx := s.ctx
	s.mu.RUnlock()
	if ctx == nil {
		return
	}
	runtime.EventsEmit(ctx, eventChannel, toWireTab(e, s.tabID))
}

// setContext stores the Wails context for this sink.
func (s *tabEventSink) setContext(ctx context.Context) {
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()
}

// clearContext clears the context so Emit becomes a no-op (used when closing a tab).
func (s *tabEventSink) clearContext() {
	s.mu.Lock()
	s.ctx = nil
	s.mu.Unlock()
}

// --- tab persistence ---

// tabsFilePath returns the path to desktop-tabs.json under the user config dir.
func tabsFilePath() string {
	dir := config.MemoryUserDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "desktop-tabs.json")
}

// tabPersistEntry is the serialisable form of a tab for desktop-tabs.json.
type tabPersistEntry struct {
	ID            string `json:"id"`
	Scope         string `json:"scope"`
	WorkspaceRoot string `json:"workspaceRoot"`
	TopicTitle    string `json:"topicTitle"`
	SessionPath   string `json:"sessionPath,omitempty"`
}

// saveTabs writes the current tab list to desktop-tabs.json so the next launch
// can restore open tabs.
func (a *App) saveTabs() {
	a.mu.RLock()
	entries := make([]tabPersistEntry, 0, len(a.tabOrder))
	for _, id := range a.tabOrder {
		tab := a.tabs[id]
		if tab == nil || tab.closing {
			continue
		}
		entries = append(entries, tabPersistEntry{
			ID:            tab.ID,
			Scope:         tab.Scope,
			WorkspaceRoot: tab.WorkspaceRoot,
			TopicTitle:    tab.TopicTitle,
			SessionPath:   tab.SessionPath,
		})
	}
	a.mu.RUnlock()

	path := tabsFilePath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("saveTabs: MkdirAll %s: %v", filepath.Dir(path), err)
		return
	}
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		log.Printf("saveTabs: MarshalIndent: %v", err)
		return
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Printf("saveTabs: WriteFile %s: %v", path, err)
	}
}

// loadTabs reads desktop-tabs.json and returns the persisted entries.
func loadTabs() []tabPersistEntry {
	path := tabsFilePath()
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		log.Printf("loadTabs: ReadFile %s: %v", path, err)
		return nil
	}
	var entries []tabPersistEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		log.Printf("loadTabs: Unmarshal %s: %v", path, err)
		return nil
	}
	return entries
}
