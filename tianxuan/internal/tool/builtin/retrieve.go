package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"tianxuan/internal/ccr"
	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(retrieveTool{}) }

// SetCCRDir configures the CCR storage directory. Call once from boot.
func SetCCRDir(dir string) { ccr.SetDir(dir) }

type retrieveTool struct{}

func (retrieveTool) Name() string { return "retrieve" }

func (retrieveTool) Description() string {
	return "Retrieve the original full content of a previously compressed tool output by its retrieval key (8-char hash). Use when a tool result shows a [compressed ccr:KEY …] marker and you need the full data."
}

func (retrieveTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"key": {"type": "string", "description": "The retrieval key (8-char hex hash) from a [compressed ccr:KEY, ...] marker."}
		},
		"required": ["key"]
	}`)
}

func (retrieveTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("retrieve: invalid args: %w", err)
	}
	if p.Key == "" {
		return "", fmt.Errorf("retrieve: key is required")
	}
	v := ccr.Read(p.Key)
	if v == "" {
		return fmt.Sprintf("retrieve: key %q not found (content may be from a different session or expired)", p.Key), nil
	}
	return v, nil
}

func (retrieveTool) ReadOnly() bool { return true }
