package control

import (
	"context"
	"strings"

	"tianxuan/internal/skill"
)

// PlanModeMarker is prepended to every user turn while plan mode is on. It rides
// in the user message (not the system prompt or tools), so the cache-stable
// prompt prefix is left untouched and the toggle costs nothing in cache hits.
const PlanModeMarker = "[Plan mode — read-only. Explore the codebase first (read_file, ls, grep, glob, web_fetch, task are available; writers are refused by the harness), then present a LAYERED plan as your reply and stop — do not write files, edit, or run side-effecting bash. Structure the plan as a two-level markdown list: each PHASE is a top-level numbered list item (a coherent milestone, e.g. \"1. Add the config loader\"), and each phase's concrete, verifiable sub-steps are bullets indented beneath it (e.g. \"   - parse the TOML into Config\"). Use plain numbered list items for phases — do NOT write phases as markdown headings (##, ###) — so both levels parse. Keep phases few (about 2-6). Wrap the entire plan in <plan>...</plan> tags so the frontend can extract and display it in the plan panel. The user will be asked to approve before any changes are made.]"

// ExploreModeMarker is the turn prefix for explore mode (V6.0 P3).
// Stronger than PlanModeMarker: explicitly states this is a read-only exploration
// session and the model should research, not plan for changes.
const ExploreModeMarker = "[Explore mode — read-only research. You are in exploration mode: investigate the codebase, understand architecture, trace call chains, and answer questions. Use read_file, ls, grep, glob, codegraph tools, and read-only task sub-agents liberally. Do NOT propose or write any code changes — this is pure research. Present your findings clearly with file:line citations.]"

// DevelopModeMarker is the turn prefix for develop mode (V6.0 P3).
// This is the default: full tools, no restrictions beyond normal gates.
const DevelopModeMarker = ""

// OrchestrateModeMarker is the turn prefix for orchestrate mode (V6.0 P3).
// Mimics MiMo-Code's compose mode: built-in planning→execution→review→TDD flow.
// Plan mode is on initially; after plan approval, execution proceeds with
// structured checkpoints.
const OrchestrateModeMarker = "[Orchestrate mode — structured delivery. You are in orchestration mode with a built-in plan→execute→verify workflow. First, explore and present a layered plan (use <plan>...</plan> tags). After the plan is approved, implement each phase systematically: mark the sub-step in_progress with todo_write, execute, sign off with complete_step attaching evidence, and move to the next. Run tests after each phase. Only stop when every phase is complete and verified.]"

// Compose applies the mode-appropriate marker to a turn's text, returning
// the message to actually send to the model. The frontend keeps showing the
// raw text as the user bubble.
// V6.0 P3: uses agentMode to select the right marker (explore/develop/orchestrate).
func (c *Controller) Compose(text string) string {
	c.mu.Lock()
	plan := c.planMode
	mode := c.agentMode
	notes := c.pendingMemory
	c.pendingMemory = nil
	// V3.0: profile is now in L2 RuntimeLayer via SetProject, no longer
	// injected as turn-tail prefix. Keep this block for backward compat
	// when ctxMgr is not wired.
	profile := ""
	if c.ctxMgr == nil {
		// Legacy: inject profile in first user message.
		// (This path is never reached with ctxMgr wired.)
	}
	c.mu.Unlock()

	if profile != "" {
		text = "<project-profile>\n" + profile + "\n</project-profile>\n\n" + text
	}

	// V6.0 P3: mode-specific marker (replaces legacy plan-mode marker)
	if mode == "explore" {
		text = ExploreModeMarker + "\n\n" + text
	} else if mode == "orchestrate" && plan {
		text = OrchestrateModeMarker + "\n\n" + text
	} else if plan {
		// Legacy plan mode (backward compat when agentMode is empty)
		text = PlanModeMarker + "\n\n" + text
	}

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
		b.WriteString("</memory-update>\n\n")
		text = b.String() + text
	}

	// Background jobs that finished since the last turn ride the turn too, so the
	// model learns of completions even though the user-facing notices don't reach
	// its context. Like memory, this never touches the cache-stable prefix.
	if c.jobs != nil {
		if note := c.jobs.DrainCompletedNote(); note != "" {
			text = "<background-jobs>\n" + note + "\n</background-jobs>\n\n" + text
		}
	}
	return text
}

// CustomCommand resolves a "/name args…" line against the loaded custom slash
// commands, returning the rendered prompt to send (found=false when no command
// matches). It does not apply the plan-mode marker — call Compose for that.
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
// caller applies Compose for plan-mode/memory framing.
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
