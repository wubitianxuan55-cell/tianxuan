// Package tool — V8.9: ToolEnvelope provides a unified JSON result format
// for all tool calls. Every tool result is wrapped as {"ok":bool, "code":..., ...}
// so the model can programmatically distinguish success/failure/timeout/denied
// instead of parsing free-form error strings.
//
// Cache safety: MarshalToolJSON uses SetEscapeHTML(false) and deterministic
// map ordering to ensure identical input always produces identical output bytes.
package tool

import (
	"bytes"
	"encoding/json"
	"strings"
)

// ToolEnvelope is the JSON envelope every tool result is wrapped in.
// The model sees this as the tool_result message Content.
type ToolEnvelope struct {
	OK      bool   `json:"ok"`
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`    // machine-readable: "ok", "timeout", "denied", "not_found", "exec_error", "validation_error"
	Error   string `json:"error,omitempty"`   // human-readable short error (for the model)
	Message string `json:"message,omitempty"` // friendly summary (for the model)
	Data    any    `json:"data,omitempty"`    // structured payload when applicable
}

// Pre-defined outcome codes.
const (
	CodeOK              = "ok"
	CodeTimeout         = "timeout"
	CodeDenied          = "denied"
	CodeNotFound        = "not_found"
	CodeExecError       = "exec_error"
	CodeValidationError = "validation_error"
	CodeUnknownTool     = "unknown_tool"
	CodeBlocked         = "blocked"
)

// WrapResult envelopes a successful tool result.
func WrapResult(code string, data any) string {
	if code == "" {
		code = CodeOK
	}
	return mustMarshal(ToolEnvelope{
		OK:      true,
		Success: true,
		Code:    code,
		Data:    data,
	})
}

// WrapError envelopes a tool error with a machine-readable code.
func WrapError(code, errMsg string, data any) string {
	if code == "" {
		code = CodeExecError
	}
	return mustMarshal(ToolEnvelope{
		OK:      false,
		Success: false,
		Code:    code,
		Error:   strings.TrimSpace(errMsg),
		Data:    data,
	})
}

// WrapText wraps free-form text (legacy tool output) in a success envelope.
// Used when a tool's output is informational rather than structured.
func WrapText(text string) string {
	return mustMarshal(ToolEnvelope{
		OK:      true,
		Success: true,
		Code:    CodeOK,
		Message: strings.TrimSpace(text),
	})
}

// ParseEnvelope attempts to parse a tool result as a ToolEnvelope.
// Returns (envelope, true) on success, (zero, false) on any failure.
func ParseEnvelope(raw string) (ToolEnvelope, bool) {
	var env ToolEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &env); err != nil {
		return ToolEnvelope{}, false
	}
	return env, true
}

// mustMarshal serialises without HTML escaping (SetEscapeHTML(false)) so that
// payload fragments containing & < > survive byte-for-byte. Panics on marshal
// failure — which never happens for ToolEnvelope (all fields are basic types).
func mustMarshal(env ToolEnvelope) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		panic("tool: marshal ToolEnvelope: " + err.Error())
	}
	return strings.TrimSpace(buf.String())
}
