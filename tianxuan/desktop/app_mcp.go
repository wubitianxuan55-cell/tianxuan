package main

import (
	"fmt"
	"sort"

	"tianxuan/internal/config"
	"tianxuan/internal/control"
	"tianxuan/internal/plugin"
)

// CapabilitiesView is the MCP & Skills drawer's data: connected/failed MCP
// servers and the discoverable skills, the GUI counterpart to `/mcp` + `/skill`.
type CapabilitiesView struct {
	Servers []ServerView `json:"servers"`
	Skills  []SkillView  `json:"skills"`
}

// ServerView is one MCP server for the drawer. Status is "connected" (with
// tool/prompt/resource counts) or "failed" (with the connection error).
type ServerView struct {
	Name      string     `json:"name"`
	Transport string     `json:"transport"`
	Status    string     `json:"status"`
	Tools     int        `json:"tools"`
	Prompts   int        `json:"prompts"`
	Resources int        `json:"resources"`
	Error     string     `json:"error,omitempty"`
	ToolList  []ToolView `json:"toolList,omitempty"`
}

type ToolView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SkillView is one discoverable skill for the drawer.
type SkillView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
	RunAs       string `json:"runAs"`
}

// MCPServerInput is the drawer's "add server" form. Transport is "stdio" (Command
// + Args + Env) or "http"/"sse" (URL). Mirrors config.PluginEntry's writable shape.
type MCPServerInput struct {
	Name      string            `json:"name"`
	Transport string            `json:"transport"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	URL       string            `json:"url"`
	Env       map[string]string `json:"env"`
}

// Capabilities projects the session's MCP servers (connected + failed) and skills
// for the MCP & Skills drawer. Non-nil slices so the frontend can map over them.
func (a *App) Capabilities() CapabilitiesView {
	out := CapabilitiesView{Servers: []ServerView{}, Skills: []SkillView{}}
	a.mu.RLock()
	ctrl := a.ctrl
	disabled := make(map[string]ServerView, len(a.disabledMCP))
	for name, s := range a.disabledMCP {
		disabled[name] = s
	}
	order := append([]string(nil), a.mcpOrder...)
	a.mu.RUnlock()
	if ctrl == nil {
		return out
	}
	seen := map[string]bool{}
	connected := map[string]bool{}
	retainedDisabled := map[string]ServerView{}
	codegraphConfigured := false
	if h := ctrl.Host(); h != nil {
		for _, s := range h.Servers() {
			seen[s.Name] = true
			connected[s.Name] = true
			out.Servers = append(out.Servers, ServerView{
				Name: s.Name, Transport: s.Transport, Status: "connected",
				Tools: s.Tools, Prompts: s.Prompts, Resources: s.Resources,
				ToolList: pluginToolsToView(s.ToolList),
			})
		}
		for _, f := range h.Failures() {
			seen[f.Name] = true
			out.Servers = append(out.Servers, ServerView{
				Name: f.Name, Transport: f.Transport, Status: "failed", Error: f.Error,
			})
		}
	}
	// Configured servers that are neither connected nor failed are toggled off
	// (disconnected this session, or auto_start=false) — shown with an off switch.
	if cfg, err := config.Load(); err == nil {
		codegraphConfigured = cfg.Codegraph.Enabled
		for _, p := range cfg.Plugins {
			if seen[p.Name] {
				continue
			}
			tt := p.Type
			if tt == "" {
				tt = "stdio"
			}
			if s, ok := disabled[p.Name]; ok {
				s.Status = "disabled"
				s.Transport = tt
				s.Error = ""
				out.Servers = append(out.Servers, s)
				retainedDisabled[p.Name] = s
				seen[p.Name] = true
				delete(disabled, p.Name)
				continue
			}
			out.Servers = append(out.Servers, ServerView{Name: p.Name, Transport: tt, Status: "disabled"})
			seen[p.Name] = true
		}
	}
	for name, s := range disabled {
		if seen[name] {
			continue
		}
		if name != "codegraph" || !codegraphConfigured {
			continue
		}
		s.Status = "disabled"
		s.Error = ""
		out.Servers = append(out.Servers, s)
		retainedDisabled[name] = s
	}
	out.Servers = orderServerViews(out.Servers, order)

	a.mu.Lock()
	for name := range connected {
		delete(retainedDisabled, name)
	}
	a.disabledMCP = retainedDisabled
	a.mcpOrder = mergeServerOrder(a.mcpOrder, out.Servers)
	a.mu.Unlock()

	for _, s := range ctrl.Skills() {
		out.Skills = append(out.Skills, SkillView{
			Name: s.Name, Description: s.Description,
			Scope: string(s.Scope), RunAs: string(s.RunAs),
		})
	}
	return out
}

// AddMCPServer connects a server live and persists it to config (Customize → MCP →
// Add). Returns the number of tools it exposed.
func (a *App) AddMCPServer(in MCPServerInput) (int, error) {
	if a.ctrl == nil {
		return 0, fmt.Errorf("no active session")
	}
	return a.ctrl.AddMCPServer(config.PluginEntry{
		Name:    in.Name,
		Type:    in.Transport,
		Command: in.Command,
		Args:    in.Args,
		URL:     in.URL,
		Env:     in.Env,
	})
}

// RemoveMCPServer disconnects a live server and drops it from config (the row's ✕).
func (a *App) RemoveMCPServer(name string) error {
	if a.ctrl == nil {
		return fmt.Errorf("no active session")
	}
	_, err := a.ctrl.RemoveMCPServer(name)
	if err == nil {
		a.mu.Lock()
		delete(a.disabledMCP, name)
		a.mcpOrder = removeServerOrder(a.mcpOrder, name)
		a.mu.Unlock()
	}
	return err
}

// RetryMCPServer reconnects a configured server that failed or was disconnected,
// without touching config (the failed row's retry button).
func (a *App) RetryMCPServer(name string) error {
	if a.ctrl == nil {
		return fmt.Errorf("no active session")
	}
	_, err := a.ctrl.ConnectConfiguredMCPServer(name)
	return err
}

// SetMCPServerEnabled is the connector toggle: on reconnects a configured server
// for this session, off disconnects it (config untouched either way — like Claude
// Code's per-conversation enable/disable, it resets on the next session start).
func (a *App) SetMCPServerEnabled(name string, enabled bool) error {
	if a.ctrl == nil {
		return fmt.Errorf("no active session")
	}
	if enabled {
		_, err := a.ctrl.ConnectConfiguredMCPServer(name)
		if err == nil {
			a.mu.Lock()
			delete(a.disabledMCP, name)
			a.mu.Unlock()
		}
		return err
	}
	if s, ok := findMCPServerView(a.ctrl, name); ok {
		s.Status = "disabled"
		s.Error = ""
		a.mu.Lock()
		if a.disabledMCP == nil {
			a.disabledMCP = map[string]ServerView{}
		}
		a.disabledMCP[name] = s
		a.mcpOrder = mergeServerOrder(a.mcpOrder, []ServerView{s})
		a.mu.Unlock()
	}
	a.ctrl.DisconnectMCPServer(name)
	return nil
}

func findMCPServerView(ctrl *control.Controller, name string) (ServerView, bool) {
	if ctrl == nil || ctrl.Host() == nil {
		return ServerView{}, false
	}
	for _, s := range ctrl.Host().Servers() {
		if s.Name == name {
			return ServerView{
				Name: s.Name, Transport: s.Transport, Status: "connected",
				Tools: s.Tools, Prompts: s.Prompts, Resources: s.Resources,
				ToolList: pluginToolsToView(s.ToolList),
			}, true
		}
	}
	for _, f := range ctrl.Host().Failures() {
		if f.Name == name {
			return ServerView{Name: f.Name, Transport: f.Transport, Status: "failed", Error: f.Error}, true
		}
	}
	return ServerView{}, false
}

func pluginToolsToView(tools []plugin.ToolInfo) []ToolView {
	if len(tools) == 0 {
		return nil
	}
	out := make([]ToolView, 0, len(tools))
	for _, t := range tools {
		out = append(out, ToolView{Name: t.Name, Description: t.Description})
	}
	return out
}

func orderServerViews(servers []ServerView, order []string) []ServerView {
	pos := make(map[string]int, len(order))
	for i, name := range order {
		pos[name] = i
	}
	sort.SliceStable(servers, func(i, j int) bool {
		pi, iok := pos[servers[i].Name]
		pj, jok := pos[servers[j].Name]
		switch {
		case iok && jok:
			return pi < pj
		case iok:
			return true
		case jok:
			return false
		default:
			return false
		}
	})
	return servers
}

func mergeServerOrder(order []string, servers []ServerView) []string {
	seen := make(map[string]bool, len(order)+len(servers))
	next := make([]string, 0, len(order)+len(servers))
	for _, name := range order {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		next = append(next, name)
	}
	for _, s := range servers {
		if s.Name == "" || seen[s.Name] {
			continue
		}
		seen[s.Name] = true
		next = append(next, s.Name)
	}
	return next
}

func removeServerOrder(order []string, name string) []string {
	if name == "" || len(order) == 0 {
		return order
	}
	next := order[:0]
	for _, n := range order {
		if n != name {
			next = append(next, n)
		}
	}
	return next
}
