package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"tianxuan/internal/agent/offload"
	"tianxuan/internal/tool"
)

// SearchLargeOutput lets the agent query offloaded tool outputs that were
// too large for the context window. Supports three operations:
//
//	list       — show metadata for all offloaded files
//	read       — retrieve full content of one offloaded file
//	search     — search across all offloaded files for a substring
//
// The offload store is injected via WireSearchLargeOutputStore during boot.
type searchLargeOutput struct {
	store *offload.Store
}

func (t *searchLargeOutput) Name() string        { return "search_large_output" }
func (t *searchLargeOutput) Description() string { return "查询被卸载到磁盘的大型工具输出。支持 list(列出所有)/read(读取指定文件)/search(跨文件搜索)" }
func (t *searchLargeOutput) ReadOnly() bool       { return true }

func (t *searchLargeOutput) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "operation": {
      "type": "string",
      "description": "操作类型",
      "enum": ["list", "read", "search"]
    },
    "name": {
      "type": "string",
      "description": "文件名(read操作必需，从list结果中获取)"
    },
    "query": {
      "type": "string",
      "description": "搜索关键词(search操作必需，大小写不敏感)"
    }
  },
  "required": ["operation"]
}`)
}

func (t *searchLargeOutput) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Operation string `json:"operation"`
		Name      string `json:"name"`
		Query     string `json:"query"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if t.store == nil {
		return "", fmt.Errorf("search_large_output: no offload store available (context offloading not enabled)")
	}

	switch in.Operation {
	case "list":
		return t.doList()
	case "read":
		if in.Name == "" {
			return "", fmt.Errorf("read operation requires 'name' parameter")
		}
		return t.doRead(in.Name)
	case "search":
		if in.Query == "" {
			return "", fmt.Errorf("search operation requires 'query' parameter")
		}
		return t.doSearch(in.Query)
	default:
		return "", fmt.Errorf("unknown operation %q (want list, read, or search)", in.Operation)
	}
}

func (t *searchLargeOutput) doList() (string, error) {
	files, err := t.store.List()
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "(no offloaded files)", nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d offloaded file(s):\n", len(files)))
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("  %s (%d bytes)\n", f.Name, f.Size))
	}
	return sb.String(), nil
}

func (t *searchLargeOutput) doRead(name string) (string, error) {
	return t.store.Read(name)
}

func (t *searchLargeOutput) doSearch(query string) (string, error) {
	return t.store.Search(query, 20)
}

func (t *searchLargeOutput) CompactDescription() string { return compactDesc["search_large_output"] }
func (t *searchLargeOutput) CompactSchema() json.RawMessage {
	return compactSchema["search_large_output"]
}

func init() {
	tool.RegisterBuiltin(&searchLargeOutput{})
}

// WireSearchLargeOutputStore injects the offload store into the registered
// search_large_output tool instance. Call from boot after the agent and its
// offload store are constructed.
func WireSearchLargeOutputStore(store *offload.Store) {
	for _, t := range tool.Builtins() {
		if slo, ok := t.(*searchLargeOutput); ok {
			slo.store = store
			return
		}
	}
}
