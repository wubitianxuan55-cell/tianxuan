package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// subagentSkillTool exposes one runAs=subagent skill as its own model-visible
// tool (explore / research / review / security_review). A dedicated entry lets
// DeepSeek-style native tool calling see the capability directly, instead of
// requiring the model to recall the skill name and route through run_skill.
// Execution reuses the same isolated read-only subagent runner as run_skill.
type subagentSkillTool struct {
	store       *Store
	runner      SubagentRunner
	name        string // skill identifier (may contain hyphens, e.g. security-review)
	desc        string
	compactDesc string
}

// NewSubagentSkillTool builds a dedicated tool for one built-in subagent skill.
// name is the skill identifier and tool name (e.g. "explore"); desc is the
// full model-facing description; compactDesc is the token-light variant.
func NewSubagentSkillTool(store *Store, runner SubagentRunner, name, desc, compactDesc string) tool.Tool {
	return &subagentSkillTool{store: store, runner: runner, name: name, desc: desc, compactDesc: compactDesc}
}

// Name returns the model-visible tool name: the skill identifier with hyphens
// normalised to underscores (security-review → security_review), matching the
// built-in tool naming convention.
func (t *subagentSkillTool) Name() string {
	return strings.ReplaceAll(t.name, "-", "_")
}

func (t *subagentSkillTool) Description() string { return t.desc }

func (*subagentSkillTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"task":{"type":"string","description":"The concrete investigation/review question. The sub-agent runs in an isolated read-only context and sees only this text, so it must be self-contained."}},"required":["task"]}`)
}

// ReadOnly is true: dedicated subagent skills are read-only investigations
// (their sub-agent toolset is filtered to read-only tools), so parallel
// dispatch may batch them safely.
func (*subagentSkillTool) ReadOnly() bool { return true }

func (t *subagentSkillTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if strings.TrimSpace(p.Task) == "" {
		return "", fmt.Errorf("%s requires a 'task' argument", t.name)
	}
	sk, ok := t.store.Read(t.name)
	if !ok {
		// The tool name may differ from the skill identifier only in
		// underscore/hyphen spelling (security_review vs security-review).
		if alt := strings.ReplaceAll(t.name, "_", "-"); alt != t.name {
			sk, ok = t.store.Read(alt)
		}
	}
	if !ok {
		return "", fmt.Errorf("unknown skill %q", t.name)
	}
	if sk.RunAs != RunSubagent {
		return "", fmt.Errorf("skill %q is not runAs=subagent", t.name)
	}
	if t.runner == nil {
		return "", fmt.Errorf("%s: no subagent runner configured in this session", t.name)
	}
	return t.runner(ctx, sk, p.Task)
}

func (t *subagentSkillTool) CompactDescription() string { return t.compactDesc }

func (*subagentSkillTool) CompactSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"task":{"type":"string"}},"required":["task"]}`)
}

// --- ui_styling (script-backed styling tool) ---

type uiStylingTool struct {
	store *Store
}

// NewUiStylingTool builds the ui-styling script tool: it locates the
// ui-styling skill's scripts and references inside the skill store and runs /
// returns them on demand, so the model does not need to remember the script
// paths or invoke bash itself.
func NewUiStylingTool(store *Store) tool.Tool {
	return &uiStylingTool{store: store}
}

func (*uiStylingTool) Name() string { return "ui_styling" }

func (*uiStylingTool) Description() string {
	return "Generate Tailwind config or fetch shadcn/ui styling guidance from the ui-styling skill. action=config passes args directly to tailwind_config_gen.py (e.g. \"--colors brand:blue --fonts display:Inter\"); action=guide takes a keyword (e.g. \"dialog\", \"dark mode\") and returns the matching guide excerpt."
}

func (*uiStylingTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["config","guide"],"description":"config: run tailwind_config_gen.py with args; guide: fetch a reference guide by keyword."},"args":{"type":"string"}},"required":["action"]}`)
}

// ReadOnly is false: the config action runs a Python script that may write
// files, so the parallel-dispatch path must not batch it with other calls.
func (*uiStylingTool) ReadOnly() bool { return false }

func (t *uiStylingTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Action string `json:"action"`
		Args   string `json:"args"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	sk, ok := t.store.Read("ui-styling")
	if !ok {
		return "", fmt.Errorf("ui-styling skill not found")
	}
	dir := filepath.Dir(sk.Path)
	switch p.Action {
	case "config":
		script := filepath.Join(dir, "scripts", "tailwind_config_gen.py")
		if _, err := os.Stat(script); err != nil {
			return "", fmt.Errorf("tailwind_config_gen.py not found at %s", script)
		}
		return runPythonScript(script, p.Args)
	case "guide":
		return uiStylingGuide(dir, p.Args)
	default:
		return "", fmt.Errorf("ui_styling action must be \"config\" or \"guide\"")
	}
}

func (*uiStylingTool) CompactDescription() string {
	return "生成 Tailwind 配置或读取 shadcn/ui 样式指南（action: config|guide）"
}

func (*uiStylingTool) CompactSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"action":{"type":"string"},"args":{"type":"string"}},"required":["action"]}`)
}

func runPythonScript(script, args string) (string, error) {
	py, err := exec.LookPath("python")
	if err != nil {
		if py3, err3 := exec.LookPath("python3"); err3 == nil {
			py = py3
		} else {
			return "", fmt.Errorf("python not found: %w", err)
		}
	}
	argv := []string{script}
	if strings.TrimSpace(args) != "" {
		argv = append(argv, strings.Fields(args)...)
	}
	cmd := exec.Command(py, argv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("python %s: %w", script, err)
	}
	return string(out), nil
}

func uiStylingGuide(dir, keyword string) (string, error) {
	refDir := filepath.Join(dir, "references")
	entries, err := os.ReadDir(refDir)
	if err != nil {
		return "", fmt.Errorf("read ui-styling references: %w", err)
	}
	var names, mdFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			names = append(names, e.Name())
			mdFiles = append(mdFiles, filepath.Join(refDir, e.Name()))
		}
	}
	if kw := strings.ToLower(strings.TrimSpace(keyword)); kw != "" {
		// Filename match first: whole guide is the answer.
		for _, n := range names {
			if strings.Contains(strings.ToLower(n), kw) {
				return truncateGuide(readGuideFile(filepath.Join(refDir, n))), nil
			}
		}
		// Content match: return the matching lines with context.
		for _, path := range mdFiles {
			b, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			lines := strings.Split(string(b), "\n")
			for i, ln := range lines {
				if strings.Contains(strings.ToLower(ln), kw) {
					start := i - 2
					if start < 0 {
						start = 0
					}
					end := i + 6
					if end > len(lines) {
						end = len(lines)
					}
					return fmt.Sprintf("file: %s\n%s", filepath.Base(path), strings.Join(lines[start:end], "\n")), nil
				}
			}
		}
	}
	return "Available ui-styling references:\n- " + strings.Join(names, "\n- ") +
		"\n\nMatch a keyword to read one; otherwise use read_skill for the full SKILL.md.", nil
}

func readGuideFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func truncateGuide(text string) string {
	if len(text) > 6000 {
		return text[:6000] + "\n...[truncated]"
	}
	return text
}

// --- design_router ---

type designRouterTool struct {
	store *Store
}

// NewDesignRouterTool builds the design routing tool: it classifies a design
// task and returns the sub-agent skill (plus a rewritten task) the model
// should dispatch, or the design-internal script to use for logo/icon work.
func NewDesignRouterTool(store *Store) tool.Tool {
	return &designRouterTool{store: store}
}

func (*designRouterTool) Name() string { return "design_router" }

func (*designRouterTool) Description() string {
	return "Route a design task to the right sub-agent skill: banner\u2192banner-design; slides/deck\u2192slides; brand\u2192brand; token/component specs\u2192design-system; logo/icon\u2192design; aesthetics/anti-template/landing/portfolio/redesign\u2192taste-skill; UI/color/typography/UX\u2192ui-ux-pro-max; implementation\u2192ui_styling."
}

func (*designRouterTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"task":{"type":"string","description":"The design task description to route."}},"required":["task"]}`)
}

func (*designRouterTool) ReadOnly() bool { return true }

func (t *designRouterTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if strings.TrimSpace(p.Task) == "" {
		return "", fmt.Errorf("design_router requires a 'task' argument")
	}
	return routeDesignTask(t.store, p.Task), nil
}

func (*designRouterTool) CompactDescription() string {
	return "路由设计任务到对应子代理技能（banner/slides/brand/design-system/taste-skill/ui-ux-pro-max/ui_styling）"
}

func (*designRouterTool) CompactSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"task":{"type":"string"}},"required":["task"]}`)
}

func hasAny(lower string, kws ...string) bool {
	for _, k := range kws {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

func routeDesignTask(store *Store, task string) string {
	lower := strings.ToLower(task)
	type route struct {
		subagent string
		hint     string
	}
	routeJSON := func(r route) string {
		b, _ := json.Marshal(map[string]string{"subagent": r.subagent, "task": task, "hint": r.hint})
		return string(b)
	}
	switch {
	case hasAny(lower, "banner", "横幅", "封面图", "广告图", "社媒图", "social media"):
		return routeJSON(route{"banner-design", "派发 banner-design 子代理产出首稿;完成后按需迭代风格"})
	case hasAny(lower, "slide", "presentation", "演示", "幻灯片", "ppt", "宣讲", "deck"):
		return routeJSON(route{"slides", "派发 slides 子代理生成自包含 HTML 演示文件"})
	case hasAny(lower, "brand", "品牌", "商标", "slogan", "口号", "品牌一致性"):
		return routeJSON(route{"brand", "派发 brand 子代理生成品牌指南/一致性审查"})
	case hasAny(lower, "token", "design token", "组件规范", "设计令牌", "design system"):
		return routeJSON(route{"design-system", "派发 design-system 子代理产出 token/组件规格"})
	case hasAny(lower, "logo", "标志", "icon", "图标", "favicon"):
		return `{"subagent":"design","task":"` + task + `","hint":"design 技能内含 logo/icon 生成脚本(.tianxuan/skills/design/scripts/logo|icon),按 SKILL.md 的脚本调用方式执行"}`
	case hasAny(lower, "tailwind", "shadcn", "样式实现", "css"):
		return routeJSON(route{"ui_styling", "调用 ui_styling 工具(action=config|guide)生成配置或读取组件指南"})
	case hasAny(lower, "landing", "portfolio", "作品集", "审美", "品味", "anti-slop", "反模板", "模板风", "千篇一律", "设计风格", "界面风格", "redesign", "改版"):
		return routeJSON(route{"taste-skill", "派发 taste-skill 子代理做审美判断：先读需求输出 Design Read，再定三拨盘与设计系统，返回反 AI Tells 的验收清单"})
	case hasAny(lower, "ui", "界面", "页面", "配色", "色板", "字体", "typography", "ux", "组件", "响应式"):
		return routeJSON(route{"ui-ux-pro-max", "派发 ui-ux-pro-max 检索子代理,返回综合设计推荐包"})
	default:
		return routeJSON(route{"explore", "先用 explore 子代理调查现状,再结合设计技能决策"})
	}
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
	return "Invoke a playbook from the Skills index. Use bare name (e.g. 'tdd'), not the [🧬 subagent] tag. 任务匹配技能描述时必须调用——技能承载提示词之外的详细工作流（tdd / systematic-debugging / requesting-code-review / finish-development-branch），跳过会遗漏关键步骤。Subagent skills spawn an isolated loop — only final answer returns; supply arguments as task. Inline skills fold the body as tool result. 领域工作流一律通过 run_skill 获取完整 playbook，不要用通用工具自行拼凑。"
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

func (*runSkillTool) CompactDescription() string {
	return "Invoke a skill by name. Subagent skills spawn isolated loop; inline skills fold body as tool result."
}
func (*runSkillTool) CompactSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"arguments":{"type":"string"}},"required":["name"]}`)
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
	return "Author and save a new skill — reusable playbook invoked via run_skill or /<name>. Runnable immediately; appears in Skills index on next launch. " + scope
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

func (*installSkillTool) CompactDescription() string {
	return "Create a new skill from name + description + markdown body. Saves to project or global skills directory."
}
func (*installSkillTool) CompactSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"description":{"type":"string"},"body":{"type":"string"},"scope":{"type":"string","enum":["project","global"]},"runAs":{"type":"string","enum":["inline","subagent","pipeline"]}},"required":["name","description","body"]}`)
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
	return "并行派发多个子代理技能同时执行，完成后汇总结果。适用于 2+ 个独立任务（如并行探索多模块）。有依赖时请分次调用 run_skill。"
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

func (*parallelSkillsTool) CompactDescription() string {
	return "Run multiple skills in parallel or DAG order, collect results."
}
func (*parallelSkillsTool) CompactSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{"skill":{"type":"string"},"arguments":{"type":"string"},"id":{"type":"string"},"depends_on":{"type":"array","items":{"type":"string"}}},"required":["skill","arguments"]}}},"required":["tasks"]}`)
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
