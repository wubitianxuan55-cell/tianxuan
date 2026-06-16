package skill

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// RunPipeline 是技能运行模式：管道定义了一系列步骤，按依赖关系分波并行执行。
const RunPipeline RunAs = "pipeline"

// Pipeline 定义了一组可复用的技能执行步骤，支持参数化（{param} 占位符）。
type Pipeline struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Steps       []PipelineStep `json:"steps"`
}

// PipelineStep 是管道中的单步，直接映射到 ParallelTask。
type PipelineStep struct {
	ID        string   `json:"id"`
	Skill     string   `json:"skill"`
	Arguments string   `json:"arguments"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// LoadPipelineJSON 从 reader 中加载 JSON 格式的管道定义。
func LoadPipelineJSON(r io.Reader) (*Pipeline, error) {
	var pl Pipeline
	if err := json.NewDecoder(r).Decode(&pl); err != nil {
		return nil, fmt.Errorf("pipeline: invalid JSON: %w", err)
	}
	if len(pl.Steps) == 0 {
		return nil, fmt.Errorf("pipeline: steps must not be empty")
	}
	return &pl, nil
}

// Resolve 用参数映射替换步骤中的 {param} 占位符，返回新的 Pipeline 实例。
// 未提供值的参数保持原样（{missing} 不会报错，留给执行时处理）。
func (pl *Pipeline) Resolve(params map[string]string) *Pipeline {
	resolved := &Pipeline{
		Name:        pl.Name,
		Description: pl.Description,
		Steps:       make([]PipelineStep, len(pl.Steps)),
	}
	for i, step := range pl.Steps {
		resolved.Steps[i] = PipelineStep{
			ID:        step.ID,
			Skill:     step.Skill,
			Arguments: resolveParams(step.Arguments, params),
			DependsOn: step.DependsOn,
		}
	}
	return resolved
}

// ToTasks 将管道步骤转换为 ParallelTask 列表，可直接传给 RunDAG。
func (pl *Pipeline) ToTasks() []ParallelTask {
	tasks := make([]ParallelTask, len(pl.Steps))
	for i, step := range pl.Steps {
		deps := step.DependsOn
		if deps == nil {
			deps = nil // 保持 nil，避免空切片
		}
		tasks[i] = ParallelTask{
			Skill:     step.Skill,
			Arguments: step.Arguments,
			ID:        step.ID,
			DependsOn: deps,
		}
	}
	return tasks
}

// resolveParams 替换字符串中的 {key} 占位符为 params 中对应的值。
func resolveParams(s string, params map[string]string) string {
	if len(params) == 0 || !strings.Contains(s, "{") {
		return s
	}
	for key, val := range params {
		s = strings.ReplaceAll(s, "{"+key+"}", val)
	}
	return s
}
