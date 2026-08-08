package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// SearchHit is one tool directory match returned to the model: the tool's
// model-visible name plus its (possibly compact) description.
type SearchHit struct {
	Name string
	Desc string
}

// searchToolsKey is the context key for the directory-search provider injected
// by executeOne before every tool call.
type searchToolsKey struct{}

// withSearchTools stamps ctx with a closure that searches the live tool
// registry (visible tools only) by query.
func withSearchTools(ctx context.Context, fn func(query string, limit int) []SearchHit) context.Context {
	return context.WithValue(ctx, searchToolsKey{}, fn)
}

// searchToolsOf returns the injected search provider, if any. ok is false for
// a plain context (tool tests, calls outside the agent run loop).
func searchToolsOf(ctx context.Context) (func(string, int) []SearchHit, bool) {
	fn, ok := ctx.Value(searchToolsKey{}).(func(string, int) []SearchHit)
	return fn, ok
}

// SearchTool lets the model discover tools by functionality keywords
// (distilled from codex CLI's tool_search). The schema list is fixed — the
// result is plain tool output, so the tools parameter never changes at
// runtime and the prefix-cache invariant holds. Value: MCP tool names
// (mcp__<server>__<tool>) grow long and numerous; instead of guessing a name
// or burning a failed call, the model queries the directory once and learns
// the exact names and purposes.
type SearchTool struct{}

func NewSearchTool() *SearchTool { return &SearchTool{} }

func (*SearchTool) Name() string { return "search_tool" }

func (*SearchTool) Description() string {
	return "Search the tool directory by functionality keywords and list matching tool names " +
		"with their purposes. Use this when you are not sure which tool (especially an MCP " +
		"tool like mcp__<server>__<tool>) fits a task, or when you need the exact name to call. " +
		"Returns the model-visible name and a short description for each match; the tools " +
		"themselves are unchanged."
}

func (*SearchTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Functionality keywords to search tool names and descriptions (e.g. \"search files\", \"git commit\", \"edit lines\")"},"limit":{"type":"integer","minimum":1,"maximum":10,"description":"Max results to return (default 5)"}},"required":["query"]}`)
}

func (*SearchTool) CompactDescription() string {
	return "按关键词搜索工具目录（名字+描述），返回工具名与用途"
}

func (*SearchTool) CompactSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"搜索工具名与描述的关键词（如 \"搜索文件\"、\"git\"、\"编辑行\"）"},"limit":{"type":"integer","minimum":1,"maximum":10,"description":"最多返回条数（默认 5）"}},"required":["query"]}`)
}

// ReadOnly is true: searching the directory has no side effects, so it needs
// no approval and stays available in plan mode.
func (*SearchTool) ReadOnly() bool { return true }

func (*SearchTool) Kind() tool.ToolKind { return tool.KindRead }

func (*SearchTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var p struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("search_tool: invalid arguments: %v", err)
	}
	query := strings.TrimSpace(p.Query)
	if query == "" {
		return "", fmt.Errorf("search_tool: query is required (search the tool directory by functionality keywords)")
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 10 {
		limit = 10
	}
	fn, ok := searchToolsOf(ctx)
	if !ok || fn == nil {
		return "", fmt.Errorf("search_tool is only available inside an agent run (no tool directory in context)")
	}
	hits := fn(query, limit)
	if len(hits) == 0 {
		return fmt.Sprintf("No tools match %q. Try broader keywords (e.g. \"search\", \"git\", \"edit\").", query), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Matching tools for %q:\n", query)
	for _, h := range hits {
		desc := h.Desc
		if len(desc) > 120 {
			desc = desc[:117] + "..."
		}
		fmt.Fprintf(&b, "- %s: %s\n", h.Name, desc)
	}
	return b.String(), nil
}

// searchTools matches visible tool schemas against a case-insensitive query,
// ranking name hits before description-only hits; both buckets sort by name.
// limit caps the result; empty query or no matches yield an empty slice.
func searchTools(schemas []provider.ToolSchema, query string, limit int) []SearchHit {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" || limit <= 0 {
		return nil
	}
	var nameHits, descHits []SearchHit
	for _, s := range schemas {
		if s.Name == "" {
			continue
		}
		lowerName := strings.ToLower(s.Name)
		if strings.Contains(lowerName, q) {
			nameHits = append(nameHits, SearchHit{Name: s.Name, Desc: s.Description})
			continue
		}
		if strings.Contains(strings.ToLower(s.Description), q) {
			descHits = append(descHits, SearchHit{Name: s.Name, Desc: s.Description})
		}
	}
	sort.Slice(nameHits, func(i, j int) bool { return nameHits[i].Name < nameHits[j].Name })
	sort.Slice(descHits, func(i, j int) bool { return descHits[i].Name < descHits[j].Name })
	hits := append(nameHits, descHits...)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// searchToolsProvider backs the search_tool with the live registry's visible
// tools. Bound on AgentRunner so the tool has no direct registry dependency.
func (a *AgentRunner) searchToolsProvider(query string, limit int) []SearchHit {
	return searchTools(a.tools.FilteredSchemas(nil), query, limit)
}
