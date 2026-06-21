package strutil

import (
	"fmt"
	"testing"
)

// TestItoaIdenticalToOriginals verifies Strutil.Itoa produces byte-identical
// output to every inlined itoa copy that previously existed across the codebase.
// This guarantees the refactored strutil.Itoa does not change any runtime
// message byte — a hard requirement for DeepSeek prefix cache stability.
func TestItoaIdenticalToOriginals(t *testing.T) {
	// compact.go / runtime.go / dream.go: "if n <= 0 { return \"0\" }"
	// diff.go:                            "if n == 0 { return \"0\" }"
	oldCompact := func(n int) string {
		if n <= 0 {
			return "0"
		}
		var buf [20]byte
		i := len(buf)
		for n > 0 {
			i--
			buf[i] = byte('0' + n%10)
			n /= 10
		}
		return string(buf[i:])
	}
	oldDiff := func(n int) string {
		if n == 0 {
			return "0"
		}
		var buf [20]byte
		i := len(buf)
		for n > 0 {
			i--
			buf[i] = byte('0' + n%10)
			n /= 10
		}
		return string(buf[i:])
	}

	// Every actual caller across the codebase passes non-negative integers:
	// compact.go:  len(truncated), turnCount, prefixCount, len(msgs), len(legacyRep), len(replacement), t.count
	// diff.go:     start (line number ≥ 1), count (≥ 0), c.Added (≥ 0), c.Removed (≥ 0)
	// runtime.go:  rc.project.TotalFiles (≥ 0)
	// dream.go:    i+1 (≥ 1)
	// cache_guard.go: d.callCount (≥ 0), d.lastRead (≥ 0), len(tools) (≥ 0)
	// compress.go: childCount (≥ 0), omitted (≥ 0)

	for n := 0; n <= 100_000; n++ {
		got := Itoa(n)
		if got != oldCompact(n) {
			t.Fatalf("Itoa(%d)=%q, oldCompact(%d)=%q", n, got, n, oldCompact(n))
		}
		if got != oldDiff(n) {
			t.Fatalf("Itoa(%d)=%q, oldDiff(%d)=%q", n, got, n, oldDiff(n))
		}
	}

	// Boundary: the largest plausible value for any caller (e.g. TotalFiles).
	for _, n := range []int{999_999, 1_000_000, 10_000_000} {
		if Itoa(n) != fmt.Sprintf("%d", n) {
			t.Errorf("Itoa(%d)=%q, want %q", n, Itoa(n), fmt.Sprintf("%d", n))
		}
	}
}
