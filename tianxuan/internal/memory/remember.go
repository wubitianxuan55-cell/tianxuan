package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"tianxuan/internal/tool"
)

// rememberTool lets the model persist a durable fact to the auto-memory store.
// It is stateful (bound to one project's Store), so boot constructs it and adds
// it to the registry — the same pattern as the task tool — rather than
// self-registering as a stateless built-in.
type rememberTool struct{ store Store }

// NewRememberTool returns the `remember` tool bound to store. A zero/disabled
// store yields a tool that reports the store is unavailable rather than silently
// dropping saves.
func NewRememberTool(store Store) tool.Tool { return rememberTool{store: store} }

func (rememberTool) Name() string { return "remember" }

func (rememberTool) Description() string {
	return "Save a durable fact to project memory so it survives across sessions. " +
		"Use for things worth remembering long-term: who the user is and their preferences (type \"user\"); " +
		"guidance on how to work, including the why (type \"feedback\"); ongoing goals or constraints not " +
		"derivable from the code (type \"project\"); or pointers to external resources (type \"reference\"). " +
		"For feedback/project, structure the body with a \"**Why:**\" line and a \"**How to apply:**\" line so the fact is actionable later; " +
		"link related memories inline with [[their-name]]. " +
		"Do NOT save what the repo already records (code structure, git history) or facts that only matter to the current conversation; " +
		"if asked to remember one of those, save instead the non-obvious point behind it. " +
		"Before saving, check the loaded memory index for an entry that already covers this — reuse that name to update it rather than create a near-duplicate, and use `forget` to drop one that is now wrong. " +
		"The saved index loads into context at the start of each session. " +
		"Set session=true to save tentatively — the fact is visible this session but not persisted to disk until you call promote_session_facts."
}

func (rememberTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type": "object",
"properties": {
  "name": {"type": "string", "description": "Short kebab-case slug identifying the fact, e.g. \"prefers-tabs\". Reusing a name overwrites that memory — do that to update an existing fact. Omit to derive one from the description."},
  "title": {"type": "string", "description": "Short human-readable label shown in the memory index, e.g. \"Prefers tabs\". Omit to derive one from the name."},
  "description": {"type": "string", "description": "One-line hook shown in the index — the phrase a future session reads to decide whether to open this memory. Make it specific."},
  "type": {"type": "string", "enum": ["user", "feedback", "project", "reference"], "description": "Category of the fact."},
  "body": {"type": "string", "description": "The fact itself (Markdown). For feedback/project, include a \"**Why:**\" line and a \"**How to apply:**\" line; link related memories with [[their-name]]."},
  "session": {"type": "boolean", "description": "If true, save to session-only memory (not permanent). Session facts persist across turns and are re-injected each turn. Use for tentative/temporary facts. Call promote_session_facts to finalize them."}
},
"required": ["description", "body"]
}`)
}

func (t rememberTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Body        string `json:"body"`
		Session     bool   `json:"session"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if in.Description == "" || in.Body == "" {
		return "", fmt.Errorf("description and body are required")
	}
	name := in.Name
	if name == "" {
		name = in.Title // Save slugifies; the title (or, below, the description) makes a serviceable slug
	}
	if name == "" {
		name = in.Description
	}
	m := Memory{
		Name:        name,
		Title:       in.Title,
		Description: in.Description,
		Type:        NormalizeType(in.Type),
		Body:        in.Body,
	}

	// Session-only save: store in-memory, not to disk.
	if in.Session {
		ss, ok := SessionSaverFromContext(ctx)
		if !ok {
			return "", fmt.Errorf("session memory unavailable")
		}
		note := ss.SaveSession(m)
		if q, ok := QueueFromContext(ctx); ok {
			q.QueueMemory(note)
		}
		return fmt.Sprintf("Saved to session memory (\"%s\"). It applies this session. Call promote_session_facts to make it permanent.", slug(name)), nil
	}

	// Permanent save: write to disk.
	path, err := t.store.Save(m)
	if err != nil {
		return "", err
	}
	if q, ok := QueueFromContext(ctx); ok {
		q.QueueMemory("Saved memory \"" + slug(name) + "\": " + oneLine(in.Description))
	}
	return fmt.Sprintf("Saved memory to %s (it applies now and loads automatically in future sessions).", path), nil
}

func (rememberTool) ReadOnly() bool { return false }
