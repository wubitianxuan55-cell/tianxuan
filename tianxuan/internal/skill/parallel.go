package skill

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ParallelTask 描述一个要并行执行的子代理任务。
type ParallelTask struct {
	Skill     string   `json:"skill"`               // 技能名称
	Arguments string   `json:"arguments"`           // 传给技能的任务描述
	ID        string   `json:"id,omitempty"`        // 可选标识，用于 depends_on 引用
	DependsOn []string `json:"depends_on,omitempty"` // 依赖的任务 ID 列表
}

// ParallelResult 保存单个子代理任务的结果或错误。
type ParallelResult struct {
	Skill  string `json:"skill"`
	Task   string `json:"task"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// RunParallel 并行执行多个子代理任务，等待全部完成后返回汇总结果。
// 每个任务在独立 goroutine 中执行，共享同一个 ctx（用于外层取消）。
// 单个任务失败记录在对应 ParallelResult.Error 中，不影响其他任务。
func RunParallel(ctx context.Context, tasks []ParallelTask, runner SubagentRunner) ([]ParallelResult, error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	results := make([]ParallelResult, len(tasks))
	var wg sync.WaitGroup

	for i, t := range tasks {
		wg.Add(1)
		go func(idx int, task ParallelTask) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("skill: parallel task panic", "idx", idx, "skill", task.Skill, "panic", r)
					results[idx].Skill = task.Skill
					results[idx].Task = task.Arguments
					results[idx].Error = fmt.Sprintf("panic: %v", r)
				}
			}()

			subCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
			defer cancel()

			sk := Skill{Name: task.Skill}
			result, err := runner(subCtx, sk, task.Arguments)

			results[idx].Skill = task.Skill
			results[idx].Task = task.Arguments
			if err != nil {
				results[idx].Error = err.Error()
			} else {
				results[idx].Result = result
			}
		}(i, t)
	}

	wg.Wait()
	return results, nil
}

// topoSort 按 Kahn 算法对 DAG 任务进行拓扑排序，返回分波（level-order）结果。
// 每波内的任务无相互依赖，可以安全并行执行。检测到环时返回错误。
func topoSort(tasks []ParallelTask) ([][]ParallelTask, error) {
	// 构建 ID → 索引映射
	byID := make(map[string]int, len(tasks))
	for i, t := range tasks {
		if t.ID != "" {
			byID[t.ID] = i
		}
	}

	// 计算入度（依赖数量）
	inDegree := make([]int, len(tasks))
	dependents := make([][]int, len(tasks)) // 谁依赖我 → 我完成后通知谁
	for i, t := range tasks {
		for _, depID := range t.DependsOn {
			if depID == "" {
				continue
			}
			depIdx, ok := byID[depID]
			if !ok {
				return nil, fmt.Errorf("task %q depends on unknown task %q", t.ID, depID)
			}
			inDegree[i]++
			dependents[depIdx] = append(dependents[depIdx], i)
		}
	}

	// Kahn BFS：从入度为 0 的节点开始
	var waves [][]ParallelTask
	queue := make([]int, 0, len(tasks))
	for i, d := range inDegree {
		if d == 0 {
			queue = append(queue, i)
		}
	}

	processed := 0
	for len(queue) > 0 {
		// 当前波：所有入度为 0 的任务
		wave := make([]ParallelTask, len(queue))
		for i, idx := range queue {
			wave[i] = tasks[idx]
		}
		waves = append(waves, wave)

		// 下一波候选
		var next []int
		for _, idx := range queue {
			processed++
			for _, dep := range dependents[idx] {
				inDegree[dep]--
				if inDegree[dep] == 0 {
					next = append(next, dep)
				}
			}
		}
		queue = next
	}

	// 如果未处理完所有节点，存在环
	if processed != len(tasks) {
		return nil, fmt.Errorf("circular dependency detected: %d/%d tasks processed", processed, len(tasks))
	}

	return waves, nil
}

// RunDAG 按依赖关系分波并行执行子代理任务。
// 通过拓扑排序确定执行顺序：无依赖的任务先并行执行，完成后将其结果注入到依赖它的任务的参数中。
// 单波内使用 RunParallel 并行执行。
func RunDAG(ctx context.Context, tasks []ParallelTask, runner SubagentRunner) ([]ParallelResult, error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	// 没有 ID 或没有 depends_on = 退化为纯并行
	hasDeps := false
	for _, t := range tasks {
		if len(t.DependsOn) > 0 {
			hasDeps = true
			break
		}
	}
	if !hasDeps {
		return RunParallel(ctx, tasks, runner)
	}

	waves, err := topoSort(tasks)
	if err != nil {
		return nil, err
	}

	// 收集所有结果，按 ID 索引
	allResults := make(map[string]string) // ID → 结果文本
	var all []ParallelResult

	for _, wave := range waves {
		// 准备本波任务：如果有依赖，注入上游结果
		prepared := make([]ParallelTask, len(wave))
		for i, t := range wave {
			prepared[i] = t
			if len(t.DependsOn) > 0 {
				var parts []string
				for _, depID := range t.DependsOn {
					if result, ok := allResults[depID]; ok {
						parts = append(parts, "["+depID+"]: "+result)
					} else {
						parts = append(parts, "["+depID+"]: (failed or missing)")
					}
				}
				// 将注入上下文追加到参数后面
				if prepared[i].Arguments != "" {
					prepared[i].Arguments += "\n\n--- upstream results ---\n"
				}
				prepared[i].Arguments += "Upstream task results:\n" + strings.Join(parts, "\n")
			}
		}

		// 并行执行本波
		waveResults, err := RunParallel(ctx, prepared, runner)
		if err != nil {
			return nil, err
		}

		// 记录结果
		for i, r := range waveResults {
			if wave[i].ID != "" {
				allResults[wave[i].ID] = r.Result
				if r.Error != "" {
					allResults[wave[i].ID] = "(error: " + r.Error + ")"
				}
			}
			all = append(all, r)
		}
	}

	return all, nil
}
