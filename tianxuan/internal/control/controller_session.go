package control

import (
	"context"

	"tianxuan/internal/agent"
	"tianxuan/internal/diff"
	"tianxuan/internal/provider"
)

// NewSession snapshots the current conversation, rotates to a fresh file, and
// resets the executor to a clean session carrying the same system prompt. It
// ends the old session and starts the new one for lifecycle hooks.
func (c *Controller) NewSession() error {
	if c.executor == nil {
		return nil
	}
	if err := c.Snapshot(); err != nil {
		return err
	}
	c.hooks.SessionEnd(context.Background())
	if c.sessionDir != "" {
		c.mu.Lock()
		c.sessionPath = agent.NewSessionPath(c.sessionDir, c.label)
		c.mu.Unlock()
	}
	c.executor.SetSession(agent.NewSession(c.systemPrompt))
	c.rebindCheckpoints(c.SessionPath())
	// Reset V3.0 TCCA state so the new session starts clean.
	if c.ctxMgr != nil {
		c.ctxMgr.Flow().ReplaceMessages(nil)
	}
	c.mu.Lock()
	c.startedOnce = true // NewSession fires SessionStart itself; don't re-fire on the next turn
	c.mu.Unlock()
	c.hooks.SessionStart(context.Background())
	return nil
}

// RewindScope selects what a Rewind restores.
// Resume seeds the session from a loaded transcript and pins the active file to
// its path so auto-save keeps appending there. It also preserves the current
// executor system prompt (L1 + Instructions) so the resumed session uses the
// same behaviour rules as a fresh session — the loaded file's system prompt may
// be from an older version.
//
// V10.108: Resume now handles system prompt preservation internally instead of
// requiring every caller to do it (and sometimes getting it wrong, e.g. replacing
// the full L1+Instructions prompt with just L1 Identity, which strips behavioural
// rules from the executor on restore).
func (c *Controller) Resume(s *agent.Session, path string) {
	// Preserve the current executor system prompt (L1 + Instructions) so the
	// resumed session uses the same behaviour rules. The loaded JSONL may carry
	// an older-version system prompt, or a partial one (e.g. just L1 Identity).
	if c.executor != nil {
		if cur := c.executor.Session(); cur != nil {
			if msgs := cur.Snapshot(); len(msgs) > 0 && msgs[0].Role == provider.RoleSystem {
				preserved := msgs[0].Content
				if loadedMsgs := s.Messages; len(loadedMsgs) > 0 && loadedMsgs[0].Role == provider.RoleSystem {
					loadedMsgs[0] = provider.Message{Role: provider.RoleSystem, Content: preserved}
				}
			}
		}
	}

	if c.executor != nil {
		c.executor.SetSession(s)
	}

	// V10.108: sync FlowLayer with the loaded session so TCCA metrics and
	// compaction state reflect the restored history rather than staying empty.
	if c.ctxMgr != nil {
		c.ctxMgr.Flow().ReplaceMessages(s.Snapshot())
	}

	c.mu.Lock()
	c.sessionPath = path
	c.mu.Unlock()
	c.rebindCheckpoints(path)
}

// Snapshot writes the executor's conversation to the active session file. No-op
// when persistence is unavailable or the session has never been used (no user
// interaction). Called after every turn so a crash loses at most one in-flight
// prompt.
func (c *Controller) Snapshot() error {
	c.mu.Lock()
	path := c.sessionPath
	c.mu.Unlock()
	if c.executor == nil || path == "" {
		return nil
	}
	s := c.executor.Session()
	if !s.HasContent() {
		return nil
	}
	if err := s.Save(path); err != nil {
		return err
	}
	return agent.TouchBranchMeta(path)
}

// SetSessionPath pins where auto-save lands (a fresh session file minted by the
// caller when no resume path applies).
func (c *Controller) SetSessionPath(p string) {
	c.mu.Lock()
	c.sessionPath = p
	c.mu.Unlock()
	c.rebindCheckpoints(p)
}

// SessionDir reports the directory new session files land in ("" disables
// persistence), so the caller can decide whether to mint a path.
func (c *Controller) SessionDir() string { return c.sessionDir }

// SessionPath reports the file the current conversation auto-saves to ("" when
// persistence is disabled), so a history view can mark the active session.
func (c *Controller) SessionPath() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionPath
}

// History returns the executor's current message log (for repopulating a
// resumed frontend's view).
func (c *Controller) History() []provider.Message {
	if c.executor == nil {
		return nil
	}
	return c.executor.Session().Snapshot() // copy — a turn may be appending concurrently
}

// ContextSnapshot returns (promptTokens, contextWindow) from the most recent
// turn. Both zero means no data yet — a gauge hides itself.
func (c *Controller) ContextSnapshot() (int, int) {
	if c.executor == nil {
		return 0, 0
	}
	u := c.executor.LastUsage()
	if u == nil {
		return 0, c.executor.ContextWindow()
	}
	return u.PromptTokens, c.executor.ContextWindow()
}

// PlannerContextSnapshot returns the planner's last usage and window, or zeros
// when no planner is active (single-model mode).
func (c *Controller) PlannerContextSnapshot() (int, int) {
	if h, ok := c.runner.(interface{ PlannerContext() (int, int) }); ok {
		return h.PlannerContext()
	}
	return 0, 0
}

// CompactRatio returns the auto-compaction threshold as a fraction of the window
// (0 when the executor is unset). The status line shows headroom against it.
func (c *Controller) CompactRatio() float64 {
	if c.executor == nil {
		return 0
	}
	return c.executor.CompactRatio()
}

// LastUsage returns the most recent turn's token telemetry (nil before the first
// turn), so frontends can derive the prompt cache-hit rate for the status line.
func (c *Controller) LastUsage() *provider.Usage {
	if c.executor == nil {
		return nil
	}
	return c.executor.LastUsage()
}

// SessionCache returns cumulative cache hit/miss prompt tokens for the session,
// so a frontend can render the aggregate (session-wide) cache-hit rate — steadier
// than the single-turn rate and unaffected by compaction.
func (c *Controller) SessionCache() (hit, miss int) {
	if c.executor == nil {
		return 0, 0
	}
	return c.executor.SessionCache()
}

// WorkspaceChanges returns the files modified during the current session.
func (c *Controller) WorkspaceChanges() []diff.Change {
	if c.executor == nil {
		return nil
	}
	return c.executor.PendingDiffs()
}
