package agent

import "testing"

// O2 契约：allStepsPassed 以计划步数为 ground truth（PlannerHost 修正循环）。
// 旧实现中"无 StepResults 但有文件变更 → pass"会掩盖 maxSteps 中途
// 停止的未完成轮次（文件改了一半但一个 complete_step 都没签收）。

func TestAllStepsPassed_PlanStepsZeroEvidence(t *testing.T) {
	// 计划 3 步、执行零 complete_step、但有文件变更和 maxSteps 错误
	// → 必须视为未完成（旧行为误判为 pass，修复循环不触发）
	r := &TurnResult{
		Success:       false,
		Errors:        []string{"paused after 5 tool-call rounds (agent.max_steps)"},
		FilesModified: []string{"a.go", "b.go"},
	}
	plan := "<!--plan-->\n步骤 1：写失败测试\n步骤 2：最小实现\n步骤 3：验证"
	if (&PlannerHost{}).allStepsPassed(r, plan) {
		t.Fatal("plan with steps but zero complete_step evidence must NOT pass")
	}
}

func TestAllStepsPassed_PlanStepsAllSigned(t *testing.T) {
	plan := "<!--plan-->\n步骤 1：写失败测试\n步骤 2：最小实现"
	r := &TurnResult{
		Success: true,
		StepResults: []StepResult{
			{Step: "步骤 1：写失败测试", Status: "success"},
			{Step: "步骤 2：最小实现", Status: "success"},
		},
	}
	if !(&PlannerHost{}).allStepsPassed(r, plan) {
		t.Fatal("all signed steps should pass")
	}
}

func TestAllStepsPassed_NoPlanStepsFallback(t *testing.T) {
	// 计划无步骤（read-only/直接任务）→ 退回 Success/Errors 判定
	if !(&PlannerHost{}).allStepsPassed(&TurnResult{Success: true}, "") {
		t.Fatal("no-plan read-only success should pass")
	}
	if (&PlannerHost{}).allStepsPassed(&TurnResult{Success: false, Errors: []string{"build failed"}}, "") {
		t.Fatal("no-plan failure should not pass")
	}
}

func TestAllStepsPassed_MergedStepsStillPass(t *testing.T) {
	// 执行器允许合并琐碎步骤：StepResults 少于
	// 计划步数但全部 success → 仍通过，不做步数完整性硬校验
	plan := "<!--plan-->\n步骤 1：写失败测试\n步骤 2：最小实现\n步骤 3：清理"
	r := &TurnResult{
		Success: true,
		StepResults: []StepResult{
			{Step: "合并实现+清理", Status: "success"},
		},
	}
	if !(&PlannerHost{}).allStepsPassed(r, plan) {
		t.Fatal("merged signed steps should pass")
	}
}

func TestAllStepsPassed_NilWithPlan(t *testing.T) {
	if (&PlannerHost{}).allStepsPassed(nil, "<!--plan-->\n步骤 1：x") {
		t.Fatal("nil result should not pass")
	}
}
