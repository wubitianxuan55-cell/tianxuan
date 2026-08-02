package control

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tianxuan/internal/memory"
)

// stagePendingCandidates writes n pending candidates directly into the
// controller's memory store, bypassing auto-extract.
func stagePendingCandidates(t *testing.T, ctrl *Controller, n int) {
	t.Helper()
	store := ctrl.Memory().Store
	dir := filepath.Join(store.Dir, "pending")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		c := memory.Candidate{
			Name:        fmt.Sprintf("cand-%02d", i),
			Title:       fmt.Sprintf("Candidate %02d", i),
			Description: fmt.Sprintf("description %02d", i),
			Type:        memory.TypeUser,
			Body:        "body",
		}
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, c.Name+".json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestAutoCondenseDistillsOverflow verifies a backlog past 30 staged candidates
// is distilled into a single consolidated candidate after a turn.
func TestAutoCondenseDistillsOverflow(t *testing.T) {
	dir := t.TempDir()
	ctrl := newExtractController(t, dir, nil)
	stagePendingCandidates(t, ctrl, 31)

	ctrl.autoCondense()

	pending := ctrl.PendingMemories()
	if len(pending) != 1 {
		t.Fatalf("want 1 pending after condensation, got %d", len(pending))
	}
	if !strings.Contains(pending[0].Description, "31") {
		t.Errorf("condensed candidate must mention the count: %q", pending[0].Description)
	}
}

// TestAutoCondenseSkipsSmallBacklog verifies a backlog at the threshold is
// left untouched.
func TestAutoCondenseSkipsSmallBacklog(t *testing.T) {
	dir := t.TempDir()
	ctrl := newExtractController(t, dir, nil)
	stagePendingCandidates(t, ctrl, 30)

	ctrl.autoCondense()

	if got := len(ctrl.PendingMemories()); got != 30 {
		t.Fatalf("backlog of 30 must be untouched, got %d", got)
	}
}
