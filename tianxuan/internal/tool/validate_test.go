package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

// validateSchema is a compact schema with all supported keyword shapes used by
// the built-in tools: required, type, enum, nested object, array items.
const validateSchema = `{
	"type": "object",
	"properties": {
		"path": {"type": "string"},
		"count": {"type": "integer"},
		"enabled": {"type": "boolean"},
		"ratio": {"type": "number"},
		"mode": {"type": "string", "enum": ["fast", "slow"]},
		"edits": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"old_string": {"type": "string"},
					"replace_all": {"type": "boolean"}
				},
				"required": ["old_string"]
			}
		}
	},
	"required": ["path", "mode"]
}`

func validateArgsRaw(t *testing.T, args string, extra ...string) ([]string, error) {
	t.Helper()
	return ValidateArgs(json.RawMessage(validateSchema), json.RawMessage(args), nil, extra...)
}

func TestValidateArgsAcceptsValid(t *testing.T) {
	unknown, err := validateArgsRaw(t, `{"path":"a.go","mode":"fast","count":2,"edits":[{"old_string":"x"}]}`)
	if err != nil {
		t.Fatalf("valid args rejected: %v", err)
	}
	if len(unknown) != 0 {
		t.Fatalf("unexpected unknown fields: %v", unknown)
	}
}

func TestValidateArgsMissingRequired(t *testing.T) {
	_, err := validateArgsRaw(t, `{"path":"a.go"}`)
	if err == nil {
		t.Fatal("missing required field 'mode' accepted")
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Fatalf("error should name the missing field, got: %v", err)
	}
}

func TestValidateArgsTypeMismatch(t *testing.T) {
	cases := []struct {
		name string
		args string
		want string
	}{
		{"integer from string", `{"path":"a.go","mode":"fast","count":"2"}`, "count"},
		{"boolean from string", `{"path":"a.go","mode":"fast","enabled":"yes"}`, "enabled"},
		{"enum violation", `{"path":"a.go","mode":"turbo"}`, "mode"},
		{"nested required", `{"path":"a.go","mode":"fast","edits":[{"replace_all":true}]}`, "old_string"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateArgsRaw(t, tc.args)
			if err == nil {
				t.Fatalf("args %s accepted, want error naming %q", tc.args, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q should name %q", err, tc.want)
			}
		})
	}
}

func TestValidateArgsNullPasses(t *testing.T) {
	// json.Unmarshal maps null to the field's zero value; the validator must
	// keep that semantics so optional nullable fields don't become hard errors.
	unknown, err := validateArgsRaw(t, `{"path":"a.go","mode":"fast","count":null}`)
	if err != nil {
		t.Fatalf("null field rejected: %v", err)
	}
	if len(unknown) != 0 {
		t.Fatalf("unexpected unknown fields: %v", unknown)
	}
}

func TestValidateArgsReportsUnknownFields(t *testing.T) {
	unknown, err := validateArgsRaw(t, `{"path":"a.go","mode":"fast","timeout":5}`)
	if err != nil {
		t.Fatalf("unknown field should not block execution: %v", err)
	}
	if len(unknown) != 1 || unknown[0] != "timeout" {
		t.Fatalf("unknown = %v, want [timeout]", unknown)
	}
}

func TestValidateArgsExtraFieldsExempt(t *testing.T) {
	// Tools accept deliberate aliases (file for path, timeout_ms for seconds);
	// those must be exempted from the unknown-field report.
	unknown, err := validateArgsRaw(t, `{"path":"a.go","mode":"fast","file":"b.go"}`, "file")
	if err != nil {
		t.Fatalf("exempt field rejected: %v", err)
	}
	if len(unknown) != 0 {
		t.Fatalf("exempt field reported as unknown: %v", unknown)
	}
}

func TestValidateArgsAliasSatisfiesRequired(t *testing.T) {
	// read_file({"file": ...}) supplies "path" through its alias: the required
	// check must pass and the alias value must be type-checked as "path".
	unknown, err := ValidateArgs(
		json.RawMessage(validateSchema),
		json.RawMessage(`{"file":"a.go","mode":"fast"}`),
		map[string]string{"file": "path"},
	)
	if err != nil {
		t.Fatalf("alias should satisfy required path: %v", err)
	}
	if len(unknown) != 0 {
		t.Fatalf("alias reported unknown: %v", unknown)
	}
}

func TestValidateArgsAliasTypeCheckedAgainstCanonical(t *testing.T) {
	// The alias value must fail when the canonical field expects a string.
	_, err := ValidateArgs(
		json.RawMessage(validateSchema),
		json.RawMessage(`{"file":42,"mode":"fast"}`),
		map[string]string{"file": "path"},
	)
	if err == nil {
		t.Fatal("alias value of wrong type accepted")
	}
	if !strings.Contains(err.Error(), "path") {
		t.Fatalf("error should name canonical field path, got: %v", err)
	}
}

func TestValidateArgsMalformedJSON(t *testing.T) {
	_, err := validateArgsRaw(t, `{"path":`)
	if err == nil {
		t.Fatal("malformed JSON accepted")
	}
}

func TestValidateArgsNonObject(t *testing.T) {
	_, err := validateArgsRaw(t, `["a"]`)
	if err == nil {
		t.Fatal("array args accepted where object required")
	}
}

func TestValidateArgsInvalidSchemaSkips(t *testing.T) {
	// External/plugin schemas may be nonstandard; validation must degrade to
	// pass-through instead of blocking the tool.
	unknown, err := ValidateArgs(json.RawMessage(`{"type":"object"}`), json.RawMessage(`{"anything":1}`), nil)
	if err != nil {
		t.Fatalf("schema without properties rejected args: %v", err)
	}
	if len(unknown) != 0 {
		t.Fatalf("schema without properties flagged unknown: %v", unknown)
	}
}
