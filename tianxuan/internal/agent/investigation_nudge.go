package agent

import (
	"strings"

	"tianxuan/internal/provider"
)

// investigationNudge 提示主 agent：大批量调查信息不需要进主上下文，
// 应派发子代理在隔离上下文处理，只接收精炼结论（V10.139）。
const investigationNudge = "[system] 本轮包含大量调查类工具调用。主上下文里的" +
	"read_file/grep/glob 输出会永久占用空间——批量调查（多文件阅读、大文件内容、" +
	"深度搜索、外部研究）默认应派发 explore/research 子代理（run_skill）在隔离" +
	"上下文完成，只把精炼结论（带 file:line 锚点）带回主上下文。多个独立调查" +
	"问题用 parallel_skills 并行派发。主上下文只保留决策、实现与验证所需的信息。"

// investigationTools 是纯调查类工具：它们的输出是"不需要传递到上下文"的
// 典型——由子代理消化后只回传结论。
var investigationTools = map[string]bool{
	"read_file": true, "grep": true, "glob": true, "ls": true,
	"code_index": true, "web_search": true, "web_fetch": true,
}

// maybeNudgeInvestigation 检测单轮"纯调查膨胀"：调查类工具调用数达到阈值
// 且无写工具（说明是调查轮而非执行轮）时，注入改用子代理的引导。
// 每 turn 有注入上限，避免反复打扰。
func (a *AgentRunner) maybeNudgeInvestigation(calls []provider.ToolCall) bool {
	if a == nil || a.plannerMode || a.disableVerify {
		return false
	}
	investigations, hasWrite := 0, false
	for _, c := range calls {
		if isInvestigationTool(c.Name) {
			investigations++
			continue
		}
		if isWriterTool(c.Name) {
			hasWrite = true
		}
	}
	if investigations < InvestigationNudgeThreshold || hasWrite {
		return false
	}
	if a.investigationNudgeCount >= InvestigationNudgeCap {
		return false
	}
	a.investigationNudgeCount++
	a.session.Add(provider.Message{Role: provider.RoleUser, Content: investigationNudge})
	return true
}

func isInvestigationTool(name string) bool {
	if investigationTools[name] {
		return true
	}
	return strings.HasPrefix(name, "mcp__codegraph__")
}

// isWriterTool reports whether the tool mutates files (execution signal).
func isWriterTool(name string) bool {
	switch name {
	case "write_file", "edit_file", "multi_edit", "notebook_edit",
		"delete_range", "delete_symbol", "move_file":
		return true
	default:
		return false
	}
}
