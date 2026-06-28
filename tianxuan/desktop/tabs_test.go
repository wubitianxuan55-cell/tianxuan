package main

import (
	"encoding/json"
	"testing"
)

func TestRandomTabID(t *testing.T) {
	// IDs should be unique across many calls.
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := randomTabID()
		if id == "" {
			t.Fatal("randomTabID returned empty string")
		}
		if len(id) != 16 {
			t.Fatalf("randomTabID returned id of length %d, want 16: %q", len(id), id)
		}
		if seen[id] {
			t.Fatalf("randomTabID returned duplicate id: %q", id)
		}
		seen[id] = true
		// All characters should be hex.
		for _, c := range id {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Fatalf("randomTabID returned non-hex character %q in id %q", c, id)
			}
		}
	}
}

func TestNewWorkspaceTab(t *testing.T) {
	tab := newWorkspaceTab("project", "/tmp/test", "My Topic")
	if tab.ID == "" {
		t.Fatal("newWorkspaceTab: expected non-empty ID")
	}
	if tab.Scope != "project" {
		t.Fatalf("newWorkspaceTab: scope = %q, want %q", tab.Scope, "project")
	}
	if tab.WorkspaceRoot != "/tmp/test" {
		t.Fatalf("newWorkspaceTab: WorkspaceRoot = %q, want %q", tab.WorkspaceRoot, "/tmp/test")
	}
	if tab.TopicTitle != "My Topic" {
		t.Fatalf("newWorkspaceTab: TopicTitle = %q, want %q", tab.TopicTitle, "My Topic")
	}
	if tab.Ready {
		t.Fatal("newWorkspaceTab: Ready should be false initially")
	}
	if tab.DisabledMCP == nil {
		t.Fatal("newWorkspaceTab: DisabledMCP map should be initialised")
	}
}

func TestTabMeta(t *testing.T) {
	tab := newWorkspaceTab("global", "", "Global Tab")
	tab.Label = "deepseek"
	tab.Ready = true
	tab.ActivityStatus = "thinking"

	meta := tab.tabMeta()
	if meta.ID != tab.ID {
		t.Fatalf("tabMeta: ID = %q, want %q", meta.ID, tab.ID)
	}
	if meta.Scope != "global" {
		t.Fatalf("tabMeta: Scope = %q, want %q", meta.Scope, "global")
	}
	if meta.Title != "Global Tab" {
		t.Fatalf("tabMeta: Title = %q, want %q", meta.Title, "Global Tab")
	}
	if !meta.Ready {
		t.Fatal("tabMeta: Ready should be true")
	}
	if meta.Label != "deepseek" {
		t.Fatalf("tabMeta: Label = %q, want %q", meta.Label, "deepseek")
	}
	if meta.ActivityStatus != "thinking" {
		t.Fatalf("tabMeta: ActivityStatus = %q, want %q", meta.ActivityStatus, "thinking")
	}
}

func TestTabMetaNil(t *testing.T) {
	var tab *WorkspaceTab
	meta := tab.tabMeta()
	if meta.ID != "" {
		t.Fatalf("tabMeta on nil tab should return empty TabMeta, got ID=%q", meta.ID)
	}
}

func TestTabPersistEntryRoundTrip(t *testing.T) {
	entry := tabPersistEntry{
		ID:            "abc123",
		Scope:         "project",
		WorkspaceRoot: "/some/path",
		TopicTitle:    "Test Project",
		SessionPath:   "/tmp/session.jsonl",
	}
	b, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal tabPersistEntry: %v", err)
	}
	var got tabPersistEntry
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal tabPersistEntry: %v", err)
	}
	if got != entry {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", got, entry)
	}
}
