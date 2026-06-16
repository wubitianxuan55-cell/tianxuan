package lsp

import (
	"context"
	"encoding/json"
	"fmt"

	"tianxuan/internal/tool"
)

// Tools adapts the manager's read-only queries to the tool.Tool interface. They
// report ReadOnly=true, so a batch of them rides the agent's parallel dispatch
// and shares one server per language.
func Tools(m *Manager) []tool.Tool {
	return []tool.Tool{
		posTool{m, "lsp_definition", "Jump to where a symbol is defined. Give the file, the 1-based line the symbol appears on, and the symbol text itself.", m.Definition},
		posTool{m, "lsp_references", "List every reference to a symbol across the workspace. Give the file, the 1-based line, and the symbol text.", m.References},
		posTool{m, "lsp_hover", "Show the type signature and documentation for a symbol. Give the file, the 1-based line, and the symbol text.", m.Hover},
		completionTool{m},
		renameTool{m},
		diagTool{m},
	}
}

type posArgs struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol"`
}

type posTool struct {
	m    *Manager
	name string
	desc string
	fn   func(context.Context, string, int, string) (string, error)
}

func (t posTool) Name() string        { return t.name }
func (t posTool) Description() string { return t.desc }
func (t posTool) ReadOnly() bool      { return true }
func (t posTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "file":{"type":"string","description":"Path to the source file, relative to the workspace root or absolute."},
  "line":{"type":"integer","description":"1-based line number the symbol appears on."},
  "symbol":{"type":"string","description":"The exact symbol text on that line, e.g. \"executeBatch\". Used to locate the column."}
},
"required":["file","line","symbol"]
}`)
}

func (t posTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p posArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.File == "" || p.Symbol == "" || p.Line < 1 {
		return "", fmt.Errorf("file, line (>=1) and symbol are required")
	}
	return t.fn(ctx, p.File, p.Line, p.Symbol)
}

type diagTool struct{ m *Manager }

func (diagTool) Name() string   { return "lsp_diagnostics" }
func (diagTool) ReadOnly() bool { return true }
func (diagTool) Description() string {
	return "Report compiler/linter diagnostics (errors, warnings) for a file from its language server. Use after editing to check the change compiles."
}
func (diagTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"file":{"type":"string","description":"Path to the source file, relative to the workspace root or absolute."}},"required":["file"]}`)
}

func (t diagTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.File == "" {
		return "", fmt.Errorf("file is required")
	}
	return t.m.Diagnostics(ctx, p.File)
}

// --- lsp_completion (read-only, returns completion suggestions) ---

type completionTool struct{ m *Manager }

func (completionTool) Name() string        { return "lsp_completion" }
func (completionTool) ReadOnly() bool      { return true }
func (completionTool) Description() string {
	return "Get code completion suggestions at a cursor position. Give the file, the 1-based line, and the symbol text to locate the cursor."
}
func (completionTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "file":{"type":"string","description":"Path to the source file, relative to the workspace root or absolute."},
  "line":{"type":"integer","description":"1-based line number the cursor is on."},
  "symbol":{"type":"string","description":"The exact text around the cursor on that line, used to locate the column."}
},
"required":["file","line","symbol"]
}`)
}

func (t completionTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p posArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.File == "" || p.Symbol == "" || p.Line < 1 {
		return "", fmt.Errorf("file, line (>=1) and symbol are required")
	}
	return t.m.Completion(ctx, p.File, p.Line, p.Symbol)
}

// --- lsp_rename (writer: actually renames symbols across files) ---

type renameArgs struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Symbol  string `json:"symbol"`
	NewName string `json:"new_name"`
}

type renameTool struct{ m *Manager }

func (renameTool) Name() string        { return "lsp_rename" }
func (renameTool) ReadOnly() bool      { return false }
func (renameTool) Description() string {
	return "Rename a symbol across the workspace. Specify the file, line, symbol text, and the new name. WARNING: modifies files."
}
func (renameTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "file":{"type":"string","description":"Path to the source file, relative to the workspace root or absolute."},
  "line":{"type":"integer","description":"1-based line number the symbol appears on."},
  "symbol":{"type":"string","description":"The exact symbol text on that line, used to locate the column."},
  "new_name":{"type":"string","description":"The new name for the symbol."}
},
"required":["file","line","symbol","new_name"]
}`)
}

func (t renameTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p renameArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.File == "" || p.Symbol == "" || p.Line < 1 || p.NewName == "" {
		return "", fmt.Errorf("file, line (>=1), symbol and new_name are required")
	}
	return t.m.Rename(ctx, p.File, p.Line, p.Symbol, p.NewName)
}
