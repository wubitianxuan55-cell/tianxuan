package main

import "tianxuan/internal/tool"

// BuiltinToolView is one built-in tool for the right-drawer "工具" tab.
// Description is the display line (compact single-line description when the
// tool ships one, otherwise the full description), FullDescription keeps the
// complete backend text for hover/detail.
type BuiltinToolView struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	FullDescription string `json:"fullDescription"`
	ReadOnly        bool   `json:"readOnly"`
}

// Tools lists every compile-time built-in tool, sorted by name. Unlike
// Capabilities (MCP servers + skills), this is the fixed kernel tool set — the
// data-driven replacement for the frontend's hard-coded tool table.
func (a *App) Tools() []BuiltinToolView {
	builtins := tool.Builtins()
	out := make([]BuiltinToolView, 0, len(builtins))
	for _, t := range builtins {
		desc := t.Description()
		if cd, ok := t.(tool.CompactDescriptor); ok {
			if compact := cd.CompactDescription(); compact != "" {
				desc = compact
			}
		}
		out = append(out, BuiltinToolView{
			Name:            t.Name(),
			Description:     desc,
			FullDescription: t.Description(),
			ReadOnly:        t.ReadOnly(),
		})
	}
	return out
}
