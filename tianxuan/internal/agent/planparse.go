package agent

import "strings"

// planparse.go 统一 Hermes 计划文本的行级解析。
//
// isStepLine / extractStepTitle 曾是 hermes_confirm.go（isStepLine）与
// hermes_sdd.go（planLineRE / extractStepTitle）各自实现的重复逻辑。
// 本文件是唯一实现，计划格式演进只需改这里一处。

// isStepLine 判定一行是否为步骤标题。步骤行格式："步骤 N：标题" 或
// "Step N: title"，冒号可省略。调用方负责先 TrimSpace。
func isStepLine(trimmed string) bool {
	for _, prefix := range []string{"步骤 ", "Step "} {
		if after, ok := strings.CutPrefix(trimmed, prefix); ok {
			if len(after) > 0 && after[0] >= '0' && after[0] <= '9' {
				return true
			}
		}
	}
	return false
}

// extractStepTitle 从步骤标题行提取标题文本（跳过完整步骤号与分隔符）。
// 非步骤行原样返回。多位数步骤号（"步骤 12：xxx"）会整体跳过。
func extractStepTitle(trimmed string) string {
	for _, prefix := range []string{"步骤 ", "Step "} {
		after, ok := strings.CutPrefix(trimmed, prefix)
		if !ok {
			continue
		}
		i := 0
		for i < len(after) && after[i] >= '0' && after[i] <= '9' {
			i++
		}
		if i == 0 {
			continue // 编号不以数字开头——不是步骤行
		}
		return strings.TrimLeft(after[i:], "：: \t")
	}
	return trimmed
}

// countPlanSteps 统计计划文本中的步骤数（"步骤 N：" / "Step N:" 标题行）。
// 无标准步骤格式的计划返回 0。
func countPlanSteps(plan string) int {
	n := 0
	for _, line := range strings.Split(plan, "\n") {
		if isStepLine(strings.TrimSpace(line)) {
			n++
		}
	}
	return n
}
