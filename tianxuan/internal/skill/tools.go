package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"tianxuan/internal/tool"
)

// SubagentRunner runs a runAs=subagent skill: it spawns an isolated child loop
// with the skill body as system prompt and `task` as its only input, returning
// the final answer. boot wires this over the agent's sub-agent machinery; nil
// means subagent skills are unavailable in this session (they error rather than
// silently inlining, which would lose the isolation the author asked for).
type SubagentRunner func(ctx context.Context, sk Skill, task string) (string, error)

// InstalledHook fires after install_skill writes a new file, so a host can
// refresh UI (e.g. a skills sidebar) without a reload. nil is fine.
type InstalledHook func(name, path string, scope Scope)

// --- run_skill ---

type runSkillTool struct {
	store  *Store
	runner SubagentRunner
}

// NewRunSkillTool builds the general skill-invocation tool. runner may be nil
// (subagent skills then error).
func NewRunSkillTool(store *Store, runner SubagentRunner) tool.Tool {
	return &runSkillTool{store: store, runner: runner}
}

func (*runSkillTool) Name() string { return "run_skill" }

// ReadOnly is false: an invoked subagent skill could call writer tools, so
// classify conservatively to keep the parallel-dispatch path from racing two
// skill runs' writes (mirrors the `task` tool).
func (*runSkillTool) ReadOnly() bool { return false }

func (*runSkillTool) Description() string {
	return "Invoke a playbook from the Skills index pinned in the system prompt. For the built-in subagent skills (explore / research / review / security_review), prefer the dedicated top-level tools of the same name — they're easier to pick and do the same thing. Pass `name` as the BARE identifier (e.g. 'explore'), NOT the `[🧬 subagent]` tag that follows it in the index. `[🧬 subagent]` skills spawn an isolated subagent — only the final distilled answer returns; supply `arguments` describing the concrete task since the subagent has no other context. Untagged skills are inlined: the body becomes a tool result you read and follow."
}

func (*runSkillTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "name":{"type":"string","description":"Skill identifier as it appears in the pinned Skills index (e.g. 'explore', 'review'). Case-sensitive. Just the identifier, not the [🧬 subagent] tag."},
  "arguments":{"type":"string","description":"Free-form arguments. For inline skills: appended as an 'Arguments:' line; the skill's own instructions decide how to use them. For subagent skills: REQUIRED — becomes the entire task the subagent receives."}
},
"required":["name"]
}`)
}

func (t *runSkillTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	name := cleanSkillName(p.Name)
	if name == "" {
		return "", fmt.Errorf("run_skill requires a 'name' argument (got %q, which is just a marker/tag)", p.Name)
	}
	sk, ok := t.store.Read(name)
	if !ok {
		return "", fmt.Errorf("unknown skill %q — available: %s", name, availableNames(t.store))
	}
	rawArgs := strings.TrimSpace(p.Arguments)

	if sk.RunAs == RunSubagent {
		if t.runner == nil {
			return "", fmt.Errorf("run_skill: skill %q is runAs=subagent but no subagent runner is configured in this session", name)
		}
		if rawArgs == "" {
			return "", fmt.Errorf("run_skill: skill %q is a subagent and requires 'arguments' — the subagent has no other context, so describe the concrete task", name)
		}
		return t.runner(ctx, sk, rawArgs)
	}
	if sk.RunAs == RunPipeline {
		if t.runner == nil {
			return "", fmt.Errorf("run_skill: skill %q is runAs=pipeline but no subagent runner is configured in this session", name)
		}
		return t.runPipeline(ctx, sk, rawArgs)
	}
	return renderInline(sk, rawArgs), nil
}

// runPipeline 执行管道类型技能：解析 body 中的 JSON 管道定义，用参数填充后通过 RunDAG 执行。
func (t *runSkillTool) runPipeline(ctx context.Context, sk Skill, rawArgs string) (string, error) {
	pl, err := LoadPipelineJSON(strings.NewReader(sk.Body))
	if err != nil {
		return "", fmt.Errorf("run_skill: skill %q pipeline body is invalid: %w", sk.Name, err)
	}

	// 解析参数：key=value 格式，空格或换行分隔
	params := parsePipelineArgs(rawArgs)
	resolved := pl.Resolve(params)
	tasks := resolved.ToTasks()

	results, err := RunDAG(ctx, tasks, t.runner)
	if err != nil {
		return "", fmt.Errorf("run_skill: pipeline %q: %w", sk.Name, err)
	}

	out, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("run_skill: marshal pipeline results: %w", err)
	}
	return string(out), nil
}

// parsePipelineArgs 解析 key=value 格式的参数字符串。
func parsePipelineArgs(raw string) map[string]string {
	params := make(map[string]string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return params
	}
	// 按空白字符分割
	parts := strings.Fields(raw)
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			params[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return params
}

// --- dedicated subagent wrappers (explore / research / review / security_review) ---

type subagentSkillTool struct {
	toolName    string
	skillName   string
	description string
	taskDesc    string
	store       *Store
	runner      SubagentRunner
}

func (t *subagentSkillTool) Name() string        { return t.toolName }
func (*subagentSkillTool) ReadOnly() bool        { return false }
func (t *subagentSkillTool) Description() string { return t.description }

func (t *subagentSkillTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"task":{"type":"string","description":` +
		strconv.Quote(t.taskDesc) + `}},"required":["task"]}`)
}

func (t *subagentSkillTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	task := strings.TrimSpace(p.Task)
	if task == "" {
		return "", fmt.Errorf("%s requires a non-empty 'task' argument — describe the concrete question", t.toolName)
	}
	sk, ok := t.store.Read(t.skillName)
	if !ok {
		return "", fmt.Errorf("%s: built-in skill %q is not registered", t.toolName, t.skillName)
	}
	// A user file overriding the built-in name with runAs:inline would lose
	// isolation if dispatched here — bounce to run_skill where inline is defined.
	if sk.RunAs != RunSubagent {
		return "", fmt.Errorf("%s: skill %q is overridden as inline; invoke it via run_skill instead", t.toolName, t.skillName)
	}
	if t.runner == nil {
		return "", fmt.Errorf("%s: no subagent runner is configured in this session", t.toolName)
	}
	return t.runner(ctx, sk, task)
}

// BuiltinSubagentTools returns top-level wrapper tools for the built-in subagent
// skills, named after the verb so the model picks them naturally (affordance >
// prompt rules). Each is skipped when its underlying skill isn't present (e.g. a
// user disabled it), so the tool set never advertises a phantom skill.
func BuiltinSubagentTools(store *Store, runner SubagentRunner) []tool.Tool {
	specs := []struct {
		toolName, skillName, description, taskDesc string
	}{
		{"explore", "explore",
			"Run a focused read-only codebase investigation in an isolated subagent. Use for broad survey questions across many files — 'find all places that X', 'how does Y work across the project', 'audit Z'. Returns one distilled answer with file:line citations. Its reads + reasoning never enter your context, unlike chained read_file.",
			"Concrete investigation question. The subagent has none of your context — write a self-contained prompt naming the symbol / pattern / behavior to survey."},
		{"research", "research",
			"Combine web_search + web_fetch + code reading in an isolated subagent. Use when the answer needs both an external reference and local verification — 'is X supported by lib Y', 'compare our impl against the spec'. Returns one synthesis citing code (file:line) and web (URL).",
			"Concrete research question. The subagent has none of your context — name the external thing to look up and the local code to compare against."},
		{"review", "review",
			"Review the pending changes (current branch diff) in an isolated subagent — flags correctness / security / missing-tests / hidden behavior per file:line. Read-only; you decide what to act on. Use before suggesting a PR-shaped change or after finishing a multi-step edit.",
			"What to focus the review on (e.g. 'focus on the auth changes' or 'general'). The subagent reads the diff itself."},
		{"security_review", "security-review",
			"Security-focused review of the current branch diff in an isolated subagent — injection / authz / secrets / deserialization / path-traversal / crypto, severity-tagged. Read-only. Use when shipping changes that touch auth, input parsing, file IO, or external requests.",
			"Optional scope hint (e.g. 'focus on token handling in internal/auth/') or 'full' for everything in the diff."},
	}
	var out []tool.Tool
	for _, s := range specs {
		if _, ok := store.Read(s.skillName); !ok {
			continue
		}
		out = append(out, &subagentSkillTool{
			toolName:    s.toolName,
			skillName:   s.skillName,
			description: s.description,
			taskDesc:    s.taskDesc,
			store:       store,
			runner:      runner,
		})
	}
	return out
}

// --- install_skill ---

type installSkillTool struct {
	store       *Store
	onInstalled InstalledHook
}

// NewInstallSkillTool builds the skill-authoring tool. onInstalled may be nil.
func NewInstallSkillTool(store *Store, onInstalled InstalledHook) tool.Tool {
	return &installSkillTool{store: store, onInstalled: onInstalled}
}

func (*installSkillTool) Name() string   { return "install_skill" }
func (*installSkillTool) ReadOnly() bool { return false }

func (t *installSkillTool) Description() string {
	scope := "'global' (only option — no project workspace) writes to ~/.tianxuan/skills/."
	if t.store.HasProjectScope() {
		scope = "'project' (default) writes to <repo>/.tianxuan/skills/ (this workspace only); 'global' writes to ~/.tianxuan/skills/ (every project)."
	}
	return "Author and save a new skill — a reusable playbook future turns invoke via run_skill (or /<name>). Runnable immediately this turn; appears in the pinned Skills index on the next launch. " + scope
}

func (*installSkillTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "name":{"type":"string","description":"Identifier — letters/digits/_/-/., 1-64 chars, starts alphanumeric. Becomes the filename."},
  "description":{"type":"string","description":"≤120-char one-liner shown in the pinned Skills index — future agents read it to decide whether to invoke."},
  "body":{"type":"string","description":"Markdown playbook. For subagent skills, write the subagent's persona/rules — it gets no context besides 'arguments' at runtime."},
  "scope":{"type":"string","enum":["project","global"],"description":"Where to write. Defaults to project when a workspace exists, else global."},
  "runAs":{"type":"string","enum":["inline","subagent","pipeline"],"description":"inline (default) folds the body into the parent turn; subagent spawns an isolated child loop returning only its final answer; pipeline runs a sequence of subagent steps with dependency ordering."},
  "model":{"type":"string","description":"Optional model override for runAs=subagent (a configured provider/model name). Ignored otherwise."},
  "allowedTools":{"type":"array","items":{"type":"string"},"description":"Optional tool allowlist for runAs=subagent (e.g. ['read_file','grep'])."}
},
"required":["name","description","body"]
}`)
}

func (t *installSkillTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Name         string   `json:"name"`
		Description  string   `json:"description"`
		Body         string   `json:"body"`
		Scope        string   `json:"scope"`
		RunAs        string   `json:"runAs"`
		Model        string   `json:"model"`
		AllowedTools []string `json:"allowedTools"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	name := strings.TrimSpace(p.Name)
	desc := strings.TrimSpace(collapseSpaces(p.Description))
	if name == "" {
		return "", fmt.Errorf("install_skill requires a non-empty 'name'")
	}
	if desc == "" {
		return "", fmt.Errorf("install_skill requires a non-empty 'description' — it is what appears in the Skills index")
	}
	if strings.TrimSpace(p.Body) == "" {
		return "", fmt.Errorf("install_skill requires a non-empty 'body' — the playbook the skill executes")
	}

	scope := ScopeGlobal
	switch strings.TrimSpace(p.Scope) {
	case "global":
		scope = ScopeGlobal
	case "project":
		scope = ScopeProject
	default:
		if t.store.HasProjectScope() {
			scope = ScopeProject
		}
	}
	if scope == ScopeProject && !t.store.HasProjectScope() {
		return "", fmt.Errorf("install_skill: scope='project' requires a workspace — use scope='global'")
	}

	runAs := RunInline
	switch strings.TrimSpace(p.RunAs) {
	case "subagent":
		runAs = RunSubagent
	case "pipeline":
		runAs = RunPipeline
	}

	content := renderSkillFile(name, desc, p.Body, runAs, strings.TrimSpace(p.Model), p.AllowedTools)
	path, err := t.store.CreateWithContent(name, scope, content)
	if err != nil {
		return "", err
	}
	if t.onInstalled != nil {
		t.onInstalled(name, path, scope)
	}
	res, _ := json.Marshal(map[string]any{
		"ok":    true,
		"name":  name,
		"scope": string(scope),
		"path":  path,
		"runAs": string(runAs),
		"note":  "Callable now via run_skill({name}) or /" + name + ". Appears in the pinned Skills index on the next launch.",
	})
	return string(res), nil
}

// renderSkillFile assembles a skill file's frontmatter + body. Subagent-only
// fields (model, allowed-tools) are emitted only when relevant.
func renderSkillFile(name, desc, body string, runAs RunAs, model string, allowedTools []string) string {
	var fm strings.Builder
	fm.WriteString("---\nname: " + name + "\ndescription: " + desc + "\n")
	if runAs == RunSubagent || runAs == RunPipeline {
		fm.WriteString("runAs: " + string(runAs) + "\n")
		if runAs == RunSubagent {
			if model != "" {
				fm.WriteString("model: " + model + "\n")
			}
			var tools []string
			for _, t := range allowedTools {
				if t = strings.TrimSpace(t); t != "" {
					tools = append(tools, t)
				}
			}
			if len(tools) > 0 {
				fm.WriteString("allowed-tools: " + strings.Join(tools, ", ") + "\n")
			}
		}
	}
	fm.WriteString("---\n\n")
	return fm.String() + strings.TrimRight(body, " \t\r\n") + "\n"
}

// --- shared helpers ---

// Render builds a skill's invocation text: a header (name, description, source)
// followed by the body and any arguments. Used directly when a user invokes a
// skill via "/<name>" (sent as a turn); the run_skill tool wraps the same text
// in a skill-pin sentinel (see renderInline).
func Render(sk Skill, args string) string {
	var b strings.Builder
	b.WriteString("# Skill: " + sk.Name)
	if sk.Description != "" {
		b.WriteString("\n> " + sk.Description)
	}
	b.WriteString("\n(scope: " + string(sk.Scope) + " · " + sk.Path + ")\n\n")
	b.WriteString(sk.Body)
	if args != "" {
		b.WriteString("\n\nArguments: " + args)
	}
	return b.String()
}

// renderInline wraps Render's output in a skill-pin sentinel so context
// compaction preserves the body verbatim instead of paraphrasing it.
func renderInline(sk Skill, args string) string {
	return "<skill-pin name=" + strconv.Quote(sk.Name) + ">\n" + Render(sk, args) + "\n</skill-pin>"
}

var bracketTagRe = regexp.MustCompile(`\[[^\]]*\]`)

// cleanSkillName extracts the bare identifier from a possibly-decorated name:
// models sometimes copy the index's "explore [🧬 subagent]" verbatim into the
// `name` arg. Drop any [..] tag, then take the first token starting alphanumeric.
func cleanSkillName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	stripped := strings.TrimSpace(bracketTagRe.ReplaceAllString(raw, " "))
	for _, tok := range strings.Fields(stripped) {
		if c := tok[0]; (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return tok
		}
	}
	return ""
}

// collapseSpaces turns any run of whitespace (incl. newlines) into a single
// space, so a multi-line description stays a one-liner in the index.
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// --- parallel_skills ---

type parallelSkillsTool struct {
	store  *Store
	runner SubagentRunner
}

// NewParallelSkillsTool 构建并行技能调用工具。runner 为 nil 时工具返回错误。
func NewParallelSkillsTool(store *Store, runner SubagentRunner) tool.Tool {
	return &parallelSkillsTool{store: store, runner: runner}
}

func (*parallelSkillsTool) Name() string { return "parallel_skills" }

func (*parallelSkillsTool) ReadOnly() bool { return false }

func (*parallelSkillsTool) Description() string {
	return "并行派发多个子代理技能，所有子代理同时执行，全部完成后返回汇总结果。适用于 2+ 个互相独立的调查/分析任务（如同时探索多个模块、并行审查多个文件）。任务间不应有依赖关系——有依赖时请分次调用 run_skill。"
}

func (*parallelSkillsTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "tasks":{"type":"array","items":{
    "type":"object",
    "properties":{
      "skill":{"type":"string","description":"技能名称，如 explore、review"},
      "arguments":{"type":"string","description":"传给技能的任务描述"},
      "id":{"type":"string","description":"可选标识，用于 depends_on 引用"},
      "depends_on":{"type":"array","items":{"type":"string"},"description":"依赖的任务 id 列表——这些任务完成后才执行本任务，且其结果会作为上下文注入"}
    },
    "required":["skill","arguments"]
  },"description":"要执行的任务列表。无 depends_on 的任务并行执行；有 depends_on 的任务等待依赖完成后执行并接收其结果。"}
},
"required":["tasks"]
}`)
}

func (t *parallelSkillsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Tasks []struct {
			Skill     string   `json:"skill"`
			Arguments string   `json:"arguments"`
			ID        string   `json:"id"`
			DependsOn []string `json:"depends_on"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("parallel_skills: invalid args: %w", err)
	}
	if len(p.Tasks) == 0 {
		return "", fmt.Errorf("parallel_skills: tasks must not be empty")
	}
	if t.runner == nil {
		return "", fmt.Errorf("parallel_skills: no subagent runner configured")
	}

	// 校验所有技能名存在
	for _, task := range p.Tasks {
		name := cleanSkillName(task.Skill)
		if _, ok := t.store.Read(name); !ok {
			return "", fmt.Errorf("parallel_skills: unknown skill %q — available: %s", name, availableNames(t.store))
		}
	}

	// 构建 ParallelTask 列表（含 ID 和 DependsOn）
	ptasks := make([]ParallelTask, len(p.Tasks))
	for i, task := range p.Tasks {
		ptasks[i] = ParallelTask{
			Skill:     cleanSkillName(task.Skill),
			Arguments: task.Arguments,
			ID:        task.ID,
			DependsOn: task.DependsOn,
		}
	}

	// 使用 RunDAG 执行——有依赖时自动分波，无依赖时退化为纯并行
	results, err := RunDAG(ctx, ptasks, t.runner)
	if err != nil {
		return "", fmt.Errorf("parallel_skills: %w", err)
	}

	// 序列化为 JSON 返回
	out, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("parallel_skills: marshal results: %w", err)
	}
	return string(out), nil
}

// availableNames lists the discoverable skill names for an error message.
func availableNames(store *Store) string {
	skills := store.List()
	if len(skills) == 0 {
		return "(none — no skills defined)"
	}
	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.Name
	}
	return strings.Join(names, ", ")
}
