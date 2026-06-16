package agent

import (
	"encoding/json"
	"testing"
)

// ─── V5.12: Tool-Call-Repair (Kun tool-call-repair.ts 移植) ────────────

// TestRepairDispatchToolArguments_FlattenWrapper 验证嵌套包装器展平。
// 模型有时会把参数包一层：{"arguments": {"path": "x"}} → {"path": "x"}
func TestRepairDispatchToolArguments_FlattenWrapper(t *testing.T) {
	raw := map[string]any{
		"arguments": map[string]any{"path": "/tmp/x", "content": "hello"},
	}
	result := RepairDispatchToolArguments(raw, ToolArgumentRepairOptions{})
	args := result.Arguments
	if args["path"] != "/tmp/x" {
		t.Errorf("expected path=/tmp/x, got %v", args["path"])
	}
	if args["content"] != "hello" {
		t.Errorf("expected content=hello, got %v", args["content"])
	}
	if len(result.Notes) == 0 || result.Notes[0] == "" {
		t.Errorf("expected flatten note, got %v", result.Notes)
	}
}

// TestRepairDispatchToolArguments_ArgsWrapper 验证 "args" 键展平。
func TestRepairDispatchToolArguments_ArgsWrapper(t *testing.T) {
	raw := map[string]any{
		"args": map[string]any{"file": "main.go"},
	}
	result := RepairDispatchToolArguments(raw, ToolArgumentRepairOptions{})
	if result.Arguments["file"] != "main.go" {
		t.Errorf("expected file=main.go, got %v", result.Arguments["file"])
	}
}

// TestRepairDispatchToolArguments_ScavengeJSON 验证从字符串值中提取 JSON 对象。
// 模型有时输出 {"text": "{\"path\": \"x\"}"} 这样的嵌套 JSON 字符串。
func TestRepairDispatchToolArguments_ScavengeJSON(t *testing.T) {
	raw := map[string]any{
		"text": `{"path": "/tmp/x", "content": "hello"}`,
	}
	result := RepairDispatchToolArguments(raw, ToolArgumentRepairOptions{})
	if result.Arguments["path"] != "/tmp/x" {
		t.Errorf("expected path=/tmp/x, got %v", result.Arguments["path"])
	}
	if result.Arguments["content"] != "hello" {
		t.Errorf("expected content=hello, got %v", result.Arguments["content"])
	}
}

// TestRepairDispatchToolArguments_NoRepair 验证无需修复时原样返回。
func TestRepairDispatchToolArguments_NoRepair(t *testing.T) {
	raw := map[string]any{
		"path":    "/tmp/x",
		"content": "hello world",
	}
	result := RepairDispatchToolArguments(raw, ToolArgumentRepairOptions{})
	if result.Arguments["path"] != "/tmp/x" {
		t.Errorf("expected path=/tmp/x, got %v", result.Arguments["path"])
	}
	if result.Arguments["content"] != "hello world" {
		t.Errorf("expected content=hello world, got %v", result.Arguments["content"])
	}
}

// TestRepairDispatchToolArguments_TruncateOversized 验证超大字符串截断。
func TestRepairDispatchToolArguments_TruncateOversized(t *testing.T) {
	// 构造一个 2000 字节的字符串，maxStringBytes=1024
	big := make([]byte, 2000)
	for i := range big {
		big[i] = 'x'
	}
	raw := map[string]any{
		"content": string(big),
	}
	result := RepairDispatchToolArguments(raw, ToolArgumentRepairOptions{
		MaxStringBytes: 1024,
	})
	content, ok := result.Arguments["content"].(string)
	if !ok {
		t.Fatal("content should be string")
	}
	if len(content) >= 2000 {
		t.Errorf("expected truncated content, got %d bytes", len(content))
	}
	if len(result.Notes) == 0 {
		t.Errorf("expected truncation note")
	}
}

// TestRepairDispatchToolArguments_PreserveFileChange 验证文件变更工具不截断。
func TestRepairDispatchToolArguments_PreserveFileChange(t *testing.T) {
	big := make([]byte, 2000)
	for i := range big {
		big[i] = 'y'
	}
	raw := map[string]any{
		"content": string(big),
	}
	result := RepairDispatchToolArguments(raw, ToolArgumentRepairOptions{
		MaxStringBytes:        1024,
		PreserveLongStrings:   true, // file_change 类工具
	})
	content, ok := result.Arguments["content"].(string)
	if !ok {
		t.Fatal("content should be string")
	}
	if len(content) < 2000 {
		t.Errorf("file_change content should NOT be truncated, got %d bytes (want >=2000)", len(content))
	}
}

// TestRepairDispatchToolArguments_MetadataKeysNotFlattened 验证工具元数据键不阻碍展平。
func TestRepairDispatchToolArguments_MetadataKeysNotFlattened(t *testing.T) {
	raw := map[string]any{
		"arguments": map[string]any{"path": "/tmp/x"},
		"tool":      "read_file",
		"id":        "call_123",
	}
	result := RepairDispatchToolArguments(raw, ToolArgumentRepairOptions{})
	if result.Arguments["path"] != "/tmp/x" {
		t.Errorf("expected path=/tmp/x, got %v", result.Arguments["path"])
	}
}

// TestRepairDispatchToolArguments_JSONStringInArgs 验证真正的 JSON 字符串参数。
// 构造一个模拟真实场景：模型可能传入的参数包含 JSON 字符串形式的对象。
func TestRepairDispatchToolArguments_JSONStringInArgs(t *testing.T) {
	// 模拟 edit_file 工具：{"arguments": "{\"path\":\"main.go\",\"search\":\"old\",\"replace\":\"new\"}"}
	inner := map[string]any{
		"path":    "main.go",
		"search":  "old code",
		"replace": "new code",
	}
	innerJSON, _ := json.Marshal(inner)
	raw := map[string]any{
		"arguments": string(innerJSON),
	}
	result := RepairDispatchToolArguments(raw, ToolArgumentRepairOptions{})
	if result.Arguments["path"] != "main.go" {
		t.Errorf("expected path=main.go, got %v", result.Arguments["path"])
	}
	if result.Arguments["search"] != "old code" {
		t.Errorf("expected search=old code, got %v", result.Arguments["search"])
	}
	if result.Arguments["replace"] != "new code" {
		t.Errorf("expected replace=new code, got %v", result.Arguments["replace"])
	}
}
