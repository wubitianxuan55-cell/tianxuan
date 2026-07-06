package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalizeSchemaStable(t *testing.T) {
	input := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
	out0 := string(CanonicalizeSchema(input))
	out1 := string(CanonicalizeSchema(input))
	t.Logf("call 0: %s", out0)
	t.Logf("call 1: %s", out1)
	if out0 != out1 {
		t.Errorf("NOT DETERMINISTIC — shows key ordering changed after compression")
	}
	// Also test with a deep nested schema (multi_edit tool)
	nested := json.RawMessage(`{"type":"object","properties":{"edits":{"type":"array","items":{"type":"object","properties":{"old":{"type":"string"},"new":{"type":"string"}},"required":["old","new"]}}},"required":["edits"]}`)
	n0 := string(CanonicalizeSchema(nested))
	n1 := string(CanonicalizeSchema(nested))
	t.Logf("nested 0: %s", n0)
	t.Logf("nested 1: %s", n1)
	if n0 != n1 {
		t.Errorf("NESTED NOT DETERMINISTIC")
	}
	// Run 10 times to detect non-determinism
	results := make(map[string]int)
	for i := 0; i < 10; i++ {
		out := string(CanonicalizeSchema(input))
		results[out]++
	}
	if len(results) != 1 {
		t.Errorf("10 calls produced %d different outputs: %v", len(results), keys(results))
	}
	t.Logf("10 calls all identical: OK")
}

func keys(m map[string]int) string { var s []string; for k := range m { s = append(s, k) }; return strings.Join(s, " | ") }

func TestCanonicalizeSchemaStripsDescriptions(t *testing.T) {
	// MCP tools often include "description" on the schema object and
	// on individual properties — these are redundant with the tool-level
	// description and inflate per-turn prompt tokens.
	input := json.RawMessage(`{
		"type": "object",
		"description": "Fetches data from the API",
		"properties": {
			"url": {
				"type": "string",
				"description": "The URL to fetch from"
			},
			"timeout": {
				"type": "integer",
				"description": "Timeout in milliseconds"
			}
		},
		"required": ["url"]
	}`)
	out := string(CanonicalizeSchema(input))
	t.Logf("compressed: %s", out)
	if strings.Contains(out, `"description"`) {
		t.Errorf("description field survived compression: %s", out)
	}
}

func TestCanonicalizeSchemaStripsNestedDescriptions(t *testing.T) {
	// Nested objects (arrays of objects, etc.) should also lose descriptions.
	input := json.RawMessage(`{
		"type": "object",
		"properties": {
			"items": {
				"type": "array",
				"description": "List of things",
				"items": {
					"type": "object",
					"properties": {
						"name": {
							"type": "string",
							"description": "The name"
						}
					}
				}
			}
		}
	}`)
	out := string(CanonicalizeSchema(input))
	t.Logf("nested compressed: %s", out)
	if strings.Contains(out, `"description"`) {
		t.Errorf("description field survived nested compression: %s", out)
	}
}
