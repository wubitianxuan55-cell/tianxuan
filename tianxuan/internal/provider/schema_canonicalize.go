package provider

import (
	"encoding/json"
	"sort"
)

// compressSchemaMaxDepth limits recursion when stripping redundant fields from
// deeply nested schemas (multi_edit / complete_step have nested arrays).
const compressSchemaMaxDepth = 5
// compressSchema strips redundant fields from a JSON Schema to reduce prompt
// token count.
func compressSchema(v any, depth int) any {
	if depth > compressSchemaMaxDepth {
		return v
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return v
	}
	// Do NOT remove "type":"object" — DeepSeek API requires it.
	// Remove "type":"string" from property values (safe: implied when omitted)
	if prop, hasProps := obj["properties"]; hasProps {
		if props, ok := prop.(map[string]any); ok {
			for key, pv := range props {
				if pm, ok := pv.(map[string]any); ok {
					if pt, hasPT := pm["type"]; hasPT && pt == "string" {
						delete(pm, "type")
					}
					if len(pm) == 0 {
						props[key] = struct{}{}
					} else {
						props[key] = compressSchema(pm, depth+1)
					}
				}
			}
		}
		if items, hasItems := obj["items"]; hasItems {
			obj["items"] = compressSchema(items, depth+1)
		}
	}
	// Remove empty "required":[]
	if req, hasReq := obj["required"]; hasReq {
		if arr, ok := req.([]any); ok && len(arr) == 0 {
			delete(obj, "required")
		}
	}
	return obj
}

// CanonicalizeSchema recursively stabilizes a JSON Schema so the same logical
// schema always produces the same byte representation, then compresses
// redundant fields for minimum prompt token consumption.
func CanonicalizeSchema(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	canon := canonicalizeSchemaValue(v)
	b, err := json.Marshal(canon)
	if err != nil {
		return raw
	}
	return json.RawMessage(b)
}

var setLikeSchemaArrays = map[string]bool{
	"required":          true,
	"dependentRequired": true,
}

func canonicalizeSchemaValue(v any) any {
	v = compressSchema(v, 0)
	switch val := v.(type) {
	case map[string]any:
		for k, inner := range val {
			val[k] = canonicalizeSchemaValue(inner)
		}
		for key := range val {
			if setLikeSchemaArrays[key] {
				if arr, ok := val[key].([]any); ok {
					sort.SliceStable(arr, func(i, j int) bool {
						return schemaJSONString(arr[i]) < schemaJSONString(arr[j])
					})
				}
			}
		}
		if dr, ok := val["dependentRequired"]; ok {
			if drMap, ok := dr.(map[string]any); ok {
				for _, inner := range drMap {
					if arr, ok := inner.([]any); ok {
						sort.SliceStable(arr, func(i, j int) bool {
							return schemaJSONString(arr[i]) < schemaJSONString(arr[j])
						})
					}
				}
			}
		}
		return val
	case []any:
		for i, elem := range val {
			val[i] = canonicalizeSchemaValue(elem)
		}
		return val
	default:
		return v
	}
}

func schemaJSONString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
