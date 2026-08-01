package agent

import (
	"strings"
	"testing"
)

// 工作流覆盖度契约：规划者需要知道执行者的 complete_step 是否覆盖了
// 计划的所有步骤——执行者可能跳过某一步或全部改标题。对照是软性的
// （合并/改写标题是允许的），只产生信息性警告并让修正计划补全。

func TestCheckStepCoverage_AllMatched(t *testing.T) {
	plan := "<!--plan-->\n步骤 1：写失败测试\n步骤 2：最小实现\n步骤 3：验证"
	r := &TurnResult{StepResults: []StepResult{
		{Step: "步骤 1：写失败测试", Status: "success"},
		{Step: "最小实现", Status: "success"},
		{Step: "步骤 3: 验证", Status: "success"},
	}}
	if missing := checkStepCoverage(plan, r); len(missing) != 0 {
		t.Errorf("all steps matched, got missing: %v", missing)
	}
}

func TestCheckStepCoverage_MergedStepMatches(t *testing.T) {
	// 执行者合并步骤是允许的：计划"实现"+"重构"被单个"实现+重构"覆盖
	plan := "<!--plan-->\n步骤 1：写失败测试\n步骤 2：实现\n步骤 3：重构"
	r := &TurnResult{StepResults: []StepResult{
		{Step: "步骤 1：写失败测试", Status: "success"},
		{Step: "实现+重构", Status: "success"},
	}}
	if missing := checkStepCoverage(plan, r); len(missing) != 0 {
		t.Errorf("merged step should cover both titles, got missing: %v", missing)
	}
}

func TestCheckStepCoverage_SkippedStepDetected(t *testing.T) {
	plan := "<!--plan-->\n步骤 1：写失败测试\n步骤 2：最小实现\n步骤 3：跑测试验证"
	r := &TurnResult{StepResults: []StepResult{
		{Step: "步骤 1：写失败测试", Status: "success"},
		{Step: "步骤 2：最小实现", Status: "success"},
	}}
	missing := checkStepCoverage(plan, r)
	if len(missing) != 1 || missing[0] != "跑测试验证" {
		t.Errorf("skipped step must be detected, got: %v", missing)
	}
}

func TestCheckStepCoverage_EdgeCases(t *testing.T) {
	plan := "<!--plan-->\n步骤 1：写失败测试"
	if got := checkStepCoverage(plan, nil); got != nil {
		t.Errorf("nil result → nil, got %v", got)
	}
	if got := checkStepCoverage(plan, &TurnResult{}); got != nil {
		t.Errorf("no step results → nil, got %v", got)
	}
	if got := checkStepCoverage("", &TurnResult{StepResults: []StepResult{{Step: "x", Status: "success"}}}); got != nil {
		t.Errorf("no plan steps → nil, got %v", got)
	}
}

func TestBuildVerifyTriad_IncludesCoverage(t *testing.T) {
	plan := "<!--plan-->\n步骤 1：写失败测试\n步骤 2：最小实现"
	r := &TurnResult{
		Success: true,
		StepResults: []StepResult{
			{Step: "步骤 1：写失败测试", Status: "success"},
		},
	}
	out := buildVerifyTriad(r, plan)
	if !strings.Contains(out, "coverage=warn(1)") {
		t.Errorf("triad should warn about uncovered step, got: %s", out)
	}
}

func TestBuildFixPrompt_IncludesUncoveredSteps(t *testing.T) {
	originalPlan := "<!--plan-->\n步骤 1：写失败测试\n步骤 2：最小实现\n步骤 3：验证"
	failed := &TurnResult{
		Success: true,
		StepResults: []StepResult{
			{Step: "步骤 1：写失败测试", Status: "success"},
			{Step: "步骤 2：最小实现", Status: "success"},
		},
	}
	prompt := buildFixPrompt("原始任务", originalPlan, failed, 2, nil)
	if !strings.Contains(prompt, "未签收") || !strings.Contains(prompt, "验证") {
		t.Errorf("fix prompt must list uncovered plan steps, got:\n%s", prompt)
	}
}
