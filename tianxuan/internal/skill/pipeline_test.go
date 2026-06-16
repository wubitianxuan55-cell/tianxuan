package skill

import (
	"strings"
	"testing"
)

func TestPipelineResolveParams(t *testing.T) {
	pl := Pipeline{
		Name:        "audit",
		Description: "审计目标模块",
		Steps: []PipelineStep{
			{ID: "map", Skill: "explore", Arguments: "找出 {target} 的所有入口"},
			{ID: "review", Skill: "review", Arguments: "审查 {target} 的代码质量", DependsOn: []string{"map"}},
		},
	}
	resolved := pl.Resolve(map[string]string{"target": "src/auth/"})

	if resolved.Steps[0].Arguments != "找出 src/auth/ 的所有入口" {
		t.Errorf("step 0 args = %q, want '找出 src/auth/ 的所有入口'", resolved.Steps[0].Arguments)
	}
	if resolved.Steps[1].Arguments != "审查 src/auth/ 的代码质量" {
		t.Errorf("step 1 args = %q, want '审查 src/auth/ 的代码质量'", resolved.Steps[1].Arguments)
	}
}

func TestPipelineResolveMultipleParams(t *testing.T) {
	pl := Pipeline{
		Steps: []PipelineStep{
			{ID: "a", Skill: "explore", Arguments: "分析 {module} 模块下的 {func} 函数"},
		},
	}
	resolved := pl.Resolve(map[string]string{"module": "auth", "func": "login"})

	if resolved.Steps[0].Arguments != "分析 auth 模块下的 login 函数" {
		t.Errorf("args = %q", resolved.Steps[0].Arguments)
	}
}

func TestPipelineResolveUnknownParamKept(t *testing.T) {
	pl := Pipeline{
		Steps: []PipelineStep{
			{ID: "a", Skill: "explore", Arguments: "查 {target} 和 {missing}"},
		},
	}
	resolved := pl.Resolve(map[string]string{"target": "auth"})

	// {missing} 没有提供值，保持原样
	if resolved.Steps[0].Arguments != "查 auth 和 {missing}" {
		t.Errorf("args = %q", resolved.Steps[0].Arguments)
	}
}

func TestPipelineToTasks(t *testing.T) {
	pl := Pipeline{
		Steps: []PipelineStep{
			{ID: "map", Skill: "explore", Arguments: "找到 {target} 的入口"},
			{ID: "review", Skill: "review", Arguments: "审查 {target}", DependsOn: []string{"map"}},
		},
	}
	resolved := pl.Resolve(map[string]string{"target": "src/db/"})
	tasks := resolved.ToTasks()

	if len(tasks) != 2 {
		t.Fatalf("want 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Skill != "explore" || tasks[0].Arguments != "找到 src/db/ 的入口" {
		t.Errorf("task 0: skill=%q args=%q", tasks[0].Skill, tasks[0].Arguments)
	}
	if tasks[0].ID != "map" {
		t.Errorf("task 0 ID = %q, want 'map'", tasks[0].ID)
	}
	if len(tasks[0].DependsOn) != 0 {
		t.Errorf("task 0 should have no deps, got %v", tasks[0].DependsOn)
	}
	if tasks[1].Skill != "review" || tasks[1].Arguments != "审查 src/db/" {
		t.Errorf("task 1: skill=%q args=%q", tasks[1].Skill, tasks[1].Arguments)
	}
	if len(tasks[1].DependsOn) != 1 || tasks[1].DependsOn[0] != "map" {
		t.Errorf("task 1 DependsOn = %v, want [map]", tasks[1].DependsOn)
	}
}

func TestPipelineLoadJSON(t *testing.T) {
	jsonStr := `{
  "name": "quick-audit",
  "description": "快速审计",
  "steps": [
    {"id": "explore", "skill": "explore", "arguments": "调查 {target}"},
    {"id": "review", "skill": "review", "arguments": "审查 {target}", "depends_on": ["explore"]}
  ]
}`
	pl, err := LoadPipelineJSON(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("LoadPipelineJSON: %v", err)
	}
	if pl.Name != "quick-audit" {
		t.Errorf("name = %q, want 'quick-audit'", pl.Name)
	}
	if pl.Description != "快速审计" {
		t.Errorf("desc = %q", pl.Description)
	}
	if len(pl.Steps) != 2 {
		t.Fatalf("want 2 steps, got %d", len(pl.Steps))
	}
	if pl.Steps[0].ID != "explore" {
		t.Errorf("step 0 id = %q", pl.Steps[0].ID)
	}
	if pl.Steps[1].ID != "review" {
		t.Errorf("step 1 id = %q", pl.Steps[1].ID)
	}
	if len(pl.Steps[1].DependsOn) != 1 || pl.Steps[1].DependsOn[0] != "explore" {
		t.Errorf("step 1 deps = %v, want [explore]", pl.Steps[1].DependsOn)
	}
}

func TestPipelineLoadJSONInvalid(t *testing.T) {
	_, err := LoadPipelineJSON(strings.NewReader(`not json`))
	if err == nil {
		t.Fatal("should error on invalid JSON")
	}
}

func TestPipelineLoadJSONNoSteps(t *testing.T) {
	_, err := LoadPipelineJSON(strings.NewReader(`{"name":"empty","steps":[]}`))
	if err == nil {
		t.Fatal("should error on empty steps")
	}
}
