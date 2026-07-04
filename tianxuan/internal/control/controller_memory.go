package control

import (
	"fmt"
	"strings"

	"tianxuan/internal/memory"
	"tianxuan/internal/tool/builtin"
)

// --- memory ---
//
// c.mem is treated as an immutable snapshot guarded by c.mu: reads take the lock
// and return the pointer; writes mutate disk then swap in a freshly discovered
// snapshot. A turn-tail note is queued for each write so the change applies this
// session without disturbing the cache-stable system prefix (it folds into the
// prefix on the next session). All of these are no-ops returning "" when memory
// is disabled.

// QuickAdd appends a one-line note to the doc-memory file for scope (project
// TIANXUAN.md by default) — the write side of "#<note>". Returns the file written.
func (c *Controller) QuickAdd(scope memory.Scope, note string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mem == nil {
		return "", nil
	}
	path := c.mem.DocPath(scope)
	if path == "" {
		return "", fmt.Errorf("no target file for memory scope %q", scope)
	}
	if err := memory.AppendDoc(path, note); err != nil {
		return "", err
	}
	c.pendingMemory = append(c.pendingMemory, note)
	c.refreshMemoryLocked()
	return path, nil
}

// SaveDoc overwrites a recognized memory doc with body — the save side of the
// desktop panel's in-place editor. Returns the file written.
func (c *Controller) SaveDoc(path, body string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mem == nil {
		return "", nil
	}
	written, err := c.mem.WriteDoc(path, body)
	if err != nil {
		return "", err
	}
	// Inject the new content once on the next turn: the cached prefix still holds
	// the pre-edit version this session, so handing the model the current text
	// avoids a stale-guidance gap until the next session re-folds it into the
	// prefix. Trimmed to a single tail note (drained by Compose), not per-turn.
	c.pendingMemory = append(c.pendingMemory,
		"Memory file "+written+" was just edited. Its current contents:\n"+strings.TrimSpace(body))
	c.refreshMemoryLocked()
	return written, nil
}

// UpdateFact overwrites a saved fact's body by name, preserving all other
// fields. It re-uses Store.Save (which overwrites by stem), so the edit is
// atomic on disk and the index stays consistent. Returns the file written.
func (c *Controller) UpdateFact(name, body string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mem == nil {
		return "", nil
	}
	var target *memory.Memory
	for _, f := range c.mem.Store.List() {
		if f.Name == name {
			m := f // copy
			target = &m
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("fact %q not found", name)
	}
	target.Body = body
	path, err := c.mem.Store.Save(*target)
	if err != nil {
		return "", err
	}
	c.pendingMemory = append(c.pendingMemory,
		"Memory fact \""+name+"\" was edited. Its current body:\n"+strings.TrimSpace(body))
	c.refreshMemoryLocked()
	return path, nil
}

// ChangeFactType changes the Type of a saved fact by name (e.g. promote to
// "user" level or demote to "project"/"feedback"). All other fields are
// preserved. Refreshes the memory snapshot and queues a turn-tail note.
func (c *Controller) ChangeFactType(name, newType string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mem == nil {
		return nil
	}
	t := memory.NormalizeType(newType)
	if err := c.mem.Store.ChangeType(name, t); err != nil {
		return err
	}
	c.pendingMemory = append(c.pendingMemory,
		"Memory fact \""+name+"\" type changed to "+string(t)+".")
	c.refreshMemoryLocked()
	return nil
}

// ForgetMemory deletes a saved auto-memory by name — the panel/TUI delete action,

// ForgetMemory deletes a saved auto-memory by name — the panel/TUI delete action,
// the manual counterpart to the model's `forget` tool. It queues a turn-tail note
// so the deletion applies this session (the cached prefix still lists the fact
// until the next session re-folds the index).
func (c *Controller) ForgetMemory(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mem == nil {
		return nil
	}
	if err := c.mem.Store.Delete(name); err != nil {
		return err
	}
	c.pendingMemory = append(c.pendingMemory,
		"Deleted memory \""+name+"\" — disregard its line still shown in the saved-memories index until next session.")
	c.refreshMemoryLocked()
	return nil
}

// QueueMemory implements memory.Queue: when the model runs the remember/forget
// tool, the tool calls this with a note that rides the next turn so the change
// applies this session without touching the cache-stable prefix. It also
// refreshes the snapshot a memory panel reads.
func (c *Controller) QueueMemory(note string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pendingMemory = append(c.pendingMemory, note)
	c.refreshMemoryLocked()
}

// Memory returns the loaded memory snapshot (nil when memory is disabled), for
// frontends that surface a memory panel or the /memory command. The returned
// *Set is immutable — mutations go through QuickAdd / SaveDoc.
func (c *Controller) Memory() *memory.Set {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mem
}

// refreshMemoryLocked re-discovers memory from disk so a later Memory() reflects
// a just-applied write, and updates the search index so memory_search finds the
// change immediately. Caller holds c.mu.
func (c *Controller) refreshMemoryLocked() {
	if c.mem == nil {
		return
	}
	c.mem = memory.Load(memory.Options{CWD: c.mem.CWD, UserDir: c.mem.UserDir})
	builtin.SetMemorySearchIndex(c.mem.Search)
}

// SessionRemember saves a fact to session-only memory (not written to disk).
// Session facts persist across turns within the session, are injected into
// every turn's context, and can be promoted to permanent storage later.
func (c *Controller) SessionRemember(m memory.Memory) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionFacts = append(c.sessionFacts, m)
}

// PromoteSessionFacts saves all current session facts to permanent storage.
// Each fact is deduplicated against existing permanent memories by name:
// if a permanent memory with the same name exists, it is updated; otherwise
// a new permanent memory is created. After promotion, session facts are cleared.
// Returns the number of facts promoted.
func (c *Controller) PromoteSessionFacts() (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mem == nil || len(c.sessionFacts) == 0 {
		return 0, nil
	}
	n := 0
	for _, m := range c.sessionFacts {
		if _, err := c.mem.Store.Save(m); err != nil {
			return n, fmt.Errorf("promoting %q: %w", m.Name, err)
		}
		n++
	}
	c.sessionFacts = nil
	c.refreshMemoryLocked()
	return n, nil
}

// SessionFacts returns the current session-only facts (for the memory panel).
func (c *Controller) SessionFacts() []memory.Memory {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]memory.Memory, len(c.sessionFacts))
	copy(out, c.sessionFacts)
	return out
}

// SaveSession implements memory.SessionSaver. The remember tool calls this
// when session=true to save a fact to session-only memory (not on disk).
func (c *Controller) SaveSession(m memory.Memory) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionFacts = append(c.sessionFacts, m)
	return "Saved to session memory (\"" + slugifyName(m.Name) + "\"): " + m.Description
}

func slugifyName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.NewReplacer(" ", "-", "_", "-", ".", "-").Replace(s)
	return s
}
