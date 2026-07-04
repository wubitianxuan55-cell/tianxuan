// Package agent — CacheShape provides per-request prefix diagnostics.
// After every LLM completion, CaptureShape records the structural
// fingerprint of the request prefix; CompareShape explains why a cache
// miss happened by diffing against the previous shape.
//
// Design adopted from Reasonix: one PrefixShape per turn replaces three
// independent hash systems (verifyPrefix SHA-256 + cacheBreakDetector FNV-1a
// + computeCacheShape SHA-256).
package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"tianxuan/internal/provider"
)

// PrefixShape hashes the portions of the request prefix that influence
// provider-side prompt-cache reuse.
type PrefixShape struct {
	SystemHash        string
	ToolsHash         string
	PrefixHash        string
	LogRewriteVersion int
	ToolSchemaTokens  int
}

// CacheDiagnostics describes the outcome of a prefix-shape comparison.
type CacheDiagnostics struct {
	PrefixHash          string
	PrefixChanged       bool
	PrefixChangeReasons []string
	SystemHash          string
	ToolsHash           string
	LogRewriteVersion   int
	ToolSchemaTokens    int
	CacheMissTokens     int
	CacheHitTokens      int
}

func shortHash(v interface{}) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

// CaptureShape takes a snapshot of the current prefix state.
func (a *AgentRunner) CaptureShape() PrefixShape {
	sysPrompt := a.systemPrompt()
	tools := a.tools.Schemas()
	return captureShape(sysPrompt, tools, a.session.RewriteVersion())
}

func captureShape(systemPrompt string, schemas []provider.ToolSchema, rewriteVersion int) PrefixShape {
	normalized := NormalizeToolSchemas(schemas)
	toolsJSON, _ := json.Marshal(normalized)
	return PrefixShape{
		SystemHash:        shortHash(systemPrompt),
		ToolsHash:         shortHash(string(toolsJSON)),
		PrefixHash:        shortHash(map[string]interface{}{"system": systemPrompt, "tools": string(toolsJSON)}),
		LogRewriteVersion: rewriteVersion,
		ToolSchemaTokens:  estimateTokens(string(toolsJSON)),
	}
}

// CompareShape returns diagnostics describing what changed between two shapes.
func CompareShape(prev, cur PrefixShape, usage *provider.Usage) CacheDiagnostics {
	reasons := []string{}
	if prev.SystemHash != "" && prev.SystemHash != cur.SystemHash {
		reasons = append(reasons, "system")
	}
	if prev.ToolsHash != "" && prev.ToolsHash != cur.ToolsHash {
		reasons = append(reasons, "tools")
	}
	if prev.LogRewriteVersion != cur.LogRewriteVersion {
		reasons = append(reasons, "log_rewrite")
	}
	var miss, hit int
	if usage != nil {
		miss = usage.CacheMissTokens
		hit = usage.CacheHitTokens
	}
	return CacheDiagnostics{
		PrefixHash:          cur.PrefixHash,
		PrefixChanged:       len(reasons) > 0,
		PrefixChangeReasons: reasons,
		SystemHash:          cur.SystemHash,
		ToolsHash:           cur.ToolsHash,
		LogRewriteVersion:   cur.LogRewriteVersion,
		ToolSchemaTokens:    cur.ToolSchemaTokens,
		CacheMissTokens:     miss,
		CacheHitTokens:      hit,
	}
}

// estimateTokens gives a rough token count from byte length.
func estimateTokens(s string) int {
	if len(s) == 0 {
		return 0
	}
	return (len(s) + 3) / 4
}

// Format returns a compact human-readable representation of the diagnostics.
func (d CacheDiagnostics) Format() string {
	if !d.PrefixChanged {
		return fmt.Sprintf("cache prefix stable [hit=%d miss=%d tools=%dtok]", d.CacheHitTokens, d.CacheMissTokens, d.ToolSchemaTokens)
	}
	return fmt.Sprintf("cache prefix changed: %v [hit=%d miss=%d tools=%dtok]", d.PrefixChangeReasons, d.CacheHitTokens, d.CacheMissTokens, d.ToolSchemaTokens)
}

// CacheShape is a deprecated alias for PrefixShape (V8.18).
// Kept for backward compatibility with controller.go / serve_handlers.go.
type CacheShape = PrefixShape

// LastCacheShape is a deprecated stub (V8.18 Phase 3).
// Cache diagnostics are now real-time Notices via CompareShape.
func (a *AgentRunner) LastCacheShape() *CacheShape {
	return nil
}
