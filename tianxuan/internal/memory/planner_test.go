package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilterAGENTSForPlanner_RemovesIronLaws(t *testing.T) {
	input := `## 双模型架构

- **🔨 Hephaestus（执行者，你）**：接收 Hermes 计划→用全功能工具实现 HOW→写代码/测试/提交。

- **编码铁律**（自动生效）：
  - 🔴 **设计优先**：编码前必须完成需求探索。
  - 🔴 **TDD**：无失败测试不写产品代码。
  - 🔴 **验证强制**：声称已修复前必须运行 verify。
- **子代理隔离**：复杂任务通过 task 工具派发。
- **计划粒度**：todo_write 每步 2-5 分钟。

## 🔴 核心约束：禁止损害缓存命中率

重要约束内容。`

	got := filterAGENTSForPlanner(input)

	// Should NOT contain iron laws
	for _, banned := range []string{
		"编码铁律",
		"🔴 **设计优先**",
		"🔴 **TDD**",
		"🔴 **验证强制**",
		"**计划粒度**",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("filterAGENTSForPlanner should remove %q but it remains:\n%s", banned, got)
		}
	}

	// Should KEEP these
	for _, keep := range []string{
		"双模型架构",
		"子代理隔离",
		"task 工具派发",
		"核心约束",
		"重要约束内容",
	} {
		if !strings.Contains(got, keep) {
			t.Errorf("filterAGENTSForPlanner should keep %q but it was removed", keep)
		}
	}

	// Role address should be fixed
	if strings.Contains(got, "（执行者，你）") {
		t.Error("filterAGENTSForPlanner should remove '你' from role address")
	}
	if !strings.Contains(got, "（执行者）") {
		t.Error("filterAGENTSForPlanner should keep '（执行者）' in role address")
	}
}

func TestFilterAGENTSForPlanner_RemovesSuperpowers(t *testing.T) {
	input := `## 技能系统

工具 = 原子操作；技能 = 领域知识。

## 🦸 Superpowers 开发方法论

以下法则来自 obra/superpowers：

### 1. 设计优先法则
编码前必须先探索需求。

### 2. TDD 铁律
先写失败测试。`

	got := filterAGENTSForPlanner(input)

	if strings.Contains(got, "Superpowers") {
		t.Error("filterAGENTSForPlanner should remove Superpowers section")
	}
	if strings.Contains(got, "TDD 铁律") {
		t.Error("filterAGENTSForPlanner should remove TDD from Superpowers section")
	}

	// Should keep content before Superpowers
	if !strings.Contains(got, "技能系统") {
		t.Error("filterAGENTSForPlanner should keep content before Superpowers section")
	}
	if !strings.Contains(got, "原子操作") {
		t.Error("filterAGENTSForPlanner should keep content before Superpowers section")
	}
}

func TestFilterAGENTSForPlanner_EmptyInput(t *testing.T) {
	got := filterAGENTSForPlanner("")
	if got != "" {
		t.Errorf("filterAGENTSForPlanner on empty string should return empty, got %q", got)
	}
}

func TestFilterAGENTSForPlanner_NoFilterableContent(t *testing.T) {
	input := `## Git 仓库

- 远程: git@github.com:foo/bar.git
- 默认分支: main`

	got := filterAGENTSForPlanner(input)
	if got != input {
		t.Errorf("filterAGENTSForPlanner on content without filterable sections should be identity:\n got  %q\n want %q", got, input)
	}
}

func TestPlannerBlock_FiltersAGENTS(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, "AGENTS.md")
	content := `# Project memory

## Notes

- **编码铁律**（自动生效）：
  - 🔴 **TDD**：无失败测试不写产品代码。
- **子代理隔离**：复杂任务通过 task 工具派发。

## 🦸 Superpowers 开发方法论

执行者专属内容。`
	if err := os.WriteFile(agentsPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	set := Load(Options{CWD: dir, UserDir: ""})
	if len(set.Docs) == 0 {
		t.Fatal("no docs loaded from test directory")
	}

	got := set.PlannerBlock()

	// Planner block should NOT contain executor-only content
	for _, banned := range []string{"TDD", "Superpowers"} {
		if strings.Contains(got, banned) {
			t.Errorf("PlannerBlock should not contain %q:\n%s", banned, got)
		}
	}

	// Planner block SHOULD contain shared context
	if !strings.Contains(got, "子代理隔离") {
		t.Error("PlannerBlock should keep shared context like 子代理隔离")
	}
	if !strings.Contains(got, "task 工具") {
		t.Error("PlannerBlock should keep shared context like task tool reference")
	}

	// Executor block (from same set) should contain everything
	execBlock := set.Block()
	if !strings.Contains(execBlock, "TDD") {
		t.Error("Block (executor) should contain TDD")
	}
	if !strings.Contains(execBlock, "Superpowers") {
		t.Error("Block (executor) should contain Superpowers")
	}
}
