// Package cache implements the four-domain prefix architecture.
// Each domain is independently byte-stable so DeepSeek's server-side
// context cache can serve them independently when only later domains change.
package cache

import (
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// Compiler assembles the L1 cache domain into the byte-stable prefix.
// V3.0: Compiler delegates identity operations to IdentityLayer.
// Deprecated: use context.ContextManager.Identity() instead.
type Compiler struct {
	l1 *IdentityLayer
}

// New creates a Compiler. basePrompt is the fully-assembled system prompt.
func New(basePrompt string, reg *tool.Registry) *Compiler {
	return &Compiler{
		l1: NewIdentityLayer(basePrompt, reg),
	}
}

// SystemPrompt returns the combined L1 system prompt.
func (c *Compiler) SystemPrompt() string { return c.l1.SystemPrompt() }

// Identity returns the identity domain content.
func (c *Compiler) Identity() string { return c.l1.Identity() }

// Context returns the context domain content.
func (c *Compiler) Context() string { return c.l1.Context() }

// FilteredSchemas returns tool schemas for only the named tools.
func (c *Compiler) FilteredSchemas(names []string) []provider.ToolSchema {
	return c.l1.FilteredSchemas(names)
}

// SetRegistry updates the tool registry.
func (c *Compiler) SetRegistry(reg *tool.Registry) { c.l1.SetRegistry(reg) }

// Fork creates a child Compiler for a sub-agent.
func (c *Compiler) Fork() *Compiler {
	return &Compiler{l1: c.l1.Fork()}
}

// Registry returns the tool registry.
func (c *Compiler) Registry() *tool.Registry { return c.l1.Registry() }

// IdentityLayer returns the underlying L1 identity.
func (c *Compiler) IdentityLayer() *IdentityLayer { return c.l1 }
