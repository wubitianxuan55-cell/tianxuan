package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"tianxuan/internal/provider"
)

// fakeSchemas returns a small deterministic tool directory for search tests.
func fakeSchemas() []provider.ToolSchema {
	return []provider.ToolSchema{
		{Name: "grep", Description: "Search file contents by regex in the workspace"},
		{Name: "read_file", Description: "Read a file with optional line range"},
		{Name: "edit_file", Description: "Replace exact text in a file"},
		{Name: "bash", Description: "Execute a command in the shell"},
		{Name: "mcp__codegraph__codegraph_context", Description: "Return entry points for a code question"},
		{Name: "git_commit", Description: "Commit staged changes"},
	}
}

func TestSearchToolsMatchesName(t *testing.T) {
	hits := searchTools(fakeSchemas(), "grep", 5)
	if len(hits) == 0 {
		t.Fatal("query \"grep\" should hit the grep tool")
	}
	if hits[0].Name != "grep" {
		t.Errorf("first hit = %q, want grep (name match ranked first)", hits[0].Name)
	}
}

func TestSearchToolsMatchesDescription(t *testing.T) {
	hits := searchTools(fakeSchemas(), "staged changes", 5)
	found := false
	for _, h := range hits {
		if h.Name == "git_commit" {
			found = true
		}
	}
	if !found {
		t.Errorf("query \"staged changes\" should hit git_commit via description, got %+v", hits)
	}
}

func TestSearchToolsCaseInsensitive(t *testing.T) {
	hits := searchTools(fakeSchemas(), "GREP", 5)
	if len(hits) == 0 || hits[0].Name != "grep" {
		t.Errorf("query \"GREP\" should hit grep case-insensitively, got %+v", hits)
	}
}

func TestSearchToolsLimit(t *testing.T) {
	hits := searchTools(fakeSchemas(), "file", 2)
	if len(hits) != 2 {
		t.Errorf("limit=2 should return 2 hits, got %d: %+v", len(hits), hits)
	}
}

func TestSearchToolsNameMatchRankedFirst(t *testing.T) {
	// "codegraph" appears in one name and in another's description region —
	// name matches must sort before description-only matches.
	schemas := []provider.ToolSchema{
		{Name: "codegraph_util", Description: "helper for code graphs"},
		{Name: "helper", Description: "codegraph utilities for symbol lookup"},
	}
	hits := searchTools(schemas, "codegraph", 5)
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d", len(hits))
	}
	if hits[0].Name != "codegraph_util" {
		t.Errorf("name match should rank first, got %q first", hits[0].Name)
	}
}

func TestSearchToolsMCPNamesSearchable(t *testing.T) {
	hits := searchTools(fakeSchemas(), "codegraph", 5)
	if len(hits) == 0 {
		t.Fatal("MCP tool should be searchable by server name")
	}
	if !strings.HasPrefix(hits[0].Name, "mcp__codegraph__") {
		t.Errorf("first hit = %q, want mcp__codegraph__ tool", hits[0].Name)
	}
}

func TestSearchToolsNoMatchReturnsEmpty(t *testing.T) {
	hits := searchTools(fakeSchemas(), "zzz-no-such-tool", 5)
	if len(hits) != 0 {
		t.Errorf("no-match query should return empty slice, got %+v", hits)
	}
}

func TestSearchToolExecuteEmptyQueryErrors(t *testing.T) {
	ctx := withSearchTools(context.Background(), func(string, int) []SearchHit { return nil })
	_, err := NewSearchTool().Execute(ctx, json.RawMessage(`{"query":"  "}`))
	if err == nil {
		t.Fatal("empty/blank query should error loudly")
	}
}

func TestSearchToolExecuteNoProviderErrors(t *testing.T) {
	_, err := NewSearchTool().Execute(context.Background(), json.RawMessage(`{"query":"grep"}`))
	if err == nil {
		t.Fatal("execute without injected provider should error loudly")
	}
}

func TestSearchToolExecuteFormatsHits(t *testing.T) {
	ctx := withSearchTools(context.Background(), func(q string, limit int) []SearchHit {
		if q != "grep" {
			t.Errorf("provider got query %q, want grep", q)
		}
		if limit != 3 {
			t.Errorf("provider got limit %d, want 3", limit)
		}
		return []SearchHit{{Name: "grep", Desc: "Search file contents"}}
	})
	out, err := NewSearchTool().Execute(ctx, json.RawMessage(`{"query":"grep","limit":3}`))
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !strings.Contains(out, "grep") || !strings.Contains(out, "Search file contents") {
		t.Errorf("output should contain tool name and description, got %q", out)
	}
}

func TestSearchToolExecuteDefaultLimit(t *testing.T) {
	ctx := withSearchTools(context.Background(), func(q string, limit int) []SearchHit {
		if limit != 5 {
			t.Errorf("default limit = %d, want 5", limit)
		}
		return nil
	})
	if _, err := NewSearchTool().Execute(ctx, json.RawMessage(`{"query":"grep"}`)); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
}
