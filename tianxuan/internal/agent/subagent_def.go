// Package agent — subagent_def.go: structured sub-agent definition.
// Pattern distilled from Gemini CLI's LocalAgentDefinition (agents/types.ts).
//
// SubagentDefinition provides a type-safe representation of the parameters a
// sub-agent needs — model, prompt, tool whitelist, step budget, write paths,
// and output schema. It is used by TaskTool.Execute() to construct the
// sub-agent Options, and can be serialized for config-file or TIANXUAN.md
// declarations in the future.
package agent

import (
	"encoding/json"

	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// SubagentDefinition is a typed specification for spawning a sub-agent. It
// mirrors the task tool's JSON args but exposes them as a structured Go type
// so callers can construct sub-agents programmatically.
//
// Zero fields inherit sensible defaults from the parent agent: empty Model
// uses the parent's provider, empty Tools inherits all (minus meta-tools), 0
// MaxSteps means unlimited, nil WritePaths allows all writes.
type SubagentDefinition struct {
	// Prompt is the task for the sub-agent — mandatory.
	Prompt string `json:"prompt"`

	// Description is an optional short label for UI and logging.
	Description string `json:"description,omitempty"`

	// Model overrides the LLM model for this sub-agent. When empty the parent
	// model is used. This enables cost optimisation: cheap models for simple
	// exploration, expensive models for code generation.
	Model string `json:"model,omitempty"`

	// Tools is the whitelist of tool names available to the sub-agent.
	// When nil or empty, all parent tools (minus meta-tools like task/skill)
	// are inherited. Meta-tool exclusion is always enforced.
	Tools []string `json:"tools,omitempty"`

	// MaxSteps caps the tool-call rounds. 0 means unlimited (sub-agent runs
	// until it produces a final answer or times out).
	MaxSteps int `json:"max_steps,omitempty"`

	// WritePaths restricts file writes to these workspace-relative
	// directories. When nil or empty, the sub-agent inherits full write
	// access. Primarily used for read_only sub-agents — pass nil
	// (and ReadOnly=true) to deny all writes.
	WritePaths []string `json:"write_paths,omitempty"`

	// ReadOnly, when true, spawns the sub-agent without any write-capable
	// tools. Used for exploration/research sub-agents.
	ReadOnly bool `json:"read_only,omitempty"`

	// Background, when true, runs the sub-agent asynchronously (returns a
	// job ID immediately). The parent can collect the result with wait.
	Background bool `json:"run_in_background,omitempty"`

	// OutputSchema is an optional JSON Schema the sub-agent's final answer
	// must conform to. When set, the parent validates the result is
	// well-formed JSON before returning it.
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

// SubagentResult is the structured output from a completed sub-agent run.
type SubagentResult struct {
	// Text is the final assistant message.
	Text string `json:"text"`

	// Usage is the accumulated token usage across the sub-agent's turns.
	Usage *provider.Usage `json:"usage,omitempty"`

	// JobID is non-empty for background sub-agents — use wait to collect.
	JobID string `json:"job_id,omitempty"`
}

// SubagentBuilder constructs Options and tool registry for a SubagentDefinition.
// It resolves defaults from the parent agent's environment.
type SubagentBuilder struct {
	parentReg *tool.Registry
	gate      Gate
}

// NewSubagentBuilder creates a builder wired to the parent's tool registry
// and permission gate.
func NewSubagentBuilder(parentReg *tool.Registry, gate Gate) *SubagentBuilder {
	return &SubagentBuilder{parentReg: parentReg, gate: gate}
}

// BuildRegistry creates a filtered tool registry for the sub-agent.
// Meta-tools (task, skill, explore, etc.) are always excluded.
// When def.ReadOnly is true, only ReadOnly tools are included.
func (b *SubagentBuilder) BuildRegistry(def SubagentDefinition) *tool.Registry {
	exclude := SubagentMetaTools()
	sub := FilterRegistry(b.parentReg, def.Tools, exclude...)
	if def.ReadOnly {
		// Further filter: keep only ReadOnly tools
		filtered := tool.NewRegistry()
		for _, name := range sub.Names() {
			if t, ok := sub.Get(name); ok && t.ReadOnly() {
				filtered.Add(t)
			}
		}
		return filtered
	}
	return sub
}

// BuildOptions constructs Agent Options from the definition.
func (b *SubagentBuilder) BuildOptions(def SubagentDefinition) Options {
	return Options{
		MaxSteps:    def.MaxSteps,
		Gate:        b.gate,
		Compaction:  CompactionConfig{},
	}
}

// BuildTaskArgs marshals the definition into task-tool-compatible JSON args.
func (def SubagentDefinition) BuildTaskArgs() json.RawMessage {
	type raw struct {
		Prompt          string          `json:"prompt"`
		Description     string          `json:"description,omitempty"`
		Tools           []string        `json:"tools,omitempty"`
		MaxSteps        int             `json:"max_steps,omitempty"`
		RunInBackground bool            `json:"run_in_background,omitempty"`
		OutputSchema    json.RawMessage `json:"output_schema,omitempty"`
	}
	r := raw{
		Prompt:          def.Prompt,
		Description:     def.Description,
		Tools:           def.Tools,
		MaxSteps:        def.MaxSteps,
		RunInBackground: def.Background,
		OutputSchema:    def.OutputSchema,
	}
	b, _ := json.Marshal(r)
	return b
}
