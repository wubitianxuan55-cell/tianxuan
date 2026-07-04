package agent

import "tianxuan/internal/agent/cache"

// Re-exported types and functions from the cache sub-package.
type ToolCatalogFingerprint = cache.ToolCatalogFingerprint
type ToolCatalogDriftKind = cache.ToolCatalogDriftKind

var (
	BuildToolCatalogFingerprint = cache.BuildToolCatalogFingerprint
	DetectToolCatalogDrift      = cache.DetectToolCatalogDrift
	FormatDriftReason           = cache.FormatDriftReason
	NormalizeToolSchemas        = cache.NormalizeToolSchemas
	CanonicalizeValue           = cache.CanonicalizeValue
)

// toolCache constructor aliased from cache.New.
var newToolCache = cache.New
