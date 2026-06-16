package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// IdentityLayer is the L1 cache domain — byte-stable across every turn of a
// session. It is immutable after construction: no Set methods exist, so
// DeepSeek's server-side prefix cache always hits on this segment.
//
// V3.0: extracted from Compiler to enforce single responsibility.
// Compiler now delegates identity operations to IdentityLayer.
type IdentityLayer struct {
	systemPrompt string // fully-assembled L1 system prompt
	context      string // reserved, currently empty
	registry     *tool.Registry
}

// NewIdentityLayer creates an immutable L1 identity.
// systemPrompt is already the fully-assembled prompt (memory, skills,
// language policy etc. composed by boot.Build).
func NewIdentityLayer(systemPrompt string, reg *tool.Registry) *IdentityLayer {
	return &IdentityLayer{
		systemPrompt: systemPrompt,
		registry:     reg,
	}
}

// Bytes returns the complete L1 system prompt bytes — always the same for
// this session, so DeepSeek serves it from its server-side cache.
func (l *IdentityLayer) Bytes() []byte {
	return []byte(l.SystemPrompt())
}

// SystemPrompt returns the combined L1 system prompt.
func (l *IdentityLayer) SystemPrompt() string {
	var parts []string
	if l.systemPrompt != "" {
		parts = append(parts, l.systemPrompt)
	}
	if l.context != "" {
		parts = append(parts, l.context)
	}
	return strings.Join(parts, "\n\n")
}

// Identity returns the raw identity domain content.
func (l *IdentityLayer) Identity() string { return l.systemPrompt }

// Context returns the reserved context domain content.
func (l *IdentityLayer) Context() string { return l.context }

// FilteredSchemas returns tool schemas for only the named tools.
func (l *IdentityLayer) FilteredSchemas(names []string) []provider.ToolSchema {
	if l.registry == nil || len(names) == 0 {
		if l.registry != nil {
			return l.registry.Schemas()
		}
		return nil
	}
	var out []provider.ToolSchema
	for _, name := range names {
		if t, ok := l.registry.Get(name); ok {
			out = append(out, provider.ToolSchema{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Schema(),
			})
		}
	}
	return out
}

// SetRegistry updates the tool registry (called once at boot).
func (l *IdentityLayer) SetRegistry(reg *tool.Registry) { l.registry = reg }

// Registry returns the tool registry.
func (l *IdentityLayer) Registry() *tool.Registry { return l.registry }

// Fork creates a child IdentityLayer for a sub-agent. Identity bytes are
// shared (same bytes → DeepSeek cache hit). The caller appends the sub-agent's
// task body separately.
func (l *IdentityLayer) Fork() *IdentityLayer {
	return &IdentityLayer{
		systemPrompt: l.systemPrompt,
		context:      l.context,
		registry:     l.registry,
	}
}

// ContentHash returns a hex-encoded SHA-256 of the L1 system prompt.
// V3.3: used for cross-session cache validation — if the hash matches the
// stored value, the disk cache from the previous session is still valid.
func (l *IdentityLayer) ContentHash() string {
	h := sha256.Sum256([]byte(l.SystemPrompt()))
	return hex.EncodeToString(h[:])
}

// SaveHash persists the L1 content hash to dir/identity.hash.
// V3.3: called at session start so the next session can validate cache warmth.
func (l *IdentityLayer) SaveHash(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "identity.hash"), []byte(l.ContentHash()), 0644)
}

// LoadAndCompareHash reads the stored L1 hash from dir/identity.hash and
// compares it with the current hash. Returns true if they match (cache warm).
// V3.3: non-existent or mismatched hash → cache cold.
func (l *IdentityLayer) LoadAndCompareHash(dir string) bool {
	if dir == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(dir, "identity.hash"))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == l.ContentHash()
}
