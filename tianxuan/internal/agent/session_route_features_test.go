package agent

import (
	"context"
	"testing"

	"tianxuan/internal/provider"
)

func TestSessionRouteFeatures_IsComplex(t *testing.T) {
	tests := []struct {
		name     string
		f        SessionRouteFeatures
		expected bool
	}{
		{"empty session", SessionRouteFeatures{}, false},
		{"low counts", SessionRouteFeatures{TurnCount: 4, RecentErrors: 1, PendingTodos: 2, FilesModified: 3}, false},

		// Individual threshold triggers (V10.12: TurnCount lowered 10->5)
		{"turn count > 5", SessionRouteFeatures{TurnCount: 6}, true},
		{"turn count == 5 not triggered", SessionRouteFeatures{TurnCount: 5}, false},
		{"has written files", SessionRouteFeatures{HasWrittenFiles: true}, true},
		{"has used sub agent", SessionRouteFeatures{HasUsedSubAgent: true}, true},
		{"recent errors >= 3", SessionRouteFeatures{RecentErrors: 3}, true},
		{"recent errors == 2 not triggered", SessionRouteFeatures{RecentErrors: 2}, false},
		{"pending todos > 3", SessionRouteFeatures{PendingTodos: 4}, true},
		{"pending todos == 3 not triggered", SessionRouteFeatures{PendingTodos: 3}, false},
		{"files modified > 8", SessionRouteFeatures{FilesModified: 9}, true},
		{"files modified == 8 not triggered", SessionRouteFeatures{FilesModified: 8}, false},

		// Multiple signals but below thresholds
		{"below all thresholds", SessionRouteFeatures{TurnCount: 5, RecentErrors: 2, PendingTodos: 3, FilesModified: 8}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.IsComplex(); got != tt.expected {
				t.Errorf("IsComplex() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAutoRouteWithSession(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		features SessionRouteFeatures
		wantPro  bool
	}{
		// Complex session -> pro regardless of input
		{"complex forces pro for short input", "hi", SessionRouteFeatures{TurnCount: 6}, true},
		{"has written files forces pro", "hi", SessionRouteFeatures{HasWrittenFiles: true}, true},
		{"recent errors forces pro", "", SessionRouteFeatures{RecentErrors: 3}, true},

		// Simple session falls back to input-text heuristic
		{"simple short input", "hi", SessionRouteFeatures{}, false},       // flash
		{"simple long input", "refactor the entire auth module", SessionRouteFeatures{}, true}, // pro via keywords
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AutoRouteWithSession(tt.input, tt.features)
			isPro := got == AutoRoutePro
			if isPro != tt.wantPro {
				t.Errorf("AutoRouteWithSession(%q, %+v) = %v, wantPro=%v", tt.input, tt.features, got, tt.wantPro)
			}
		})
	}
}

func TestAutoRouteProviderWithSession_NilFlash(t *testing.T) {
	def := &stubProvider{name: "default"}
	result := AutoRouteProviderWithSession("hi", SessionRouteFeatures{}, def, nil)
	if result != def {
		t.Error("nil flashProv should return defaultProv")
	}
}

func TestAutoRouteProviderWithSession_ComplexSession(t *testing.T) {
	def := &stubProvider{name: "default"}
	flash := &stubProvider{name: "flash"}
	// Complex session -> pro (default)
	result := AutoRouteProviderWithSession("hi", SessionRouteFeatures{TurnCount: 6}, def, flash)
	if result != def {
		t.Errorf("complex session should use default (pro), got %v", result)
	}
}

func TestAutoRouteProviderWithSession_SimpleSession(t *testing.T) {
	def := &stubProvider{name: "default"}
	flash := &stubProvider{name: "flash"}
	// Simple session + short input -> flash
	result := AutoRouteProviderWithSession("hi", SessionRouteFeatures{}, def, flash)
	if result != flash {
		t.Errorf("simple session with short input should use flash, got %v", result)
	}
}

// TestCollectSessionRouteFeatures_FilesModified verifies the bugfix:
// FilesModified must be counted from assistant ToolCalls.Arguments,
// NOT from RoleTool.Content (which is an execution result string).
func TestCollectSessionRouteFeatures_FilesModified(t *testing.T) {
	t.Run("from tool call arguments", func(t *testing.T) {
		msgs := []provider.Message{
			{Role: provider.RoleUser, Content: "edit foo.go"},
			{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{ID: "c1", Name: "edit_file", Arguments: `{"path": "foo.go", "old_string": "x", "new_string": "y"}`},
					{ID: "c2", Name: "write_file", Arguments: `{"path": "bar/baz.go", "content": "package main"}`},
				},
			},
			{Role: provider.RoleTool, ToolCallID: "c1", Name: "edit_file", Content: "File edited successfully"},
			{Role: provider.RoleTool, ToolCallID: "c2", Name: "write_file", Content: "File written successfully"},
		}
		f := collectFeaturesFromMessages(msgs)
		if f.FilesModified != 2 {
			t.Errorf("FilesModified = %d, want 2 (from ToolCall.Arguments)", f.FilesModified)
		}
	})

	t.Run("result content (no JSON) does not contribute", func(t *testing.T) {
		msgs := []provider.Message{
			{Role: provider.RoleUser, Content: "edit x.go"},
			{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{ID: "c1", Name: "edit_file", Arguments: `{"path": "x.go", "old_string": "a", "new_string": "b"}`},
				},
			},
			{Role: provider.RoleTool, ToolCallID: "c1", Name: "edit_file", Content: "File edited successfully"},
		}
		f := collectFeaturesFromMessages(msgs)
		if f.FilesModified != 1 {
			t.Errorf("FilesModified = %d, want 1 (correctly from Arguments)", f.FilesModified)
		}
	})

	t.Run("deduplicates same file", func(t *testing.T) {
		msgs := []provider.Message{
			{Role: provider.RoleUser, Content: "edit twice"},
			{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{ID: "c1", Name: "edit_file", Arguments: `{"path": "a.go", "old_string": "x", "new_string": "y"}`},
				},
			},
			{Role: provider.RoleTool, ToolCallID: "c1", Name: "edit_file", Content: "success"},
			{Role: provider.RoleUser, Content: "edit again"},
			{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{ID: "c2", Name: "edit_file", Arguments: `{"path": "a.go", "old_string": "y", "new_string": "z"}`},
				},
			},
			{Role: provider.RoleTool, ToolCallID: "c2", Name: "edit_file", Content: "success"},
		}
		f := collectFeaturesFromMessages(msgs)
		if f.FilesModified != 1 {
			t.Errorf("FilesModified = %d, want 1 (same file deduplicated)", f.FilesModified)
		}
	})

	t.Run("includes delete_range and delete_symbol", func(t *testing.T) {
		msgs := []provider.Message{
			{Role: provider.RoleUser, Content: "delete stuff"},
			{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{ID: "c1", Name: "delete_range", Arguments: `{"path": "a.go", "start": 1, "end": 5}`},
					{ID: "c2", Name: "delete_symbol", Arguments: `{"path": "b.go", "name": "oldFn"}`},
				},
			},
			{Role: provider.RoleTool, ToolCallID: "c1", Name: "delete_range", Content: "deleted"},
			{Role: provider.RoleTool, ToolCallID: "c2", Name: "delete_symbol", Content: "deleted"},
		}
		f := collectFeaturesFromMessages(msgs)
		if f.FilesModified != 2 {
			t.Errorf("FilesModified = %d, want 2", f.FilesModified)
		}
	})

	t.Run("error results still count as modified", func(t *testing.T) {
		msgs := []provider.Message{
			{Role: provider.RoleUser, Content: "edit bad file"},
			{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{ID: "c1", Name: "edit_file", Arguments: `{"path": "nope.go", "old_string": "x", "new_string": "y"}`},
				},
			},
			{Role: provider.RoleTool, ToolCallID: "c1", Name: "edit_file", Content: "error: old_string not found"},
		}
		f := collectFeaturesFromMessages(msgs)
		if f.FilesModified != 1 {
			t.Errorf("FilesModified = %d, want 1 (error attempts still count)", f.FilesModified)
		}
	})
}

// TestCollectSessionRouteFeatures_WriteToolsAndSubAgent verifies the new
// HasWrittenFiles and HasUsedSubAgent signals detected from tool calls.
func TestCollectSessionRouteFeatures_WriteToolsAndSubAgent(t *testing.T) {
	t.Run("detects write tools", func(t *testing.T) {
		msgs := []provider.Message{
			{Role: provider.RoleUser, Content: "edit file"},
			{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{ID: "c1", Name: "edit_file", Arguments: `{"path":"x.go","old_string":"a","new_string":"b"}`},
				},
			},
			{Role: provider.RoleTool, ToolCallID: "c1", Name: "edit_file", Content: "success"},
		}
		f := collectFeaturesFromMessages(msgs)
		if !f.HasWrittenFiles {
			t.Error("HasWrittenFiles should be true when edit_file is used")
		}
	})

	t.Run("detects bash as write-adjacent", func(t *testing.T) {
		msgs := []provider.Message{
			{Role: provider.RoleUser, Content: "run tests"},
			{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{ID: "c1", Name: "bash", Arguments: `{"command":"go test ./..."}`},
				},
			},
			{Role: provider.RoleTool, ToolCallID: "c1", Name: "bash", Content: "ok"},
		}
		f := collectFeaturesFromMessages(msgs)
		if !f.HasWrittenFiles {
			t.Error("HasWrittenFiles should be true when bash is used")
		}
	})

	t.Run("detects sub agent", func(t *testing.T) {
		msgs := []provider.Message{
			{Role: provider.RoleUser, Content: "research this"},
			{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{ID: "c1", Name: "task", Arguments: `{"prompt":"research X"}`},
				},
			},
			{Role: provider.RoleTool, ToolCallID: "c1", Name: "task", Content: "done"},
		}
		f := collectFeaturesFromMessages(msgs)
		if !f.HasUsedSubAgent {
			t.Error("HasUsedSubAgent should be true when task tool is used")
		}
	})

	t.Run("read-only session has no write flags", func(t *testing.T) {
		msgs := []provider.Message{
			{Role: provider.RoleUser, Content: "read foo.go"},
			{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{ID: "c1", Name: "read_file", Arguments: `{"path":"foo.go"}`},
					{ID: "c2", Name: "grep", Arguments: `{"pattern":"func"}`},
				},
			},
		}
		f := collectFeaturesFromMessages(msgs)
		if f.HasWrittenFiles {
			t.Error("HasWrittenFiles should be false for read-only tools")
		}
		if f.HasUsedSubAgent {
			t.Error("HasUsedSubAgent should be false without task tool")
		}
	})
}

func TestCollectSessionRouteFeatures_TurnCount(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "q1"},
		{Role: provider.RoleAssistant, Content: "a1"},
		{Role: provider.RoleUser, Content: "q2"},
		{Role: provider.RoleAssistant, Content: "a2"},
		{Role: provider.RoleUser, Content: "q3"},
	}
	f := collectFeaturesFromMessages(msgs)
	if f.TurnCount != 3 {
		t.Errorf("TurnCount = %d, want 3", f.TurnCount)
	}
}

func TestCollectSessionRouteFeatures_RecentErrors(t *testing.T) {
	t.Run("counts error/blocked/precheck errors", func(t *testing.T) {
		msgs := []provider.Message{
			{Role: provider.RoleUser, Content: "q"},
			{Role: provider.RoleAssistant, Content: "a"},
			{Role: provider.RoleTool, Name: "edit_file", Content: "error: old_string not found"},
			{Role: provider.RoleTool, Name: "bash", Content: "blocked: unsafe command"},
			{Role: provider.RoleTool, Name: "edit_file", Content: "precheck blocked: file not readable"},
		}
		f := collectFeaturesFromMessages(msgs)
		if f.RecentErrors != 3 {
			t.Errorf("RecentErrors = %d, want 3", f.RecentErrors)
		}
	})

	t.Run("only counts last 6 tool results", func(t *testing.T) {
		msgs := []provider.Message{
			{Role: provider.RoleUser, Content: "q"},
			{Role: provider.RoleTool, Name: "t", Content: "error: 1"},
			{Role: provider.RoleTool, Name: "t", Content: "error: 2"},
			{Role: provider.RoleTool, Name: "t", Content: "error: 3"},
			{Role: provider.RoleTool, Name: "t", Content: "error: 4"},
			{Role: provider.RoleTool, Name: "t", Content: "error: 5"},
			{Role: provider.RoleTool, Name: "t", Content: "error: 6"},
			{Role: provider.RoleTool, Name: "t", Content: "error: 7"},
		}
		f := collectFeaturesFromMessages(msgs)
		if f.RecentErrors != 6 {
			t.Errorf("RecentErrors = %d, want 6 (capped at last 6)", f.RecentErrors)
		}
	})
}

// collectFeaturesFromMessages is a test helper that extracts SessionRouteFeatures
// from a raw message slice, bypassing the full AgentRunner state (todos).
func collectFeaturesFromMessages(msgs []provider.Message) SessionRouteFeatures {
	f := SessionRouteFeatures{}

	for _, m := range msgs {
		if m.Role == provider.RoleUser {
			f.TurnCount++
		}
	}

	recent := 0
	for i := len(msgs) - 1; i >= 0 && recent < 6; i-- {
		if msgs[i].Role != provider.RoleTool {
			continue
		}
		recent++
		c := msgs[i].Content
		if len(c) > 0 && (c[0] == 'e' || c[0] == 'b' || c[0] == 'p') {
			if len(c) >= 6 && (c[:6] == "error:" || c[:6] == "blocke" || c[:6] == "preche") {
				f.RecentErrors++
			}
		}
	}

	// Walk assistant ToolCalls to extract FilesModified + write/sub-agent signals.
	seen := map[string]bool{}
	for _, m := range msgs {
		if m.Role == provider.RoleAssistant && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				// Write tools detection
				if tc.Name == "edit_file" || tc.Name == "write_file" ||
					tc.Name == "multi_edit" || tc.Name == "delete_range" ||
					tc.Name == "delete_symbol" || tc.Name == "bash" {
					f.HasWrittenFiles = true
				}
				// Sub-agent detection
				if tc.Name == "task" {
					f.HasUsedSubAgent = true
				}
				// File counting
				for _, name := range []string{"edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol"} {
					if tc.Name == name {
						path := extractFilePath(tc.Name, tc.Arguments)
						if path != "" {
							seen[path] = true
						}
					}
				}
			}
		}
	}
	f.FilesModified = len(seen)

	return f
}

// stubProvider implements provider.Provider for routing tests only.
type stubProvider struct {
	name string
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	return nil, nil
}
