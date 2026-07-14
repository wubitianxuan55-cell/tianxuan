package main

import "tianxuan/internal/config"

// --- Search settings ---

type SearchSettingsView struct {
	LocalSearXNGURL string   `json:"localSearXNGUrl"`
	TavilyAPIKeyEnv string   `json:"tavilyApiKeyEnv"`
	BraveAPIKeyEnv  string   `json:"braveApiKeyEnv"`
	TimeoutSeconds  int      `json:"timeoutSeconds"`
	AllowDomains    []string `json:"allowDomains"`
	DenyDomains     []string `json:"denyDomains"`
}

func (a *App) SearchSettings() SearchSettingsView {
	cfg, err := config.Load()
	if err != nil {
		return SearchSettingsView{}
	}
	return SearchSettingsView{
		LocalSearXNGURL: cfg.Search.LocalSearXNGURL,
		TavilyAPIKeyEnv: cfg.Search.TavilyAPIKeyEnv,
		BraveAPIKeyEnv:  cfg.Search.BraveAPIKeyEnv,
		TimeoutSeconds:  cfg.Search.TimeoutSeconds,
		AllowDomains:    nonNil(cfg.Search.AllowDomains),
		DenyDomains:     nonNil(cfg.Search.DenyDomains),
	}
}

func (a *App) SaveSearchSettings(v SearchSettingsView) error {
	return a.applyConfigChange(func(c *config.Config) error {
		c.Search.LocalSearXNGURL = v.LocalSearXNGURL
		c.Search.TavilyAPIKeyEnv = v.TavilyAPIKeyEnv
		c.Search.BraveAPIKeyEnv = v.BraveAPIKeyEnv
		c.Search.TimeoutSeconds = v.TimeoutSeconds
		c.Search.AllowDomains = v.AllowDomains
		c.Search.DenyDomains = v.DenyDomains
		return nil
	})
}

// --- LSP settings ---

type LSPSettingsView struct {
	Enabled bool              `json:"enabled"`
	Servers map[string]LSPEntryView `json:"servers"`
}

type LSPEntryView struct {
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Env         map[string]string `json:"env"`
	LanguageID  string            `json:"languageId"`
	Extensions  []string          `json:"extensions"`
	InstallHint string            `json:"installHint"`
}

func (a *App) LSPSettings() LSPSettingsView {
	cfg, err := config.Load()
	if err != nil {
		return LSPSettingsView{}
	}
	servers := map[string]LSPEntryView{}
	for lang, s := range cfg.LSP.Servers {
		servers[lang] = LSPEntryView{
			Command: s.Command, Args: nonNil(s.Args), Env: s.Env,
			LanguageID: s.LanguageID, Extensions: nonNil(s.Extensions), InstallHint: s.InstallHint,
		}
	}
	return LSPSettingsView{Enabled: cfg.LSP.Enabled, Servers: servers}
}

func (a *App) SaveLSPSettings(v LSPSettingsView) error {
	return a.applyConfigChange(func(c *config.Config) error {
		c.LSP.Enabled = v.Enabled
		c.LSP.Servers = map[string]config.LSPServer{}
		for lang, s := range v.Servers {
			c.LSP.Servers[lang] = config.LSPServer{
				Command: s.Command, Args: s.Args, Env: s.Env,
				LanguageID: s.LanguageID, Extensions: s.Extensions, InstallHint: s.InstallHint,
			}
		}
		return nil
	})
}

// --- Codegraph settings ---

type CodegraphSettingsView struct {
	Enabled     bool   `json:"enabled"`
	AutoInstall bool   `json:"autoInstall"`
	Path        string `json:"path"`
}

func (a *App) CodegraphSettings() CodegraphSettingsView {
	cfg, err := config.Load()
	if err != nil {
		return CodegraphSettingsView{}
	}
	return CodegraphSettingsView{
		Enabled: cfg.Codegraph.Enabled, AutoInstall: cfg.Codegraph.AutoInstall,
		Path: cfg.Codegraph.Path,
	}
}

func (a *App) SaveCodegraphSettings(v CodegraphSettingsView) error {
	return a.applyConfigChange(func(c *config.Config) error {
		c.Codegraph.Enabled = v.Enabled
		c.Codegraph.AutoInstall = v.AutoInstall
		c.Codegraph.Path = v.Path
		return nil
	})
}

// --- Skills advanced settings ---

type SkillsSettingsView struct {
	Paths []string `json:"paths"`
}

func (a *App) SkillsSettingsAdvanced() SkillsSettingsView {
	cfg, err := config.Load()
	if err != nil {
		return SkillsSettingsView{}
	}
	return SkillsSettingsView{Paths: nonNil(cfg.Skills.Paths)}
}

func (a *App) SaveSkillsSettings(v SkillsSettingsView) error {
	return a.applyConfigChange(func(c *config.Config) error {
		c.Skills.Paths = v.Paths
		return nil
	})
}

// --- Agent advanced settings ---

type AgentAdvancedView struct {
	SystemPromptFile    string `json:"systemPromptFile"`
	AutoPlanClassifier  string `json:"autoPlanClassifier"`
}

func (a *App) AgentAdvancedSettings() AgentAdvancedView {
	cfg, err := config.Load()
	if err != nil {
		return AgentAdvancedView{}
	}
	return AgentAdvancedView{
		SystemPromptFile:   cfg.Agent.SystemPromptFile,
		AutoPlanClassifier: cfg.Agent.AutoPlanClassifier,
	}
}

func (a *App) SaveAgentAdvancedSettings(v AgentAdvancedView) error {
	return a.applyConfigChange(func(c *config.Config) error {
		c.Agent.SystemPromptFile = v.SystemPromptFile
		c.Agent.AutoPlanClassifier = v.AutoPlanClassifier
		return nil
	})
}
