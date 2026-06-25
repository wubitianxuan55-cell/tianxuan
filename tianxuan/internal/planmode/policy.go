package planmode

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Marker is the model-facing plan-mode instruction block. It rides in the user
// turn, not the system prompt or tool schema, so plan toggles preserve cache shape.
const Marker = "[Plan mode — planning only. You may research the codebase and web, ask clarifying questions with ask, maintain planning state with todo_write, and delegate isolated read-only research with read_only_task or read_only_skill. You must not write files, run unsafe shell commands, install capabilities, mutate memory, delegate to writer-capable sub-agents or skills, control long-lived processes, or mark execution steps complete. Before planning, if a decision that is genuinely the user's — tech stack, an ambiguous requirement, scope, an irreversible choice — would materially shape the plan and you can't settle it from the codebase or a sensible default, use the ask tool to clarify it first; otherwise pick the obvious default and state the assumption in the plan instead of asking. Then present a LAYERED plan as your reply and stop. Structure the plan as a two-level markdown list so it becomes a layered task list: each PHASE is a top-level numbered list item (a coherent milestone, e.g. \"1. Add the config loader\"), and each phase's concrete, verifiable sub-steps are bullets indented beneath it (e.g. \"   - parse the TOML into Config\"). Use plain numbered list items for phases — do NOT write phases as markdown headings (##, ###) — so both levels parse. Keep phases few (about 2-6). The user will be asked to approve before any changes are made.]"

// PlanSafety is a tool's self-reported stance on running during the planning
// phase, surfaced via tool.PlanModeClassifier. It is deliberately distinct from
// ReadOnly(): a tool can be side-effect-free (ReadOnly) yet still belong only to
// the post-approval execution phase — complete_step is the canonical example.
type PlanSafety int

const (
	// PlanSafetyUnknown means the tool does not implement PlanModeClassifier, so
	// the policy falls back to its audited read-only whitelist.
	PlanSafetyUnknown PlanSafety = iota
	// PlanSafetySafe means the tool asserts it is safe to run while planning.
	PlanSafetySafe
	// PlanSafetyUnsafe means the tool asserts it must not run while planning,
	// even though ReadOnly() may be true.
	PlanSafetyUnsafe
)

// Call is the plan-mode view of one tool invocation.
type Call struct {
	Name     string
	ReadOnly bool
	// Untrusted is true when ReadOnly came from an external, untrusted source —
	// an MCP server's readOnlyHint. Plan mode does not take such a flag at face
	// value and gates the tool like a writer. Set by the agent from
	// tool.PlanModeUntrustedReadOnly at the gate call site.
	Untrusted bool
	// Safety is the tool's self-reported plan-mode stance. It is Unknown when
	// the tool does not implement tool.PlanModeClassifier; the agent translates
	// the interface result into this field at the gate call site.
	Safety PlanSafety
	Args   json.RawMessage
}

// Decision reports whether plan mode refuses a call and why.
type Decision struct {
	Blocked bool
	Message string
}

// Policy is the single plan-mode stage policy.
type Policy struct {
	AllowedTools []string
}

var knownBlockedTools = map[string]bool{
	"write_file":      true,
	"edit_file":       true,
	"multi_edit":      true,
	"move_file":       true,
	"apply_patch":     true,
	"edit_notebook":   true,
	"notebook_edit":   true,
	"range_delete":    true,
	"symbol_delete":   true,
	"delete_range":    true,
	"delete_symbol":   true,
	"complete_step":   true,
	"task":            true,
	"parallel_tasks":  true,
	"run_skill":       true,
	"explore":         true,
	"research":        true,
	"review":          true,
	"security_review": true,
	"security-review": true,
	"install_source":  true,
	"install_skill":   true,
	"remember":        true,
	"forget":          true,
	"kill_shell":      true,
}

var alwaysAllowedTools = map[string]bool{
	"ask":        true,
	"todo_write": true,
}

// planSafeReadOnly is the audited set of read-only built-in tools confirmed safe
// to run during planning. It is the AUDIT record, not Decide's allow path: Decide
// already trusts any in-process ReadOnly()==true tool. reconcile_test.go uses this
// map (via Classify) to force every built-in into an explicit bucket, so a newly
// added built-in cannot merge without a reviewer recording its plan-mode stance —
// here, in knownBlockedTools, or via tool.PlanModeClassifier.
var planSafeReadOnly = map[string]bool{
	"read_file":    true,
	"ls":           true,
	"glob":         true,
	"grep":         true,
	"code_index":   true,
	"web_fetch":    true,
	"bash_output":  true,
	"connect_tool_source": true,
	"lsp_hover":       true,
	"lsp_definition":  true,
	"lsp_references":  true,
	"lsp_diagnostics": true,
	"read_skill":      true,
	"mcp__tool__query": true, // read-only MCP tool pattern
}

// Classifier describes how a tool is treated during planning: either permitted,
// blocked, or needing individual assessment.
type Classifier int

const (
	ClassDefaultBlocked    Classifier = iota
	ClassPlanSafeAudited              // in planSafeReadOnly map
	ClassPlanSafeSelfReported         // tool implements PlanModeClassifier and returns safe
	ClassPlanUnsafeSelfReported       // tool implements PlanModeClassifier and returns unsafe
	ClassAlwaysAllowed                // in alwaysAllowedTools map
	ClassKnownBlocked                 // in knownBlockedTools map
)

// Classify assigns a tool to a plan-mode bucket given its name, read-only
// status, and self-reported safety. The agent's gate uses this to drive Decide;
// the reconcile test uses it to force every built-in into an explicit bucket.
func Classify(name string, readOnly bool, safety PlanSafety) Classifier {
	if alwaysAllowedTools[name] {
		return ClassAlwaysAllowed
	}
	if knownBlockedTools[name] {
		return ClassKnownBlocked
	}
	if planSafeReadOnly[name] {
		if !readOnly {
			return ClassDefaultBlocked
		}
		return ClassPlanSafeAudited
	}
	switch safety {
	case PlanSafetySafe:
		if !readOnly {
			return ClassDefaultBlocked
		}
		return ClassPlanSafeSelfReported
	case PlanSafetyUnsafe:
		return ClassPlanUnsafeSelfReported
	}
	return ClassDefaultBlocked
}

// Decide evaluates a single tool call against the plan-mode policy.
// It returns a Decision: Blocked = true means the call is refused.
func (p Policy) Decide(call Call) Decision {
	name := call.Name

	// 1. Always-allowed tools: never blocked.
	if alwaysAllowedTools[name] {
		return Decision{}
	}

	// 2. Known-blocked tools: always blocked, even if in AllowedTools override.
	if knownBlockedTools[name] {
		msg := fmt.Sprintf("not available in plan mode — %q is a writer/executor tool", name)
		if name == "complete_step" {
			msg = "not available in plan mode — execution step sign-off belongs after plan approval"
		}
		return Decision{Blocked: true, Message: msg}
	}

	// 3. Read-only trusted: in-process tools with ReadOnly()==true.
	//    If Untrusted (MCP readOnlyHint), fail closed.
	if call.ReadOnly && !call.Untrusted {
		return Decision{}
	}

	// 4. Untrusted read-only: an MCP tool claiming ReadOnly via readOnlyHint.
	//    Block it unless explicitly overridden.
	if call.ReadOnly && call.Untrusted {
		if p.isAllowed(name) {
			return Decision{}
		}
		return Decision{Blocked: true, Message: fmt.Sprintf("not available in plan mode — trust %q was marked as readOnlyHint, which is not a verified read-only label. Declare it in plan_mode_allowed_tools to enable it", name)}
	}

	// 5. Self-reported plan-safe: PlanModeClassifier returns safe.
	if call.Safety == PlanSafetySafe {
		if !call.ReadOnly {
			return Decision{Blocked: true, Message: fmt.Sprintf("invariant violation: tool %q reports PlanSafe but ReadOnly is false. Correct its PlanModeClassifier or mark it ReadOnly", name)}
		}
		return Decision{}
	}

	// 6. Self-reported plan-unsafe.
	if call.Safety == PlanSafetyUnsafe {
		return Decision{Blocked: true, Message: fmt.Sprintf("not available in plan mode — %q reports it is not safe to use during planning", name)}
	}

	// 7. Fail-closed for unclassified external tools (MCP/plugin, no safety report).
	if p.isAllowed(name) {
		return Decision{}
	}
	return Decision{Blocked: true, Message: fmt.Sprintf("not available in plan mode — %q must be declared in plan_mode_allowed_tools to be used during planning", name)}
}

// IgnoredAllowedTools returns tools listed in AllowedTools that were overridden
// by a higher-priority block (known blocked tools). Callers should surface this.
func (p Policy) IgnoredAllowedTools() []string {
	var ignored []string
	for _, name := range p.AllowedTools {
		if knownBlockedTools[name] {
			ignored = append(ignored, name)
		}
	}
	sort.Strings(ignored)
	return ignored
}

func (p Policy) isAllowed(name string) bool {
	for _, a := range p.AllowedTools {
		if a == name {
			return true
		}
	}
	return false
}

// SafeBashCommands returns the set of shell commands audited as read-only and
// safe during planning. This is planmode's sole responsibility — bash itself
// does not know about plan mode.
var SafeBashCommands = map[string]bool{
	// inspection
	"cat":    true,
	"head":   true,
	"tail":   true,
	"less":   true,
	"more":   true,
	"nl":     true,
	"wc":     true,
	"od":     true,
	"xxd":    true,
	"hexdump": true,
	"file":   true,
	"stat":   true,
	"du":     true,
	"df":     true,
	"which":  true,
	"type":   true,
	"command -v": true,

	// directory listing
	"ls":  true,
	"find": true,
	"tree": true,
	"exa": true,
	"eza": true,

	// text processing (read-only)
	"grep":    true,
	"egrep":   true,
	"fgrep":   true,
	"rg":      true,
	"ripgrep": true,
	"awk":     true,
	"sed":     true, // sed -n (no -i) is safe; full validation below
	"sort":    true,
	"uniq":    true,
	"cut":     true,
	"tr":      true,
	"diff":    true,
	"comm":    true,
	"cmp":     true,
	"strings": true,
	"iconv":   true,
	"jq":      true,
	"yq":      true,

	// file content display
	"echo":    true,
	"printf":  true,
	"env":     true,
	"printenv": true,
	"pwd":     true,
	"date":    true,
	"cal":     true,
	"nproc":   true,
	"uname":   true,
	"hostname": true,
	"whoami":  true,
	"id":      true,
	"logname": true,
	"groups":  true,
	"tty":     true,

	// network read-only
	"curl":   true,
	"wget":   true,
	"httpie": true,
	"ping":   true,
	"traceroute": true,
	"nslookup": true,
	"dig":    true,
	"host":   true,
	"nc":     true,

	// process inspection (read-only)
	"ps":     true,
	"top":    true,
	"htop":   true,
	"btop":   true,
	"lsof":   true,
	"pgrep":  true,
	"pidstat": true,
	"vmstat": true,
	"iostat": true,
	"netstat": true,
	"ss":     true,
	"lscpu":  true,
	"lsblk":  true,
	"lsusb":  true,
	"lspci":  true,
	"dmesg":  true,
	"sysctl": true,
	"uptime": true,
	"w":      true,
	"who":    true,
	"last":   true,
	"lastb":  true,
	"free":   true,
	"mpstat": true,

	// development read-only
	"go":      true, // go vet/build/test may write; validated in Decode
	"go list": true,
	"go vet":  true,
	"go build": true,
	"go test": true,
	"cargo":   true,
	"rustc":   true,
	"npm":     true,
	"npx":     true,
	"node":    true,
	"python":  true,
	"python3": true,
	"pip":     true,
	"pip3":    true,

	// git read-only operations
	"git":       true,
	"git status": true,
	"git diff":  true,
	"git log":   true,
	"git show":  true,
	"git grep":  true,
	"git blame": true,
	"git branch -a": true,
	"git branch --list": true,
	"git branch -r": true,
	"git ls-files": true,
	"git ls-tree": true,
	"git describe": true,
	"git shortlog": true,
	"git stash list": true,
	"git tag -l": true,
	"git tag --list": true,
}

// findWriteArgs lists find arguments that write or execute.
var findWriteArgs = map[string]bool{
	"-delete":  true,
	"-exec":    true,
	"-execdir": true,
	"-ok":      true,
	"-okdir":   true,
	"-fls":     true,
	"-fprint":  true,
	"-fprint0": true,
	"-printf":  true, // can write to arbitrary files with %() format
}

// goWriteOrExecArgs lists go arguments that write or execute code.
var goWriteOrExecArgs = map[string]bool{
	"-exec":     true,
	"-run":      true,
	"-cover":    true,
	"-race":     true,
	"-msan":     true,
	"-asan":     true,
	"-bench":    true,
	"-cpu":      true,
	"-count":    true,
	"-timeout":  true,
	"-list":     true,
	"-find":     true,
	"-json":     true,
	"-o":        true,
	"-a":        true,
	"-n":        true,
	"-x":        true,
	"-v":        true,
	"-work":     true,
	"-p":        true,
}

// DecodeBash returns a decision for a bash command. It allows commands on the
// safe list and blocks everything else, including known-writer arguments.
func (p Policy) DecodeBash(cmd string, runInBackground, preserveBg bool) Decision {
	// Background / process-preservation flags are always blocked in plan mode.
	if runInBackground {
		return Decision{Blocked: true, Message: "background execution is not available in plan mode"}
	}
	if preserveBg {
		return Decision{Blocked: true, Message: "process preservation is not available in plan mode"}
	}

	lower := strings.TrimSpace(strings.ToLower(cmd))
	if lower == "" {
		return Decision{Blocked: true, Message: "empty command"}
	}

	// Direct lookup: exact match wins.
	if SafeBashCommands[lower] {
		return Decision{}
	}

	// Prefix match against multi-word safe commands.
	for safe := range SafeBashCommands {
		if strings.Contains(safe, " ") && bashMatchesSafePrefix(lower, safe) {
			// Check command-specific unsafe arguments.
			if arg, msg := unsafeSafeCommandArg(cmd, safe); arg != "" {
				if msg != "" {
					return Decision{Blocked: true, Message: fmt.Sprintf("blocked: %s", msg)}
				}
				return Decision{Blocked: true, Message: fmt.Sprintf("blocked: %q is not a safe argument for %q in plan mode", arg, safe)}
			}
			return Decision{}
		}
	}

	return Decision{Blocked: true, Message: fmt.Sprintf("blocked: bash commands in plan mode must be read-only. %q is not in the safe command list. Use read-only tools for exploration, then exit plan mode to run this command.", cmd)}
}

func bashMatchesSafePrefix(lower, safe string) bool {
	if !strings.HasPrefix(lower, safe) {
		return false
	}
	if len(lower) == len(safe) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(lower[len(safe):])
	return unicode.IsSpace(r)
}

func unsafeSafeCommandArg(cmd, safe string) (string, string) {
	fields, err := shellFields(cmd)
	if err != "" {
		return "", err
	}
	base := strings.Fields(safe)
	if len(fields) <= len(base) {
		return "", ""
	}
	args := fields[len(base):]
	lowerArgs := make([]string, len(args))
	for i, arg := range args {
		lowerArgs[i] = strings.ToLower(arg)
	}
	if strings.HasPrefix(safe, "git ") {
		for _, arg := range lowerArgs {
			if arg == "--output" || strings.HasPrefix(arg, "--output=") || arg == "--ext-diff" {
				return arg, ""
			}
		}
	}
	switch safe {
	case "git grep":
		for i, arg := range args {
			lowerArg := lowerArgs[i]
			if arg == "-O" || strings.HasPrefix(arg, "-O") || strings.HasPrefix(lowerArg, "--open-files-in-pager") {
				return arg, ""
			}
		}
	case "find":
		for _, arg := range lowerArgs {
			if findWriteArgs[arg] {
				return arg, ""
			}
		}
	case "go list", "go vet":
		for _, arg := range lowerArgs {
			if goWriteOrExecArgs[arg] || strings.HasPrefix(arg, "-mod=mod") || strings.HasPrefix(arg, "-modfile=") || strings.HasPrefix(arg, "-toolexec=") || strings.HasPrefix(arg, "-vettool=") {
				return arg, ""
			}
		}
	}
	return "", ""
}

func shellFields(s string) ([]string, string) {
	var fields []string
	var b strings.Builder
	inSingle := false
	inDouble := false
	escaped := false
	haveField := false
	flush := func() {
		if haveField {
			fields = append(fields, b.String())
			b.Reset()
			haveField = false
		}
	}
	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			haveField = true
			escaped = false
			continue
		}
		if inSingle {
			if r == '\'' {
				inSingle = false
				continue
			}
			b.WriteRune(r)
			haveField = true
			continue
		}
		if inDouble {
			switch r {
			case '"':
				inDouble = false
			case '\\':
				escaped = true
			default:
				b.WriteRune(r)
				haveField = true
			}
			continue
		}
		switch {
		case unicode.IsSpace(r):
			flush()
		case r == '\'':
			inSingle = true
			haveField = true
		case r == '"':
			inDouble = true
			haveField = true
		case r == '\\':
			escaped = true
			haveField = true
		default:
			b.WriteRune(r)
			haveField = true
		}
	}
	if escaped {
		return nil, "dangling escape"
	}
	if inSingle {
		return nil, "unterminated single quote"
	}
	if inDouble {
		return nil, "unterminated double quote"
	}
	flush()
	return fields, ""
}
