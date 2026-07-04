package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(multiEdit{}) }

// multiEdit applies a batch of edits to one file. roots confines the target to
// the workspace when non-empty (see writeFile); workDir, when non-empty, is the
// directory a relative path resolves against (see resolveIn).
type multiEdit struct {
	roots   []string
	workDir string
}

// editStep is one edit in a multi_edit operation. Mirrors edit_file's args
// plus a per-step replace_all toggle so a single call can mix targeted and
// sweep replacements (e.g. rename a function with replace_all, then patch
// one specific call site with a unique-match edit).
type editStep struct {
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func (multiEdit) Name() string { return "multi_edit" }

func (multiEdit) Description() string {
	return "Apply a list of edits to a single file atomically: each edit runs against the result of the previous one, all in memory; the file is rewritten only if every edit succeeds. Line endings are auto-adapted (see edit_file). Cheaper and safer than chaining edit_file calls — a failure in step 3 leaves the file untouched instead of half-edited."
}

func (multiEdit) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "path":{"type":"string","description":"File path"},
  "edits":{
    "type":"array",
    "minItems":1,
    "description":"Ordered edits. Each step sees the file as left by the previous step.",
    "items":{
      "type":"object",
      "properties":{
        "old_string":{"type":"string","description":"Exact text to find. Without replace_all, must match exactly once. Line endings auto-adapted."},
        "new_string":{"type":"string","description":"Replacement text (empty deletes). Line endings auto-adapted."},
        "replace_all":{"type":"boolean","description":"Replace every occurrence instead of requiring uniqueness."}
      },
      "required":["old_string","new_string"]
    }
  }
},
"required":["path","edits"]
}`)
}

func (multiEdit) ReadOnly() bool { return false }

func (multiEdit) CompactDescription() string { return compactDesc["multi_edit"] }
func (multiEdit) CompactSchema() json.RawMessage   { return compactSchema["multi_edit"] }

func (m multiEdit) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path  string     `json:"path"`
		Edits []editStep `json:"edits"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if len(p.Edits) == 0 {
		return "", fmt.Errorf("edits must not be empty")
	}
	p.Path = resolveIn(m.workDir, p.Path)
	if err := confine(m.roots, p.Path); err != nil {
		return "", err
	}

	content, enc, err := readFileEncoded(p.Path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", p.Path, err)
	}

	// Apply edits in order against the running in-memory buffer.
	applied := 0
	for i, step := range p.Edits {
		if step.OldString == "" {
			return "", fmt.Errorf("edit %d: old_string is required", i+1)
		}
		result := applyOldStringEdit(content, step.OldString, step.NewString, step.ReplaceAll)
		if result.matches == 0 {
			return "", fmt.Errorf("edit %d: %w", i+1, oldStringNotFoundError(p.Path, step.OldString, content))
		}
		if !step.ReplaceAll && result.matches > 1 {
			return "", fmt.Errorf("edit %d: %w", i+1, oldStringNotUniqueError(p.Path, step.OldString, content, result.matches, false))
		}
		content = result.updated
		applied += result.applied
	}

	// Preserve original file permissions (e.g. executable scripts).
	fi, err := os.Stat(p.Path)
	mode := os.FileMode(0o644)
	if err == nil {
		mode = fi.Mode().Perm()
	}
	if err := writeFileEncoded(p.Path, content, enc, mode); err != nil {
		return "", fmt.Errorf("write %s: %w", p.Path, err)
	}
	return fmt.Sprintf("multi_edit %s: %d edits applied (%d total replacements)", p.Path, len(p.Edits), applied), nil
}
