package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"tianxuan/internal/provider"
)

// ─── V5.11: 工具目录指纹 (Kun tool-catalog-fingerprint.ts 移植) ──────────

// ToolCatalogFingerprint 是一组工具 schemas 的规范化 SHA256 指纹。
// 用于检测两轮之间工具集是否发生变化（drift），变化会破坏 DeepSeek 前缀缓存。
type ToolCatalogFingerprint struct {
	Fingerprint string            // SHA256 前 16 字符
	ToolCount   int               // 工具数量
	ToolNames   []string          // 排序后的工具名称列表
	ToolHashes  map[string]string // 每个工具的独立 SHA256
}

// ToolCatalogDrift 描述工具集变化类型。
type ToolCatalogDriftKind string

const (
	DriftNone     ToolCatalogDriftKind = "none"     // 无变化
	DriftAdditive ToolCatalogDriftKind = "additive" // 仅新增工具（缓存可复用已有前缀）
	DriftBreaking ToolCatalogDriftKind = "breaking" // 工具被删除/重命名/schema 变化（缓存断裂）
)

// BuildToolCatalogFingerprint 计算工具集的规范化指纹。
// 规范化的顺序：按名称排序 + schema key 递归排序，确保同一工具集无论注册顺序如何都产生相同指纹。
func BuildToolCatalogFingerprint(tools []provider.ToolSchema) ToolCatalogFingerprint {
	canonical := normalizeToolSchemas(tools)
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

// DetectToolCatalogDrift 比较前后两个工具指纹，判断变化类型。
func DetectToolCatalogDrift(prev, curr ToolCatalogFingerprint) ToolCatalogDriftKind {
	if prev.Fingerprint == curr.Fingerprint {
		return DriftNone
	}
	// 检查是否仅是新增工具（所有旧工具名称和 hash 不变，仅增加新工具）
	prevSet := make(map[string]bool, len(prev.ToolNames))
	for _, n := range prev.ToolNames {
		prevSet[n] = true
	}
	for _, n := range prev.ToolNames {
		if currHash, ok := curr.ToolHashes[n]; !ok {
			return DriftBreaking // 旧工具被删除
		} else if currHash != prev.ToolHashes[n] {
			return DriftBreaking // 旧工具 schema 变化
		}
	}
	// 所有旧工具均存在且 hash 不变 → 仅新增
	if curr.ToolCount > prev.ToolCount {
		return DriftAdditive
	}
	return DriftBreaking // 工具数量不变或减少但指纹不同 → 破坏性
}

// ─── 内部规范化 ──────────────────────────────────────────────────────

type canonicalToolSpec struct {
	name        string
	description string
	schema      json.RawMessage
}

func normalizeToolSchemas(tools []provider.ToolSchema) []canonicalToolSpec {
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

// canonicalizeJSON 递归排序 JSON 对象的所有 key，
// 确保相同内容的 JSON 无论 key 顺序如何都产生相同字节流。
func canonicalizeJSON(raw json.RawMessage) json.RawMessage {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw // 解析失败，保留原样
	}
	canonical := canonicalizeValue(value)
	result, err := json.Marshal(canonical)
	if err != nil {
		return raw
	}
	return result
}

func canonicalizeValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		// 递归处理每个值，然后按 key 排序输出
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[k] = canonicalizeValue(val)
		}
		return out // json.Marshal 默认按 key 排序 map
	case []any:
		out := make([]any, len(v))
		for i, val := range v {
			out[i] = canonicalizeValue(val)
		}
		return out
	default:
		return v
	}
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

// FormatDriftReason 生成人类可读的漂移原因说明。
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
