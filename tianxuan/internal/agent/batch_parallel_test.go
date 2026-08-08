package agent

import (
	"context"
	"encoding/json"
	"testing"

	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// fakeSkillTool mimics a subagent skill tool (explore/research/review): it is
// ReadOnly (its sub-agent runs read-only) and safe to batch in parallel with
// other read-only tools. The batch partitioner must treat it like any other
// read-only tool, not as a global-conflict serial call.
type fakeSkillTool struct {
	name string
}

func (f fakeSkillTool) Name() string            { return f.name }
func (f fakeSkillTool) Description() string     { return "subagent skill" }
func (f fakeSkillTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f fakeSkillTool) ReadOnly() bool          { return true }
func (f fakeSkillTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	return f.name + " done", nil
}

// TestPartitionBatchesSkillWithReadOnlyTools locks the regression: V10.124
// removed explore/research/review from getConflictKey's !spawn list (they were
// pure skills then), but V10.147 re-registered them as first-class tools while
// getConflictKey was never updated. They now fall into the default branch and
// return "" — which partitionToolCalls treats as a global conflict, forcing
// them into their own serial batch. A skill tool must instead batch in
// parallel with other read-only tools (its ReadOnly()=true contract).
func TestPartitionBatchesSkillWithReadOnlyTools(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_file", readOnly: true})
	reg.Add(fakeSkillTool{name: "explore"})
	reg.Add(fakeSkillTool{name: "research"})

	calls := []provider.ToolCall{
		{ID: "c1", Name: "read_file", Arguments: `{"path":"a.go"}`},
		{ID: "c2", Name: "explore", Arguments: `{"task":"find X"}`},
		{ID: "c3", Name: "research", Arguments: `{"task":"research Y"}`},
	}
	batches := partitionToolCalls(reg, calls)

	// read_file + explore + research are all read-only and non-conflicting:
	// they must land in ONE parallel batch.
	if len(batches) != 1 {
		t.Fatalf("got %d batches, want 1 (all read-only tools parallel): %+v", len(batches), batches)
	}
	if !batches[0].parallel {
		t.Fatalf("batch 0 parallel = false, want true (read-only skill tools must run in parallel)")
	}
	if batches[0].start != 0 || batches[0].end != 3 {
		t.Fatalf("batch 0 covers calls [%d,%d), want [0,3)", batches[0].start, batches[0].end)
	}
}

// TestPartitionSkillToolDoesNotConflictWithReadFile verifies a skill tool and a
// read_file targeting a different path share a parallel batch — the same
// contract read_file+read_file already enjoys.
func TestPartitionSkillToolDoesNotConflictWithReadFile(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "read_file", readOnly: true})
	reg.Add(fakeSkillTool{name: "explore"})

	calls := []provider.ToolCall{
		{ID: "c1", Name: "explore", Arguments: `{"task":"a"}`},
		{ID: "c2", Name: "read_file", Arguments: `{"path":"b.go"}`},
	}
	batches := partitionToolCalls(reg, calls)
	if len(batches) != 1 || !batches[0].parallel {
		t.Fatalf("explore + read_file(b.go) = %+v, want one parallel batch", batches)
	}
}

// TestPartitionSkillToolSerialAfterWriter preserves the safety invariant: a
// skill tool (which could spawn a sub-agent) must never jump ahead of a
// pending writer. It still batches with readers AFTER the writer's batch.
func TestPartitionSkillToolSerialAfterWriter(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "write_file", readOnly: false})
	reg.Add(fakeSkillTool{name: "explore"})

	calls := []provider.ToolCall{
		{ID: "c1", Name: "write_file", Arguments: `{"path":"a.go"}`},
		{ID: "c2", Name: "explore", Arguments: `{"task":"after write"}`},
	}
	batches := partitionToolCalls(reg, calls)
	// writer is its own serial batch; the skill follows in its own batch —
	// order preserved, no crash, explore must not run concurrently with write.
	if len(batches) != 2 {
		t.Fatalf("got %d batches, want 2 (writer then skill): %+v", len(batches), batches)
	}
	if batches[0].parallel {
		t.Fatalf("writer batch must be serial, got parallel")
	}
}
