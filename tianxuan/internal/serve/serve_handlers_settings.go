package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"tianxuan/internal/config"
	"tianxuan/internal/permission"
	"tianxuan/internal/provider"
)

// ── Settings ──────────────────────────────────────────────────────────

// settings returns the full SettingsView for the settings panel.
func (s *Server) settings(w http.ResponseWriter, _ *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeJSON(w, map[string]any{"providers": []any{}})
		return
	}
	bash := cfg.Sandbox.Bash
	if bash == "" {
		bash = "enforce"
	}
	type pv struct {
		Name          string   `json:"name"`
		Kind          string   `json:"kind"`
		BaseURL       string   `json:"baseUrl"`
		Models        []string `json:"models"`
		Default       string   `json:"default"`
		APIKeyEnv     string   `json:"apiKeyEnv"`
		KeySet        bool     `json:"keySet"`
		BalanceURL    string   `json:"balanceUrl"`
		ContextWindow int      `json:"contextWindow"`
	}
	providers := []pv{}
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		providers = append(providers, pv{
			Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL,
			Models:        nonEmpty(p.ModelList()),
			Default:       emptyDefault(p.DefaultModel()),
			APIKeyEnv:     p.APIKeyEnv,
			KeySet:        p.APIKeyEnv != "" && os.Getenv(p.APIKeyEnv) != "",
			BalanceURL:    p.BalanceURL,
			ContextWindow: p.ContextWindow,
		})
	}
	writeJSON(w, map[string]any{
		"defaultModel": cfg.DefaultModel,
		"providers":    providers,
		"permissions": map[string]any{
			"mode":  orDef(cfg.Permissions.Mode, "ask"),
			"allow": nonEmpty(cfg.Permissions.Allow),
			"ask":   nonEmpty(cfg.Permissions.Ask),
			"deny":  nonEmpty(cfg.Permissions.Deny),
		},
		"sandbox": map[string]any{
			"bash":          bash,
			"network":       cfg.Sandbox.Network,
			"workspaceRoot": cfg.Sandbox.WorkspaceRoot,
			"allowWrite":    nonEmpty(cfg.Sandbox.AllowWrite),
		},
		"agent": map[string]any{
			"temperature":  cfg.Agent.Temperature,
			"maxSteps":     cfg.Agent.MaxSteps,
			"systemPrompt": cfg.Agent.SystemPrompt,
		},
		"configPath":    config.SourcePath(),
		"providerKinds": provider.Kinds(),
		"autoApprove":   s.ctrl.PermLevel() != "ask",
	})
}

func (s *Server) setBypass(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On bool `json:"on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	if body.On {
		s.ctrl.SetPermLevel("yolo")
	} else {
		s.ctrl.SetPermLevel("ask")
	}
	w.WriteHeader(204)
}

func (s *Server) setModel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ref string `json:"ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	path := config.UserConfigPath()
	if path == "" {
		http.Error(w, "cannot resolve user config directory", 500)
		return
	}
	cfg := config.LoadForEdit(path)
	cfg.DefaultModel = body.Ref
	if err := cfg.SaveTo(path); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := s.Rebuild(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.meta(w, nil)
}

func (s *Server) setDefaultModel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ref string `json:"ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	path := config.UserConfigPath()
	if path == "" {
		http.Error(w, "cannot resolve user config directory", 500)
		return
	}
	cfg := config.LoadForEdit(path)
	if err := cfg.SetDefaultModel(body.Ref); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := cfg.SaveTo(path); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) saveProvider(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string   `json:"name"`
		Kind          string   `json:"kind"`
		BaseURL       string   `json:"baseUrl"`
		Models        []string `json:"models"`
		Default       string   `json:"default"`
		APIKeyEnv     string   `json:"apiKeyEnv"`
		BalanceURL    string   `json:"balanceUrl"`
		ContextWindow int      `json:"contextWindow"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	path := config.UserConfigPath()
	if path == "" {
		http.Error(w, "cannot resolve user config directory", 500)
		return
	}
	cfg := config.LoadForEdit(path)
	e := config.ProviderEntry{
		Name: body.Name, Kind: body.Kind, BaseURL: body.BaseURL,
		Models: body.Models, Default: body.Default,
		APIKeyEnv: body.APIKeyEnv, BalanceURL: body.BalanceURL,
		ContextWindow: body.ContextWindow,
	}
	if err := cfg.UpsertProvider(e); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := cfg.SaveTo(path); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) deleteProvider(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	path := config.UserConfigPath()
	if path == "" {
		http.Error(w, "cannot resolve user config directory", 500)
		return
	}
	cfg := config.LoadForEdit(path)
	if err := cfg.RemoveProvider(body.Name); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := cfg.SaveTo(path); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) setProviderKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIKeyEnv string `json:"apiKeyEnv"`
		Value     string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	if body.APIKeyEnv == "" {
		http.Error(w, "missing apiKeyEnv", 400)
		return
	}
	dotenv := ".env"
	lines := map[string]string{}
	if data, err := os.ReadFile(dotenv); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if i := strings.IndexByte(line, '='); i >= 0 {
				lines[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
			}
		}
	}
	lines[body.APIKeyEnv] = body.Value
	var b strings.Builder
	for k, v := range lines {
		fmt.Fprintf(&b, "%s=%s\n", k, v)
	}
	if err := os.WriteFile(dotenv, []byte(b.String()), 0600); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) setAgentParams(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Temperature  float64 `json:"temperature"`
		MaxSteps     int     `json:"maxSteps"`
		SystemPrompt string  `json:"systemPrompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	path := config.UserConfigPath()
	if path == "" {
		http.Error(w, "cannot resolve user config directory", 500)
		return
	}
	cfg := config.LoadForEdit(path)
	if body.Temperature > 0 {
		cfg.Agent.Temperature = body.Temperature
	}
	if body.MaxSteps > 0 {
		cfg.Agent.MaxSteps = body.MaxSteps
	}
	if body.SystemPrompt != "" {
		cfg.Agent.SystemPrompt = body.SystemPrompt
	}
	if err := cfg.SaveTo(path); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) setSandbox(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Bash          string   `json:"bash"`
		Network       bool     `json:"network"`
		WorkspaceRoot string   `json:"workspaceRoot"`
		AllowWrite    []string `json:"allowWrite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	path := config.UserConfigPath()
	if path == "" {
		http.Error(w, "cannot resolve user config directory", 500)
		return
	}
	cfg := config.LoadForEdit(path)
	cfg.Sandbox.Bash = body.Bash
	cfg.Sandbox.Network = body.Network
	cfg.Sandbox.WorkspaceRoot = body.WorkspaceRoot
	cfg.Sandbox.AllowWrite = body.AllowWrite
	if err := cfg.SaveTo(path); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) setPermissionMode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	path := config.UserConfigPath()
	if path == "" {
		http.Error(w, "cannot resolve user config directory", 500)
		return
	}
	cfg := config.LoadForEdit(path)
	if err := cfg.SetPermissionMode(body.Mode); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := cfg.SaveTo(path); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) addPermissionRule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		List string `json:"list"`
		Rule string `json:"rule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	if _, ok := permission.ParseRule(body.Rule); !ok {
		http.Error(w, "invalid rule: "+body.Rule, 400)
		return
	}
	path := config.UserConfigPath()
	if path == "" {
		http.Error(w, "cannot resolve user config directory", 500)
		return
	}
	cfg := config.LoadForEdit(path)
	if err := cfg.AddPermissionRule(body.List, body.Rule); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := cfg.SaveTo(path); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) removePermissionRule(w http.ResponseWriter, r *http.Request) {
	list := r.URL.Query().Get("list")
	rule := r.URL.Query().Get("rule")
	if list == "" || rule == "" {
		http.Error(w, "missing list or rule", 400)
		return
	}
	path := config.UserConfigPath()
	if path == "" {
		http.Error(w, "cannot resolve user config directory", 500)
		return
	}
	cfg := config.LoadForEdit(path)
	ok, err := cfg.RemovePermissionRule(list, rule)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if !ok {
		http.Error(w, "rule not found", 404)
		return
	}
	if err := cfg.SaveTo(path); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

// ── MCP Management ────────────────────────────────────────────────────

func (s *Server) addMCPServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string            `json:"name"`
		Type    string            `json:"type"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	e := config.PluginEntry{
		Name: body.Name, Type: body.Type, Command: body.Command,
		Args: body.Args, Env: body.Env, URL: body.URL, Headers: body.Headers,
	}
	n, err := s.ctrl.AddMCPServer(e)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"tools": n})
}

func (s *Server) removeMCPServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	disconnected, err := s.ctrl.RemoveMCPServer(body.Name)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"disconnected": disconnected})
}

func (s *Server) retryMCPServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	n, err := s.ctrl.ConnectConfiguredMCPServer(body.Name)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"tools": n})
}

func (s *Server) setMCPServerEnabled(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	if body.Enabled {
		n, err := s.ctrl.ConnectConfiguredMCPServer(body.Name)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, map[string]any{"tools": n})
	} else {
		disconnected := s.ctrl.DisconnectMCPServer(body.Name)
		writeJSON(w, map[string]any{"disconnected": disconnected})
	}
}
