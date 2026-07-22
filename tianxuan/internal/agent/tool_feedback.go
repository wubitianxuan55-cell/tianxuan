// Package agent
// ── 工具失败的结构化反馈 ──────────────────────────────────────────────────
//
// 当一轮中有 2+ 工具返回 error/blocked 时，注入归纳性的系统消息，
// 帮助 LLM 理解失败模式并调整策略。连续注入不超过 3 轮以防止干扰过度。
//
// 设计参考 Aider 的 reflected_message 模式：将失败信息以 user 消息形式
// 注入会话，让 LLM 在下一轮直接看到问题，而不是仅依靠分散的 tool_result。

package agent

import (
	"fmt"
	"strings"

	"tianxuan/internal/provider"
)

// ToolFeedbackCap 连续工具反馈消息的最大注入轮数。
const ToolFeedbackCap = 3

// toolFeedbackThreshold 触发反馈的最小失败工具数。
const toolFeedbackThreshold = 2

// maybeInjectToolFeedback 检查工具执行结果并在连续失败时注入结构化反馈。
// 返回 true 表示注入了消息（调用方应发出 notice 事件）。
//
// 两层机制：
//   - 温和模式（2+ 错误）: 注入分类错误分析 + 修正建议（参考 Aider reflected_message）
//   - 强硬模式（全批次非 blocked 调用都失败 + 已连续 >=3 轮）: STOP and re-assess
func (a *AgentRunner) maybeInjectToolFeedback(calls []provider.ToolCall, results []string) bool {
	if a.plannerMode {
		return false
	}

	errCount, blockedCount, details := analyzeResults(calls, results)

	// All blocked → normal with permission gating, ignore.
	if blockedCount == len(results) {
		a.toolFeedbackCount = 0
		return false
	}

	// Not enough failures → reset counter.
	if errCount < toolFeedbackThreshold {
		a.toolFeedbackCount = 0
		return false
	}

	a.toolFeedbackCount++
	if a.toolFeedbackCount > ToolFeedbackCap {
		return false
	}

	// All non-blocked calls failed → stuck, use firm steer at count 1 and 3.
	allFailed := errCount == len(results) && blockedCount < len(results)

	var msg string
	if allFailed && a.toolFeedbackCount >= 3 {
		msg = "[system] 你已连续多轮全部操作失败。停下来重新评估方案。\n" +
			"先用 read_file/ls/glob 了解现状，或用 ask 工具向用户确认方向。\n" +
			"不要继续当前做法。\n\n" + buildToolFeedbackMessage(errCount, len(results), details)
	} else if allFailed && a.toolFeedbackCount == 1 {
		msg = "[system] 本轮全部操作都失败了。尝试不同方法——先读取相关代码再动手，" +
			"把任务拆成更小的步骤，或在不确定时用 ask 工具询问。\n\n" +
			buildToolFeedbackMessage(errCount, len(results), details)
	} else {
		msg = buildToolFeedbackMessage(errCount, len(results), details)
	}

	a.session.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: msg,
	})
	return true
}

// analyzeResults counts errors and blocked calls in a batch of tool results.
// blocked is tracked separately for the "all-blocked is normal" heuristic,
// but counted together with errors for the feedback trigger threshold (matching
// isErrorResult's broad definition that includes blocked/error/Error/[error).
func analyzeResults(calls []provider.ToolCall, results []string) (errors, blocked int, details []string) {
	for i, r := range results {
		if strings.HasPrefix(r, "blocked:") {
			blocked++
			errors++ // treat as error for trigger threshold (matching isErrorResult)
			if len(details) < 5 {
				name := ""
				if i < len(calls) {
					name = calls[i].Name
				}
				details = append(details, fmt.Sprintf("  %s → %s", name, truncateStr(r, 100)))
			}
		} else if isErrorResult(r) {
			errors++
			if len(details) < 5 {
				name := ""
				if i < len(calls) {
					name = calls[i].Name
				}
				details = append(details, fmt.Sprintf("  %s → %s", name, truncateStr(r, 100)))
			}
		}
	}
	return
}

// buildToolFeedbackMessage 根据错误信息构建结构化反馈消息。
func buildToolFeedbackMessage(errCount, total int, details []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		"[system] 本轮 %d 个工具调用中 %d 个失败。请分析以下错误,调整策略后重试:\n",
		total, errCount))
	for _, d := range details {
		b.WriteString(d)
		b.WriteString("\n")
	}

	// 按错误类型给出建议
	categories := categorizeErrors(details)
	if len(categories) > 0 {
		b.WriteString("\n错误类型分析:\n")
		for _, cat := range categories {
			b.WriteString(fmt.Sprintf("  • %s: %s\n", cat.label, cat.hint))
		}
	}

	b.WriteString("\n不要重复相同操作。如果同一工具连续失败，改用其他工具或方法。")
	return b.String()
}

// errorCategory 错误分类及其建议。
type errorCategory struct {
	label string
	hint  string
}

// categorizeErrors 按错误关键词分类并给出修正建议。
func categorizeErrors(details []string) []errorCategory {
	var cats []errorCategory
	seen := map[string]bool{}

	for _, d := range details {
		lower := strings.ToLower(d)

		switch {
		case strings.Contains(lower, "not found") || strings.Contains(lower, "no such file"):
			if !seen["file_missing"] {
				seen["file_missing"] = true
				cats = append(cats, errorCategory{
					"文件缺失", "检查文件路径是否正确，或用 glob/ls 确认文件存在",
				})
			}
		case strings.Contains(lower, "undefined:") || strings.Contains(lower, "cannot use"):
			if !seen["compile"] {
				seen["compile"] = true
				cats = append(cats, errorCategory{
					"编译错误", "检查 import 和类型定义，运行 go build 验证修复",
				})
			}
		case strings.Contains(lower, "permission") || strings.Contains(lower, "denied"):
			if !seen["permission"] {
				seen["permission"] = true
				cats = append(cats, errorCategory{
					"权限错误", "确认文件/目录权限，或改用可写的路径",
				})
			}
		case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline"):
			if !seen["timeout"] {
				seen["timeout"] = true
				cats = append(cats, errorCategory{
					"超时", "缩小操作范围、增加超时时间，或拆分任务",
				})
			}
		case strings.Contains(lower, "blocked:") && !seen["blocked"]:
			seen["blocked"] = true
			cats = append(cats, errorCategory{
				"被阻止", "部分操作被安全策略/风暴断路器阻止，改用其他方法",
			})
		}
	}

	// 通用建议（如果没有特定分类）
	if len(cats) == 0 && !seen["generic"] {
		cats = append(cats, errorCategory{
			"通用错误", "检查错误消息中的具体原因，修正参数或改用其他工具",
		})
	}

	return cats
}
