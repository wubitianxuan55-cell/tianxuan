package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"tianxuan/internal/memory"
	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(memorySearch{}) }

// SetMemorySearchIndex injects the search index for the memory_search tool.
// Called by boot after memory is loaded. Thread-safe: set once before any
// turn runs, read-only thereafter.
func SetMemorySearchIndex(idx *memory.SearchIndex) { memorySearchIndex = idx }

var memorySearchIndex *memory.SearchIndex

type memorySearch struct{}

func (memorySearch) Name() string { return "memory_search" }

func (memorySearch) Description() string {
	return "Search saved memories by keyword. Returns matching memory entries ranked by relevance with preview descriptions. Optionally filter by kind: semantic (facts/prefs), episodic (past experiences), procedural (rules). Use this to find prior context, user preferences, or project facts before answering — don't ask the user about something memory may already record."
}

func (memorySearch) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "query":{"type":"string","description":"Search query — one or more keywords. The tool OR-matches tokens and ranks by relevance."},
  "kind":{"type":"string","enum":["semantic","episodic","procedural"],"description":"Filter by cognitive kind. Omit to search all."}
},
"required":["query"]
}`)
}

func (memorySearch) ReadOnly() bool { return true }
func (memorySearch) Kind() tool.ToolKind { return tool.KindSearch }

func (memorySearch) CompactDescription() string { return compactDesc["memory_search"] }
func (memorySearch) CompactSchema() json.RawMessage   { return compactSchema["memory_search"] }

func (memorySearch) Execute(_ context.Context, args json.RawMessage) (string, error) {
	idx := memorySearchIndex
	if idx == nil {
		return "", fmt.Errorf("memory index not available — save some memories first with the remember tool")
	}

	var p struct {
		Query string `json:"query"`
		Kind  string `json:"kind"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if strings.TrimSpace(p.Query) == "" {
		return "", fmt.Errorf("query is required")
	}

	var matches []memory.SearchMatch
	if p.Kind != "" {
		matches = idx.SearchByKind(p.Query, memory.NormalizeKind(p.Kind))
	} else {
		matches = idx.Search(p.Query)
	}
	if len(matches) == 0 {
		return "No memories matched your query.", nil
	}

	// Return top 10 results with preview descriptions.
	limit := len(matches)
	if limit > 10 {
		limit = 10
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d matching memories:\n\n", len(matches)))
	for i, m := range matches[:limit] {
		bars := scoreBars(int(m.Score*100), len(p.Query))
		preview := m.Preview
		if len(preview) > 160 {
			preview = preview[:160] + "…"
		}
		fmt.Fprintf(&b, "%d. %s %s\n   %s\n", i+1, bars, m.Name, preview)
	}
	if len(matches) > limit {
		fmt.Fprintf(&b, "\n... and %d more. Narrow your query for fewer results.\n", len(matches)-limit)
	}
	b.WriteString("\nUse read_file to view a specific memory's full content.")
	return b.String(), nil
}

// scoreBars renders a relevance indicator proportional to the score.
func scoreBars(score, maxScore int) string {
	if maxScore < 1 {
		maxScore = 1
	}
	ratio := float64(score) / float64(maxScore)
	switch {
	case ratio >= 0.8:
		return "███"
	case ratio >= 0.5:
		return "██"
	default:
		return "█"
	}
}

// sort builtins
var _ = sort.Ints
