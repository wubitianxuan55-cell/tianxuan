package agent

import "testing"

// O5 契约：Route 三值化——RouteExecOnly（跳过规划者直接执行）、
// RoutePlannerChat（规划者直接回答，不产生计划不派发执行者）、
// RoutePlanAndExec（完整规划）。此前聊天类输入也返回 RouteExecOnly，
// 与 Run 只认 3 种 reason 的白名单语义重载。

func TestDecidePlannerRoute_ChatInputsArePlannerChat(t *testing.T) {
	inputs := []string{
		"ok", "好的", "谢谢", "got it", // short_reply / conversation
		"什么是缓存", "怎么用这个工具", "how does this work", // low_risk_question
		"hello", "今天天气", // no_work
		"保存会话", "提交代码", // "保存"/"提交" 不在 work 词表 → no_work，规划者处理（与历史运行行为一致）
		"",          // empty
		"/unknown",  // slash_command（headless 防御，planner 处理保持原行为）
	}
	for _, q := range inputs {
		d := DecidePlannerRoute(q)
		if d.Route != RoutePlannerChat {
			t.Errorf("%q should be planner_chat (got %s reason=%s)", q, d.Route, d.Reason)
		}
	}
}

func TestDecidePlannerRoute_WorkInputsStayExecOnly(t *testing.T) {
	for _, q := range []string{"fix typo in readme", "运行测试", "构建", "删除旧文件"} {
		d := DecidePlannerRoute(q)
		if d.Route != RouteExecOnly {
			t.Errorf("%q should be executor_only (got %s reason=%s)", q, d.Route, d.Reason)
		}
	}
}
