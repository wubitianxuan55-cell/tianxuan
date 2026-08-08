package agent

import (
	"context"
	"fmt"
	"strings"

	"tianxuan/internal/event"
)

// confirmPlan 通过 ask 确认卡请用户批准计划。返回用户备注、仅聊天标志、
// 是否按意见修改（revise），以及取消错误。headless（asker==nil）自动确认。
//
// 确认卡选项：
//   提交执行          — 同意计划，直接执行
//   仅聊天            — 计划误触发，仅作对话回复，不派发执行
//   按用户意见修改计划 — 将修改意见送回重新规划
//   取消              — 放弃本次任务
func (h *PlannerHost) confirmPlan(ctx context.Context, task, plan string) (note string, chatOnly bool, revise bool, err error) {
	if h.asker == nil {
		return "", false, false, nil // headless: auto-confirm
	}
	answers, err := h.asker.Ask(ctx, []event.AskQuestion{{
		ID:     "confirm",
		Header: "计划确认",
		Prompt: fmt.Sprintf("任务：%s", truncateStr(task, 200)),
		Plan:   displayPlan(plan), // 只展示 <!--plan--> 之后的部分
		Options: []event.AskOption{
			{Label: "提交执行", Description: "按计划立即执行"},
			{Label: "仅聊天", Description: "计划误触发，仅作普通对话回复，不派发执行"},
			{Label: "按用户意见修改计划", Description: "将修改意见送回重新规划"},
			{Label: "取消", Description: "放弃本次任务，不做任何更改"},
		},
	}})
	if err != nil {
		return "", false, false, fmt.Errorf("plan confirmation cancelled: %w", err)
	}
	if len(answers) == 0 || len(answers[0].Selected) == 0 {
		return "", false, false, fmt.Errorf("计划被取消（无回复）")
	}
	return resolveConfirmChoice(answers[0].Selected[0], answers[0].Selected[1:])
}

// shouldAutoConfirm 简单计划自动确认：≤3 步、无新文件（[NEW]）、
// 且每个步骤的 Verify 都是可执行命令。减少琐碎变更的一次往返。
func shouldAutoConfirm(plan string) bool {
	steps := 0
	for _, line := range strings.Split(plan, "\n") {
		trimmed := strings.TrimSpace(line)
		if isStepLine(trimmed) {
			steps++
		}
	}
	if steps == 0 {
		return false
	}
	if steps > 3 {
		return false
	}
	if strings.Contains(plan, "[NEW]") {
		return false
	}
	if hasUnverifiableSteps(plan) {
		return false
	}
	return true
}

// hasUnverifiableSteps 检查计划中是否有步骤的 Verify 字段缺失或
// 仅含人工验证（"手动测试"/"manual" 等），这类计划不应自动确认。
func hasUnverifiableSteps(plan string) bool {
	soft := []string{"手动测试", "手动验证", "目测", "manual test", "manual check", "manually"}
	stepCount := 0
	hasVerify := false
	for _, line := range strings.Split(plan, "\n") {
		trimmed := strings.TrimSpace(line)
		if isStepLine(trimmed) {
			stepCount++
		}
		if strings.HasPrefix(trimmed, "- **Verify**") {
			hasVerify = true
			content := strings.TrimPrefix(trimmed, "- **Verify**")
			content = strings.TrimPrefix(content, "：")
			content = strings.TrimPrefix(content, ":")
			content = strings.TrimSpace(content)
			if content == "" {
				return true
			}
			for _, s := range soft {
				if strings.Contains(strings.ToLower(content), strings.ToLower(s)) {
					return true
				}
			}
		}
	}
	return stepCount > 0 && !hasVerify
}

// resolveConfirmChoice 将确认卡选项映射为 (note, chatOnly, revise, err)。
// 单独提取以便单元测试。
func resolveConfirmChoice(selected string, extra []string) (note string, chatOnly bool, revise bool, err error) {
	switch selected {
	case "提交执行":
		return "", false, false, nil
	case "仅聊天":
		return "", true, false, nil
	case "按用户意见修改计划":
		feedback := ""
		if len(extra) > 0 {
			feedback = extra[0]
		}
		return feedback, false, true, nil
	case "取消":
		return "", false, false, fmt.Errorf("计划被用户取消")
	default:
		return selected, false, false, nil
	}
}

// displayPlan 提取 <!--plan--> 之后的结构化计划部分用于展示。
// 前导分析（推理/项目记忆）不应泄漏进用户可见的计划卡。
func displayPlan(full string) string {
	const marker = "<!--plan-->"
	if idx := strings.Index(full, marker); idx >= 0 {
		return strings.TrimSpace(full[idx:])
	}
	return full
}
