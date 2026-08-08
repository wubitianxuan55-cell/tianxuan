package tool

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExampleFromSchema returns a compact example JSON for a tool schema, built
// from its required fields and property types — e.g. {"command": "<command>"}.
// Validation errors append it so the model can fix arguments in one shot
// instead of re-reading the full schema. Returns "" for schemas with no
// describable required shape.
func ExampleFromSchema(schema json.RawMessage) string {
	var schemaObj map[string]any
	if err := json.Unmarshal(schema, &schemaObj); err != nil {
		return ""
	}
	props, ok := schemaObj["properties"].(map[string]any)
	if !ok || len(props) == 0 {
		return ""
	}
	var names []string
	if required, ok := schemaObj["required"].([]any); ok {
		for _, r := range required {
			if name, _ := r.(string); name != "" {
				names = append(names, name)
			}
		}
	}
	// No required list: show the first few properties as a hint.
	if len(names) == 0 {
		for name := range props {
			names = append(names, name)
			if len(names) >= 3 {
				break
			}
		}
	}
	if len(names) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("{")
	for i, name := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%q: %s", name, exampleValue(name, props[name])))
	}
	b.WriteString("}")
	return b.String()
}

// exampleValue renders one property's example value from its JSON Schema type.
func exampleValue(name string, propRaw any) string {
	prop, _ := propRaw.(map[string]any)
	typ, _ := prop["type"].(string)
	switch typ {
	case "boolean":
		return "true"
	case "integer", "number":
		return "0"
	case "array":
		return "[]"
	case "object":
		return "{}"
	default:
		return fmt.Sprintf("%q", "<"+name+">")
	}
}

// MisuseHint detects known cross-tool argument mix-ups. Currently: chat-API
// style arguments (model/messages/max_tokens/temperature/top_p) passed to
// bash — the model treating the shell tool like an LLM endpoint. Returns a
// corrective hint, or "" when the call looks normal.
func MisuseHint(toolName string, args map[string]any) string {
	if toolName != "bash" {
		return ""
	}
	for _, key := range []string{"messages", "max_tokens", "model", "temperature", "top_p"} {
		if _, present := args[key]; present {
			return "these look like chat-API parameters, but bash only accepts the command field — pass the shell command as {\"command\": \"...\"}"
		}
	}
	return ""
}
