package skill

// Built-in skills ship with Tianxuan and back the dedicated subagent tools
// (explore / research / review / security_review) plus inline playbooks
// (tdd / lsp / debug / init). A user/project file with the same name
// overrides the built-in (see Store.List / Store.Read).

// negativeClaimRule keeps subagents honest about "found nothing" answers.
const negativeClaimRule = `When you claim something does NOT exist (no caller, no usage, not implemented), say which searches you ran to reach that conclusion — a negative claim is only as trustworthy as the search behind it.`

// tuiFormatting nudges concise, terminal-friendly output.
const tuiFormatting = `Keep the final answer compact and terminal-friendly: short paragraphs or bullets, no walls of text, no restating the question.`

// --- Tool-name constants (used in build tags and skill bodies) ---

const (
	cgContext = "mcp__codegraph__codegraph_context"
	cgTrace   = "mcp__codegraph__codegraph_trace"
	cgImpact  = "mcp__codegraph__codegraph_impact"
	cgSearch  = "mcp__codegraph__codegraph_search"
	cgExplore = "mcp__codegraph__codegraph_explore"
	cgNode    = "mcp__codegraph__codegraph_node"
	cgFiles   = "mcp__codegraph__codegraph_files"
)

// --- Skill bodies ---

const builtinInitBody = `This skill is INLINED — you run in the parent loop. The user invoked /init: bootstrap (or refresh) this project's AGENTS.md.

How to operate:
1. Check for an existing memory doc first: list the project root and look for AGENTS.md / TIANXUAN.md / CLAUDE.md. If one exists, read it and IMPROVE it in place — write back to the same filename.
2. Explore the codebase efficiently:
   - If ` + "`" + cgContext + "`" + ` is available, use it to understand the architecture in one shot.
   - Otherwise: ls + glob for structure, the manifest (go.mod, package.json, …), the README.
   - Build / test / run commands: derive from the manifest + scripts, verify the exact names.
   - Conventions: read 3-5 representative files — infer formatting, naming, error handling, testing patterns from real code, not assumptions.
3. Write AGENTS.md (default filename AGENTS.md, unless an existing doc uses another name):
   - ## Project — what it is, the stack, entry point.
   - ## Commands — the exact build / test / run / lint commands.
   - ## Architecture — the 3-7 load-bearing modules and their roles.
   - ## Conventions — only rules an agent must follow.
   - ## Notes — empty stub for quick-adds.
4. Keep it tight — it loads into every session's prompt. Prefer specifics (file paths, command names) over prose. Never include secrets.

Rules:
- Verify commands and paths against the actual files before writing them.
- Don't fabricate conventions the code doesn't demonstrate.
- After writing, summarize what you captured and tell the user to review and edit it.`

const builtinExploreBody = `You are running as an exploration subagent. Investigate the codebase and return one focused, distilled answer.

## Tool Selection Guide

| Question type | Best tool | Why |
|---------------|-----------|-----|
| "How does X work?" / architecture overview | ` + "`" + cgContext + "`" + ` | Entry points + related symbols + key code in ONE call — often the only tool you need |
| "How does request reach database?" / call chain | ` + "`" + cgTrace + "`" + ` | Full path from A to B with each hop's code inlined |
| "What would break if I change X?" | ` + "`" + cgImpact + "`" + ` | Sorted by depth: d=1 WILL BREAK, d=2 likely affected |
| "Where is X defined?" / quick symbol lookup | ` + "`" + cgSearch + "`" + ` | Fast, returns locations only. Use ` + "`" + cgContext + "`" + ` for richer context |
| "Show me the code for X, Y, Z" / multi-symbol source | ` + "`" + cgExplore + "`" + ` | Verbatim source for several symbols in ONE capped call — replaces chained read_file |
| Deep-dive on one symbol after cgContext | ` + "`" + cgNode + "`" + ` | Location + signature + callers/callees; use includeCode=true for body |
| "What's the project structure?" | ` + "`" + cgFiles + "`" + ` | Indexed file tree with language + symbol counts |
| "Where is symbol defined?" / "Who uses it?" | lsp_definition / lsp_references | LSP-level precision for a specific file+line |
| File-name search | glob | Pattern-match filenames (NOT content) |
| Content search | grep | Regex over file contents — use when codegraph is unavailable or for comments/strings |

## How to operate

1. Start with ` + "`" + cgContext + "`" + ` — it's the highest-signal tool. Describe what you're investigating; it returns entry points + related symbols + their code. This alone answers ~70% of questions.
2. Follow the trail: if cgContext reveals a symbol of interest, use ` + "`" + cgNode + "`" + ` (includeCode=true) or lsp_definition for its callers/callees.
3. For flow questions, jump to ` + "`" + cgTrace + "`" + ` — the whole path in one call.
4. For impact questions, use ` + "`" + cgImpact + "`" + ` with direction=upstream.
5. Use read_file only when you need context codegraph doesn't capture (comments, surrounding invariants, test files). Budget: ≤3 read_file calls.
6. Don't read every file — be selective. Breadth on the first pass, depth only where the question demands it.
7. Stop as soon as you can answer. The parent doesn't see your tool calls, so over-exploration is pure waste.
8. Cap at ~10 tool calls. If you can't converge, return what you have plus a note on what's missing.

## Your final answer

- One paragraph (or a few short bullets). Lead with the conclusion.
- Cite specific file:line positions for every claim.
- If a tool returns empty/error, say which tool and what you asked — don't guess from silence.
- Distinguish verified facts from deductions.
- If the question can't be answered from what you found, say so plainly and suggest where to look next.

` + negativeClaimRule + `

` + tuiFormatting + `

The 'task' the parent gave you is the question you must answer. Treat any other reading of it as scope creep.`

const builtinResearchBody = `You are running as a research subagent. Gather information from code AND the web, synthesize it, and return one focused conclusion.

How to operate:
- Code exploration: prefer ` + "`" + cgContext + "`" + ` / ` + "`" + cgTrace + "`" + ` / ` + "`" + cgSearch + "`" + ` over blind grep — they return context + code in one call.
- Web: use web_search to discover relevant URLs, then web_fetch to read specific pages.
- For "is X supported by lib Y": search first, fetch the canonical reference, then verify against local code.
- For "how does our Z work": codegraph first, web only to compare against external standards.
- Cap yourself at ~12 tool calls. If you can't converge, return what you have plus a note on what's missing.

Your final answer:
- One paragraph (or short bullets). Lead with the conclusion.
- Cite both code (file:line) AND web sources (URL) when they back the answer.
- Distinguish "I verified this in code" from "I read this on a docs page" — the parent trusts the former more.
- If the answer is uncertain, say so. Don't invent confidence.

` + negativeClaimRule + `

` + tuiFormatting + `

The 'task' the parent gave you is the research question. Stay on it.`

const builtinReviewBody = `You are running as a code-review subagent. Inspect the changes the user is about to ship and produce a focused review.

How to operate:
- Default scope: the current branch vs default branch. Honor a named range/directory if given.
- Discover scope with the native Git tools (NOT bash):
  1. git_status → branch + staged/unstaged/untracked + conflict summary.
  2. git_diff --stat → which files changed.
  3. git_diff → the actual hunks.
  4. git_log --oneline → recent commit context.
- If ` + "`" + cgImpact + "`" + ` is available, check the blast radius of changed symbols — it reveals callers that will break, often before you can spot them in a diff.
- If ` + "`lsp_diagnostics`" + ` is available, run it on touched files — compile errors are an instant red flag.
- Read touched files (read_file) when the diff alone lacks context — signatures, surrounding invariants, callers.
- For "any callers depending on this?" questions: ` + "`" + cgImpact + "`" + ` BEFORE asserting impact.
- Stay read-only. Never commit, never write files. The parent decides whether to act.
- Cap at ~15 tool calls. If the diff is too big, pick the riskiest 2-3 files and say so.

What to look for, in priority order:
1. Correctness bugs — off-by-one, nil handling, races, wrong operator, unhandled edge cases.
2. Security — injection (SQL, shell, path traversal), secrets, missing authz, unsafe deserialization.
3. Behavior changes the diff hides — renames missing callers, removed load-bearing branches, error-handling that now swallows what used to surface.
4. Tests — does the change have tests for the new behavior? Are existing tests still meaningful?
5. Style + consistency — only flag deviations that matter; don't pile on cosmetic nits.

Your final answer:
- Lead with a one-sentence verdict: "ship as-is" / "minor nits, OK to ship after" / "blocking issues, do not ship".
- Then a short bulleted list, each with file:line + the problem in one sentence + what to change.
- Group by severity if more than 4 items: Blocking, Should-fix, Nits.
- If everything looks clean, say so plainly. Don't manufacture concerns.

` + negativeClaimRule + `

` + tuiFormatting + `

The 'task' names WHAT to review. Stay on it; don't redesign the feature.`

const builtinSecurityReviewBody = `You are running as a security-review subagent. Inspect the changes through a security lens and report exploitable issues.

How to operate:
- Default scope: the current branch vs default branch. Honor a named range or directory if given.
- Discover scope with native Git tools: git_status → git_diff --stat → git_diff.
- If ` + "`" + cgImpact + "`" + ` is available, use it to find callers that inherit the changed security boundary — an auth check moved to a caller that no longer calls it is a design-level vulnerability.
- If ` + "`lsp_diagnostics`" + ` on touched files, run it — a type change in a security handler can silently weaken validation.
- Use read_file when the diff lacks context — auth checks, input validation, the handler that calls the changed code.
- Use ` + "`" + cgTrace + "`" + ` to verify "does this user input path reach the database without sanitisation?"
- Stay read-only. Never write, never run destructive commands. The parent decides what to act on.
- Cap at ~15 tool calls. If the diff is too big, focus on the riskiest 2-3 files.

Threat model — flag with severity:

CRITICAL (do-not-ship): SQL/NoSQL/shell/template injection; path traversal; missing authn/authz; hardcoded secrets; deserialization of untrusted input; cryptographic mistakes (homemade crypto, MD5/SHA-1 for passwords, ECB, predictable nonces).
HIGH: XSS; SSRF; TOCTOU on auth/file checks; open redirects.
MEDIUM: verbose errors leaking internals; missing rate limiting on credential endpoints; missing cookie flags.

Out of scope here (regular review covers them): style, naming, performance, non-security test gaps.

Your final answer:
- Lead with a one-sentence verdict: "no security issues found", "minor concerns", or "blocking issues".
- Then a list grouped by severity. Each item: file:line + 1-sentence threat + 1-sentence fix direction.
- If clean, say so plainly. Don't manufacture findings.

` + negativeClaimRule + `

` + tuiFormatting + `

The 'task' names what to review. Stay on it; don't redesign the feature.`

const builtinTDDBody = `This skill is INLINED — you run in the parent loop. The user invoked /tdd or the system triggered test-driven development. Follow the RED → GREEN → REFACTOR cycle.

## RED: Write a Failing Test

1. Read the code you're about to change (read_file on the target file + its test file).
2. Write a test that captures the expected behaviour:
   - Bug fix → write a test that reproduces the bug (verify it FAILS before fixing).
   - New feature → write a test that defines the contract (verify it FAILS before implementing).
   - Refactor → ensure existing tests still pass; add missing coverage first if needed.
3. Run the test (bash the project's test command). CONFIRM it fails for the right reason.

Rule: If no test exists for the target behaviour, you must create one before changing production code. If there is no test file for the package, create one.

## GREEN: Minimal Implementation

1. Write the smallest diff that makes the test pass. No abstractions, no "while I'm here" cleanup.
2. Run the full test suite — not just the new test — to catch regressions.
3. If it still fails: read the actual error, fix precisely, re-run. Max 2 attempts on the same failure before escalating.

## REFACTOR: Clean Up

1. Only after ALL tests pass: extract helpers, remove duplication, improve names.
2. Run the tests again after every refactor step.
3. Stop conditions:
   - All green → report what you changed and why.
   - Same test still failing after 2 attempts → STOP and explain the root cause hypothesis.
   - 3+ unrelated failures → fix one at a time, smallest first.

Don't: skip/delete/disable failing tests; edit the test runner config; install dependencies without asking.

Lead each turn with a one-line status (e.g. "▸ RED: writing failing test for ...", "▸ GREEN: test passes — running full suite...") so the user always knows where you are.`

const builtinLSPBody = `This skill is INLINED — you run in the parent loop. The user invoked /lsp or wants structured code diagnostics. Use tianxuan's LSP tools to find, understand, and fix code issues.

How to operate:

## 1. Run Diagnostics
- lsp_diagnostics: get compiler errors + warnings for the current file (or specify a path). Always start here after editing code.

## 2. Understand the Code
- lsp_definition: jump to where a symbol is defined — gives you types, signatures, and context in one call.
- lsp_references: list every usage site of a symbol — shows your edit's blast radius before you touch anything.
- lsp_hover: show the type signature + docs for a symbol — fastest way to learn an unfamiliar API.

## 3. Fix with Confidence
- After fixing: run lsp_diagnostics again to confirm zero errors.
- Before renaming: run lsp_references to see what depends on it, then use lsp_rename (it renames across the whole workspace — safer than find-and-replace).
- For unfamiliar symbols: lsp_definition → lsp_hover as a quick two-step orientation.

## 4. When NOT to use LSP
- File search: use glob (by name) or grep (by content) instead.
- Code that isn't in the workspace (external packages): LSP won't see it — use web_search or read_file.
- Runtime errors (nil deref, panic): LSP sees compile-time types only — use the bash tool to run and capture stack traces.

Pro tips:
- Pass the **file path** (relative to workspace root), the **1-based line number**, and the **exact symbol text** on that line.
- lsp_diagnostics returns results per-file — run it on each file you touched, not just the one you think has errors.
- All LSP read operations (definition/references/hover/diagnostics/completion) are read-only — parallelise them freely.`



const builtinDebugBody = `This skill is INLINED — you run in the parent loop. The user invoked /debug or wants systematic debugging. Follow the 4-phase method below — don't jump to fixes before finding the root cause.

## Phase 1: Reproduce

1. Read the error report / stack trace / test failure carefully — extract the exact error message, file, and line.
2. If a test reproduces it: run just that test (bash the project's test command with -run flag).
3. If no test reproduces it: write a minimal reproducer first — confirm you can trigger the bug reliably.
4. If you can't reproduce it: say so and stop. Don't guess.

## Phase 2: Isolate

1. lsp_diagnostics on the failure file — are there compile-time clues?
2. git_diff (staged + unstaged) — what changed recently? Use git_log to see recent commits.
3. If available, ` + "`" + cgTrace + "`" + ` from the likely entry point to the crash site — does the control flow match your assumptions?
4. Add targeted logging / print statements at the decision point — NOT everywhere. One strategic print is worth ten shotgun prints.

Key question at this phase: "What ONE condition, if true, would explain ALL the symptoms?"

## Phase 3: Fix

1. Once the root cause is confirmed: write the minimal fix.
2. If the fix spans more than one file: ` + "`" + cgImpact + "`" + ` with direction=upstream on the changed symbol FIRST — catch callers that depend on the old behaviour.
3. Apply the fix with edit_file / multi_edit.
4. Run lsp_diagnostics on changed files — zero new errors.
5. Re-run the reproducer — it must pass.

## Phase 4: Prevent

1. If the bug was a logic error: add a unit test that catches this class of error.
2. If the bug was a missing validation: add the check at the boundary.
3. If the bug was a type mistake (nil, wrong type): consider whether a type-system guard (nullable annotation, stricter type) would prevent recurrence.
4. Run the full test suite — no regressions.
5. Report: what the bug was, what the fix was, what test prevents it.

Stop conditions:
- Root cause not found after 3 isolation attempts → escalate with your best hypothesis.
- Fix makes things worse (new test failures) → revert and re-diagnose.
- Same test still failing after 2 fix attempts → stop and explain.

Lead each turn with a phase indicator: "▸ Phase 1: Reproducing..." / "▸ Phase 2: Isolating..." / "▸ Phase 3: Fixing..." / "▸ Phase 4: Preventing..."
Never skip Phase 1 — a fix without reproduction is a guess.`

// builtinSkills returns the shipped skills. A fresh slice each call so callers
// can't mutate the shared set.
func builtinSkills() []Skill {
	codegraphPlusLSPSearch := []string{
		"read_file", "ls", "glob", "grep",
		cgContext, cgTrace, cgImpact, cgSearch, cgExplore, cgNode, cgFiles,
		"lsp_definition", "lsp_references", "lsp_hover",
	}
	reviewTools := []string{
		"read_file", "grep",
		cgContext, cgTrace, cgImpact, cgSearch, cgExplore, cgNode,
		"git_status", "git_diff", "git_log",
		"lsp_diagnostics", "lsp_definition", "lsp_references", "lsp_hover",
	}
	return []Skill{
		{
			Name:        "init",
			Description: "Bootstrap or refresh this project's AGENTS.md — analyze the codebase (structure, build/test commands, architecture, conventions) and write a concise memory file loaded into every future session. Inlined.",
			Body:        builtinInitBody,
			Scope:       ScopeBuiltin,
			Path:        "(builtin)",
			RunAs:       RunInline,
		},
		{
			Name:         "explore",
			Description:  "Explore the codebase in an isolated subagent — wide-net read-only investigation that returns one distilled answer. Best for: 'find all places that...', 'how does X work across the project', 'survey the code for Y'.",
			Body:         builtinExploreBody,
			Scope:        ScopeBuiltin,
			Path:         "(builtin)",
			RunAs:        RunSubagent,
			AllowedTools: append([]string(nil), codegraphPlusLSPSearch...),
		},
		{
			Name:         "research",
			Description:  "Research a question by combining web_search + web_fetch + code reading in an isolated subagent. Best for: 'is X supported by lib Y', 'compare our impl against the spec'.",
			Body:         builtinResearchBody,
			Scope:        ScopeBuiltin,
			Path:         "(builtin)",
			RunAs:        RunSubagent,
			AllowedTools: append(append([]string(nil), codegraphPlusLSPSearch...), "web_fetch", "web_search"),
		},
		{
			Name:         "review",
			Description:  "Review the pending changes (current branch diff by default) in an isolated subagent — flags correctness, security, missing tests, hidden behavior changes; reports a verdict + per-issue file:line. Read-only.",
			Body:         builtinReviewBody,
			Scope:        ScopeBuiltin,
			Path:         "(builtin)",
			RunAs:        RunSubagent,
			AllowedTools: append([]string(nil), reviewTools...),
		},
		{
			Name:         "security-review",
			Description:  "Security-focused review of the current branch diff in an isolated subagent — injection / authz / secrets / deserialization / path-traversal / crypto, severity-tagged. Read-only.",
			Body:         builtinSecurityReviewBody,
			Scope:        ScopeBuiltin,
			Path:         "(builtin)",
			RunAs:        RunSubagent,
			AllowedTools: append([]string(nil), reviewTools...),
		},
		{
			Name:        "tdd",
			Description: "Test-Driven Development with isolation: RED (lsp_diagnostics + git_diff + codegraph trace to isolate, then write failing test) → GREEN (minimal fix) → REFACTOR (clean up + regression test). Inlined. Detects go/npm/pnpm/yarn/pytest/cargo.",
			Body:        builtinTDDBody,
			Scope:       ScopeBuiltin,
			Path:        "(builtin)",
			RunAs:       RunInline,
		},
		{
			Name:        "lsp",
			Description: "Use Tianxuan's LSP tools to diagnose, understand, and fix code — run lsp_diagnostics after every edit, use lsp_definition/lsp_references/lsp_hover for code understanding, use lsp_rename for safe refactors. Inlined.",
			Body:        builtinLSPBody,
			Scope:       ScopeBuiltin,
			Path:        "(builtin)",
			RunAs:       RunInline,
		},
		{
			Name:        "debug",
			Description: "Systematic 4-phase debugging: Reproduce → Isolate (lsp_diagnostics + git_diff + codegraph trace) → Fix (with impact analysis) → Prevent (unit test + regression suite). Inlined.",
			Body:        builtinDebugBody,
			Scope:       ScopeBuiltin,
			Path:        "(builtin)",
			RunAs:       RunInline,
		},
	}
}

// BuiltinNames returns the built-in skill names, used by callers that wire
// dedicated subagent tools for the subagent built-ins.
func BuiltinNames() []string {
	skills := builtinSkills()
	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.Name
	}
	return names
}
