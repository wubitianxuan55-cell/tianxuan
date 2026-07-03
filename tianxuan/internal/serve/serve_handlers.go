package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"tianxuan/internal/agent"
	"tianxuan/internal/config"
	"tianxuan/internal/control"
	"tianxuan/internal/event"
	"tianxuan/internal/memory"
	"tianxuan/internal/permission"
	"tianxuan/internal/provider"
)

// meta returns session metadata for the frontend status bar.
func (s *Server) meta(w http.ResponseWriter, _ *http.Request) {
	view := map[string]any{
		"cwd":     s.ctrl.SessionDir(),
		"label":   s.ctrl.Label(),
		"ready":   true,
		"running": s.ctrl.Running(),
	}
	writeJSON(w, view)
}

// memory returns the full memory view (docs, facts, scopes).
func (s *Server) memory(w http.ResponseWriter, _ *http.Request) {
	type memDoc struct {
		Path  string `json:"path"`
		Scope string `json:"scope"`
		Body  string `json:"body"`
	}
	type memFact struct {
		Name        string `json:"name"`
		Title       string `json:"title,omitempty"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Body        string `json:"body"`
	}
	type memScope struct {
		Scope string `json:"scope"`
		Path  string `json:"path"`
	}
	docs := []memDoc{}
	facts := []memFact{}
	scopes := []memScope{}
	set := s.ctrl.Memory()
	if set != nil {
		for _, d := range set.Docs {
			docs = append(docs, memDoc{Path: d.Path, Scope: string(d.Scope), Body: d.Body})
		}
		for _, f := range set.Store.List() {
			facts = append(facts, memFact{
				Name: f.Name, Title: f.Title, Description: f.Description,
				Type: string(f.Type), Body: f.Body,
			})
		}
	}
	writeJSON(w, map[string]any{
		"docs": docs, "facts": facts, "scopes": scopes,
		"storeDir": s.ctrl.SessionDir(), "available": set != nil,
	})
}

// remember quick-adds a one-line note to the doc-memory file.
func (s *Server) remember(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Scope string `json:"scope"`
		Note  string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	s.ctrl.QuickAdd(parseServeScope(body.Scope), body.Note)
	w.WriteHeader(204)
}

// forget deletes a saved auto-memory by name.
func (s *Server) forget(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	s.ctrl.ForgetMemory(body.Name)
	w.WriteHeader(204)
}

// saveDoc overwrites a memory doc.
func (s *Server) saveDoc(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	s.ctrl.SaveDoc(body.Path, body.Body)
	w.WriteHeader(204)
}

// answer resolves a pending ask_request.
func (s *Server) answer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string `json:"id"`
		Answers []struct {
			QuestionID string   `json:"questionId"`
			Selected   []string `json:"selected"`
		} `json:"answers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, "missing id", 400)
		return
	}
	out := make([]event.AskAnswer, len(body.Answers))
	for i, a := range body.Answers {
		out[i] = event.AskAnswer{QuestionID: a.QuestionID, Selected: a.Selected}
	}
	s.ctrl.AnswerQuestion(body.ID, out)
	w.WriteHeader(204)
}

// models lists available provider/model pairs from config.
func (s *Server) models(w http.ResponseWriter, _ *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeJSON(w, []any{})
		return
	}
	type modelInfo struct {
		Ref      string `json:"ref"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	out := []modelInfo{}
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		if !p.Configured() {
			continue
		}
		for _, m := range p.ModelList() {
			out = append(out, modelInfo{
				Ref: p.Name + "/" + m, Provider: p.Name, Model: m,
			})
		}
	}
	writeJSON(w, out)
}

// sessions lists saved sessions from the session directory.
func (s *Server) sessions(w http.ResponseWriter, _ *http.Request) {
	dir := s.ctrl.SessionDir()
	if dir == "" {
		writeJSON(w, []any{})
		return
	}
	es, err := os.ReadDir(dir)
	if err != nil {
		writeJSON(w, []any{})
		return
	}
	type sessMeta struct {
		Path  string `json:"path"`
		Label string `json:"label"`
	}
	out := []sessMeta{}
	for _, e := range es {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		out = append(out, sessMeta{
			Path:  e.Name(),
			Label: strings.TrimSuffix(e.Name(), ".jsonl"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label > out[j].Label })
	writeJSON(w, out)
}

// deleteSession removes a saved session file.
func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		http.Error(w, "missing path", 400)
		return
	}
	dir := s.ctrl.SessionDir()
	if dir != "" {
		os.Remove(body.Path)
	}
	w.WriteHeader(204)
}

// resumeSession loads a saved session and makes it the active one.
func (s *Server) resumeSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		http.Error(w, "missing path", 400)
		return
	}
	sess, err := agent.LoadSession(body.Path)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.ctrl.Resume(sess, body.Path)
	w.WriteHeader(204)
}

// listDir lists one directory level for the @-menu.
func (s *Server) listDir(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	base := s.ctrl.SessionDir()
	if base == "" {
		var err error
		base, err = os.Getwd()
		if err != nil {
			writeJSON(w, []any{})
			return
		}
	}
	dir := base
	if rel != "" {
		dir = base + "/" + rel
	}
	es, err := os.ReadDir(dir)
	if err != nil {
		writeJSON(w, []any{})
		return
	}
	type entry struct {
		Name  string `json:"name"`
		IsDir bool   `json:"isDir"`
	}
	out := []entry{}
	for _, e := range es {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, entry{Name: name, IsDir: e.IsDir()})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	writeJSON(w, out)
}

// readFile returns a text preview for a workspace file.
func (s *Server) readFile(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	base := s.ctrl.SessionDir()
	if base == "" {
		var err error
		base, err = os.Getwd()
		if err != nil {
			writeJSON(w, map[string]any{"err": err.Error()})
			return
		}
	}
	path := base + "/" + rel
	data, err := os.ReadFile(path)
	out := map[string]any{"path": rel}
	if err != nil {
		out["err"] = err.Error()
	} else {
		if len(data) > 256*1024 {
			data = data[:256*1024]
			out["truncated"] = true
		}
		out["body"] = string(data)
	}
	writeJSON(w, out)
}

// balance returns wallet balance info for the current provider.
func (s *Server) balance(w http.ResponseWriter, _ *http.Request) {
	b, err := s.ctrl.Balance(context.Background())
	if err != nil || b == nil {
		writeJSON(w, map[string]any{"available": false})
		return
	}
	amount := "0.00"
	if len(b.Infos) > 0 {
		amount = b.Infos[0].TotalBalance
	}
	writeJSON(w, map[string]any{
		"available": true,
		"display":   amount,
	})
}

// jobs returns active background jobs.
func (s *Server) jobs(w http.ResponseWriter, _ *http.Request) {
	views := s.ctrl.Jobs()
	type jobView struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		Label     string `json:"label"`
		Status    string `json:"status"`
		StartedAt int64  `json:"startedAt"`
	}
	out := make([]jobView, len(views))
	for i, v := range views {
		out[i] = jobView{
			ID: v.ID, Kind: v.Kind, Label: v.Label,
			Status:    v.Status,
			StartedAt: v.StartedAt,
		}
	}
	writeJSON(w, out)
}

// commands returns registered slash commands.
func (s *Server) commands(w http.ResponseWriter, _ *http.Request) {
	type cmdInfo struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	cmds := s.ctrl.Commands()
	out := make([]cmdInfo, len(cmds))
	for i, c := range cmds {
		out[i] = cmdInfo{Name: c.Name, Description: c.Description}
	}
	writeJSON(w, out)
}

// capabilities returns MCP servers and skills.
func (s *Server) capabilities(w http.ResponseWriter, _ *http.Request) {
	type serverView struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	type skillView struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	servers := []serverView{}
	if host := s.ctrl.Host(); host != nil {
		for _, name := range host.ServerNames() {
			servers = append(servers, serverView{Name: name, Status: "connected"})
		}
	}
	skills := []skillView{}
	for _, sk := range s.ctrl.Skills() {
		skills = append(skills, skillView{Name: sk.Name, Description: sk.Description})
	}
	writeJSON(w, map[string]any{"servers": servers, "skills": skills})
}

// parseServeScope maps a frontend scope string to memory.Scope.
func parseServeScope(s string) memory.Scope {
	switch s {
	case "user":
		return memory.ScopeUser
	case "local":
		return memory.ScopeLocal
	default:
		return memory.ScopeProject
	}
}

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
		"defaultModel":  cfg.DefaultModel,
		"plannerModel":  cfg.Agent.PlannerModel,
		"providers":     providers,
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
	var body struct{ On bool `json:"on"` }
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
	var body struct{ Ref string `json:"ref"` }
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
	var body struct{ Ref string `json:"ref"` }
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
	var body struct{ Name string `json:"name"` }
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
	var body struct{ Mode string `json:"mode"` }
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
	var body struct{ Name string `json:"name"` }
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
	var body struct{ Name string `json:"name"` }
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

// ── Checkpoints ───────────────────────────────────────────────────────

func (s *Server) checkpoints(w http.ResponseWriter, _ *http.Request) {
	metas := s.ctrl.Checkpoints()
	type cm struct {
		Turn    int    `json:"turn"`
		Summary string `json:"summary"`
	}
	out := make([]cm, len(metas))
	for i, m := range metas {
		out[i] = cm{Turn: m.Turn, Summary: m.Prompt}
	}
	writeJSON(w, out)
}

func (s *Server) rewindCheckpoint(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Turn  int    `json:"turn"`
		Scope string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	var scope control.RewindScope
	switch body.Scope {
	case "code":
		scope = control.RewindCode
	case "conversation":
		scope = control.RewindConversation
	default:
		scope = control.RewindBoth
	}
	if err := s.ctrl.Rewind(body.Turn, scope); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) forkCheckpoint(w http.ResponseWriter, r *http.Request) {
	var body struct{ Turn int `json:"turn"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	_, err := s.ctrl.Fork(body.Turn)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) summarizeFrom(w http.ResponseWriter, r *http.Request) {
	var body struct{ Turn int `json:"turn"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	if err := s.ctrl.SummarizeFrom(r.Context(), body.Turn); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) summarizeUpTo(w http.ResponseWriter, r *http.Request) {
	var body struct{ Turn int `json:"turn"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", 400)
		return
	}
	if err := s.ctrl.SummarizeUpTo(r.Context(), body.Turn); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

// ── Session ───────────────────────────────────────────────────────────

func (s *Server) renameSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path  string `json:"path"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		http.Error(w, "missing path", 400)
		return
	}
	dir := s.ctrl.SessionDir()
	if dir == "" {
		http.Error(w, "no session directory", 500)
		return
	}
	oldPath := dir + "/" + body.Path
	newPath := dir + "/" + body.Title + ".jsonl"
	if err := os.Rename(oldPath, newPath); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

// ── Slash / TCCA ──────────────────────────────────────────────────────

func (s *Server) slashArgs(w http.ResponseWriter, r *http.Request) {
	input := r.URL.Query().Get("input")
	cfg, _ := config.Load()
	modelRefs := []string{}
	currentModel := ""
	if cfg != nil {
		currentModel = cfg.DefaultModel
		for _, p := range cfg.Providers {
			for _, m := range p.ModelList() {
				modelRefs = append(modelRefs, p.Name+"/"+m)
			}
		}
	}
	ad := control.ArgData{
		Skills:          s.ctrl.Skills(),
		CurrentModel:    currentModel,
		ModelRefs:       modelRefs,
		ConfiguredMCP:   s.ctrl.ConfiguredMCPNames(),
		DisconnectedMCP: s.ctrl.DisconnectedMCPNames(),
	}
	items, from := control.SlashArgItems(input, ad)
	type si struct {
		Label   string `json:"label"`
		Insert  string `json:"insert"`
		Hint    string `json:"hint"`
		Descend bool   `json:"descend"`
	}
	out := make([]si, len(items))
	for i, item := range items {
		out[i] = si{Label: item.Label, Insert: item.Insert, Hint: item.Hint, Descend: item.Descend}
	}
	writeJSON(w, map[string]any{"items": out, "from": from, "total": len(out)})
}

func (s *Server) tccaReport(w http.ResponseWriter, _ *http.Request) {
	r := s.ctrl.TCCAReport()
	resp := map[string]any{
		"l1Size":          r.L1Size,
		"l2Size":          r.L2Size,
		"l3Version":       r.L3Version,
		"l4Messages":      r.L4Messages,
		"savedByCompact":  r.SavedByCompact,
		"savedByFork":     r.SavedByFork,
		"forkCount":       r.ForkCount,
		"savedUsd":        r.SavedUSD,
		"savedLatencyMs":  r.SavedLatencyMs,
		"compactionCount": r.CompactionCount,
		"cacheHitTokens":  r.CacheHitTokens,
		"cacheMissTokens": r.CacheMissTokens,
		"breakCount":      r.BreakCount,
	}
	if shape := s.ctrl.LastCacheShape(); shape != nil {
		resp["lastShape"] = map[string]any{
			"prefixHash":        shape.PrefixHash,
			"systemHash":        shape.SystemHash,
			"toolsHash":         shape.ToolsHash,
			"toolSchemaTokens":  shape.ToolSchemaTokens,
			"logRewriteVersion": shape.LogRewriteVersion,
		}
	}
}

// ── helpers ────────────────────────────────────────────────────────────

func nonEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func emptyDefault(s string) string {
	if s == "default" {
		return ""
	}
	return s
}

func orDef(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
