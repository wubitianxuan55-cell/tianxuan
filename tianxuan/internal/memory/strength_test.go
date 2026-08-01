package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestReinforceAccessCountsRecalls verifies repeated recalls accumulate a
// strength counter and refresh the last-access timestamp.
func TestReinforceAccessCountsRecalls(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := ReinforceAccess(s, "build-rule"); err != nil {
		t.Fatal(err)
	}
	if err := ReinforceAccess(s, "build-rule"); err != nil {
		t.Fatal(err)
	}
	st, err := ReadStrength(s, "build-rule")
	if err != nil {
		t.Fatal(err)
	}
	if st.Count != 2 {
		t.Fatalf("count = %d, want 2", st.Count)
	}
	if st.LastAccessAt.IsZero() {
		t.Fatal("lastAccessAt must be set")
	}
}

// TestEvictStaleArchivesOnlyColdMemories verifies TTL eviction archives
// memories with no recent access and low count, while recently-accessed ones
// survive.
func TestEvictStaleArchivesOnlyColdMemories(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	if _, err := s.Save(Memory{Name: "hot-rule", Description: "d", Type: TypeProject, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Memory{Name: "cold-rule", Description: "d", Type: TypeProject, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := ReinforceAccess(s, "hot-rule"); err != nil { // accessed just now
		t.Fatal(err)
	}
	// Cold memory: last access 120 days ago, count 1.
	if err := WriteStrength(s, "cold-rule", Strength{Count: 1, LastAccessAt: now.Add(-120 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}

	n, err := EvictStale(s, 90*24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want 1 stale memory archived, got %d", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "cold-rule.md")); !os.IsNotExist(err) {
		t.Fatal("stale memory should be archived")
	}
	if _, err := os.Stat(filepath.Join(dir, "hot-rule.md")); err != nil {
		t.Fatalf("recently-accessed memory must survive: %v", err)
	}
}

// TestBuildProfileSummarizesProject verifies the project profile condenses the
// saved memory set into concepts, files, and common errors.
func TestBuildProfileSummarizesProject(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for _, m := range []Memory{
		{Name: "auth-flow", Title: "Auth Flow", Description: "JWT 认证流程设计", Type: TypeProject, Body: "使用 JWT 认证"},
		{Name: "jwt-secret", Title: "JWT Secret", Description: "JWT 密钥位置", Type: TypeReference, Body: "JWT 密钥在 env"},
		{Name: "error-log", Title: "Error Log", Description: "JWT 过期错误处理", Type: TypeProject, Body: "JWT 过期报错，需要刷新"},
	} {
		if _, err := s.Save(m); err != nil {
			t.Fatal(err)
		}
	}
	p, err := BuildProfile(s)
	if err != nil {
		t.Fatal(err)
	}
	if p.TotalMemories != 3 {
		t.Fatalf("TotalMemories = %d, want 3", p.TotalMemories)
	}
	if len(p.TopConcepts) == 0 {
		t.Fatal("profile must surface top concepts")
	}
	joined := strings.ToLower(strings.Join(p.TopConcepts, " "))
	if !strings.Contains(joined, "jwt") {
		t.Fatalf("profile should rank jwt as a top concept: %+v", p.TopConcepts)
	}
}
