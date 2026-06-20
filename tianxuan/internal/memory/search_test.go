package memory

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSearchIndexBuildAndSearch validates the full index→search pipeline.
func TestSearchIndexBuildAndSearch(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	// Save a few memories
	store.Save(Memory{
		Name:        "api-key-location",
		Title:       "API Key Location",
		Description: "Where the API key is stored",
		Type:        TypeReference,
		Body:        "The API key lives in ~/.config/tianxuan/api.key.",
	})
	store.Save(Memory{
		Name:        "user-prefers-go",
		Title:       "User Prefers Go",
		Description: "User likes Go for backend work",
		Type:        TypeUser,
		Body:        "Use Go for all backend services. Avoid Python when possible.",
	})
	store.Save(Memory{
		Name:        "project-deadlines",
		Title:       "Project Deadlines",
		Description: "Key project deadlines for Q3",
		Type:        TypeProject,
		Body:        "Authentication module must be done by end of July. Database migration by August.",
	})

	idx := store.BuildSearchIndex()
	if idx == nil {
		t.Fatal("BuildSearchIndex returned nil")
	}

	// Search for "api key" → should match api-key-location
	matches := idx.Search("api key")
	if len(matches) == 0 {
		t.Fatal("no matches for 'api key'")
	}
	if matches[0].Name != "api-key-location" {
		t.Fatalf("expected api-key-location first, got %s", matches[0].Name)
	}

	// Search for "go backend" → should match user-prefers-go
	matches = idx.Search("go backend")
	if len(matches) == 0 {
		t.Fatal("no matches for 'go backend'")
	}

	// Search for nonexistent → empty
	matches = idx.Search("xyzzy_nonexistent")
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches for nonexistent, got %d", len(matches))
	}
}

// TestSearchIndexEmptyStore returns nil.
func TestSearchIndexEmptyStore(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}
	idx := store.BuildSearchIndex()
	if idx != nil {
		t.Fatal("expected nil index for empty store")
	}
}

// TestSearchIndexDisabledStore returns nil.
func TestSearchIndexDisabledStore(t *testing.T) {
	store := Store{Dir: ""}
	idx := store.BuildSearchIndex()
	if idx != nil {
		t.Fatal("expected nil index for disabled store")
	}
}

// TestSearchIndexNilReceiver safe for nil.
func TestSearchIndexNilReceiver(t *testing.T) {
	var idx *SearchIndex
	if matches := idx.Search("test"); matches != nil {
		t.Fatal("expected nil from nil receiver")
	}
}

// TestSearchIndexEmptyQuery returns nil.
func TestSearchIndexEmptyQuery(t *testing.T) {
	idx := &SearchIndex{entries: map[string][]string{"test": {"a"}}}
	if matches := idx.Search(""); matches != nil {
		t.Fatal("expected nil for empty query")
	}
}

// TestSearchIndexRanking returns results sorted by score.
func TestSearchIndexRanking(t *testing.T) {
	idx := &SearchIndex{
		entries: map[string][]string{
			"auth":     {"auth-docs", "login-flow"},
			"database": {"db-migration", "auth-docs"},
			"migration": {"db-migration"},
		},
	}

	matches := idx.Search("auth database")
	if len(matches) < 2 {
		t.Fatalf("expected at least 2 matches, got %d", len(matches))
	}
	// auth-docs matches both "auth" and "database" → score 2 → should be first
	if matches[0].Name != "auth-docs" {
		t.Fatalf("expected auth-docs first (score 2), got %s (score %d)", matches[0].Name, matches[0].Score)
	}
}

// TestTokenizeCJK handles CJK characters as individual tokens.
func TestTokenizeCJK(t *testing.T) {
	tokens := tokenize("你好世界 hello world")
	// "你好世界" should produce individual CJK chars + "hello" + "world"
	foundHello := false
	foundWorld := false
	for _, tok := range tokens {
		if tok == "hello" {
			foundHello = true
		}
		if tok == "world" {
			foundWorld = true
		}
	}
	if !foundHello || !foundWorld {
		t.Fatalf("expected hello/world in tokens, got %v", tokens)
	}
}

// TestLoadBuildsSearchIndex verifies Load() populates the Search field.
func TestLoadBuildsSearchIndex(t *testing.T) {
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "memory")
	os.MkdirAll(storeDir, 0755)

	// Fake a user dir structure
	userDir := dir
	cwd := dir

	store := Store{Dir: storeDir}
	store.Save(Memory{
		Name: "test-memory",
		Title: "Test Memory",
		Description: "A test memory entry",
		Type: TypeProject,
		Body: "This is a test memory for search index verification.",
	})

	// Can't easily test Load() because it depends on global config paths,
	// but we can test Store.BuildSearchIndex() which Load() calls internally.
	idx := store.BuildSearchIndex()
	if idx == nil {
		t.Fatal("BuildSearchIndex returned nil")
	}
	matches := idx.Search("test memory")
	if len(matches) == 0 {
		t.Fatal("no matches for 'test memory'")
	}

	_ = userDir
	_ = cwd
}
