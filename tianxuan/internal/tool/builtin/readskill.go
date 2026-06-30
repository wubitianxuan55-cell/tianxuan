package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"tianxuan/internal/tool"
)

// readSkill lets the agent dynamically read a skill's frontmatter and body at
// runtime. This decouples the skill index (which lists available skills in the
// system prompt) from the full skill content, saving prompt tokens while still
// letting the agent load any skill on demand.
// Skill resolution is injected via SetResolver to avoid circular imports.
type readSkill struct {
	resolve func(name string) (string, error)
}

func (readSkill) Name() string        { return "read_skill" }
func (readSkill) Description() string { return "读取指定技能(skill)的完整内容(前置元数据+正文)" }
func (readSkill) ReadOnly() bool      { return true }

func (readSkill) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {"type": "string", "description": "技能名称(如 'explore', 'tdd', 'review')"}
  },
  "required": ["name"]
}`)
}

func (r readSkill) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if in.Name == "" {
		return "", fmt.Errorf("read_skill requires a 'name' parameter")
	}
	if r.resolve == nil {
		return "", fmt.Errorf("read_skill resolver not configured")
	}
	content, err := r.resolve(in.Name)
	if err != nil {
		return "", fmt.Errorf("skill not found: %s", in.Name)
	}
	return content, nil
}

func (readSkill) CompactDescription() string { return compactDesc["read_skill"] }
func (readSkill) CompactSchema() json.RawMessage   { return compactSchema["read_skill"] }

func init() {
	tool.RegisterBuiltin(readSkill{})
}

// readSkillResolver is wired during boot to resolve skill names to content.
var readSkillResolver func(name string) (string, error)

func (r *readSkill) setResolve(fn func(name string) (string, error)) { r.resolve = fn }

// WireReadSkillResolver injects the skill resolution callback. Call from boot.
func WireReadSkillResolver(resolve func(name string) (string, error)) {
	readSkillResolver = resolve
	// Update all registered readSkill instances.
	for _, t := range tool.Builtins() {
		if rs, ok := t.(*readSkill); ok {
			rs.resolve = resolve
		}
	}
}
