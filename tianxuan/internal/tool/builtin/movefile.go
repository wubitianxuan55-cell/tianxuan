package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(moveFile{}) }

// moveFile moves or renames a file. workDir, when non-empty, is the directory
// a relative path resolves against (see resolveIn).
type moveFile struct {
	roots   []string
	workDir string
}

func (moveFile) Name() string { return "move_file" }

func (moveFile) Description() string {
	return "Move or rename a file from source_path to destination_path. Creates the destination parent directory as needed. Use instead of shell mv for workspace-confined moves."
}

func (moveFile) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "source_path":{"type":"string","description":"Existing file path to move"},
  "destination_path":{"type":"string","description":"Destination file path; must not already exist"}
},
"required":["source_path","destination_path"]
}`)
}

func (moveFile) ReadOnly() bool { return false }

func (m moveFile) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Source      string `json:"source_path"`
		Destination string `json:"destination_path"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.Source == "" || p.Destination == "" {
		return "", fmt.Errorf("source_path and destination_path are required")
	}

	src := resolveIn(m.workDir, p.Source)
	dst := resolveIn(m.workDir, p.Destination)

	if err := confine(m.roots, src); err != nil {
		return "", err
	}
	if err := confine(m.roots, dst); err != nil {
		return "", err
	}

	// Create destination parent directory.
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	if err := os.Rename(src, dst); err != nil {
		return "", fmt.Errorf("move %s → %s: %w", src, dst, err)
	}

	return fmt.Sprintf("moved %s → %s", src, dst), nil
}
