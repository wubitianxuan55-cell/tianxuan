package main

import (
	"path/filepath"
	"strings"
	"time"

	"tianxuan/internal/agent"
	"tianxuan/internal/config"
	"tianxuan/internal/control"
	"tianxuan/internal/provider"
)

// CheckpointMeta summarises one rewind point (a user turn) for the desktop.
type CheckpointMeta struct {
	Turn   int      `json:"turn"`
	Prompt string   `json:"prompt"`
	Files  []string `json:"files"` // paths changed during the turn
	Time   int64    `json:"time"`  // unix milliseconds
}

// SessionMeta summarises one saved session for the history panel.
type SessionMeta struct {
	Path    string `json:"path"`
	Preview string `json:"preview"`         // first user message
	Title   string `json:"title,omitempty"` // user-chosen name, when set (overrides preview)
	Turns   int    `json:"turns"`
	ModTime int64  `json:"modTime"` // unix milliseconds, for the frontend to group/format
	Current bool   `json:"current"`
}

// HistoryMessage is one prior turn, for the frontend to repopulate its transcript
// after a reload.
type HistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// NewSession snapshots the current conversation and rotates to a fresh one.
func (a *App) NewSession() error {
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return nil
	}
	return ctrl.NewSession()
}

// Checkpoints lists the session's rewind points, oldest first, for the rewind UI.
func (a *App) Checkpoints() []CheckpointMeta {
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return []CheckpointMeta{}
	}
	metas := ctrl.Checkpoints()
	out := make([]CheckpointMeta, 0, len(metas))
	for _, m := range metas {
		out = append(out, CheckpointMeta{Turn: m.Turn, Prompt: m.Prompt, Files: m.Paths, Time: m.Time.UnixMilli()})
	}
	return out
}

// Rewind restores the session to the start of turn. scope is "code",
// "conversation", or "both" (anything else is treated as "both"). The frontend
// re-reads History after this resolves.
func (a *App) Rewind(turn int, scope string) error {
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return nil
	}
	s := control.RewindBoth
	switch scope {
	case "code":
		s = control.RewindCode
	case "conversation":
		s = control.RewindConversation
	}
	return ctrl.Rewind(turn, s)
}

// Fork branches the conversation at the start of turn into a new session
// (preserving the current one), keeping code intact, and switches to the branch.
// The frontend re-reads History after this resolves.
func (a *App) Fork(turn int) error {
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return nil
	}
	_, err := ctrl.Fork(turn)
	return err
}

// SummarizeFrom / SummarizeUpTo compress the conversation from / up to the start
// of turn into one summary (Claude Code's "summarize from/up to here"), keeping
// code intact. The frontend re-reads History after this resolves.
func (a *App) SummarizeFrom(turn int) error {
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return nil
	}
	return ctrl.SummarizeFrom(a.ctx, turn)
}

func (a *App) SummarizeUpTo(turn int) error {
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return nil
	}
	return ctrl.SummarizeUpTo(a.ctx, turn)
}

// ListSessions returns the saved sessions newest-first for the history panel,
// marking the one the current conversation is writing to and attaching any
// user-chosen titles.
func (a *App) ListSessions() []SessionMeta {
	dir := config.WorkspaceSessionDir("")
	infos, err := agent.ListSessions(dir)
	if err != nil {
		return []SessionMeta{}
	}
	titles := loadSessionTitles(dir)
	ctrl := a.ctrlByTabID("")
	cur := ""
	if ctrl != nil {
		cur = ctrl.SessionPath()
	}
	out := make([]SessionMeta, 0, len(infos)+1)
	curFound := false
	for _, s := range infos {
		if s.Path == cur {
			curFound = true
		}
		out = append(out, SessionMeta{
			Path:    s.Path,
			Preview: s.Preview,
			Title:   titles[filepath.Base(s.Path)],
			Turns:   s.Turns,
			ModTime: s.ModTime.UnixMilli(),
			Current: s.Path == cur,
		})
	}
	// V5.26: 当前会话尚未持久化时（NewSession 后、首次对话保存前），
	// 磁盘文件列表中不存在该路径。补充合成条目，确保前端 currentSessionKey
	// 始终使用真实路径，避免未保存→已保存时键漂移导致统计面板数据清零。
	if cur != "" && !curFound {
		out = append(out, SessionMeta{
			Path:    cur,
			Preview: "(新会话)",
			Title:   "",
			Turns:   0,
			ModTime: time.Now().UnixMilli(),
			Current: true,
		})
	}
	return out
}

// DeleteSession removes a saved session (and its title). It refuses the active
// session — that's the conversation on screen, and auto-save would recreate the
// file on the next turn; start a new session first to retire it.
func (a *App) DeleteSession(path string) error {
	ctrl := a.ctrlByTabID("")
	if ctrl != nil && ctrl.SessionPath() == path {
		return errActiveSession
	}
	return deleteSessionFile(config.WorkspaceSessionDir(""), path)
}

// RenameSession sets a custom display name for a session (empty clears it back to
// the preview). It only affects the history panel; the file on disk is unchanged.
func (a *App) RenameSession(path, title string) error {
	return setSessionTitle(config.WorkspaceSessionDir(""), path, title)
}

// ResumeSession snapshots the current conversation, then loads the session at
// path and continues it — auto-save keeps appending to that file. The model and
// working folder are unchanged (same controller); only the transcript is swapped.
// Returns the resumed messages for the frontend to render.
func (a *App) ResumeSession(path string) ([]HistoryMessage, error) {
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return []HistoryMessage{}, nil
	}
	loaded, err := agent.LoadSession(path)
	if err != nil {
		return nil, err
	}
	_ = ctrl.Snapshot() // persist the current session before switching away
	ctrl.Resume(loaded, path)
	return a.History(), nil
}

// History returns the session's message log.
func (a *App) History() []HistoryMessage {
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return nil
	}
	msgs := ctrl.History()
	resolve := sessionDisplayResolver(config.WorkspaceSessionDir(""), ctrl.SessionPath())
	out := make([]HistoryMessage, 0, len(msgs))
	for _, m := range msgs {
		content := m.Content
		if m.Role == provider.RoleUser {
			// Strip transient blocks (reasoning-language, response-language,
			// etc.) injected by withTurnPreferences, then resolve any
			// display override (e.g. paste-expansion labels) keyed by the
			// raw user input.
			raw := agent.StripTransientBlocks(m.Content)

			// Handoff messages (Hermes→Hephaestus instructions) embed the
			// original user task. Extract and display it instead of the
			// full handoff prompt.
			if strings.HasPrefix(strings.TrimSpace(raw), "# tianxuan hephaestus handoff") {
				if task := extractOriginalTask(raw); task != "" {
					raw = agent.StripTransientBlocks(task)
				} else {
					continue // defensive: malformed handoff, skip
				}
			}

			// Compaction summaries are LLM-generated digests injected as
			// user messages for cache-prefix stability. They are NOT user
			// content — show a brief label instead of the English summary.
			if trimmed := strings.TrimSpace(raw); strings.HasPrefix(trimmed, "<compaction-summary>") {
				content = "〈会话摘要〉"
			} else {
				display := resolve(raw)
				if display != raw {
					content = display
				} else {
					content = raw
				}
			}
		}
		out = append(out, HistoryMessage{Role: string(m.Role), Content: content})
	}
	return out
}

// extractOriginalTask extracts the original user task from a Hermes→Hephaestus
// handoff message. Returns empty string if the content is not a valid handoff.
func extractOriginalTask(content string) string {
	const header = "Original task:\n"
	i := strings.Index(content, header)
	if i < 0 {
		return ""
	}
	rest := content[i+len(header):]
	if j := strings.Index(rest, "\n\nHermes output:"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}
