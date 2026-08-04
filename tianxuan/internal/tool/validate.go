package tool

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidateArgs checks model-supplied tool arguments against the tool's JSON
// Schema before execution. It mirrors the codex CLI argument contract:
// missing required fields, type mismatches, and enum violations fail loudly
// (so the model can fix them in the next turn), while schema-unknown fields
// are reported as warnings instead of silently dropped.
//
// aliases maps deliberate aliases a tool accepts to their canonical schema
// property (e.g. read_file's "file" -> "path"). An alias satisfies the
// canonical field's required check and is type-checked against the canonical
// property's schema. extraFields lists schema-less tolerated fields (they only
// avoid the unknown-field report); neither ever counts as unknown.
//
// A schema without a "properties" object (or an otherwise unparseable schema)
// degrades to pass-through: external/plugin tools must not be blocked by
// assumptions about their schema shape.
func ValidateArgs(schema json.RawMessage, args json.RawMessage, aliases map[string]string, extraFields ...string) (unknown []string, err error) {
	var schemaObj map[string]any
	if err := json.Unmarshal(schema, &schemaObj); err != nil {
		return nil, nil // unparseable schema: pass through
	}
	props, ok := schemaObj["properties"].(map[string]any)
	if !ok {
		return nil, nil // no property table: nothing to validate
	}
	if aliases == nil {
		aliases = map[string]string{}
	}

	var argsObj map[string]any
	if err := json.Unmarshal(args, &argsObj); err != nil {
		return nil, fmt.Errorf("invalid args: not valid JSON: %w", err)
	}
	if argsObj == nil {
		argsObj = map[string]any{}
	}

	extra := map[string]bool{}
	for _, f := range extraFields {
		extra[f] = true
	}

	// Unknown-field scan (non-blocking report).
	for key := range argsObj {
		if _, inSchema := props[key]; !inSchema && !extra[key] && aliases[key] == "" {
			unknown = append(unknown, key)
		}
	}

	// Required-field check.
	if required, ok := schemaObj["required"].([]any); ok {
		for _, r := range required {
			name, _ := r.(string)
			if name == "" {
				continue
			}
			if v, present := argsObj[name]; !present || v == nil {
				if !aliasPresent(argsObj, name, aliases) {
					return unknown, fmt.Errorf("missing required field %q", name)
				}
			}
		}
	}

	// Per-property check for fields the model actually supplied, resolving
	// aliases to their canonical property before type validation.
	checkField := func(key string, value any) error {
		propRaw, inSchema := props[key]
		if !inSchema {
			return nil
		}
		propSchema, ok := propRaw.(map[string]any)
		if !ok {
			return nil
		}
		return validateValue(key, value, propSchema)
	}
	for key, value := range argsObj {
		if value == nil {
			continue
		}
		if _, inSchema := props[key]; inSchema {
			if verr := checkField(key, value); verr != nil {
				return unknown, verr
			}
			continue
		}
		if canonical := aliases[key]; canonical != "" {
			if verr := checkField(canonical, value); verr != nil {
				return unknown, verr
			}
		}
	}
	return unknown, nil
}

// aliasPresent reports whether any supplied alias resolves to the given
// canonical field name.
func aliasPresent(argsObj map[string]any, canonical string, aliases map[string]string) bool {
	for alias, target := range aliases {
		if target != canonical {
			continue
		}
		if v, present := argsObj[alias]; present && v != nil {
			return true
		}
	}
	return false
}

// validateValue checks one argument value against its property schema.
func validateValue(field string, value any, schema map[string]any) error {
	if typ, ok := schema["type"].(string); ok {
		switch typ {
		case "string":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("field %q must be a string, got %T", field, value)
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("field %q must be a boolean, got %T", field, value)
			}
		case "number":
			if !isNumber(value) {
				return fmt.Errorf("field %q must be a number, got %T", field, value)
			}
		case "integer":
			if !isInteger(value) {
				return fmt.Errorf("field %q must be an integer, got %T", field, value)
			}
		case "object":
			obj, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("field %q must be an object, got %T", field, value)
			}
			if verr := validateNestedObject(field, obj, schema); verr != nil {
				return verr
			}
		case "array":
			arr, ok := value.([]any)
			if !ok {
				return fmt.Errorf("field %q must be an array, got %T", field, value)
			}
			if verr := validateArrayItems(field, arr, schema); verr != nil {
				return verr
			}
		}
	}

	if enum, ok := schema["enum"].([]any); ok {
		for _, e := range enum {
			if jsonEqual(e, value) {
				return nil
			}
		}
		return fmt.Errorf("field %q must be one of %s, got %v", field, enumValues(enum), value)
	}
	return nil
}

func validateNestedObject(field string, obj map[string]any, schema map[string]any) error {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	if required, ok := schema["required"].([]any); ok {
		for _, r := range required {
			name, _ := r.(string)
			if name == "" {
				continue
			}
			if v, present := obj[name]; !present || v == nil {
				return fmt.Errorf("field %q.%s is required", field, name)
			}
		}
	}
	for key, propRaw := range props {
		value, present := obj[key]
		if !present || value == nil {
			continue
		}
		propSchema, ok := propRaw.(map[string]any)
		if !ok {
			continue
		}
		if verr := validateValue(field+"."+key, value, propSchema); verr != nil {
			return verr
		}
	}
	return nil
}

func validateArrayItems(field string, arr []any, schema map[string]any) error {
	items, ok := schema["items"].(map[string]any)
	if !ok {
		return nil
	}
	for i, item := range arr {
		if item == nil {
			continue
		}
		if verr := validateValue(fmt.Sprintf("%s[%d]", field, i), item, items); verr != nil {
			return verr
		}
	}
	return nil
}

func isNumber(v any) bool {
	switch v.(type) {
	case float64, json.Number:
		return true
	default:
		return false
	}
}

func isInteger(v any) bool {
	switch n := v.(type) {
	case json.Number:
		return !strings.ContainsAny(n.String(), ".eE")
	case float64:
		return n == float64(int64(n))
	default:
		return false
	}
}

func jsonEqual(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ab) == string(bb)
}

func enumValues(enum []any) string {
	parts := make([]string, 0, len(enum))
	for _, e := range enum {
		b, _ := json.Marshal(e)
		parts = append(parts, string(b))
	}
	return strings.Join(parts, ", ")
}
