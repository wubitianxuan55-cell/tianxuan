package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"tianxuan/internal/provider"
)

// ToolCatalogFingerprint is a canonical SHA256 fingerprint of a tool registry.
type ToolCatalogFingerprint struct {
	Fingerprint string
	ToolCount   int
	ToolNames   []string
	ToolHashes  map[string]string
}

// ToolCatalogDriftKind describes the type of tool-set change.
type ToolCatalogDriftKind string

const (
	DriftNone     ToolCatalogDriftKind = "none"
	DriftAdditive ToolCatalogDriftKind = "additive"
	DriftBreaking ToolCatalogDriftKind = "breaking"
)

// BuildToolCatalogFingerprint computes a canonical fingerprint.
func BuildToolCatalogFingerprint(tools []provider.ToolSchema) ToolCatalogFingerprint {
	canonical := NormalizeToolSchemas(tools)
	names := make([]string, len(canonical))
	hashes := make(map[string]string, len(canonical))
	for i, t := range canonical {
		names[i] = t.name
		hashes[t.name] = hashToolSpec(t)
	}
	return ToolCatalogFingerprint{
		Fingerprint: hashToolList(canonical),
		ToolCount:   len(canonical),
		ToolNames:   names,
		ToolHashes:  hashes,
	}
}

// DetectToolCatalogDrift compares two fingerprints.
func DetectToolCatalogDrift(prev, curr ToolCatalogFingerprint) ToolCatalogDriftKind {
	if prev.Fingerprint == curr.Fingerprint {
		return DriftNone
	}
	prevSet := make(map[string]bool, len(prev.ToolNames))
	for _, n := range prev.ToolNames {
		prevSet[n] = true
	}
	for _, n := range prev.ToolNames {
		if currHash, ok := curr.ToolHashes[n]; !ok {
			return DriftBreaking
		} else if currHash != prev.ToolHashes[n] {
			return DriftBreaking
		}
	}
	if curr.ToolCount > prev.ToolCount {
		return DriftAdditive
	}
	return DriftBreaking
}

type canonicalToolSpec struct {
	name        string
	description string
	schema      json.RawMessage
}

// NormalizeToolSchemas sorts and canonicalizes tool schemas for fingerprinting.
func NormalizeToolSchemas(tools []provider.ToolSchema) []canonicalToolSpec {
	out := make([]canonicalToolSpec, len(tools))
	for i, t := range tools {
		out[i] = canonicalToolSpec{
			name:        t.Name,
			description: t.Description,
			schema:      canonicalizeJSON(t.Parameters),
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// CanonicalizeValue recursively sorts JSON object keys for canonical output.
func CanonicalizeValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[k] = CanonicalizeValue(val)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, val := range v {
			out[i] = CanonicalizeValue(val)
		}
		return out
	default:
		return v
	}
}

func canonicalizeJSON(raw json.RawMessage) json.RawMessage {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	canonical := CanonicalizeValue(value)
	result, err := json.Marshal(canonical)
	if err != nil {
		return raw
	}
	return result
}

func hashToolSpec(t canonicalToolSpec) string {
	h := sha256.New()
	h.Write([]byte(t.name))
	h.Write([]byte{0})
	h.Write([]byte(t.description))
	h.Write([]byte{0})
	h.Write(t.schema)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func hashToolList(tools []canonicalToolSpec) string {
	h := sha256.New()
	for _, t := range tools {
		h.Write([]byte(t.name))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// FormatDriftReason generates a human-readable drift explanation.
func FormatDriftReason(prev, curr ToolCatalogFingerprint, kind ToolCatalogDriftKind) string {
	var sb strings.Builder
	switch kind {
	case DriftNone:
		return "工具目录无变化"
	case DriftAdditive:
		added := diffToolNames(curr.ToolNames, prev.ToolNames)
		sb.WriteString("工具目录新增（additive）——缓存可复用已有前缀。")
		if len(added) > 0 {
			sb.WriteString(" 新增: ")
			sb.WriteString(strings.Join(added, ", "))
		}
	case DriftBreaking:
		sb.WriteString("工具目录破坏性变化（breaking）——这将导致 DeepSeek 前缀缓存全量 miss！")
		removed := diffToolNames(prev.ToolNames, curr.ToolNames)
		changed := diffToolHashes(prev.ToolHashes, curr.ToolHashes)
		if len(removed) > 0 {
			sb.WriteString(" 删除: ")
			sb.WriteString(strings.Join(removed, ", "))
		}
		if len(changed) > 0 {
			sb.WriteString(" Schema变化: ")
			sb.WriteString(strings.Join(changed, ", "))
		}
	}
	return sb.String()
}

func diffToolNames(a, b []string) []string {
	bSet := make(map[string]bool, len(b))
	for _, n := range b {
		bSet[n] = true
	}
	var diff []string
	for _, n := range a {
		if !bSet[n] {
			diff = append(diff, n)
		}
	}
	return diff
}

func diffToolHashes(prev, curr map[string]string) []string {
	var changed []string
	for name, prevHash := range prev {
		if currHash, ok := curr[name]; ok && currHash != prevHash {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed
}
