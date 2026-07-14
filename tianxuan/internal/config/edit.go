package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tianxuan/internal/permission"
)

// edit.go is the programmatic mutation surface a settings UI drives: change the
// default model, add/remove a provider, set the planner, edit permission rules,
// add/remove an MCP server — each validated, then persisted with SaveTo. It is
// separate from the `tianxuan setup` wizard (cli) so a GUI can apply one setting at a
// time without replaying the whole interactive flow. Every mutator works on the
// in-memory *Config; nothing writes to disk until SaveTo/Save is called, so a UI
// can stage several changes and commit once. Mutations round-trip through
// RenderTOML → Load (the wizard relies on the same guarantee).

// permission rule list names accepted by the rule mutators.
const (
	listAllow = "allow"
	listAsk   = "ask"
	listDeny  = "deny"
)

// SetDefaultModel points default_model at an existing provider. It errors if no
// provider by that name is configured, so a UI can't strand the config on a
// model that doesn't exist.
func (c *Config) SetDefaultModel(name string) error {
	if _, ok := c.Provider(name); !ok {
		return fmt.Errorf("set default: no provider %q (configured: %s)", name, c.providerNames())
	}
	c.DefaultModel = name
	return nil
}

// SetPlannerModel sets (or, with "", clears) agent.planner_model for two-model
// collaboration. A non-empty name must be a configured provider.
func (c *Config) SetPlannerModel(name string) error {
	if name == "" {
		c.Agent.PlannerModel = ""
		return nil
	}
	if _, ok := c.ResolveModel(name); !ok {
		return fmt.Errorf("set planner: no provider %q (configured: %s)", name, c.providerNames())
	}
	c.Agent.PlannerModel = name
	return nil
}

// SetSubagentModel sets (or, with "", clears) agent.subagent_model — the default
// model for sub-agents (task tool and runAs=subagent skills). An empty string
// means the sub-agent inherits the parent's execution provider. A non-empty name
// must be a configured provider.
func (c *Config) SetSubagentModel(name string) error {
	if name == "" {
		c.Agent.SubagentModel = ""
		return nil
	}
	if _, ok := c.ResolveModel(name); !ok {
		return fmt.Errorf("set subagent: no provider %q (configured: %s)", name, c.providerNames())
	}
	c.Agent.SubagentModel = name
	return nil
}

// SetSubagentModelForSkill sets (or, with "", clears) agent.subagent_models[skill]
// — a per-skill override for sub-agent model selection. An empty ref clears the
// override so the skill falls back to the global subagent_model. A non-empty ref
// must be a configured provider.
func (c *Config) SetSubagentModelForSkill(skill, ref string) error {
	if c.Agent.SubagentModels == nil {
		c.Agent.SubagentModels = make(map[string]string)
	}
	if ref == "" {
		delete(c.Agent.SubagentModels, skill)
		return nil
	}
	if _, ok := c.ResolveModel(ref); !ok {
		return fmt.Errorf("set subagent model for %s: no provider %q (configured: %s)", skill, ref, c.providerNames())
	}
	c.Agent.SubagentModels[skill] = ref
	return nil
}

// UpsertProvider adds e, or replaces an existing provider with the same name
// (preserving its position). Required fields (name, kind, base_url, model) are
// validated; whether the kind is actually registered and the key resolves is
// checked later by provider.New / Validate, which give actionable errors.
func (c *Config) UpsertProvider(e ProviderEntry) error {
	if err := validateProvider(e); err != nil {
		return err
	}
	for i := range c.Providers {
		if c.Providers[i].Name == e.Name {
			c.Providers[i] = e
			return nil
		}
	}
	c.Providers = append(c.Providers, e)
	return nil
}

// RemoveProvider deletes the named provider. It refuses to remove the current
// default_model (reassign it first, so the config never points at a missing
// model); if the removed provider was the planner, planner_model is cleared as
// a side effect since it is optional. Errors when the name isn't configured.
func (c *Config) RemoveProvider(name string) error {
	idx := -1
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("remove provider: no provider %q", name)
	}
	if c.DefaultModel == name {
		return fmt.Errorf("remove provider: %q is the default model — set a different default_model first", name)
	}
	c.Providers = append(c.Providers[:idx], c.Providers[idx+1:]...)
	if c.Agent.PlannerModel == name || strings.HasPrefix(c.Agent.PlannerModel, name+"/") {
		c.Agent.PlannerModel = ""
	}
	return nil
}

// validateProvider checks the fields a provider can't function without.
func validateProvider(e ProviderEntry) error {
	switch {
	case strings.TrimSpace(e.Name) == "":
		return fmt.Errorf("provider: name is required")
	case strings.TrimSpace(e.Kind) == "":
		return fmt.Errorf("provider %q: kind is required", e.Name)
	case strings.TrimSpace(e.BaseURL) == "":
		return fmt.Errorf("provider %q: base_url is required", e.Name)
	case strings.TrimSpace(e.Model) == "":
		return fmt.Errorf("provider %q: model is required", e.Name)
	}
	return nil
}

// SetPermissionMode sets the writer-fallback mode. Accepts "ask", "allow", or
// "deny" (case-insensitive); anything else errors rather than silently
// defaulting, so a UI surfaces a typo instead of installing a surprising mode.
func (c *Config) SetPermissionMode(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "ask", "allow", "deny":
		c.Permissions.Mode = strings.ToLower(strings.TrimSpace(mode))
		return nil
	default:
		return fmt.Errorf("permission mode %q: must be ask|allow|deny", mode)
	}
}

// AddPermissionRule appends a rule ("ToolName" or "ToolName(glob)") to the
// allow / ask / deny list. The rule is validated with the same parser the gate
// uses, and a duplicate is a no-op so a UI can call it idempotently.
func (c *Config) AddPermissionRule(list, rule string) error {
	target, err := c.ruleList(list)
	if err != nil {
		return err
	}
	rule = strings.TrimSpace(rule)
	if _, ok := permission.ParseRule(rule); !ok {
		return fmt.Errorf("invalid permission rule %q (want \"ToolName\" or \"ToolName(glob)\")", rule)
	}
	for _, existing := range *target {
		if existing == rule {
			return nil // already present
		}
	}
	*target = append(*target, rule)
	return nil
}

// RemovePermissionRule drops the first exact match of rule from the named list,
// reporting whether anything was removed.
func (c *Config) RemovePermissionRule(list, rule string) (bool, error) {
	target, err := c.ruleList(list)
	if err != nil {
		return false, err
	}
	rule = strings.TrimSpace(rule)
	for i, existing := range *target {
		if existing == rule {
			*target = append((*target)[:i], (*target)[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

// ruleList returns a pointer to the named rule slice so mutators can append to
// it in place. An unknown list name errors.
func (c *Config) ruleList(list string) (*[]string, error) {
	switch strings.ToLower(strings.TrimSpace(list)) {
	case listAllow:
		return &c.Permissions.Allow, nil
	case listAsk:
		return &c.Permissions.Ask, nil
	case listDeny:
		return &c.Permissions.Deny, nil
	default:
		return nil, fmt.Errorf("unknown permission list %q (want allow|ask|deny)", list)
	}
}

// UpsertPlugin adds e, or replaces an MCP server with the same name (preserving
// position). The transport-specific required fields are validated: stdio needs
// a command, http/sse need a url.
func (c *Config) UpsertPlugin(e PluginEntry) error {
	if err := validatePlugin(e); err != nil {
		return err
	}
	for i := range c.Plugins {
		if c.Plugins[i].Name == e.Name {
			c.Plugins[i] = e
			return nil
		}
	}
	c.Plugins = append(c.Plugins, e)
	return nil
}

// RemovePlugin deletes the named MCP server, reporting whether it was present.
func (c *Config) RemovePlugin(name string) bool {
	for i := range c.Plugins {
		if c.Plugins[i].Name == name {
			c.Plugins = append(c.Plugins[:i], c.Plugins[i+1:]...)
			return true
		}
	}
	return false
}

// validatePlugin checks a plugin entry by transport. An empty Type means stdio.
func validatePlugin(e PluginEntry) error {
	if strings.TrimSpace(e.Name) == "" {
		return fmt.Errorf("plugin: name is required")
	}
	switch strings.ToLower(strings.TrimSpace(e.Type)) {
	case "", "stdio":
		if strings.TrimSpace(e.Command) == "" {
			return fmt.Errorf("plugin %q: command is required for a stdio server", e.Name)
		}
	case "http", "sse", "streamable-http":
		if strings.TrimSpace(e.URL) == "" {
			return fmt.Errorf("plugin %q: url is required for a %s server", e.Name, e.Type)
		}
	default:
		return fmt.Errorf("plugin %q: unknown type %q (want stdio|http|sse)", e.Name, e.Type)
	}
	return nil
}

// SetPlannerMaxSteps caps the planner's tool-call rounds per turn. 0 = unlimited.
func (c *Config) SetPlannerMaxSteps(n int) error {
	if n < 0 {
		return fmt.Errorf("planner_max_steps must be >= 0, got %d", n)
	}
	c.Agent.PlannerMaxSteps = n
	return nil
}

// SetMaxSubagentDepth caps recursion depth for runAs=subagent skills. 0 = unlimited.
func (c *Config) SetMaxSubagentDepth(n int) error {
	if n < 0 {
		return fmt.Errorf("max_subagent_depth must be >= 0, got %d", n)
	}
	c.Agent.MaxSubagentDepth = n
	return nil
}

// SetColdResumePrune enables or disables pruning of expired tool results on cold resume.
func (c *Config) SetColdResumePrune(on bool) error {
	c.Agent.ColdResumePrune = &on
	return nil
}

// SetReasoningLanguage sets the language preference for model reasoning/thinking text.
// Valid values: "" (auto), "zh", "en", "auto".
func (c *Config) SetReasoningLanguage(lang string) error {
	lang = strings.TrimSpace(lang)
	switch lang {
	case "", "auto", "zh", "en":
		c.Agent.ReasoningLanguage = lang
		return nil
	default:
		return fmt.Errorf("reasoning_language %q: must be auto|zh|en", lang)
	}
}

// SetAutoPlan controls whether interactive turns auto-start in plan mode.
// Valid values: "off", "ask", "on".
func (c *Config) SetAutoPlan(mode string) error {
	mode = strings.TrimSpace(mode)
	switch mode {
	case "off", "ask", "on":
		c.Agent.AutoPlan = mode
		return nil
	default:
		return fmt.Errorf("auto_plan %q: must be off|ask|on", mode)
	}
}

// SetOutputStyle sets the persona/tone folded into the system prompt.
func (c *Config) SetOutputStyle(style string) error {
	c.Agent.OutputStyle = strings.TrimSpace(style)
	return nil
}

// SetLanguage sets the ui/model language tag (e.g. "zh"). Empty = auto-detect.
func (c *Config) SetLanguage(lang string) error {
	c.Language = strings.TrimSpace(lang)
	return nil
}


// SetDesktopLayoutStyle sets the desktop layout: "classic" | "workbench" | "creation".
func (c *Config) SetDesktopLayoutStyle(style string) error {
	style = strings.TrimSpace(style)
	if style != "" && style != "classic" && style != "workbench" && style != "creation" {
		return fmt.Errorf("layout_style %q: must be classic|workbench|creation", style)
	}
	c.Desktop.LayoutStyle = style
	return nil
}

// SetDesktopDisplayMode sets the chat density: "standard" | "compact".
func (c *Config) SetDesktopDisplayMode(mode string) error {
	mode = strings.TrimSpace(mode)
	if mode != "" && mode != "standard" && mode != "compact" {
		return fmt.Errorf("display_mode %q: must be standard|compact", mode)
	}
	c.Desktop.DisplayMode = mode
	return nil
}

// SetDesktopCloseBehavior sets the window close action: "quit" | "background".
func (c *Config) SetDesktopCloseBehavior(behavior string) error {
	behavior = strings.TrimSpace(behavior)
	if behavior != "" && behavior != "quit" && behavior != "background" {
		return fmt.Errorf("close_behavior %q: must be quit|background", behavior)
	}
	c.Desktop.CloseBehavior = behavior
	return nil
}

// SetDesktopCheckUpdates controls whether the desktop checks for new releases on launch.
func (c *Config) SetDesktopCheckUpdates(on bool) error {
	c.Desktop.CheckUpdates = on
	return nil
}

// SetDesktopTelemetry enables anonymous launch pings.
func (c *Config) SetDesktopTelemetry(on bool) error {
	c.Desktop.Telemetry = on
	return nil
}

// SetDesktopMetrics enables aggregated desktop-usage counters.
func (c *Config) SetDesktopMetrics(on bool) error {
	c.Desktop.Metrics = on
	return nil
}

// SetStatusBarStyle sets the status bar display style: "icon" | "text".
func (c *Config) SetStatusBarStyle(style string) error {
	c.Desktop.StatusBarStyle = style
	return nil
}

// SetStatusBarItems sets the ordered list of visible status bar item IDs.
func (c *Config) SetStatusBarItems(items []string) error {
	c.Desktop.StatusBarItems = items
	return nil
}

// SetBashTimeoutSeconds sets the foreground bash timeout in seconds. nil = default (120s).
func (c *Config) SetBashTimeoutSeconds(secs *int) error {
	c.Tools.BashTimeoutSeconds = secs
	return nil
}

// SetMCPCallTimeoutSeconds sets the MCP JSON-RPC call timeout. nil = default (300s).
func (c *Config) SetMCPCallTimeoutSeconds(secs *int) error {
	c.Tools.MCPCallTimeoutSeconds = secs
	return nil
}

// SetShellPreference sets the shell interpreter: "auto" | "bash" | "powershell".
func (c *Config) SetShellPreference(shell string) error {
	c.Tools.Shell = strings.TrimSpace(shell)
	return nil
}

// SetMemoryCompilerEnabled toggles the Memory v5 execution compiler.
func (c *Config) SetMemoryCompilerEnabled(on bool) error {
	c.Agent.MemoryCompilerEnabled = on
	return nil
}

// SetDesktopTheme sets the desktop color scheme: "auto" | "dark" | "light".
func (c *Config) SetDesktopTheme(theme string) error {
	switch strings.ToLower(strings.TrimSpace(theme)) {
	case "", "auto":
		c.Desktop.Theme = "auto"
	case "dark":
		c.Desktop.Theme = "dark"
	case "light":
		c.Desktop.Theme = "light"
	default:
		return fmt.Errorf("desktop theme %q: must be auto|dark|light", theme)
	}
	return nil
}

// SetDesktopThemeStyle sets the desktop theme variant name.
func (c *Config) SetDesktopThemeStyle(style string) error {
	c.Desktop.ThemeStyle = strings.TrimSpace(style)
	return nil
}

// SetDesktopTextSize sets the UI font size: "small" | "default" | "large" | "xlarge".
func (c *Config) SetDesktopTextSize(size string) error {
	size = strings.TrimSpace(size)
	if size != "" && size != "small" && size != "default" && size != "large" && size != "xlarge" {
		return fmt.Errorf("desktop text_size %q: must be small|default|large|xlarge", size)
	}
	c.Desktop.TextSize = size
	return nil
}

// SetDesktopZoomFactor sets the UI zoom percentage (70-150). 0 = default 100.
func (c *Config) SetDesktopZoomFactor(z int) error {
	if z != 0 && (z < 70 || z > 150) {
		return fmt.Errorf("desktop zoom_factor %d: must be 70-150", z)
	}
	c.Desktop.ZoomFactor = z
	return nil
}

// SetDesktopFontFamily sets the UI font family. Empty = system default.
func (c *Config) SetDesktopFontFamily(font string) error {
	c.Desktop.FontFamily = strings.TrimSpace(font)
	return nil
}

// SetDesktopMonoFontFamily sets the monospace font family. Empty = system default.
func (c *Config) SetDesktopMonoFontFamily(font string) error {
	c.Desktop.MonoFontFamily = strings.TrimSpace(font)
	return nil
}

// SetPluginEnabled toggles auto_start for a plugin by name.
// enabled=true → auto_start=true; enabled=false → auto_start=false.
func (c *Config) SetPluginEnabled(name string, enabled bool) error {
	for i, p := range c.Plugins {
		if p.Name == name {
			v := enabled
			c.Plugins[i].AutoStart = &v
			return nil
		}
	}
	return fmt.Errorf("plugin %q not found", name)
}

// SaveTo writes the configuration to path as annotated TOML, atomically: it
// writes a sibling temp file then renames, so a crash mid-write can't leave a
// half-written tianxuan.toml that fails to parse on next load. Parent directories
// are created as needed.
func (c *Config) SaveTo(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("save: empty config path")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("save: create dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".tianxuan.*.toml.tmp")
	if err != nil {
		return fmt.Errorf("save: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(RenderTOML(c)); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("save: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("save: close temp: %w", err)
	}
	return os.Rename(tmpPath, path)
}

// Save writes the configuration back to the file it was loaded from
// (SourcePath), or to ./tianxuan.toml when none exists yet — the conventional
// project-local target a fresh GUI session would create.
func (c *Config) Save() error {
	path := SourcePath()
	if path == "" {
		path = "tianxuan.toml"
	}
	return c.SaveTo(path)
}
