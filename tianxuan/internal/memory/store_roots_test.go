package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkGitRepo creates a fake git root (a directory containing .git) under t.TempDir.
func mkGitRepo(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestStoreForUsesGitRoot verifies the auto-memory store is keyed by the
// nearest git root, so sessions launched from different subdirectories of one
// repository share the same memory (Qwen/Claude convention).
func TestStoreForUsesGitRoot(t *testing.T) {
	root := mkGitRepo(t)
	subA := filepath.Join(root, "pkg", "a")
	subB := filepath.Join(root, "pkg", "b")
	if err := os.MkdirAll(subA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(subB, 0o755); err != nil {
		t.Fatal(err)
	}

	fromRoot := StoreFor("/home/me/.config/tianxuan", root)
	fromA := StoreFor("/home/me/.config/tianxuan", subA)
	fromB := StoreFor("/home/me/.config/tianxuan", subB)

	if fromA.Dir != fromB.Dir {
		t.Fatalf("subdirectories must share one store dir:\n  A=%s\n  B=%s", fromA.Dir, fromB.Dir)
	}
	if fromA.Dir != fromRoot.Dir {
		t.Fatalf("subdirectory store must equal git-root store:\n  root=%s\n  sub=%s", fromRoot.Dir, fromA.Dir)
	}
	// The slug must be the git root path, not the subdirectory path.
	if strings.Contains(fromA.Dir, slugify(absOf(subA))) {
		t.Fatalf("store dir must not contain the subdirectory slug: %s", fromA.Dir)
	}
	if !strings.Contains(fromA.Dir, slugify(absOf(root))) {
		t.Fatalf("store dir must contain the git-root slug: %s", fromA.Dir)
	}
}

// TestStoreForFallsBackToCwdWithoutGit verifies an un-versioned working
// directory still keys its memory by cwd (existing behavior preserved).
func TestStoreForFallsBackToCwdWithoutGit(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "plain")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	s := StoreFor("/home/me/.config/tianxuan", cwd)
	if !strings.Contains(s.Dir, slugify(absOf(cwd))) {
		t.Fatalf("no-git store must use the cwd slug: %s", s.Dir)
	}
}

// TestStoreForSetsGlobalDir verifies the cross-project user memory directory is
// initialized under the user config root.
func TestStoreForSetsGlobalDir(t *testing.T) {
	s := StoreFor("/home/me/.config/tianxuan", "/Users/me/proj")
	if s.GlobalDir == "" {
		t.Fatal("GlobalDir must be set")
	}
	if want := filepath.Join("/home/me/.config/tianxuan", "memories"); s.GlobalDir != want {
		t.Fatalf("GlobalDir = %s, want %s", s.GlobalDir, want)
	}
}

// TestSaveRoutesUserTypeToGlobalDir verifies user-type memories land in the
// cross-project directory while project-type memories stay project-scoped.
func TestSaveRoutesUserTypeToGlobalDir(t *testing.T) {
	s := Store{
		Dir:       filepath.Join(t.TempDir(), "project"),
		GlobalDir: filepath.Join(t.TempDir(), "global"),
	}
	if _, err := s.Save(Memory{Name: "likes-go", Description: "d", Type: TypeUser, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Memory{Name: "refactor-goal", Description: "d", Type: TypeProject, Body: "b"}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(s.GlobalDir, "likes-go.md")); err != nil {
		t.Fatalf("user memory should be in GlobalDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "likes-go.md")); !os.IsNotExist(err) {
		t.Fatalf("user memory must not be in project dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "refactor-goal.md")); err != nil {
		t.Fatalf("project memory should be in project dir: %v", err)
	}
	// Indexes are written where the fact lives.
	if !strings.Contains(s.Index(), "likes-go.md") {
		t.Fatalf("global fact missing from merged index:\n%s", s.Index())
	}
	if !strings.Contains(s.Index(), "refactor-goal.md") {
		t.Fatalf("project fact missing from merged index:\n%s", s.Index())
	}
}

// TestListSpansGlobalAndProject verifies List merges both directories.
func TestListSpansGlobalAndProject(t *testing.T) {
	s := Store{
		Dir:       filepath.Join(t.TempDir(), "project"),
		GlobalDir: filepath.Join(t.TempDir(), "global"),
	}
	if _, err := s.Save(Memory{Name: "global-fact", Description: "g", Type: TypeUser, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Memory{Name: "local-fact", Description: "l", Type: TypeProject, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	list := s.List()
	if len(list) != 2 {
		t.Fatalf("want 2 memories, got %d: %+v", len(list), list)
	}
	names := map[string]bool{}
	for _, m := range list {
		names[m.Name] = true
	}
	if !names["global-fact"] || !names["local-fact"] {
		t.Fatalf("List missing entries: %+v", names)
	}
}

// TestMigrateLegacyStore moves a pre-git-root memory directory into the
// git-root-keyed location on first load after the change.
func TestMigrateLegacyStore(t *testing.T) {
	root := mkGitRepo(t)
	sub := filepath.Join(root, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	userDir := filepath.Join(t.TempDir(), "tianxuan")
	oldDir := filepath.Join(userDir, "projects", slugify(absOf(sub)), "memory")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Seed a legacy memory file + index under the old slug.
	seed := Store{Dir: oldDir}
	if _, err := seed.Save(Memory{Name: "legacy", Description: "d", Type: TypeProject, Body: "b"}); err != nil {
		t.Fatal(err)
	}

	if err := migrateLegacyStore(userDir, sub); err != nil {
		t.Fatal(err)
	}
	newDir := filepath.Join(userDir, "projects", slugify(absOf(root)), "memory")
	if _, err := os.Stat(filepath.Join(newDir, "legacy.md")); err != nil {
		t.Fatalf("memory not migrated to git-root dir %s: %v", newDir, err)
	}
	if _, err := os.Stat(filepath.Join(oldDir, "legacy.md")); !os.IsNotExist(err) {
		t.Fatalf("old memory directory should be gone after migration: %v", err)
	}
}

// TestMigrateLegacyStoreTargetExists verifies migration never clobbers an
// existing git-root store (no merging).
func TestMigrateLegacyStoreTargetExists(t *testing.T) {
	root := mkGitRepo(t)
	sub := filepath.Join(root, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	userDir := filepath.Join(t.TempDir(), "tianxuan")
	oldDir := filepath.Join(userDir, "projects", slugify(absOf(sub)), "memory")
	newDir := filepath.Join(userDir, "projects", slugify(absOf(root)), "memory")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Dir: oldDir}).Save(Memory{Name: "old-fact", Description: "d", Type: TypeProject, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Dir: newDir}).Save(Memory{Name: "new-fact", Description: "d", Type: TypeProject, Body: "b"}); err != nil {
		t.Fatal(err)
	}

	if err := migrateLegacyStore(userDir, sub); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(newDir, "old-fact.md")); !os.IsNotExist(err) {
		t.Fatal("migration must not merge into an existing target")
	}
	if _, err := os.Stat(filepath.Join(newDir, "new-fact.md")); err != nil {
		t.Fatalf("existing target must stay intact: %v", err)
	}
}

// TestChangeTypeMovesBetweenDirs verifies promoting a fact to user type moves
// its file into the cross-project directory and removes the project copy.
func TestChangeTypeMovesBetweenDirs(t *testing.T) {
	s := Store{
		Dir:       filepath.Join(t.TempDir(), "project"),
		GlobalDir: filepath.Join(t.TempDir(), "global"),
	}
	if _, err := s.Save(Memory{Name: "tabs-rule", Description: "d", Type: TypeProject, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeType("tabs-rule", TypeUser); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.GlobalDir, "tabs-rule.md")); err != nil {
		t.Fatalf("promoted memory should live in GlobalDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "tabs-rule.md")); !os.IsNotExist(err) {
		t.Fatalf("project copy must be removed after promotion: %v", err)
	}
}

// TestArchiveLocatesFactsInBothDirs verifies forget/archive resolves a fact
// regardless of which directory it lives in.
func TestArchiveLocatesFactsInBothDirs(t *testing.T) {
	s := Store{
		Dir:       filepath.Join(t.TempDir(), "project"),
		GlobalDir: filepath.Join(t.TempDir(), "global"),
	}
	if _, err := s.Save(Memory{Name: "global-fact", Description: "g", Type: TypeUser, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("global-fact"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.GlobalDir, "global-fact.md")); !os.IsNotExist(err) {
		t.Fatalf("global fact not archived: %v", err)
	}
	if names := s.List(); len(names) != 0 {
		t.Fatalf("List after delete = %+v, want empty", names)
	}
}
