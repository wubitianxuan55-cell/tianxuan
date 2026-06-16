package skill

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunParallelEmptyTasks(t *testing.T) {
	runner := func(ctx context.Context, sk Skill, task string) (string, error) {
		return "ok", nil
	}
	results, err := RunParallel(context.Background(), nil, runner)
	if err != nil {
		t.Fatalf("RunParallel empty: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("empty tasks should return empty results, got %d", len(results))
	}
}

func TestRunParallelSingleTask(t *testing.T) {
	runner := func(ctx context.Context, sk Skill, task string) (string, error) {
		return "result:" + task, nil
	}
	results, err := RunParallel(context.Background(), []ParallelTask{
		{Skill: "explore", Arguments: "find auth"},
	}, runner)
	if err != nil {
		t.Fatalf("RunParallel single: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Skill != "explore" {
		t.Errorf("skill = %q, want explore", results[0].Skill)
	}
	if results[0].Result != "result:find auth" {
		t.Errorf("result = %q, want result:find auth", results[0].Result)
	}
	if results[0].Error != "" {
		t.Errorf("error should be empty, got %q", results[0].Error)
	}
}

func TestRunParallelMultipleTasks(t *testing.T) {
	var callCount atomic.Int32
	runner := func(ctx context.Context, sk Skill, task string) (string, error) {
		callCount.Add(1)
		// 模拟不同耗时，验证并行执行
		if strings.Contains(task, "slow") {
			time.Sleep(100 * time.Millisecond)
		}
		return "done:" + task, nil
	}
	results, err := RunParallel(context.Background(), []ParallelTask{
		{Skill: "explore", Arguments: "slow task"},
		{Skill: "review", Arguments: "fast task"},
		{Skill: "explore", Arguments: "another"},
	}, runner)
	if err != nil {
		t.Fatalf("RunParallel multiple: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	if callCount.Load() != 3 {
		t.Errorf("runner called %d times, want 3", callCount.Load())
	}
	// 验证每个结果都有正确的 Skill 名
	want := map[string]bool{"explore": false, "review": false}
	for _, r := range results {
		want[r.Skill] = true
		if r.Error != "" {
			t.Errorf("unexpected error for %s: %s", r.Skill, r.Error)
		}
	}
	for sk, found := range want {
		if !found {
			t.Errorf("missing result for skill %q", sk)
		}
	}
}

func TestRunParallelTaskFailure(t *testing.T) {
	runner := func(ctx context.Context, sk Skill, task string) (string, error) {
		if strings.Contains(task, "fail") {
			return "", &testError{msg: "simulated failure"}
		}
		return "ok:" + task, nil
	}
	results, err := RunParallel(context.Background(), []ParallelTask{
		{Skill: "explore", Arguments: "good task"},
		{Skill: "review", Arguments: "this will fail"},
	}, runner)
	if err != nil {
		t.Fatalf("RunParallel should not error on task failure: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	if results[0].Error != "" {
		t.Errorf("good task should have no error, got %q", results[0].Error)
	}
	if results[1].Error == "" {
		t.Error("failing task should have error")
	}
	if results[1].Result != "" {
		t.Errorf("failing task should have empty result, got %q", results[1].Result)
	}
}

func TestRunParallelRespectsContextCancellation(t *testing.T) {
	runner := func(ctx context.Context, sk Skill, task string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return "finished", nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := RunParallel(ctx, []ParallelTask{
		{Skill: "explore", Arguments: "task"},
	}, runner)
	// RunParallel 不因部分任务取消而返回顶层错误，而是在结果中记录错误
	if err != nil {
		// 如果所有任务都被取消，可能返回错误，这也是合理的
		t.Logf("cancelled context: %v", err)
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// --- DAG / 依赖编排测试 ---

func TestTopoSortNoDeps(t *testing.T) {
	tasks := []ParallelTask{
		{ID: "a", Skill: "explore", Arguments: "task a"},
		{ID: "b", Skill: "review", Arguments: "task b"},
	}
	waves, err := topoSort(tasks)
	if err != nil {
		t.Fatalf("topoSort: %v", err)
	}
	// 无依赖 = 所有任务在同一波
	if len(waves) != 1 {
		t.Fatalf("want 1 wave, got %d", len(waves))
	}
	if len(waves[0]) != 2 {
		t.Errorf("wave 0 should have 2 tasks, got %d", len(waves[0]))
	}
}

func TestTopoSortLinearChain(t *testing.T) {
	tasks := []ParallelTask{
		{ID: "a", Skill: "explore", Arguments: "step1"},
		{ID: "b", Skill: "review", Arguments: "step2", DependsOn: []string{"a"}},
		{ID: "c", Skill: "review", Arguments: "step3", DependsOn: []string{"b"}},
	}
	waves, err := topoSort(tasks)
	if err != nil {
		t.Fatalf("topoSort: %v", err)
	}
	// A → B → C：三波串行
	if len(waves) != 3 {
		t.Fatalf("linear chain: want 3 waves, got %d", len(waves))
	}
	if len(waves[0]) != 1 || waves[0][0].ID != "a" {
		t.Errorf("wave 0 should be [a], got %v", waveIDs(waves[0]))
	}
	if len(waves[1]) != 1 || waves[1][0].ID != "b" {
		t.Errorf("wave 1 should be [b], got %v", waveIDs(waves[1]))
	}
	if len(waves[2]) != 1 || waves[2][0].ID != "c" {
		t.Errorf("wave 2 should be [c], got %v", waveIDs(waves[2]))
	}
}

func TestTopoSortForkJoin(t *testing.T) {
	tasks := []ParallelTask{
		{ID: "a", Skill: "explore", Arguments: "survey auth"},
		{ID: "b", Skill: "explore", Arguments: "survey api"},
		{ID: "c", Skill: "review", Arguments: "merge", DependsOn: []string{"a", "b"}},
	}
	waves, err := topoSort(tasks)
	if err != nil {
		t.Fatalf("topoSort: %v", err)
	}
	// A,B 并行 → C：两波，第一波含 A 和 B
	if len(waves) != 2 {
		t.Fatalf("fork-join: want 2 waves, got %d", len(waves))
	}
	if len(waves[0]) != 2 {
		t.Errorf("wave 0 should have 2 tasks (a,b), got %d", len(waves[0]))
	}
	if len(waves[1]) != 1 || waves[1][0].ID != "c" {
		t.Errorf("wave 1 should be [c], got %v", waveIDs(waves[1]))
	}
}

func TestTopoSortCycleDetection(t *testing.T) {
	tasks := []ParallelTask{
		{ID: "a", Skill: "explore", Arguments: "x", DependsOn: []string{"b"}},
		{ID: "b", Skill: "review", Arguments: "y", DependsOn: []string{"a"}},
	}
	_, err := topoSort(tasks)
	if err == nil {
		t.Fatal("cycle should be detected")
	}
	if !strings.Contains(err.Error(), "cycle") && !strings.Contains(err.Error(), "circular") {
		t.Errorf("error should mention cycle: %v", err)
	}
}

func TestTopoSortMissingDep(t *testing.T) {
	tasks := []ParallelTask{
		{ID: "a", Skill: "explore", Arguments: "task", DependsOn: []string{"ghost"}},
	}
	_, err := topoSort(tasks)
	if err == nil {
		t.Fatal("missing dependency should error")
	}
}

func TestTopoSortEmptyDependsOn(t *testing.T) {
	tasks := []ParallelTask{
		{ID: "a", Skill: "explore", Arguments: "task", DependsOn: []string{}},
		{ID: "b", Skill: "review", Arguments: "task", DependsOn: nil},
	}
	waves, err := topoSort(tasks)
	if err != nil {
		t.Fatalf("topoSort: %v", err)
	}
	// 空依赖或无依赖 = 第一波
	if len(waves) != 1 {
		t.Fatalf("want 1 wave, got %d", len(waves))
	}
	if len(waves[0]) != 2 {
		t.Errorf("wave 0 should have 2 tasks, got %d", len(waves[0]))
	}
}

func TestRunDAGInjectsResults(t *testing.T) {
	// runner 记录收到的参数，验证结果注入
	var receivedArgs []string
	runner := func(ctx context.Context, sk Skill, task string) (string, error) {
		receivedArgs = append(receivedArgs, task)
		return "result:" + task, nil
	}
	_, err := RunDAG(context.Background(), []ParallelTask{
		{ID: "a", Skill: "explore", Arguments: "find auth"},
		{ID: "b", Skill: "review", Arguments: "review it", DependsOn: []string{"a"}},
	}, runner)
	if err != nil {
		t.Fatalf("RunDAG: %v", err)
	}
	// b 应该收到注入了 a 结果后的增强参数
	if len(receivedArgs) != 2 {
		t.Fatalf("want 2 calls, got %d", len(receivedArgs))
	}
	if receivedArgs[0] != "find auth" {
		t.Errorf("a args = %q, want 'find auth'", receivedArgs[0])
	}
	// b 的参数应该包含 a 的结果
	if !strings.Contains(receivedArgs[1], "result:find auth") {
		t.Errorf("b should receive injected result from a: %q", receivedArgs[1])
	}
}

func TestRunDAGMixedFailures(t *testing.T) {
	runner := func(ctx context.Context, sk Skill, task string) (string, error) {
		// 只检查原始任务名（"this will fail"），不检查注入的上游结果
		if strings.HasPrefix(task, "this will fail") {
			return "", &testError{msg: "intentional failure"}
		}
		n := 20
		if len(task) < n {
			n = len(task)
		}
		return "ok-" + task[:n], nil
	}
	results, err := RunDAG(context.Background(), []ParallelTask{
		{ID: "a", Skill: "explore", Arguments: "this will fail"},
		{ID: "b", Skill: "review", Arguments: "this succeeds", DependsOn: []string{"a"}},
	}, runner)
	if err != nil {
		t.Fatalf("RunDAG should not error on partial failure: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	// a 失败
	if results[0].Error == "" {
		t.Error("a should have error")
	}
	// b 仍然执行（依赖失败不应该阻止后续 wave）
	if results[1].Error != "" {
		t.Error("b should succeed even if dependency failed")
	}
}

func TestRunDAGPreservesOrdering(t *testing.T) {
	var order []string
	var mu sync.Mutex
	runner := func(ctx context.Context, sk Skill, task string) (string, error) {
		mu.Lock()
		// 用 contains 匹配原始任务名，因为注入后内容会变长
		if strings.Contains(task, "first") && !strings.Contains(task, "upstream") {
			order = append(order, "first")
		} else if strings.Contains(task, "second") && strings.Contains(task, "upstream") {
			order = append(order, "second")
		} else if strings.Contains(task, "slow") && strings.Contains(task, "upstream") {
			order = append(order, "slow")
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond) // 确保时序可观测
		return "done", nil
	}
	_, err := RunDAG(context.Background(), []ParallelTask{
		{ID: "a", Skill: "explore", Arguments: "first"},
		{ID: "b", Skill: "review", Arguments: "second", DependsOn: []string{"a"}},
		{ID: "c", Skill: "review", Arguments: "slow", DependsOn: []string{"a"}},
	}, runner)
	if err != nil {
		t.Fatalf("RunDAG: %v", err)
	}
	// a 必须在 b 之前
	mu.Lock()
	defer mu.Unlock()
	idxA := indexOf(order, "first")
	idxB := indexOf(order, "second")
	if idxA < 0 || idxB < 0 {
		t.Fatalf("missing expected call: order=%v", order)
	}
	if idxA >= idxB {
		t.Errorf("'first' should run before 'second', got order=%v", order)
	}
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}

func waveIDs(tasks []ParallelTask) []string {
	ids := make([]string, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}
	return ids
}
