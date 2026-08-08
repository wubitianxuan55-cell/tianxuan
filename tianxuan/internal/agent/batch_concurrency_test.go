package agent

import (
	"context"
	"testing"
	"time"

	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// TestExecuteBatchRunsSkillToolsInParallel is the end-to-end proof of the
// V10.168 fix: before it, explore/research fell into the default "" conflict
// key and each ran in its own serial batch (2×delay wall time). After it,
// read-only skill tools share a batch and execute concurrently, so two 300ms
// skills complete in ~300ms instead of ~600ms.
func TestExecuteBatchRunsSkillToolsInParallel(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_file", readOnly: true, delay: 300 * time.Millisecond})
	reg.Add(fakeTool{name: "explore", readOnly: true, delay: 300 * time.Millisecond})
	reg.Add(fakeTool{name: "research", readOnly: true, delay: 300 * time.Millisecond})

	a := New(nil, reg, NewSession(""), Options{}, nil)

	calls := []provider.ToolCall{
		{ID: "c1", Name: "read_file", Arguments: `{"path":"a.go"}`},
		{ID: "c2", Name: "explore", Arguments: `{"task":"x"}`},
		{ID: "c3", Name: "research", Arguments: `{"task":"y"}`},
	}

	start := time.Now()
	a.executeBatch(context.Background(), calls)
	elapsed := time.Since(start)

	// Serial would take ~900ms (3 × 300ms); parallel takes ~300ms. Allow
	// generous slack for CI scheduling: any value well under 600ms proves
	// concurrency (2+ tools ran at the same time).
	if elapsed > 600*time.Millisecond {
		t.Fatalf("3 read-only tools took %v — they ran serially, want parallel (~300ms)", elapsed)
	}
}

// TestExecuteBatchWriterBarrierStillSerial proves the safety invariant that
// prompted the ro:/file: mutual exclusion: a skill tool and a writer never
// share a parallel batch, even though both would otherwise be fast.
func TestExecuteBatchWriterBarrierStillSerial(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "write_file", readOnly: false, delay: 50 * time.Millisecond})
	reg.Add(fakeTool{name: "explore", readOnly: true, delay: 50 * time.Millisecond})

	a := New(nil, reg, NewSession(""), Options{}, nil)

	calls := []provider.ToolCall{
		{ID: "c1", Name: "write_file", Arguments: `{"path":"a.go"}`},
		{ID: "c2", Name: "explore", Arguments: `{"task":"after write"}`},
	}

	start := time.Now()
	a.executeBatch(context.Background(), calls)
	elapsed := time.Since(start)

	// Serial barrier: writer then skill → ~100ms. If they ran in parallel it
	// would be ~50ms. Assert ≥80ms to prove the barrier holds.
	if elapsed < 80*time.Millisecond {
		t.Fatalf("writer+skill took %v — they ran in parallel, want serial barrier (~100ms)", elapsed)
	}
}
