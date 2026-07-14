package control

import (
	"context"
	"fmt"
	"strings"

	"tianxuan/internal/memory"
	"tianxuan/internal/skill"
)

// Compose appends memory updates, session facts, background job notes, and memory
// block injections to a turn's text. The frontend keeps showing the raw text as
// the user bubble.
func (c *Controller) Compose(text string) string {
	c.mu.Lock()
	notes := c.pendingMemory
	c.pendingMemory = nil
	sessionFacts := c.sessionFacts
	c.mu.Unlock()

	// Memory added mid-session rides the turn (never the cached system prefix),
	// so it takes effect now without invalidating the prompt cache. It folds into
	// the system prefix on the next session, where it costs nothing per turn.
	if len(notes) > 0 {
		var b strings.Builder
		b.WriteString("<memory-update>\n")
		b.WriteString("The following project-memory changes were just made and apply from now on:\n")
		for _, n := range notes {
			b.WriteString("- " + n + "\n")
		}
		b.WriteString("</memory-update>")
		text = text + "\n\n" + b.String()
	}

	// Session facts — temporary memories saved with session=true. These persist
	// across turns and are re-injected each turn. The model can also call
	// memory_search to find them (they're in the session, not on disk).
	if len(sessionFacts) > 0 {
		var b strings.Builder
		b.WriteString("<session-facts>\n")
		b.WriteString("You saved these temporary facts this session (call promote_session_facts to make them permanent):\n")
		for _, f := range sessionFacts {
			fmt.Fprintf(&b, "- [%s] %s: %s\n", f.Name, f.Title, f.Description)
		}
		b.WriteString("</session-facts>")
		text = text + "\n\n" + b.String()
	}

	// Background jobs that finished since the last turn ride the turn too, so the
	// model learns of completions even though the user-facing notices don't reach
	// its context. Like memory, this never touches the cache-stable prefix.
	if c.jobs != nil {
		if note := c.jobs.DrainCompletedNote(); note != "" {
			text = text + "\n\n" + "<background-jobs>\n" + note + "\n</background-jobs>"
		}
	}
	// V10.18+: LangMem-inspired kind-aware memory injection.
	// Procedural rules → always injected (every turn).
	// Episodic memories → injected when user input matches trigger tags.
	if c.mem != nil {
		if rules := c.mem.ProceduralBlock(); rules != "" {
			text = text + "\n\n" + rules
		}
		if episodic := c.mem.EpisodicMatches(text); len(episodic) > 0 {
			if block := memory.EpisodicBlock(episodic); block != "" {
				text = text + "\n\n" + block
			}
		}
	}

	return text
}

// CustomCommand resolves a slash command line against the loaded custom slash
// commands, returning the rendered prompt to send (found=false when no command
// matches).
func (c *Controller) CustomCommand(input string) (sent string, found bool) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "", false
	}
	name := strings.TrimPrefix(fields[0], "/")
	for _, cmd := range c.commands {
		if cmd.Name == name {
			return cmd.Render(fields[1:]), true
		}
	}
	return "", false
}

// RunSkill resolves a "/<name> args…" line against the loaded skills, returning
// the skill's rendered body to send as a turn (found=false when no skill
// matches). Invoking a skill by slash always inlines its body — the model reads
// and follows the playbook in the main loop; a subagent skill's isolation is
// only engaged when the model calls it via run_skill / the dedicated tool. The
// caller applies Compose for memory framing.
func (c *Controller) RunSkill(input string) (sent string, found bool) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "", false
	}
	name := strings.TrimPrefix(fields[0], "/")
	for _, sk := range c.skills {
		if sk.Name == name {
			return skill.Render(sk, strings.Join(fields[1:], " ")), true
		}
	}
	return "", false
}

// MCPPrompt resolves a "/mcp__server__prompt args…" line: it maps the positional
// args onto the prompt's declared arguments and fetches the rendered prompt from
// the MCP server (an async prompts/get). found is false when no such prompt
// exists; err carries a fetch failure. Honours ctx.
func (c *Controller) MCPPrompt(ctx context.Context, input string) (sent string, found bool, err error) {
	if c.host == nil {
		return "", false, nil
	}
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "", false, nil
	}
	name := strings.TrimPrefix(fields[0], "/")

	prompts := c.host.Prompts()
	idx := -1
	for i := range prompts {
		if prompts[i].Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "", false, nil
	}

	args := map[string]string{}
	for i, a := range prompts[idx].Args {
		if i+1 < len(fields) {
			args[a.Name] = fields[i+1]
		}
	}
	text, err := prompts[idx].Get(ctx, args)
	if err != nil {
		return "", true, err
	}
	return text, true, nil
}
