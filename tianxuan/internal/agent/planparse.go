package agent

import "strings"

// planparse.go 统一计划文本的行级解析。
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

// looksLikePlan 检测文本是否呈计划结构，用于 <!--plan--> 标记缺失时的
// 漏标补偿：≥2 个步骤行，或 1 个步骤行带 Verify/File(s)/Files/Delta
// 结构化字段。普通聊天/回答不含"步骤 N："标题格式，不会误伤。
func looksLikePlan(text string) bool {
	stepLines := 0
	hasField := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if isStepLine(trimmed) {
			stepLines++
			continue
		}
		for _, field := range []string{"- **Verify**", "- **File(s)**", "- **Files**", "- **Delta**"} {
			if strings.HasPrefix(trimmed, field) {
				hasField = true
				break
			}
		}
	}
	return stepLines >= 2 || (stepLines >= 1 && hasField)
}

// extractPlanStepTitles 提取 <!--plan--> 之后所有步骤标题（保留原始标题
// 文本，供 coverage 对照与展示）。
func extractPlanStepTitles(plan string) []string {
	var titles []string
	inPlan := false
	for _, line := range strings.Split(plan, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<!--plan-->") {
			inPlan = true
			continue
		}
		if !inPlan {
			continue
		}
		if isStepLine(trimmed) {
			titles = append(titles, extractStepTitle(trimmed))
		}
	}
	return titles
}

// normalizeStepTitle 归一化步骤标题用于相似匹配：小写、去除"步骤 N："
// 编号前缀与常见图标、压缩空白。
func normalizeStepTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, prefix := range []string{"步骤 ", "step "} {
		if after, ok := strings.CutPrefix(s, prefix); ok {
			i := 0
			for i < len(after) && after[i] >= '0' && after[i] <= '9' {
				i++
			}
			if i > 0 {
				s = strings.TrimLeft(after[i:], "：: \t")
				break
			}
		}
	}
	s = strings.TrimLeft(s, "✅❌⚠️🔧📌 ")
	return strings.Join(strings.Fields(s), " ")
}

// similarStepTitle 判断两个归一化标题是否相似：任一包含另一即视为覆盖。
// 执行者可以合并步骤（"实现+重构"）或微调措辞，包含匹配容忍这类改写。
func similarStepTitle(a, b string) bool {
	return a != "" && b != "" && (strings.Contains(a, b) || strings.Contains(b, a))
}

// checkStepCoverage 对照计划步骤标题与执行者的 complete_step 步骤名，
// 返回计划中存在但没有任何签收步骤相似匹配的标题。信息性信号——执行者
// 合并/改写步骤是允许的，因此结果不参与 pass/fail 判定，只进入 verify
// 反馈与修正计划上下文，供规划者判断是否漏做。
func checkStepCoverage(plan string, r *TurnResult) []string {
	if r == nil || len(r.StepResults) == 0 {
		return nil
	}
	planTitles := extractPlanStepTitles(plan)
	if len(planTitles) == 0 {
		return nil
	}
	var signed []string
	for _, sr := range r.StepResults {
		if n := normalizeStepTitle(sr.Step); n != "" {
			signed = append(signed, n)
		}
	}
	var missing []string
	for _, pt := range planTitles {
		pn := normalizeStepTitle(pt)
		if pn == "" {
			continue
		}
		matched := false
		for _, sn := range signed {
			if similarStepTitle(pn, sn) {
				matched = true
				break
			}
		}
		if !matched {
			missing = append(missing, pt)
		}
	}
	return missing
}
