package provider

import "testing"

// ─── V5.11: SanitizeToolPairing 升级测试 (Kun model-history-repair.ts 对照) ───

// TestSanitizeBridgeItems 验证 tool_call 和 tool_result 之间的
// 桥接消息（assistant 文本）不会阻断配对。这是当前实现的主要缺陷。
func TestSanitizeBridgeItems(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "go"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "c1", Name: "read_file"},
		}},
		// 桥接消息：assistant 在 tool_call 之后的文本（如"让我来读这个文件"）
		{Role: RoleAssistant, Content: "Let me read that file."},
		// 然后才是 tool result
		{Role: RoleTool, ToolCallID: "c1", Name: "read_file", Content: "file contents"},
		{Role: RoleAssistant, Content: "Done, the file contains..."},
	}
	out := SanitizeToolPairing(msgs)

	// 验证：c1 的结果应该被保留（不是打断的占位符）
	found := false
	for _, m := range out {
		if m.Role == RoleTool && m.ToolCallID == "c1" && m.Content == "file contents" {
			found = true
		}
	}
	if !found {
		t.Fatalf("bridge item broke pairing: expected 'file contents' for c1, got: %+v", out)
	}
	// 桥接消息应该保留
	hasBridge := false
	for _, m := range out {
		if m.Role == RoleAssistant && m.Content == "Let me read that file." {
			hasBridge = true
		}
	}
	if !hasBridge {
		t.Errorf("bridge message should be preserved")
	}
}

// TestSanitizeMultiBlockToolCalls 验证连续多组 tool_call 块各自正确配对。
func TestSanitizeMultiBlockToolCalls(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "do a then b"},
		// 第一组：read_file
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "c1", Name: "read_file"},
		}},
		{Role: RoleTool, ToolCallID: "c1", Name: "read_file", Content: "content1"},
		// 第二组：grep
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "c2", Name: "grep"},
		}},
		{Role: RoleTool, ToolCallID: "c2", Name: "grep", Content: "content2"},
	}
	out := SanitizeToolPairing(msgs)

	// 验证两组都保留
	found1, found2 := false, false
	for _, m := range out {
		if m.Role == RoleTool && m.ToolCallID == "c1" && m.Content == "content1" {
			found1 = true
		}
		if m.Role == RoleTool && m.ToolCallID == "c2" && m.Content == "content2" {
			found2 = true
		}
	}
	if !found1 {
		t.Errorf("first tool call result lost")
	}
	if !found2 {
		t.Errorf("second tool call result lost")
	}
}

// TestSanitizeBridgeWithMultiBlock 验证桥接消息 + 多组的组合场景。
func TestSanitizeBridgeWithMultiBlock(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "do it"},
		// 第一组
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "c1", Name: "read_file"},
		}},
		{Role: RoleAssistant, Content: "Reading the file..."},          // 桥接
		{Role: RoleTool, ToolCallID: "c1", Name: "read_file", Content: "content1"},
		{Role: RoleAssistant, Content: "Now searching..."},              // 桥接
		// 第二组
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "c2", Name: "grep"},
			{ID: "c3", Name: "ls"},
		}},
		{Role: RoleTool, ToolCallID: "c2", Name: "grep", Content: "match"},
		{Role: RoleTool, ToolCallID: "c3", Name: "ls", Content: "files"},
	}
	out := SanitizeToolPairing(msgs)

	// 所有三个 tool result 都应保留
	c1ok, c2ok, c3ok := false, false, false
	for _, m := range out {
		if m.Role == RoleTool {
			switch m.ToolCallID {
			case "c1":
				if m.Content == "content1" {
					c1ok = true
				}
			case "c2":
				if m.Content == "match" {
					c2ok = true
				}
			case "c3":
				if m.Content == "files" {
					c3ok = true
				}
			}
		}
	}
	if !c1ok {
		t.Errorf("c1 (read_file) result lost")
	}
	if !c2ok {
		t.Errorf("c2 (grep) result lost")
	}
	if !c3ok {
		t.Errorf("c3 (ls) result lost")
	}
}

// TestSanitizeOrphanToolCallWithBridge 验证孤儿 tool_call（无匹配结果）
// 在有桥接消息和后续正常配对时，不影响正常配对。
func TestSanitizeOrphanCallDoesNotBreakNextPair(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "go"},
		// 第一组：孤儿（无 tool result）
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "orphan", Name: "bash"},
		}},
		// 第二组：正常配对
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "good", Name: "read_file"},
		}},
		{Role: RoleTool, ToolCallID: "good", Name: "read_file", Content: "ok"},
	}
	out := SanitizeToolPairing(msgs)

	// good 的结果应保留，orphan 应被回填占位符或移除
	hasGood := false
	for _, m := range out {
		if m.Role == RoleTool && m.ToolCallID == "good" && m.Content == "ok" {
			hasGood = true
		}
	}
	if !hasGood {
		t.Errorf("good tool call result lost because of orphan call")
	}
}

// TestSanitizeDanglingToolResultAfterBlock 验证 tool_call 块之后的
// 多余 tool result（不属于任何已知 call）被丢弃。
func TestSanitizeExtraToolResultDropped(t *testing.T) {
	msgs := []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{
			{ID: "c1", Name: "ls"},
		}},
		{Role: RoleTool, ToolCallID: "c1", Name: "ls", Content: "ok"},
		{Role: RoleTool, ToolCallID: "c2", Name: "grep", Content: "extra"}, // 无对应 call
		{Role: RoleAssistant, Content: "done"},
	}
	out := SanitizeToolPairing(msgs)

	// c2 的 tool result 应该被丢弃
	for _, m := range out {
		if m.Role == RoleTool && m.ToolCallID == "c2" {
			t.Errorf("orphan tool result c2 should be dropped but survived: %+v", out)
		}
	}
}
