package agent

import (
	"testing"
)

func TestIsErrorResult(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		// Legacy prefix formats
		{"prefix error:", "error: file not found", true},
		{"prefix Error:", "Error: something", true},
		{"prefix blocked:", "blocked: by storm", true},
		{"prefix [error", "[error] something", true},
		{"tool panic", "tool panic: index out of range", false},

		// JSON-envelope (V8.9+): WrapError
		{"env exec_error", `{"ok":false,"success":false,"code":"exec_error","error":"file not found"}`, true},
		{"env timeout", `{"ok":false,"success":false,"code":"timeout","error":"timed out"}`, true},
		{"env denied", `{"ok":false,"success":false,"code":"denied","error":"permission denied"}`, true},
		{"env not_found", `{"ok":false,"success":false,"code":"not_found","error":"path not found"}`, true},
		{"env validation_error", `{"ok":false,"success":false,"code":"validation_error","error":"bad input"}`, true},
		{"env blocked code", `{"ok":false,"success":false,"code":"blocked","error":"storm guard blocked"}`, true},

		// JSON-envelope: WrapResult (success)
		{"env ok", `{"ok":true,"success":true,"code":"ok","data":{"output":"done"}}`, false},
		{"env ok with message", `{"ok":true,"success":true,"code":"ok","message":"file written"}`, false},
		{"env ok no code", `{"ok":true,"success":true}`, false},

		// Non-error / non-JSON
		{"plain success", "file written successfully", false},
		{"empty string", "", false},
		{"git diff", "diff --git a/file.go b/file.go\n+ok", false},
		{"cached result", "ok (cached)", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isErrorResult(tt.input)
			if got != tt.expect {
				t.Errorf("isErrorResult(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}
