package agent

import (
	"sort"
	"strings"
	"testing"
)

func TestTokenBudgetNoLimitDisabled(t *testing.T) {
	tb := NewTokenBudget(0, nil)
	if tb == nil {
		t.Fatal("zero limit should still return a usable tracker (disabled)")
	}
	if tb.used != 0 {
		t.Fatalf("fresh tracker used = %d, want 0", tb.used)
	}
	if msg, ok := tb.Check(50_000, 10_000); ok {
		t.Fatalf("disabled budget should never remind, got %q", msg)
	}
}

func TestTokenBudgetRemindsAtThresholds(t *testing.T) {
	tb := NewTokenBudget(1_000_000, []int64{800_000, 500_000, 100_000})
	if tb == nil {
		t.Fatal("expected non-nil tracker")
	}
	// 150k used, 850k remaining — no threshold crossed.
	if msg, ok := tb.Check(0, 150_000); ok {
		t.Fatalf("no threshold crossed, got %q", msg)
	}
	// Cross 800k remaining: remind once (message reports actual remaining).
	msg, ok := tb.Check(0, 210_000)
	if !ok {
		t.Fatal("crossed 800k threshold, should remind")
	}
	if !strings.Contains(msg, "790000") {
		t.Errorf("reminder should mention remaining tokens, got %q", msg)
	}
	// Same threshold again: no duplicate.
	if _, ok := tb.Check(0, 230_000); ok {
		t.Fatal("same threshold crossed twice should not remind again")
	}
	// Cross 500k remaining: remind again.
	msg, ok = tb.Check(0, 520_000)
	if !ok {
		t.Fatal("crossed 500k threshold, should remind")
	}
	if !strings.Contains(msg, "480000") {
		t.Errorf("reminder should mention remaining tokens, got %q", msg)
	}
}

func TestTokenBudgetExhaustedBlocks(t *testing.T) {
	tb := NewTokenBudget(100_000, []int64{50_000})
	if tb == nil {
		t.Fatal("expected non-nil tracker")
	}
	// Over budget entirely (cumulative 110k > 100k limit).
	msg, ok := tb.Check(0, 110_000)
	if !ok {
		t.Fatal("exhausted budget should remind")
	}
	if !strings.Contains(msg, "exhausted") && !strings.Contains(msg, "0") {
		t.Errorf("exhausted reminder should say so, got %q", msg)
	}
}

func TestTokenBudgetRemindersSortedDesc(t *testing.T) {
	// Reminders given in arbitrary order must be applied sorted descending.
	tb := NewTokenBudget(1_000_000, []int64{100_000, 800_000, 500_000})
	if tb == nil {
		t.Fatal("expected non-nil tracker")
	}
	if len(tb.reminders) != 3 {
		t.Fatalf("reminders = %v, want 3", tb.reminders)
	}
	if !sort.SliceIsSorted(tb.reminders, func(i, j int) bool { return tb.reminders[i] > tb.reminders[j] }) {
		t.Fatalf("reminders not sorted descending: %v", tb.reminders)
	}
}
