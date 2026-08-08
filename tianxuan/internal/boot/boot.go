// Package boot assembles a ready-to-drive control.Controller from configuration:
// it loads config, resolves the model(s), builds the tool registry (built-ins +
// plugins), wires the permission gate, and constructs the executor — optionally
// wrapping it in the single-model PlannerHost. It is the one place that turns "what the
// user configured" into "a Controller a frontend can drive", so every frontend —
// the terminal TUI, the HTTP/SSE server, the desktop webview — shares the exact
// same assembly instead of each re-deriving it. Frontends pass only a sink and a
// couple of run knobs; everything else comes from config.
package boot

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sync"
	"tianxuan/internal/agent"
	"tianxuan/internal/archive"
	"tianxuan/internal/cache"
	"tianxuan/internal/command"
	"tianxuan/internal/config"
	tiancontext "tianxuan/internal/context"
	"tianxuan/internal/control"
	"tianxuan/internal/event"
	"tianxuan/internal/hook"
	"tianxuan/internal/jobs"
	"tianxuan/internal/learning"
	"tianxuan/internal/lsp"
	"tianxuan/internal/memory"
	"tianxuan/internal/permission"
	"tianxuan/internal/plugin"
	"tianxuan/internal/provider"
	"tianxuan/internal/provider/failover"
	"tianxuan/internal/sandbox"
	"tianxuan/internal/skill"
	"tianxuan/internal/tool"
	"tianxuan/internal/tool/builtin"
)

// Options carries the per-run knobs a frontend chooses; everything else is read
// from configuration. Model "" falls back to the configured default_model;
// MaxSteps 0 uses the config/default. RequireKey forces the executor's API key to
// be present (run/serve pass true so a missing key fails fast; chat/desktop pass
// false so the UI is reachable before a key is set). Sink receives the agent's
// typed event stream.
type Options struct {
	Model      string
	MaxSteps   int
	RequireKey bool
	Sink       event.Sink
	// Stderr is the writer for diagnostic warnings and plugin subprocess
	// stderr output. When nil, defaults to os.Stderr. Set to io.Discard
	// during model switch inside a bubbletea session to prevent any output
	// from corrupting the TUI's terminal raw mode.
	Stderr io.Writer
	// SessionDir overrides the global session directory. When empty,
	// config.SessionDir() (user-global) is used. Desktop mode sets this to
	// config.WorkspaceSessionDir(cwd) so sessions stay with the project.
	SessionDir string
}

// Build loads config, resolves the model(s), and returns a Controller wrapping a
// single Agent, or a PlannerHost when agent.auto_plan is set. The
// returned controller owns plugin subprocesses; call Close (via Controller.Close)
// to release them.
var (
	sandboxWarnOnce sync.Once
	bashWarnOnce    sync.Once
)

func Build(ctx context.Context, opts Options) (*control.Controller, error) {
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	modelName := opts.Model
	if modelName == "" {
		modelName = cfg.DefaultModel
	}
	entry, ok := cfg.ResolveModel(modelName)
	if !ok {
		return nil, fmt.Errorf("unknown model %q (configured: %s)", modelName, providerNames(cfg))
	}
	// Providers without explicit context_window (e.g. user's "deepseek" with
	// per-model pricing) default to 0, which hides the status-bar gauge.
	if entry.ContextWindow == 0 {
		entry.ContextWindow = 1_000_000
	}
	if opts.RequireKey {
		if err := cfg.Validate(modelName); err != nil {
			return nil, err
		}
	}

	// Serialize the frontend's sink once: background jobs (below) emit from their
	// own goroutines, which can overlap a running turn's emission, so every emitter
	// shares this synchronized sink. The job manager is session-scoped — its jobs
	// outlive a turn and are cancelled by Controller.Close.
	sink := event.Sync(opts.Sink)
	jm := jobs.NewManager(sink)

	execProv, err := NewProvider(entry)
	if cfg.Agent.Effort != "" {
		entry.Effort = cfg.Agent.Effort
	}
	if err != nil {
		return nil, err
	}

	// V10.22: system prompt + memory + skills assembled in sysprompt.go
	sp, err := buildSystemPrompt(cfg, opts.Stderr)
	if err != nil {
		return nil, err
	}
	sysPrompt := sp.prompt
	mem := sp.mem
	skills := sp.skills
	compiler := sp.compiler
	runtimeCtx := sp.runtimeCtx
	skillStore := sp.store

	// V10.96: 语义记忆自动回想 — 蒸馏自 jcode 语义记忆系统。
	// 注入 FTS5 BM25 搜索函数，Agent 在每轮开始时自动检索相关记忆。
	if mem.Search != nil {
		agent.MemorySearchFunc = func(query string, limit int) []agent.MemoryResult {
			matches := mem.Search.Search(query)
			if len(matches) > limit {
				matches = matches[:limit]
			}
			// 强化召回：命中即记一次访问，供 TTL 衰减/画像统计（agentmemory 蒸馏）。
			for _, m := range matches {
				if !strings.HasPrefix(m.Name, "doc:") {
					_ = memory.ReinforceAccess(mem.Store, m.Name)
				}
			}
			results := make([]agent.MemoryResult, len(matches))
			for i, m := range matches {
				results[i] = agent.MemoryResult{Name: m.Name, Preview: m.Preview, Body: m.Body, Mtime: m.Mtime}
			}
			return results
		}
	}

	cwd, _ := os.Getwd()
	reg := tool.NewRegistry()
	bashSpec := sandbox.Spec{Mode: cfg.BashMode(), WriteRoots: cfg.WriteRoots(), Network: cfg.Sandbox.Network}
	if bashSpec.Mode == "enforce" && !sandbox.Available() {
		sandboxWarnOnce.Do(func() {
			fmt.Fprintln(stderr, "warning: bash sandbox requested but unavailable on this platform; running bash unconfined")
		})
	}
	if sandbox.ResolveShell().Kind == sandbox.ShellPowerShell {
		bashWarnOnce.Do(func() {
			fmt.Fprintln(stderr, "warning: bash not found on PATH; the shell tool will run commands under Windows PowerShell. Install Git for Windows or WSL to use bash.")
		})
	}
	addBuiltins(reg, cfg.Tools.Enabled, cfg.WriteRoots(), bashSpec, time.Duration(cfg.BashTimeoutSeconds())*time.Second, bashPathEnv(cwd), stderr)
	builtin.ResolveRgPath() // V10.29: enable ripgrep delegation when rg is on PATH
	// Always construct a host, even with no plugins configured, so the controller's
	// host pointer is stable for the session and `/mcp add` can hot-add into it.
	// V10.22: plugins + LSP assembled in plugins.go
	po := startPlugins(ctx, cfg, reg, sink, opts.Stderr)
	pluginHost := po.host
	lspMgr := po.lspMgr
	cleanup := po.cleanup

	// V10.59: 移除 GitNexus MCP 工具 — 其代码图能力已被 mcp__codegraph__* 完全覆盖，
	// 保留 13 个冗余工具只会浪费执行者和子代理的 schema token 预算。
	reg.RemovePrefix("mcp__gitnexus__")

	maxSteps := cfg.Agent.MaxSteps
	if opts.MaxSteps > 0 {
		maxSteps = opts.MaxSteps
	}

	// Permission policy gates every tool call. The headless gate (no Approver)
	// resolves "ask" to allow — preserving `tianxuan run` autonomy — while deny
	// rules hard-block in every mode. Interactive frontends (chat, desktop) swap
	// in an interactive gate later via Controller.EnableInteractiveApproval.
	// Sub-agents always run headless: they have no UI to answer a prompt, so they
	// inherit this same gate.
	policy := permission.New(cfg.Permissions.Mode, cfg.Permissions.Allow, cfg.Permissions.Ask, cfg.Permissions.Deny)
	headlessGate := permission.NewGate(policy, nil)

	// Hooks: load the global settings.json plus the project's (only when trusted —
	// project hooks run arbitrary shell commands, so cloning a repo must not
	// silently execute them). Non-blocking hook output is surfaced to the user as
	// a Notice through the shared sink. The runner fires PreToolUse/PostToolUse in
	// the agent loop and UserPromptSubmit/Stop at the controller's turn boundary.
	hooksTrusted := hook.IsTrusted(cwd, "")
	hookRunner := hook.NewRunner(
		hook.Load(hook.LoadOptions{ProjectRoot: cwd, Trusted: hooksTrusted}),
		cwd, nil,
		func(msg string) { sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: msg}) },
	)
	if hook.ProjectDefinesHooks(cwd) && !hooksTrusted {
		sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
			Text: "this project defines hooks but they are not trusted — run /hooks trust to enable them"})
	}

	// The `task` tool spawns sub-agents that reuse the parent's provider and
	// tool registry. Wired here after the built-ins / plugins are loaded so
	// sub-agents inherit the full tool set (minus `task` itself, to keep
	// nesting out of the picture). It registers into the same reg the
	// executor uses, so the model surfaces it like any other tool.
	taskTool := agent.NewTaskTool(execProv, entry.Price, reg, maxSteps,
		entry.ContextWindow, cfg.Agent.SubagentTemp(), config.ArchiveDir(), "", headlessGate)

	// V10.22: resolve subagent model from config. When SubagentModel names a
	// configured provider, sub-agents use that provider (typically a cheaper
	// flash model) while the parent keeps its own provider — independent API
	// calls mean the parent's prefix cache is unaffected.
	if subRef := strings.TrimSpace(cfg.Agent.SubagentModel); subRef != "" {
		if subEntry, ok := cfg.ResolveModel(subRef); ok {
			if e := cfg.Agent.SubagentEffortVal(); e != "" {
				subEntry.Effort = e
			}
			if subProv, err := NewProvider(subEntry); err == nil {
				taskTool.SetSubagentProvider(subProv, subEntry.Price, subEntry.ContextWindow)
			}
		}
	}
	reg.Add(taskTool)

	// parallel_tasks: dispatches multiple independent sub-agent tasks concurrently.
	parallelTasksTool := agent.NewParallelTasksTool(execProv, entry.Price, reg, maxSteps,
		entry.ContextWindow, cfg.Agent.SubagentTemp(), config.ArchiveDir(), "", headlessGate)
	reg.Add(parallelTasksTool)

	// The `remember` tool lets the model persist durable facts to the project's
	// auto-memory store; `forget` prunes ones that turn out wrong. The saved index
	// loads into the prefix on the next session.
	reg.Add(memory.NewRememberTool(mem.Store))
	reg.Add(memory.NewForgetTool(mem.Store))
	reg.Add(memory.NewPromoteSessionFactsTool())

	// The `ask` tool puts structured multiple-choice questions to the user. It
	// reaches them through the Asker on the call context, which interactive
	// frontends wire to the controller (EnableInteractiveApproval); a headless run
	// has none, so ask resolves to "decide for yourself".
	reg.Add(agent.NewAskTool())

	// Skill tools: run_skill / install_skill plus the dedicated subagent wrappers
	// (explore / research / review / security_review). A subagent skill reuses the
	// sub-agent machinery via this runner — an isolated loop with the skill body
	// as system prompt, a tool set scoped to the skill's allowed-tools (minus the
	// task/skill meta-tools, to bar recursion), and an optional per-skill model.
	// Its tool activity nests under the invoking call, like `task`.
	skillRunner := func(sctx context.Context, sk skill.Skill, task string) (string, error) {
		prov, price, ctxWin := execProv, entry.Price, entry.ContextWindow
		if modelRef := subagentModelRef(cfg, sk); modelRef != "" {
			if me, ok := cfg.ResolveModel(modelRef); ok {
				if p, err := NewProvider(me); err == nil {
					prov, price, ctxWin = p, me.Price, me.ContextWindow
				}
			}
		}
		subReg := agent.FilterRegistry(reg, sk.AllowedTools, agent.SubagentMetaTools()...)
		steps := maxSteps
		if steps > 0 {
			if steps /= 2; steps < 5 {
				steps = 5
			}
		} else {
			// 与 TaskTool 一致：0 = 默认上限，防止子代理技能无限循环堵塞主代理。
			steps = agent.DefaultSubagentMaxSteps
		}
		// V5.25: 构建与父代理一致的 [L1][L2] 双 system 消息结构。
		// L1 来自 Fork 后的 compiler，L2 通过 opts.RuntimePrompt 注入。
		// skill body 放在 user task 前面，不混入 system 消息。
		childCompiler := compiler.Fork()
		sysPrompt := childCompiler.SystemPrompt()

		return agent.RunSubAgent(sctx, prov, subReg, sysPrompt, sk.Body+"\n\n"+task, agent.Options{
			MaxSteps:      steps,
			Temperature:   cfg.Agent.Temperature,
			Pricing:       price,
			Gate:          headlessGate,
			ContextWindow: ctxWin,
			Compaction:    agent.CompactionConfig{ArchiveDir: config.ArchiveDir()},
			RuntimePrompt: runtimeCtx.SystemPrompt(),
			// V5.30: 根据技能名查找子代理模板 — 同类子代理共享前缀缓存
			TemplatePrefix: lookupSubagentTemplatePrefix(sk.Name),
			// V10.36: 对齐父代理工具集以保证缓存命中
			ActiveSchemas: reg.Schemas(),
		}, agent.NestedSink(sctx, event.Discard), nil)
	}
	reg.Add(skill.NewRunSkillTool(skillStore, skillRunner))
	reg.Add(skill.NewParallelSkillsTool(skillStore, skillRunner))
	reg.Add(skill.NewInstallSkillTool(skillStore, nil))
	reg.Add(skill.NewUiStylingTool(skillStore))
	reg.Add(skill.NewDesignRouterTool(skillStore))
	// V10.x: 所有 runAs=subagent 技能注册为独立工具——内置 explore/research/
	// review/security-review 以及用户技能通过 frontmatter 声明 runas: subagent
	// （如 ui-ux-pro-max/slides/banner-design/brand/design-system）。DeepSeek
	// 原生 tool-calling 直接看到专用入口，无需先想起技能名再走 run_skill。
	for _, sk := range skillStore.List() {
		if sk.RunAs != skill.RunSubagent {
			continue
		}
		desc, compact := sk.Description, "在隔离子代理中执行 "+sk.Name+" 技能，返回精炼结果"
		if d, ok := subagentSkillToolDescsBySkill[sk.Name]; ok {
			desc, compact = d.desc, d.compactDesc
		}
		reg.Add(skill.NewSubagentSkillTool(skillStore, skillRunner, sk.Name, desc, compact))
	}
	// V5.30: 注册内置子代理模板，同类子代理共享 L4 前缀缓存
	for _, st := range cache.BuiltinSpawnTemplates() {
		cache.RegisterSpawnTemplate(st)
	}

	compiler.SetRegistry(reg)

	// Wire the task tool into the compiler so sub-agents inherit the parent's
	// Identity+Context domains via Fork — DeepSeek serves the shared prefix
	// from its server-side cache at near-zero token cost.
	taskTool.SetCompiler(&taskCompilerAdapter{c: compiler})
	// V5.25: 注入 L2 运行时上下文，子代理共享父代理的项目/工作区/目标
	taskTool.SetRuntimePrompt(runtimeCtx.SystemPrompt())
	parallelTasksTool.SetCompiler(&taskCompilerAdapter{c: compiler})
	parallelTasksTool.SetRuntimePrompt(runtimeCtx.SystemPrompt())

	// V2.4: centralised ToolDispatcher for pre-execution checks.
	toolDispatcher := agent.NewToolDispatcher(headlessGate, hookRunner)

	// V6.0 P8: compact toolset — hide redundant tools from model schema
	if cfg.Tools.Compact {
		applyCompactToolset(reg)
	}

	// V10.166: 单模型统一提示词——默认自适应执行，规划模式（PlannerHost）
	// 通过 planmode.Marker 在同一 session 内引导规划阶段，提示词全程不变。
	execPrompt := agent.SoloSystemPrompt
	execSess := agent.NewSession(compiler.WithInstructions(execPrompt))
	offloadDir := cfg.Agent.OffloadDir
	if offloadDir != "" && !filepath.IsAbs(offloadDir) {
		offloadDir = filepath.Join(cwd, offloadDir)
	}
	// V10.154: cross-session per-tool error stats (distilled from codex CLI's
	// ToolDispatchTrace) so the host can measure which tool/error dominates.
	toolStats := tool.NewStats(tool.DefaultStatsPath(cwd))
	executor := agent.New(execProv, reg, execSess, agent.Options{
		MaxSteps:              maxSteps,
		Temperature:           cfg.Agent.Temperature,
		Pricing:               entry.Price,
		Gate:                  headlessGate,
		Hooks:                 hookRunner,
		Jobs:                  jm,
		ContextWindow:         entry.ContextWindow,
		Compaction:            agent.CompactionConfig{ArchiveDir: config.ArchiveDir()},
		Dispatcher:            toolDispatcher,
		StrictEvidence:        false,
		OffloadDir:            offloadDir,
		OffloadThresholdChars: cfg.Agent.OffloadThresholdChars,
		ToolStats:             toolStats,
	}, sink)
	// V10.122: 技能自动触发 — executor/solo 收到输入时按确定性规则注入
	// 匹配技能的 playbook（tdd/systematic-debugging 等）。规划轮
	// 走 plannerMode 不注入；subagent 技能已工具化，也不注入正文。
	executor.SetAutoSkillStore(skillStore)

	// V7.0: session archive for cross-session Dream/Distill. The session ID is
	// derived here and always set on the executor, because context offloading
	// (V10.111) derives its per-session subdirectory from it too.
	archiveDir := filepath.Join(cwd, ".tianxuan", "archive")
	sid := filepath.Base(orDefault(opts.SessionDir, config.SessionDir()))
	if sid == "" || sid == "." {
		sid = fmt.Sprintf("session-%d", time.Now().Unix())
	}
	if ar, err := archive.Open(archiveDir); err == nil && ar != nil {
		executor.SetArchive(ar, sid)
	} else {
		executor.SetArchive(nil, sid)
	}
	// V10.111: expose the offload store to search_large_output so the model can
	// read/search offloaded tool outputs on demand.
	if store := executor.OffloadStore(); store != nil {
		builtin.WireSearchLargeOutputStore(store)
	}
	// V10.111: delete offloaded files at session end (they are per-session
	// scratch — the model reads them via search_large_output during the turn,
	// after which they have no value and would leak disk).
	if executor.OffloadStore() != nil {
		prev := cleanup
		cleanup = func() {
			prev()
			executor.CloseOffload()
		}
	}

	// Custom slash commands (.tianxuan/commands + user dir). Best-effort: a malformed
	// file is skipped, and a load error never blocks the session.
	cmds, _ := command.Load(config.CommandDirs()...)

	// Expose the loaded slash commands (skills + custom commands) to the model via
	// the slash_command tool, so it can invoke a project playbook by name the way a
	// user types "/name". Skills are added first, then commands, so a command wins
	// a name clash — matching the prompt's command-over-skill precedence.
	var slashEntries []command.SlashEntry
	for _, sk := range skills {
		sk := sk
		slashEntries = append(slashEntries, command.SlashEntry{
			Name:        sk.Name,
			Description: sk.Description,
			Render:      func(args []string) string { return skill.Render(sk, strings.Join(args, " ")) },
		})
	}
	if lspMgr != nil {
		executor.SetLSPManager(lspMgr)
	}

	// V7.4: cross-session error pattern learning
	if patPath, err := resolvePatternsPath(); err == nil {
		if patternStore, err2 := learning.LoadStore(patPath); err2 == nil {
			patternExtractor := learning.NewExtractor(patPath)
			executor.SetPatternExtractor(patternExtractor)
			if active := learning.ActivePatterns(patternStore, 3); len(active) > 0 {
				sysPrompt += "\n\n" + learning.FormatGuide(active)
			}
		}
	}

	for _, cmd := range cmds {
		cmd := cmd
		slashEntries = append(slashEntries, command.SlashEntry{
			Name:        cmd.Name,
			Description: cmd.Description,
			ArgHint:     cmd.ArgHint,
			Render:      func(args []string) string { return cmd.Render(args) },
		})
	}
	reg.Add(command.NewSlashCommandTool(slashEntries))

	// V10.32: use provider name as label so users can distinguish models from
	// different providers (e.g. "flash" vs "pro") even when they share the same
	// underlying model name (e.g. both "deepseek-chat").
	label := entry.Name

	// V10.166: 单模型规划模式——auto_plan 非 off 时用 PlannerHost 包装
	// executor：同一模型在同一 session 内先规划（只读门控）再执行。
	var runner agent.Runner = executor
	if cfg.Agent.AutoPlan != "" && cfg.Agent.AutoPlan != "off" {
		runner = agent.NewPlannerHost(executor, cfg.Agent.AutoPlan, sink)
		label = entry.Name + " · 规划模式"
	}

	skillLayer := cache.NewSkillLayer()

	// V3.0 Phase 5: ContextManager wraps the four-layer cache kernel.
	ctxMgr := tiancontext.NewContextManager(
		compiler.IdentityLayer(),
		runtimeCtx,
		skillLayer,
		tiancontext.NewFlowLayer(tiancontext.CompactPolicy{
			Window:     entry.ContextWindow,
			TailTokens: 16384,
		}),
	)

	// Wire ContextManager into AgentRunner and ToolDispatcher.
	executor.SetCtxMgr(ctxMgr)

	// V3.4: cache warmup — save L1 hash and check cross-session validity
	cacheDir := filepath.Join(cwd, ".tianxuan", "cache")
	if warm := compiler.IdentityLayer().LoadAndCompareHash(cacheDir); warm {
		sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
			Text: "cache warm: L1 identity matches previous session"})
	}
	compiler.IdentityLayer().SaveHash(cacheDir) // best-effort

	ctrlOpts := control.Options{
		Runner:        runner,
		Executor:      executor,
		Sink:          sink,
		Policy:        policy,
		Label:         label,
		SystemPrompt:  sysPrompt,
		SessionDir:    orDefault(opts.SessionDir, config.SessionDir()),
		Host:          pluginHost,
		Commands:      cmds,
		Skills:        skills,
		Hooks:         hookRunner,
		Memory:        mem,
		Cleanup:       cleanup,
		BalanceURL:    entry.BalanceURL,
		BalanceKey:    entry.APIKey(),
		Jobs:          jm,
		Registry:      reg,
		PluginCtx:     ctx,
		CtxMgr:        ctxMgr,
		WorkspaceRoot: cwd,
	}
	ctrl := control.New(ctrlOpts)
	// V10.156: inherit_context (Qwen /fork semantics) — the task tool forks
	// the parent conversation snapshot so a background/parallel sub-agent can
	// start with the session's context instead of a blank slate.
	taskTool.SetForkContext(func() string { return forkContextOf(ctrl) })
	return ctrl, nil
}

// forkContextOf renders a compact snapshot of the parent conversation for the
// task tool's inherit_context option: the last user input and the turns that
// follow, truncated to a fixed budget. Read-only — it never mutates the parent
// session, so the prefix cache is untouched.
func forkContextOf(c *control.Controller) string {
	ex := c.Executor()
	if ex == nil {
		return ""
	}
	s := ex.Session()
	if s == nil {
		return ""
	}
	return forkContextText(s.Snapshot())
}

// forkContextText renders a compact snapshot of a conversation for
// inherit_context: the last user input and the turns that follow, truncated to
// a fixed budget. Pure so the extraction is unit-tested without a controller.
func forkContextText(msgs []provider.Message) string {
	start := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleUser {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	const budget = 4000
	// Reserve a floor for the turns after the user input so a very long user
	// message cannot starve the follow-up context (assistant/tool turns).
	const followBudget = 800
	var follows []string
	followUsed := 0
	for i := start + 1; i < len(msgs) && followUsed < followBudget; i++ {
		text := strings.TrimSpace(msgs[i].Content)
		if text == "" {
			continue
		}
		if len(text) > followBudget-followUsed {
			text = text[:followBudget-followUsed]
		}
		follows = append(follows, renderForkRole(msgs[i].Role)+": "+text)
		followUsed += len(text) + 3
	}
	userText := strings.TrimSpace(msgs[start].Content)
	if len(userText) > budget-followUsed {
		userText = userText[:budget-followUsed]
	}
	var b strings.Builder
	b.WriteString("user: " + userText + "\n")
	for _, f := range follows {
		b.WriteString(f + "\n")
	}
	return strings.TrimSpace(b.String())
}

// renderForkRole maps a message role to its label in the fork snapshot.
func renderForkRole(r provider.Role) string {
	switch r {
	case provider.RoleAssistant:
		return "assistant"
	case provider.RoleTool:
		return "tool"
	default:
		return "user"
	}
}

func subagentModelRef(cfg *config.Config, sk skill.Skill) string {
	if cfg != nil {
		for _, key := range subagentModelKeys(sk.Name) {
			if m := strings.TrimSpace(cfg.Agent.SubagentModels[key]); m != "" {
				return m
			}
		}
	}
	if m := strings.TrimSpace(sk.Model); m != "" {
		return m
	}
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Agent.SubagentModel)
}

func subagentModelKeys(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	keys := []string{name}
	for _, alias := range []string{
		strings.ReplaceAll(name, "-", "_"),
		strings.ReplaceAll(name, "_", "-"),
	} {
		if alias == "" {
			continue
		}
		seen := false
		for _, key := range keys {
			if key == alias {
				seen = true
				break
			}
		}
		if !seen {
			keys = append(keys, alias)
		}
	}
	return keys
}

// NewProvider builds a provider.Provider from a configured entry. Exported so
// custom assemblers (e.g. the ACP per-session factory) can reuse it without
// going through the full Build. When the entry declares Fallbacks, the
// returned provider is a turn-local failover chain (distilled from OpenClaw
// model-failover): the primary model answers normally; on failover-worthy
// errors (rate limit/overload/transport) the chain tries each fallback model,
// and the next turn starts from the primary again to keep its prompt-cache
// prefix stable.
func NewProvider(e *config.ProviderEntry) (provider.Provider, error) {
	primary, err := buildProvider(e, e.Model)
	if err != nil {
		return nil, err
	}
	if len(e.Fallbacks) == 0 {
		return primary, nil
	}
	var fallbacks []provider.Provider
	for _, model := range e.Fallbacks {
		if model == "" || model == e.Model {
			continue
		}
		fb, err := buildProvider(e, model)
		if err != nil {
			return nil, fmt.Errorf("boot: fallback model %q: %w", model, err)
		}
		fallbacks = append(fallbacks, &namedProvider{Provider: fb, name: e.Name + "/" + model})
	}
	if len(fallbacks) == 0 {
		return primary, nil
	}
	return failover.New(primary, fallbacks, failover.Options{}), nil
}

// buildProvider instantiates one model of a provider entry.
func buildProvider(e *config.ProviderEntry, model string) (provider.Provider, error) {
	entry := *e
	return provider.New(entry.Kind, provider.Config{
		Name:    entry.Name,
		BaseURL: entry.BaseURL,
		Model:   model,
		APIKey:  entry.APIKey(),
		// Pass the key's env var so auth failures can name where to fix it, plus
		// provider-kind-specific knobs (the anthropic provider reads thinking/effort;
		// the openai one ignores them).
		Extra: map[string]any{
			"api_key_env": entry.APIKeyEnv,
			"thinking":    entry.Thinking,
			"effort":      entry.Effort,
		},
	})
}

// namedProvider overrides the display name of a wrapped provider so fallback
// candidates surface as "instance/model" in switch notices and diagnostics.
type namedProvider struct {
	provider.Provider
	name string
}

// Name returns the override label.
func (p *namedProvider) Name() string { return p.name }

// addBuiltins adds enabled built-in tools to reg. An empty list means all of
// them. writeRoots confines the file-writing built-ins to the workspace: after
// the (unconfined) defaults are added, each enabled writer is replaced by an
// instance bound to writeRoots (preserving registry order). bashTimeout is the
// host-injected foreground cap (config tools.bash_timeout_seconds; 0 = no cap).
func addBuiltins(reg *tool.Registry, enabled, writeRoots []string, bashSpec sandbox.Spec, bashTimeout time.Duration, bashEnv []string, stderr io.Writer) {
	if len(enabled) == 0 {
		for _, t := range tool.Builtins() {
			reg.Add(t)
		}
	} else {
		for _, name := range enabled {
			if t, ok := tool.LookupBuiltin(name); ok {
				reg.Add(t)
			} else {
				fmt.Fprintf(stderr, "warning: unknown built-in tool %q\n", name)
			}
		}
	}
	// Replace the unconfined defaults with confined instances (registry order is
	// preserved on replace): file-writers bound to the workspace, bash to the OS
	// sandbox. Only replace tools actually enabled/present.
	confined := append(builtin.ConfineWriters(writeRoots), builtin.ConfineBash(bashSpec,
		builtin.WithBashTimeout(bashTimeout), builtin.WithBashEnv(bashEnv)))
	for _, t := range confined {
		if _, ok := reg.Get(t.Name()); ok {
			reg.Add(t)
		}
	}
}

// bashPathEnv builds an augmented PATH for the bash tool: the project's
// bundled tools (tools/go/bin, tools/node) plus common Windows install dirs
// (Go, Node, Git). Without it the model wastes rounds probing for go/node
// locations in every fresh session. Returns nil when nothing to inject (bash
// then inherits the process environment unchanged).
func bashPathEnv(cwd string) []string {
	var dirs []string
	add := func(p string) {
		if p == "" {
			return
		}
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			dirs = append(dirs, p)
		}
	}
	for _, rel := range []string{"tools/go/bin", "tools/node"} {
		add(filepath.Join(cwd, rel))
	}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		root := os.Getenv(env)
		if root == "" {
			continue
		}
		for _, rel := range []string{"Go/bin", "nodejs", "Git/cmd", "Git/usr/bin"} {
			add(filepath.Join(root, rel))
		}
	}
	if len(dirs) == 0 {
		return nil
	}
	cur := os.Getenv("PATH")
	merged := append(append([]string{}, dirs...), cur)
	return []string{"PATH=" + strings.Join(merged, string(os.PathListSeparator))}
}

// PluginSpecs maps configured plugin entries to plugin.Spec, expanding ${VAR}
// references. Exported so custom assemblers can connect the config's plugins
// alongside their own (e.g. ACP's per-session MCP servers).
func PluginSpecs(entries []config.PluginEntry) []plugin.Spec {
	specs := make([]plugin.Spec, len(entries))
	for i, e := range entries {
		e = e.ExpandedPlugin() // resolve ${VAR} / ${VAR:-default} from the environment
		specs[i] = plugin.Spec{
			Name:    e.Name,
			Type:    e.Type,
			Command: e.Command,
			Args:    e.Args,
			Env:     e.Env,
			URL:     e.URL,
			Headers: e.Headers,
		}
	}
	return specs
}

// MCPStartupNotice formats the warning shown when configured MCP servers failed
// to connect, naming the first few; ok is false when none failed.
func MCPStartupNotice(failures []plugin.Failure) (text string, ok bool) {
	if len(failures) == 0 {
		return "", false
	}
	names := make([]string, 0, min(len(failures), 3))
	for i, f := range failures {
		if i >= 3 {
			break
		}
		names = append(names, f.Name)
	}
	more := ""
	if len(failures) > len(names) {
		more = fmt.Sprintf(" (+%d more)", len(failures)-len(names))
	}
	return fmt.Sprintf("%d MCP server(s) failed to start: %s%s — run /mcp for details",
		len(failures), strings.Join(names, ", "), more), true
}

// LSPSpecs returns the language → server map: the built-in defaults overlaid with
// any user overrides. A user entry may set only the fields it wants to change;
// empty fields keep the default for that language.
func LSPSpecs(cfg config.LSPConfig) map[string]lsp.ServerSpec {
	specs := lsp.DefaultSpecs()
	for lang, s := range cfg.Servers {
		spec := specs[lang]
		if s.Command != "" {
			spec.Command = s.Command
		}
		if s.Args != nil {
			spec.Args = s.Args
		}
		if s.Env != nil {
			spec.Env = s.Env
		}
		if s.LanguageID != "" {
			spec.LanguageID = s.LanguageID
		}
		if s.Extensions != nil {
			spec.Extensions = s.Extensions
		}
		if s.InstallHint != "" {
			spec.InstallHint = s.InstallHint
		}
		if spec.LanguageID == "" {
			spec.LanguageID = lang
		}
		specs[lang] = spec
	}
	return specs
}

func providerNames(cfg *config.Config) string {
	names := make([]string, len(cfg.Providers))
	for i, p := range cfg.Providers {
		names[i] = p.Name
	}
	return strings.Join(names, "/")
}

// taskCompilerAdapter wraps *cache.Compiler to satisfy agent.TaskCompiler,
// bridging the return-type mismatch between Fork() *cache.Compiler (concrete)
// and Fork() interface{SystemPrompt() string} (interface).
type taskCompilerAdapter struct {
	c *cache.Compiler
}

func (a *taskCompilerAdapter) Fork() interface{ SystemPrompt() string } { return a.c.Fork() }
func (a *taskCompilerAdapter) SystemPrompt() string                     { return a.c.SystemPrompt() }

func orDefault(val, def string) string {
	if val != "" {
		return val
	}
	return def
}

func subagentSkillToTemplateKind(skillName string) cache.TaskKind {
	switch skillName {
	case "explore":
		return "subagent_explore"
	case "research":
		return "subagent_research"
	case "review", "code-review":
		return "subagent_review"
	case "security-review", "security_review":
		return "subagent_security"
	default:
		return ""
	}
}

// lookupSubagentTemplatePrefix 根据技能名称查找子代理模板前缀（V5.30）。
// 同类子代理共享相同模板前缀 → DeepSeek 前缀缓存命中。
func lookupSubagentTemplatePrefix(skillName string) string {
	kind := subagentSkillToTemplateKind(skillName)
	if kind == "" {
		return ""
	}
	tmpl, ok := cache.LookupSpawnTemplate(kind)
	if !ok || tmpl.Prefix == "" {
		return ""
	}
	return tmpl.Prefix
}

// subagentSkillToolDescs holds the model-facing descriptions for the built-in
// subagent skill tools (ordered). They are first-class tools so DeepSeek's
// native tool calling sees a dedicated entry instead of routing through
// run_skill. Keys match the built-in skill identifiers (security-review uses
// the hyphen form; the model-visible tool name is normalised to underscores).
var subagentSkillToolDescs = []struct {
	name        string
	desc        string
	compactDesc string
}{
	{
		name:        "explore",
		desc:        "Explore the codebase in an isolated read-only sub-agent. 在隔离的只读子代理中深入调查代码库：架构、调用链、符号定义、影响面、跨文件综合问题；返回带 file:line 引用的精炼结论。适合宽泛调查、多处调用、理解系统结构——不要在主线上下文里反复 grep/read_file 堆砌探索。",
		compactDesc: "在隔离只读子代理中调查代码库（架构/调用链/影响面），返回带引用的精炼结论",
	},
	{
		name:        "research",
		desc:        "Research a question combining web + code in an isolated read-only sub-agent. 在隔离的只读子代理中结合网页资料与本地代码调研（web_search/web_fetch + 代码检索），返回带代码与 URL 引用的综合结论。适合“某库是否支持 X”、外部方案对比、版本兼容等调研。",
		compactDesc: "在隔离只读子代理中结合网页与代码调研问题，返回带引用的综合结论",
	},
	{
		name:        "review",
		desc:        "Review the current branch diff in an isolated read-only sub-agent. 在隔离的只读子代理中审查当前分支改动：正确性、安全、缺失测试、隐藏行为变化；按 file:line 给出问题与严重度，返回审查结论。只读，不提交不改文件。",
		compactDesc: "在隔离只读子代理中审查当前分支 diff，按严重度返回问题清单",
	},
	{
		name:        "security-review",
		desc:        "Security-focused review of the current branch diff in an isolated read-only sub-agent. 在隔离的只读子代理中做安全专项审查：注入、越权、密钥泄露、反序列化、路径穿越、加密误用；按严重度标注并给出 file:line。只读，不提交不改文件。",
		compactDesc: "在隔离只读子代理中做安全专项审查，按严重度返回风险清单",
	},
	{
		name:        "taste-skill",
		desc:        "Frontend aesthetic judgment in an isolated read-only sub-agent (distilled from Leonxlnx/taste-skill, MIT). 在隔离只读子代理中做前端审美判断与反模板验收：先读需求推断设计方向（Design Read），按 DESIGN_VARIANCE/MOTION_INTENSITY/VISUAL_DENSITY 三拨盘定视觉基调，映射真实设计系统，输出布局/排版/配色/动效指令与 AI Tells 反模式清单。适合 landing page、作品集、改版等界面设计的前置决策与评审；只读，返回建议，主代理实现。",
		compactDesc: "在隔离只读子代理中做前端审美判断（Design Read+三拨盘+设计系统映射+AI Tells 验收）",
	},
}

// subagentSkillToolDescsBySkill indexes the custom descriptions by skill name.
var subagentSkillToolDescsBySkill = func() map[string]struct{ desc, compactDesc string } {
	m := map[string]struct{ desc, compactDesc string }{}
	for _, d := range subagentSkillToolDescs {
		m[d.name] = struct{ desc, compactDesc string }{d.desc, d.compactDesc}
	}
	return m
}()

// applyCompactToolset hides redundant tools from the model's tool schema list
// while keeping them callable by name. V6.0 P8: reduces visible tool count
// from ~41 to ~25, lowering model cognitive load.
func applyCompactToolset(reg *tool.Registry) {
	// File deletion: merge delete_range + delete_symbol into edit_file
	// edit_file already supports delete via mode parameter
	reg.HideUnlessOnly([]string{"delete_range", "delete_symbol"}, []string{"edit_file"})

	// Background job management: merge kill_shell + wait into bash/bgjobs
	reg.HideUnlessOnly([]string{"kill_shell", "wait"}, []string{"bash", "bash_output"})

	// Notebook editing: rarely used, hide unless explicitly enabled
	reg.HideUnlessOnly([]string{"notebook_edit"}, []string{"edit_file", "write_file"})

	// CodeGraph deep-analysis tools: the parent only needs the high-signal
	// query tools; trace/explore/impact/callers/callees/status stay available
	// inside explore sub-agents (FilterRegistry copies tools without hiding
	// markers), so hiding them here only trims the parent's schema tokens.
	reg.HideUnlessOnly([]string{
		"mcp__codegraph__codegraph_trace",
		"mcp__codegraph__codegraph_explore",
		"mcp__codegraph__codegraph_impact",
		"mcp__codegraph__codegraph_callers",
		"mcp__codegraph__codegraph_callees",
		"mcp__codegraph__codegraph_status",
	}, []string{
		"mcp__codegraph__codegraph_context",
		"mcp__codegraph__codegraph_search",
		"mcp__codegraph__codegraph_node",
	})

	// Skill authoring is a user/CLI action, not a per-turn model capability;
	// run_skill covers invocation.
	reg.HideUnlessOnly([]string{"install_skill"}, []string{"run_skill"})

	// LSP rename/completion are lower-frequency mutations; the read-side
	// tools (definition/references/hover/diagnostics) cover investigation.
	reg.HideUnlessOnly([]string{"lsp_rename", "lsp_completion"}, []string{"lsp_definition", "lsp_references", "lsp_diagnostics"})

	// File listing: glob is redundant with ls (which supports patterns)
	reg.HideUnlessOnly([]string{"glob"}, []string{"ls"})
}

// resolvePatternsPath returns the path to the project's learned-patterns.toml,
// or an error if the .tianxuan directory doesn't exist.
func resolvePatternsPath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// Use .tianxuan/ relative to cwd
	if _, err := os.Stat(filepath.Join(cwd, ".tianxuan")); err == nil {
		return filepath.Join(cwd, learning.DefaultPatternsPath), nil
	}
	return learning.DefaultPatternsPath, nil
}

// newReadOnlyRegistry builds a tool registry containing only the read-only tools
// from the full registry. Used to give the planner AgentRunner powers to
// investigate code (read_file, grep, glob, web_search, web_fetch, lsp_*, etc.)
// without any write/destructive capability. MCP tools are included when their
// ReadOnly() returns true, except for built-in codegraph tools which are always
// included — CodeGraph is a code-intelligence engine whose graph tools are
// inherently read-only and essential for efficient planning.
// Subagent-spawning tools (task, explore, research, review, security_review,
// run_skill, parallel_skills) are excluded here regardless of ReadOnly: their
// default sub-agent toolset is the full registry, which would let a planner-
// spawned sub-agent write files through the headless gate. The dual-model
// wiring in Boot re-adds them explicitly with a read-only sub-agent registry
// (subagentReg), so planner sub-agents stay investigation-only.
func newReadOnlyRegistry(full *tool.Registry) *tool.Registry {
	ro := tool.NewRegistry()
	if full == nil {
		return ro
	}
	// Subagent-spawning tools are excluded regardless of ReadOnly (see the
	// function doc for why); Boot re-adds read-only versions explicitly.
	exclude := map[string]bool{
		"task": true, "run_skill": true, "parallel_skills": true, "parallel_tasks": true,
		"explore": true, "research": true, "review": true, "security_review": true,
	}
	for _, name := range full.Names() {
		if exclude[name] {
			continue
		}
		t, ok := full.Get(name)
		if !ok {
			continue
		}
		// CodeGraph MCP tools (mcp__codegraph__*) are always included — the
		// local code-intelligence engine is inherently read-only.
		// GitNexus and GitHub MCP tools are explicitly excluded: GitNexus is
		// redundant with CodeGraph, and GitHub tools (search_code, list_issues,
		// etc.) are irrelevant to local code investigation.
		if strings.HasPrefix(name, "mcp__gitnexus__") || strings.HasPrefix(name, "mcp__modelcontextprotocol-server-github__") {
			continue
		}
		if t.ReadOnly() || strings.HasPrefix(name, "mcp__codegraph__") {
			ro.Add(t)
		}
	}
	return ro
}
