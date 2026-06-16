package agent

import (
	"os"
	"path/filepath"
	"testing"

	"tianxuan/internal/provider"
)

// TestExtractTodosFromSession extracts todo items from a session with todo_write.
func TestExtractTodosFromSession(t *testing.T) {
	msgs := []provider.Message{
		makeTodoMsg("completed", "in_progress", "pending"),
	}
	todos := extractTodos(msgs)
	if len(todos) != 3 {
		t.Fatalf("expected 3 todos, got %d", len(todos))
	}
	if todos[0].Status != "completed" {
		t.Fatalf("todo[0].status: expected completed, got %s", todos[0].Status)
	}
	if todos[1].Status != "in_progress" {
		t.Fatalf("todo[1].status: expected in_progress, got %s", todos[1].Status)
	}
}

// TestExtractTodosEmpty returns nil when no todo_write exists.
func TestExtractTodosEmpty(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "hello"},
	}
	todos := extractTodos(msgs)
	if todos != nil {
		t.Fatalf("expected nil, got %v", todos)
	}
}

// TestExtractTodosLastWins only reads the last todo_write call.
func TestExtractTodosLastWins(t *testing.T) {
	msgs := []provider.Message{
		makeTodoMsg("pending", "pending"),
		makeTodoMsg("completed", "completed", "completed"),
	}
	todos := extractTodos(msgs)
	if len(todos) != 3 {
		t.Fatalf("expected 3 (last wins), got %d", len(todos))
	}
	if todos[0].Status != "completed" {
		t.Fatalf("expected completed, got %s", todos[0].Status)
	}
}

// TestCheckpointWriteLoadRoundTrip writes and reads back a checkpoint.
func TestCheckpointWriteLoadRoundTrip(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "tianxuan-ck-test-"+t.Name())
	defer os.RemoveAll(dir)

	s := NewSession("")
	s.Add(makeTodoMsg("completed", "in_progress"))
	s.Add(provider.Message{Role: provider.RoleUser, Content: "fix the bug"})

	a := &AgentRunner{
		session:    s,
		compaction: CompactionConfig{RecentKeep: 5, Window: 100000, Ratio: 0.8},
	}

	if err := a.WriteCheckpoint(dir); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	cp := LoadCheckpoint(dir)
	if cp == nil {
		t.Fatal("LoadCheckpoint returned nil")
	}
	if len(cp.Todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(cp.Todos))
	}
	if cp.TruncateCount != 0 {
		t.Fatalf("expected TruncateCount=0, got %d", cp.TruncateCount)
	}
}

// TestCheckpointEmptyDirReturnsNil for both write (no-op) and load.
func TestCheckpointEmptyDirReturnsNil(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s, compaction: CompactionConfig{RecentKeep: 5}}
	if err := a.WriteCheckpoint(""); err != nil {
		t.Fatalf("WriteCheckpoint on empty dir should be no-op: %v", err)
	}
	if cp := LoadCheckpoint(""); cp != nil {
		t.Fatal("LoadCheckpoint on empty dir should return nil")
	}
}

// TestCheckpointLoadNonexistent returns nil.
func TestCheckpointLoadNonexistent(t *testing.T) {
	cp := LoadCheckpoint(filepath.Join(os.TempDir(), "nonexistent-ck-"+t.Name()))
	if cp != nil {
		t.Fatal("expected nil for nonexistent checkpoint")
	}
}

// TestCheckpointFormatForLLM produces non-empty output with todo icons.
func TestCheckpointFormatForLLM(t *testing.T) {
	cp := CheckpointData{
		Summary: "- Scope: 5 messages compacted, 2 turns",
		Todos: []checkpointTodo{
			{Content: "add parser", Status: "completed"},
			{Content: "add tests", Status: "in_progress"},
			{Content: "update docs", Status: "pending"},
		},
		Goal:      "add JSON parser",
		EditFiles: []string{"parser.go", "parser_test.go"},
	}
	out := cp.formatForLLM()
	if out == "" {
		t.Fatal("formatForLLM returned empty")
	}
	// Verify key content is present
	for _, want := range []string{"add parser", "add tests", "update docs", "parser.go", "add JSON parser", "✓", "▶", "○"} {
		if !containsStr(out, want) {
			t.Errorf("formatForLLM output missing %q", want)
		}
	}
}

// TestCheckpointFormatForLLMEmpty produces valid output for empty data.
func TestCheckpointFormatForLLMEmpty(t *testing.T) {
	cp := CheckpointData{}
	out := cp.formatForLLM()
	if out == "" {
		t.Fatal("formatForLLM returned empty for empty data")
	}
}

// TestCheckpointDeterministic same input produces same JSON bytes.
func TestCheckpointDeterministic(t *testing.T) {
	s1 := NewSession("")
	s1.Add(makeTodoMsg("pending", "completed"))
	a1 := &AgentRunner{session: s1, compaction: CompactionConfig{RecentKeep: 5, Window: 100000, Ratio: 0.8}}

	s2 := NewSession("")
	s2.Add(makeTodoMsg("pending", "completed"))
	a2 := &AgentRunner{session: s2, compaction: CompactionConfig{RecentKeep: 5, Window: 100000, Ratio: 0.8}}

	dir1 := filepath.Join(os.TempDir(), "tianxuan-ck-det1-"+t.Name())
	dir2 := filepath.Join(os.TempDir(), "tianxuan-ck-det2-"+t.Name())
	defer os.RemoveAll(dir1)
	defer os.RemoveAll(dir2)

	if err := a1.WriteCheckpoint(dir1); err != nil {
		t.Fatalf("WriteCheckpoint 1: %v", err)
	}
	if err := a2.WriteCheckpoint(dir2); err != nil {
		t.Fatalf("WriteCheckpoint 2: %v", err)
	}

	data1, _ := os.ReadFile(filepath.Join(dir1, "checkpoint.json"))
	data2, _ := os.ReadFile(filepath.Join(dir2, "checkpoint.json"))
	if string(data1) != string(data2) {
		t.Fatalf("checkpoint JSON not deterministic:\n  got:  %s\n  want: %s", data1, data2)
	}
}

// TestCheckpointFormatForLLMDeterministic same data produces same output.
func TestCheckpointFormatForLLMDeterministic(t *testing.T) {
	cp := CheckpointData{
		Summary: "test",
		Todos:   []checkpointTodo{{Content: "x", Status: "completed"}},
	}
	out1 := cp.formatForLLM()
	out2 := cp.formatForLLM()
	if out1 != out2 {
		t.Fatal("formatForLLM not deterministic across calls")
	}
}

// containsStr checks if s contains substr.
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
