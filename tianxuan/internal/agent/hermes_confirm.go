package agent

import (
	"context"
	"fmt"

	"tianxuan/internal/event"
)

// confirmPlan asks the user to approve the planner's output before handing off to
// the executor. Returns the user's free-typed note ("" when none), a chatOnly
// flag, and a revise flag (= user clicked "按用户意见修改计划"), and an error on
// cancellation. In headless mode (asker == nil) it auto-confirms.
//
// The confirmation dialog shows:
//   ○ 提交执行          — 同意计划，直接交由 Hephaestus 执行
//   ○ 仅聊天            — 计划误触发，仅作为普通对话回复，不派送执行者
//   ○ 按用户意见修改计划   — 将修改意见送回 Hermes 重新规划
//   ○ 取消              — 放弃本次任务
//   📝 文本框 — 输入修改意见
//
// For "按用户意见修改计划", the note text is extracted from Selected[1] (when
// available) and returned as the first string so the caller can feed it back
// to Hermes for re-planning.
func (h *Hermes) confirmPlan(ctx context.Context, task, plan string) (note string, chatOnly bool, revise bool, err error) {
	if h.asker == nil {
		return "", false, false, nil // headless: auto-confirm
	}
	answers, err := h.asker.Ask(ctx, []event.AskQuestion{{
		ID:     "confirm",
		Header: "计划确认",
		Prompt: fmt.Sprintf("任务：%s", truncateStr(task, 200)),
		Plan:   plan, // full plan rendered by PlanCard with Markdown
		Options: []event.AskOption{
			{Label: "提交执行", Description: "按计划交由 Hephaestus 立即执行"},
			{Label: "仅聊天", Description: "计划误触发，仅作为普通对话回复，不派送执行者"},
			{Label: "按用户意见修改计划", Description: "将修改意见送回 Hermes 重新规划"},
			{Label: "取消", Description: "放弃本次任务，不做任何更改"},
		},
	}})
	if err != nil {
		return "", false, false, fmt.Errorf("plan confirmation cancelled: %w", err)
	}
	if len(answers) == 0 || len(answers[0].Selected) == 0 {
		return "", false, false, fmt.Errorf("计划被取消（无回复）")
	}
	selected := answers[0].Selected[0]
	switch selected {
	case "提交执行":
		return "", false, false, nil // agree without notes
	case "仅聊天":
		return "", true, false, nil // chat-only: don't dispatch to executor
	case "按用户意见修改计划":
		feedback := ""
		if len(answers[0].Selected) > 1 {
			feedback = answers[0].Selected[1]
		}
		return feedback, false, true, nil // revise: re-plan with feedback
	case "取消":
		return "", false, false, fmt.Errorf("计划被用户取消")
	default:
		// User typed free-text in the input box without selecting a preset option.
		// Treat as "提交执行" with the typed text as execution notes.
		return selected, false, false, nil
	}
}
