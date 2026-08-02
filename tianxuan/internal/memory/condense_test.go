package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stagePending writes n pending candidates into the store's pending dir.
func stagePending(t *testing.T, s Store, n int) {
	t.Helper()
	dir := filepath.Join(s.Dir, pendingDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		c := Candidate{
			Name:        fmt.Sprintf("cand-%02d", i),
			Title:       fmt.Sprintf("Candidate %02d", i),
			Description: fmt.Sprintf("description %02d", i),
			Type:        TypeUser,
			Body:        "body",
		}
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pendingPath(s, c.Name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestCondensePendingBelowThreshold verifies a backlog of exactly 30 staged
// candidates is left untouched: condensation only fires past the threshold.
func TestCondensePendingBelowThreshold(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	stagePending(t, s, pendingCondenseThreshold)

	n, err := CondensePending(s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("condense must not trigger at threshold, staged %d", n)
	}
	if got := len(PendingCandidates(s)); got != pendingCondenseThreshold {
		t.Fatalf("pending count changed: got %d, want %d", got, pendingCondenseThreshold)
	}
}

// TestCondensePendingAboveThreshold verifies 31 staged candidates are distilled
// into a single consolidated candidate that folds in the original content.
func TestCondensePendingAboveThreshold(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	stagePending(t, s, pendingCondenseThreshold+1)

	n, err := CondensePending(s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("condense should stage 1 candidate, got %d", n)
	}
	pending := PendingCandidates(s)
	if len(pending) != 1 {
		t.Fatalf("want 1 pending after condensation, got %d", len(pending))
	}
	c := pending[0]
	if !strings.Contains(c.Description, "31") {
		t.Errorf("condensed candidate must mention the count: %q", c.Description)
	}
	if !strings.Contains(c.Body, "Candidate 00") || !strings.Contains(c.Body, "Candidate 30") {
		t.Errorf("condensed body must fold in original titles: %q", c.Body)
	}
	if c.Type != TypeProject {
		t.Errorf("condensed candidate type = %q, want %q", c.Type, TypeProject)
	}
}

// TestCondensePendingEmpty verifies a store with no staged candidates is a no-op.
func TestCondensePendingEmpty(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	n, err := CondensePending(s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("empty pending must not condense, staged %d", n)
	}
}
