// Package config loads Tianxuan's runtime configuration from TOML. Resolution order:
// flag > project ./tianxuan.toml > user ~/.config/tianxuan/config.toml > built-in defaults.
// Secrets come from the environment via api_key_env and are never stored in
// config files.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"tianxuan/internal/provider"
)

// Config is Tianxuan's runtime configuration.
type Config struct {
	DefaultModel string            `toml:"default_model"`
	Language     string            `toml:"language"` // ui/model language tag (e.g. "zh"); empty = auto-detect from $LANG / $TIANXUAN_LANG
	Agent        AgentConfig       `toml:"agent"`
	Providers    []ProviderEntry   `toml:"providers"`
	Tools        ToolsConfig       `toml:"tools"`
	Permissions  PermissionsConfig `toml:"permissions"`
	Sandbox      SandboxConfig     `toml:"sandbox"`
	Plugins      []PluginEntry     `toml:"plugins"`
	Skills       SkillsConfig      `toml:"skills"`
	Codegraph    CodegraphConfig   `toml:"codegraph"`
	Statusline   StatuslineConfig  `toml:"statusline"`
	Notify       NotifyConfig      `toml:"notifications"`
	LSP          LSPConfig         `toml:"lsp"`
	Search       SearchConfig      `toml:"search"`
}

// SearchConfig configures web search engines. Resolution order: local SearXNG
// (fastest, private) → Tavily API → Brave Search API → public SearXNG instances.
// Each engine requires its own credentials; only configured engines are tried.
type SearchConfig struct {
	// LocalSearXNGURL is the base URL of a self-hosted SearXNG instance
	// (e.g. "http://localhost:8080"). Empty disables it.
	LocalSearXNGURL string `toml:"local_searxng_url"`
	// TavilyAPIKeyEnv names the environment variable holding a Tavily API key
	// (free tier: 1000 searches/month). Empty disables Tavily.
	TavilyAPIKeyEnv string `toml:"tavily_api_key_env"`
	// BraveAPIKeyEnv names the environment variable holding a Brave Search API key
	// (free tier: 2000 searches/month). Empty disables Brave.
	BraveAPIKeyEnv string `toml:"brave_api_key_env"`
	// TimeoutSeconds is the per-engine HTTP timeout in seconds (default 10).
	TimeoutSeconds int `toml:"timeout_seconds"`
}

// TavilyKey resolves the Tavily API key from the configured environment variable.
func (c *SearchConfig) TavilyKey() string {
	if c.TavilyAPIKeyEnv == "" {
		return ""
	}
	return os.Getenv(c.TavilyAPIKeyEnv)
}

// BraveKey resolves the Brave Search API key.
func (c *SearchConfig) BraveKey() string {
	if c.BraveAPIKeyEnv == "" {
		return ""
	}
	return os.Getenv(c.BraveAPIKeyEnv)
}

// SearchTimeout returns the configured timeout with a safe floor of 5s.
func (c *SearchConfig) SearchTimeout() time.Duration {
	if c.TimeoutSeconds < 5 {
		return 10 * time.Second
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// LSPConfig governs the optional Language Server Protocol tools (lsp_definition,
// lsp_references, lsp_hover, lsp_diagnostics). Enabled defaults to true; the
// servers themselves are never bundled — each resolves on PATH and the tool
// returns an install hint when it is missing, so the capability is dormant until
// the user installs a server. Servers overrides or extends the built-in language
// → server map, keyed by language id (e.g. "go", "rust", "python").
type LSPConfig struct {
	Enabled bool                 `toml:"enabled"`
	Servers map[string]LSPServer `toml:"servers"`
}

// LSPServer overrides a built-in language's server or, when keyed by a new
// language, adds one. An empty field falls back to the built-in default for that
// language; Extensions is required when adding a language the built-ins don't
// cover (e.g. ".ex" for Elixir) so files route to it.
type LSPServer struct {
	Command     string            `toml:"command"`
	Args        []string          `toml:"args"`
	Env         map[string]string `toml:"env"`
	LanguageID  string            `toml:"language_id"`
	Extensions  []string          `toml:"extensions"`
	InstallHint string            `toml:"install_hint"`
}

// StatuslineConfig configures a custom status line. Command, when set, is run at
// startup and after each turn; its first line of stdout replaces the built-in
// status data row. A JSON payload (model, context tokens, cwd) is fed on stdin.
type StatuslineConfig struct {
	Command string `toml:"command"`
}

// NotifyConfig configures desktop notifications for turn completion.
// Enabled by default when the platform supports it (macOS, Linux w/ notify-send,
// Windows). Set enabled = false to suppress all notifications.
type NotifyConfig struct {
	Enabled     bool `toml:"enabled"`
	MinDuration int  `toml:"min_duration"` // seconds; minimum turn duration before a notification fires (default 5)
}

// CodegraphConfig governs the built-in CodeGraph MCP server — symbol/call-graph
// code intelligence (tree-sitter + SQLite) that gives the agent codegraph_*
// search / context / explore / trace / node tools. Enabled defaults to true; set
// enabled = false to drop those tools and fall back to grep/glob. AutoInstall
// (default true) lets tianxuan fetch the CodeGraph runtime into its cache on first
// use; set false to require an explicit `tianxuan codegraph install` (e.g. for
// air-gapped or headless runs). Path overrides binary resolution; empty resolves
// the cache, then a `codegraph` on PATH, then a bundle beside the executable.
type CodegraphConfig struct {
	Enabled     bool   `toml:"enabled"`
	AutoInstall bool   `toml:"auto_install"`
	Path        string `toml:"path"`
}

// SkillsConfig configures skill discovery. Paths adds extra "custom"-scope skill
// roots — each a directory of SKILL.md / <name>.md playbooks — scanned between
// the project roots (.tianxuan/.agents/.claude under the workspace) and the
// global roots (the same three under the home dir). ~ and relative paths and
// ${VAR} expansion are supported.
type SkillsConfig struct {
	Paths []string `toml:"paths"`
}

// SkillCustomPaths returns the configured custom skill roots with ${VAR}
// expanded; empty entries are dropped.
func (c *Config) SkillCustomPaths() []string {
	var out []string
	for _, p := range c.Skills.Paths {
		if p = ExpandVars(p); strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

// SandboxConfig bounds the blast radius of tool calls (Phase 0: file-writer
// confinement). WorkspaceRoot is the directory the built-in file writers
// (write_file / edit_file / multi_edit) may modify; empty means the current
// working directory, so writes stay inside the project by default. AllowWrite
// lists extra directories writers may also touch (e.g. a sibling repo or a temp
// dir). Both support ${VAR} / ${VAR:-default} expansion. Reads are unrestricted;
// confining `bash` is Phase 1 (OS-level sandbox).
type SandboxConfig struct {
	WorkspaceRoot string   `toml:"workspace_root"`
	AllowWrite    []string `toml:"allow_write"`
	// Bash is the OS-sandbox mode for the bash tool: "enforce" (default) jails
	// each command, "off" runs it unconfined. Phase 1; macOS only for now, with
	// a graceful fallback elsewhere (see internal/sandbox).
	Bash string `toml:"bash"`
	// Network allows network egress from inside the bash sandbox. Defaults true
	// so module/package downloads keep working; the boundary is then writes.
	Network bool `toml:"network"`
}

// WriteRoots returns the directories file-writer tools may modify: the
// workspace root (defaulting to the current working directory when unset) plus
// any AllowWrite extras, with ${VAR} expanded. The roots are returned as given
// (relative or absolute); the confiner resolves them to absolute, symlink-free
// paths. The result is always non-empty, so confinement is on by default.
func (c *Config) WriteRoots() []string {
	root := ExpandVars(c.Sandbox.WorkspaceRoot)
	if root == "" {
		if wd, err := os.Getwd(); err == nil {
			root = wd
		} else {
			root = "."
		}
	}
	roots := []string{root}
	for _, d := range c.Sandbox.AllowWrite {
		if d = ExpandVars(d); d != "" {
			roots = append(roots, d)
		}
	}
	return roots
}

// BashMode normalises the bash-sandbox mode: only an explicit "off" disables
// it; empty or any other value resolves to "enforce", so the sandbox is on by
// default and fails safe.
func (c *Config) BashMode() string {
	if c.Sandbox.Bash == "off" {
		return "off"
	}
	return "enforce"
}

// AgentConfig configures the harness loop. PlannerModel is optional: when set
// to another provider's name it enables two-model collaboration, where the
// planner handles low-frequency planning in its own session (kept separate so
// each model's prompt prefix stays cache-stable). SubagentModel is the optional
// default for runAs=subagent skills; SubagentModels overrides it per skill name.
type AgentConfig struct {
	SystemPrompt     string            `toml:"system_prompt"`
	SystemPromptFile string            `toml:"system_prompt_file"`
	MaxSteps         int               `toml:"max_steps"` // tool-call rounds per turn; 0 = unlimited
	Temperature      float64           `toml:"temperature"`
	PlannerModel     string            `toml:"planner_model"`
	SubagentModel    string            `toml:"subagent_model"`
	SubagentModels   map[string]string `toml:"subagent_models"`
	// startup (a built-in like "explanatory"/"learning"/"concise", or a custom
	// .tianxuan/output-styles/<name>.md). Empty = the unmodified prompt.
	OutputStyle string `toml:"output_style"`
	// AutoPlan controls whether interactive turns that look multi-step start in
	// plan mode automatically: "off" disables it, "ask"/"on" enable the gate.
	AutoPlan string `toml:"auto_plan"`
	// AutoPlanClassifier optionally names a provider/model used to classify
	// borderline auto-plan decisions. Empty keeps the zero-cost heuristic path.
	AutoPlanClassifier string `toml:"auto_plan_classifier"`
}
// ProviderEntry declares a model provider instance. ContextWindow is the model's
// token budget; the harness compacts older history as a turn's prompt approaches
// it (see agent compaction). 0 disables compaction for the instance.
type ProviderEntry struct {
	Name          string            `toml:"name"`
	Kind          string            `toml:"kind"`
	BaseURL       string            `toml:"base_url"`
	Model         string            `toml:"model"`   // a single model (back-compat)
	Models        []string          `toml:"models"`  // a vendor's model list (one base_url/key, many models)
	Default       string            `toml:"default"` // default model when Models is set (else Models[0])
	APIKeyEnv     string            `toml:"api_key_env"`
	BalanceURL    string            `toml:"balance_url"` // optional; a provider-specific wallet-balance endpoint (DeepSeek: https://api.deepseek.com/user/balance). Empty = no balance readout.
	ContextWindow int               `toml:"context_window"`
	Price         *provider.Pricing `toml:"price"`
	// Thinking / Effort are provider-kind-specific knobs forwarded to the provider
	// via Config.Extra. The anthropic provider reads Thinking="adaptive" to enable
	// extended thinking and Effort ("low".."max") to tune depth. The
	// openai-compatible provider forwards Effort as reasoning_effort for
	// thinking-capable models (e.g. MiMo) and ignores Thinking. Empty = provider default.
	Thinking string `toml:"thinking"`
	Effort   string `toml:"effort"`
}

// ModelList returns the models this provider exposes: the explicit `models` list,
// or the single `model` as a one-element list (back-compat). Empty if neither set.
func (e *ProviderEntry) ModelList() []string {
	if len(e.Models) > 0 {
		return e.Models
	}
	if e.Model != "" {
		return []string{e.Model}
	}
	return nil
}

// DefaultModel returns the provider's default model: the explicit `default`, else
// the first of ModelList.
func (e *ProviderEntry) DefaultModel() string {
	if e.Default != "" {
		return e.Default
	}
	if l := e.ModelList(); len(l) > 0 {
		return l[0]
	}
	return ""
}

// HasModel reports whether m is one of the provider's models.
func (e *ProviderEntry) HasModel(m string) bool {
	for _, x := range e.ModelList() {
		if x == m {
			return true
		}
	}
	return false
}

// ToolsConfig selects which built-in tools are enabled. Empty means all of them.
type ToolsConfig struct {
	Enabled []string `toml:"enabled"`
	// Compact enables V6.0 P8 reduced toolset (hides redundant tools from model view).
	// Hidden tools remain callable by name but don't appear in the schema list,
	// reducing model cognitive load from ~41 to ~25 visible tools.
	Compact bool `toml:"compact"`
}

// PermissionsConfig declares the per-call permission policy (see
// internal/permission). Mode is the fallback decision for writer tools when no
// rule matches ("ask" | "allow" | "deny"; default "ask"); read-only tools always
// fall back to allow. Allow/Ask/Deny are rule lists of the form "ToolName" or
// "ToolName(glob)". Precedence: deny > ask > allow > fallback.
type PermissionsConfig struct {
	Mode  string   `toml:"mode"`
	Allow []string `toml:"allow"`
	Ask   []string `toml:"ask"`
	Deny  []string `toml:"deny"`
}

// PluginEntry declares an external MCP server. Type selects the transport:
// "stdio" (default) launches Command/Args/Env as a subprocess; "http"
// (a.k.a. streamable-http) and "sse" connect to a remote URL with optional
// static Headers. String fields support ${VAR} / ${VAR:-default} expansion so
// secrets (bearer tokens, keys) come from the environment, not the file. The
// fields mirror Claude Code's mcpServers spec, so entries can come from either
// tianxuan.toml's [[plugins]] or a project-root .mcp.json (see loadMCPJSON).
type PluginEntry struct {
	Name    string            `toml:"name"`
	Type    string            `toml:"type"` // "stdio" (default) | "http" | "sse"
	Command string            `toml:"command"`
	Args    []string          `toml:"args"`
	Env     map[string]string `toml:"env"`
	URL     string            `toml:"url"`
	Headers map[string]string `toml:"headers"`
	// AutoStart controls whether the server connects during session startup.
	// Nil preserves historical behavior: configured servers start automatically.
	AutoStart *bool `toml:"auto_start"`
}

func (e PluginEntry) ShouldAutoStart() bool {
	return e.AutoStart == nil || *e.AutoStart
}

func (c *Config) AutoStartPlugins() []PluginEntry {
	out := make([]PluginEntry, 0, len(c.Plugins))
	for _, p := range c.Plugins {
		if p.ShouldAutoStart() {
			out = append(out, p)
		}
	}
	return out
}

// DefaultSystemPrompt is used when config provides none.
const DefaultSystemPrompt = `你是 tianxuan，一个中文编程助手。所有思考和输出必须使用中文。
你是 tianxuan，一个专注于执行代码任务的编码代理。
使用提供的工具读取和写入文件以及运行 shell 命令。

**原则：**
- 理解请求后再行动；用工具验证而非猜测；保持变更最小且正确；完成后简要总结。
- 遇到用户真正需要决策的问题时（方案选择、范围、影响重大的判断），使用 ask 工具列出 2-4 个具体选项，不要猜测或把问题埋在文字里。有明确默认值时直接选择，不要为了确认而提问。
- 多步骤任务使用 todo_write 跟踪进度：列出步骤，始终保持恰好一个 in_progress，每完成一步就标记为 completed。随时更新列表，不要等到最后。
- Plan mode 下写工具被阻拦：只做只读研究，给出简洁计划后停止。用户批准后按步骤执行并更新任务列表。
- 所有独立操作必须在一个响应中完成：并行读取多个文件、编辑不同文件、运行 shell 命令。只有顺序操作（编辑+验证同一文件、任务子代理）才分开发送。工具系统支持非冲突工具的并行执行——积极利用。

**子代理：**
task 工具可派发隔离子代理。以下场景优先使用子代理：
- 需读取 3+ 文件：用 explore 子代理一次返回提炼结果，节省上下文
- 需同时查代码和外部文档：用 research
- 准备 PR 或多文件变更完成前：用 review 子代理审查 diff
- 安全敏感变更（认证、输入解析、文件 IO、令牌）：用 security-review
子代理在独立上下文中运行——其工具调用不会撑大你的上下文。犹豫时直接派发。内置子代理技能（explore/research/review/security-review）见下方 Skills 索引，用 run_skill 按名称调用或直接用 task。

**记忆：**
用 remember/forget 跨会话持久化事实：
- 用户纠正偏好或事实：记住，避免后续重复犯错
- 发现非显而易见的项目事实（构建命令、架构决策、复杂依赖）：记住供后续参考
- 记忆被证明错误：用 forget 删除
不要记录瞬时状态或用户明确要求不保存的内容。记忆是持久的——只保存跨会话不变的事实。`

// LanguagePolicy is the forced language directive appended to the system prompt.
// Always Chinese — the user is a native Chinese speaker and cannot read English.
// Static text, so it stays part of the cache-stable prefix.
const LanguagePolicy = `所有思考过程和输出必须使用中文。不要使用英文——用户看不懂英文。` +
	`代码标识符（变量名、函数名、API 路由、数据库字段名）保持英文，但注释、` +
	`解释、分析、回复全部使用中文。即使收到的消息是英文，也始终用中文回复。`

// Default returns the built-in default configuration (DeepSeek + MiMo presets).
func Default() *Config {
	return &Config{
		DefaultModel: "deepseek-flash",
		Agent: AgentConfig{
			SystemPrompt: DefaultSystemPrompt,
			// 0 = no step cap: the agent loops until the model gives a final answer,
			// the user cancels, or the provider errors. Context stays bounded by
			// compaction, not by a round count. Set a positive agent.max_steps only
			// if you want a hard guard against runaway.
			MaxSteps: 0,
			AutoPlan: "on",
		},
		// Mode "ask" with no rules keeps `tianxuan run` autonomous (no TTY → ask
		// resolves to allow) while `tianxuan chat` prompts before writers. Users add
		// deny/allow rules to harden or quiet specific tools.
		Permissions: PermissionsConfig{Mode: "ask", Allow: []string{"run_skill"}},
		// Sandbox on by default: bash is jailed (macOS), network allowed so
		// builds/downloads work. Set bash = "off" to disable. Network=true here
		// so an absent [sandbox] in a user's file keeps egress (zero value would
		// wrongly deny it).
		Sandbox: SandboxConfig{Bash: "enforce", Network: true},
		// CodeGraph code-intelligence on by default: when it resolves it is injected
		// as a built-in MCP server, and AutoInstall fetches it into the cache on
		// first use. Set enabled = false to opt out, or auto_install = false to
		// require an explicit `tianxuan codegraph install`.
		Codegraph: CodegraphConfig{Enabled: true, AutoInstall: true},
		// LSP tools on by default, but dormant until a language server is on PATH;
		// a missing server yields an install hint rather than an error.
		LSP: LSPConfig{Enabled: true},
		Notify: NotifyConfig{Enabled: true, MinDuration: 5},
		Tools: ToolsConfig{Enabled: []string{
			"read_file", "write_file", "edit_file", "edit_lines",
			"ls", "grep", "bash",
			"web_fetch", "web_search",
			"todo_write", "complete_step",
			"memory_search",
			"git_status", "git_diff", "git_commit", "git_log", "git_worktree",
		}},
		Providers: []ProviderEntry{
			{Name: "deepseek-flash", Kind: "openai", BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-flash", APIKeyEnv: "DEEPSEEK_API_KEY", BalanceURL: "https://api.deepseek.com/user/balance", ContextWindow: 1_000_000, Price: &provider.Pricing{CacheHit: 0.02, Input: 1, Output: 2, Currency: "¥"}},
			{Name: "deepseek-pro", Kind: "openai", BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-pro", APIKeyEnv: "DEEPSEEK_API_KEY", BalanceURL: "https://api.deepseek.com/user/balance", ContextWindow: 1_000_000, Price: &provider.Pricing{CacheHit: 0.025, Input: 3, Output: 6, Currency: "¥"}},
			{Name: "mimo-pro", Kind: "openai", BaseURL: "https://token-plan-cn.xiaomimimo.com/v1", Model: "mimo-v2.5-pro", APIKeyEnv: "MIMO_API_KEY", ContextWindow: 1_000_000, Price: &provider.Pricing{CacheHit: 0.025, Input: 3, Output: 6, Currency: "¥"}},
			{Name: "mimo-flash", Kind: "openai", BaseURL: "https://token-plan-cn.xiaomimimo.com/v1", Model: "mimo-v2.5", APIKeyEnv: "MIMO_API_KEY", ContextWindow: 1_000_000, Price: &provider.Pricing{CacheHit: 0.02, Input: 1, Output: 2, Currency: "¥"}},
		},
	}
}

// Load builds the configuration: defaults, then user config, then project
// config, then any MCP servers from Claude Code's .mcp.json. A .env in the
// working directory is loaded first so api_key_env can resolve.
func Load() (*Config, error) {
	loadDotEnv()
	cfg := Default()

	if uc := userConfigPath(); uc != "" {
		if err := mergeFile(cfg, uc); err != nil {
			return nil, err
		}
	}
	if err := mergeFile(cfg, "tianxuan.toml"); err != nil {
		return nil, err
	}
	// Claude Code's .mcp.json (project root) is read last and merged into
	// [[plugins]], so a server configured for Claude works here unchanged.
	// tianxuan.toml wins on a name collision (see mergeMCPJSON).
	entries, err := loadMCPJSON(mcpJSONFile)
	if err != nil {
		return nil, err
	}
	cfg.mergeMCPJSON(entries)
	return cfg, nil
}

// LoadForEdit returns a config to seed the `tianxuan setup` wizard when reconfiguring:
// the built-in defaults with the file at path (if present) decoded on top, so a
// reconfigure preserves the user's existing providers and agent settings instead
// of resetting to defaults. .env is loaded so api_key_env resolution works while
// the wizard decides which keys are still missing.
func LoadForEdit(path string) *Config {
	loadDotEnv()
	cfg := Default()
	if err := mergeFile(cfg, path); err != nil {
		slog.Warn("config: load for edit failed, using defaults", "path", path, "err", err)
	}
	return cfg
}

// mergeFile decodes a TOML file onto cfg if it exists. An absent file is not an error.
func mergeFile(cfg *Config, path string) error {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return fmt.Errorf("config %s: %w", path, err)
	}
	return nil
}

func userConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tianxuan", "config.toml")
}

// UserConfigPath is the user-global config file (~/.config/tianxuan/config.toml),
// or "" when the user config dir can't be resolved.
func UserConfigPath() string { return userConfigPath() }

// ArchiveDir is where compacted conversation history is archived for
// traceability (one timestamped .jsonl per compaction). Empty if the user config
// directory cannot be resolved, in which case archiving is skipped.
func ArchiveDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tianxuan", "archive")
}

// SessionDir is where chat sessions are persisted (one .jsonl per session).
// Used by `tianxuan chat --continue` / `--resume` to find the recent ones. Empty
// if the user config dir can't be resolved — sessions then aren't saved.
func SessionDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tianxuan", "sessions")
}

// WorkspaceSessionDir returns the workspace-scoped session directory under
// cwd/.tianxuan/sessions/. Sessions are isolated per workspace so switching
// projects shows only that workspace's history.
func WorkspaceSessionDir(cwd string) string {
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		} else {
			return SessionDir()
		}
	}
	return filepath.Join(cwd, ".tianxuan", "sessions")
}

// MemoryUserDir returns the tianxuan user config root (…/tianxuan), under which
// the user-global TIANXUAN.md and the per-project auto-memory store live. Empty
// when the user config dir can't be resolved, which disables user-scoped memory.
func MemoryUserDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tianxuan")
}

// ConventionDirs are the parent directories scanned for agent assets (skills,
// commands), in canonical-first order. .tianxuan is ours; .agents / .agent /
// .claude let users drop in assets authored for other agent tools without moving
// files. Shared so skills (internal/skill) and commands (CommandDirs) discover
// the same set. Note: hooks are NOT scanned across these — a .claude/settings.json
// uses a different hook schema that can't be parsed as ours, so hooks stay in
// .tianxuan/settings.json (see internal/hook).
var ConventionDirs = []string{".tianxuan", ".agents", ".agent", ".claude"}

// conventionSubdirsAsc joins sub under each ConventionDir of base, in ascending
// priority (reverse of ConventionDirs) so the canonical .tianxuan ends up the
// highest-priority entry — command.Load lets a later directory win on a clash.
func conventionSubdirsAsc(base, sub string) []string {
	out := make([]string, 0, len(ConventionDirs))
	for i := len(ConventionDirs) - 1; i >= 0; i-- {
		out = append(out, filepath.Join(base, ConventionDirs[i], sub))
	}
	return out
}

// CommandDirs returns the directories scanned for custom slash commands, lowest
// priority first, so a later (more specific) directory overrides an earlier one
// on a name clash. Order: home-dir convention dirs (~/.claude/commands … ~/.tianxuan/commands),
// the legacy XDG user dir (~/.config/tianxuan/commands), then the project's
// convention dirs (.claude/commands … .tianxuan/commands). Scanning the .claude /
// .agents / .agent dirs lets commands authored for other agent tools (same .md +
// frontmatter format) work here unchanged.
func CommandDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, conventionSubdirsAsc(home, "commands")...)
	}
	if dir, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(dir, "tianxuan", "commands")) // legacy XDG user dir
	}
	dirs = append(dirs, conventionSubdirsAsc(".", "commands")...)
	return dirs
}

// SourcePath returns the highest-priority config file that exists, or "" if none.
func SourcePath() string {
	if _, err := os.Stat("tianxuan.toml"); err == nil {
		return "tianxuan.toml"
	}
	if uc := userConfigPath(); uc != "" {
		if _, err := os.Stat(uc); err == nil {
			return uc
		}
	}
	return ""
}

// WriteFile writes the configuration to path as annotated TOML.
func (c *Config) WriteFile(path string) error {
	return os.WriteFile(path, []byte(RenderTOML(c)), 0o644)
}

// Provider returns the named provider entry.
func (c *Config) Provider(name string) (*ProviderEntry, bool) {
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			return &c.Providers[i], true
		}
	}
	return nil, false
}

// ResolveModel resolves a model reference to a provider entry whose Model is the
// selected model string (a copy, so the config's lists stay intact). It accepts:
//   - "provider/model" — that exact model under that provider;
//   - a provider name   — the provider's default model;
//   - a bare model name — the (first) provider that lists it.
//
// The returned entry is ready to build a provider from (NewProvider reads .Model),
// so a single "vendor with many models" entry yields one instance per model
// without duplicating base_url/api_key_env. Single-`model` entries still resolve
// by provider name, keeping older configs working unchanged.
func (c *Config) ResolveModel(ref string) (*ProviderEntry, bool) {
	if ref == "" {
		return nil, false
	}
	// "provider/model"
	if prov, model, ok := strings.Cut(ref, "/"); ok {
		if e, found := c.Provider(prov); found && e.HasModel(model) {
			cp := *e
			cp.Model = model
			return &cp, true
		}
	}
	// a provider name → its default model
	if e, found := c.Provider(ref); found {
		cp := *e
		cp.Model = e.DefaultModel()
		return &cp, true
	}
	// a bare model name → the provider that lists it
	for i := range c.Providers {
		if c.Providers[i].HasModel(ref) {
			cp := c.Providers[i]
			cp.Model = ref
			return &cp, true
		}
	}
	return nil, false
}

// APIKey resolves the entry's API key from its api_key_env.
func (e *ProviderEntry) APIKey() string {
	if e.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(e.APIKeyEnv)
}

// Configured reports whether the provider's api_key_env is set — the same check
// Validate enforces, so pickers can filter on it.
func (e *ProviderEntry) Configured() bool {
	return e.APIKey() != ""
}

// ResolveSystemPrompt returns the system prompt, reading system_prompt_file if set.
func (c *Config) ResolveSystemPrompt() (string, error) {
	if c.Agent.SystemPromptFile != "" {
		b, err := os.ReadFile(c.Agent.SystemPromptFile)
		if err != nil {
			return "", fmt.Errorf("system_prompt_file: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	if strings.TrimSpace(c.Agent.SystemPrompt) == "" {
		return DefaultSystemPrompt, nil
	}
	return c.Agent.SystemPrompt, nil
}

// Validate checks that the selected model's provider is usable.
func (c *Config) Validate(model string) error {
	e, ok := c.ResolveModel(model)
	if !ok {
		return fmt.Errorf("unknown model %q (configured: %s)", model, c.providerNames())
	}
	if e.Kind == "" {
		return fmt.Errorf("provider %q: kind is required", model)
	}
	if e.BaseURL == "" {
		return fmt.Errorf("provider %q: base_url is required", model)
	}
	if e.APIKey() == "" {
		return fmt.Errorf("provider %q: missing env %s", model, e.APIKeyEnv)
	}
	return nil
}

func (c *Config) providerNames() string {
	names := make([]string, len(c.Providers))
	for i, p := range c.Providers {
		names[i] = p.Name
	}
	return strings.Join(names, ", ")
}
