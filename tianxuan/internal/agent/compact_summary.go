package agent

import (
"sort"
"strings"

"tianxuan/internal/provider"
	"tianxuan/internal/strutil"
)

// 提取：用户请求、工具统计、编辑文件、待办项、关键文件、最近工作。
// 完全确定性：相同输入 → 相同输出，不影响缓存稳定性。
func BuildCompactSummary(truncated []provider.Message) string {
	if len(truncated) == 0 {
		return ""
	}

	// 统计
	var filesEdited []string
	seenFiles := make(map[string]bool)
	toolCounts := make(map[string]int)
	turnCount := 0
	var recentUserReqs []string    // 最近 3 条用户请求
	var pendingItems []string      // 待办项（含 todo/next/pending/follow up）
	var keyFiles []string          // 引用到的关键文件
	seenKeyFiles := make(map[string]bool)

	for _, msg := range truncated {
		switch msg.Role {
		case provider.RoleUser:
			if msg.Content != "" && !strings.HasPrefix(msg.Content, "[") {
				turnCount++
				// 收集最近用户请求（最多5条）
				short := truncateText(msg.Content, 160)
				if short != "" {
					recentUserReqs = append(recentUserReqs, short)
				}
			}
		case provider.RoleAssistant:
			// 工具统计
			for _, tc := range msg.ToolCalls {
				toolCounts[tc.Name]++
				// 提取编辑操作的文件路径
				switch tc.Name {
				case "edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol":
					path := extractFilePath(tc.Name, tc.Arguments)
					if path != "" && !seenFiles[path] {
						filesEdited = append(filesEdited, path)
						seenFiles[path] = true
					}
				}
			}
			// 检测待办项
			lower := strings.ToLower(msg.Content)
			for _, kw := range []string{"todo", "next", "pending", "follow up", "remaining"} {
				if strings.Contains(lower, kw) {
					short := truncateText(msg.Content, 160)
					if short != "" {
						pendingItems = append(pendingItems, short)
					}
					break
				}
			}
		}
		// 提取关键文件路径
		for _, fp := range extractKeyFiles(msg) {
			if !seenKeyFiles[fp] {
				keyFiles = append(keyFiles, fp)
				seenKeyFiles[fp] = true
			}
		}
	}

	if turnCount == 0 && len(toolCounts) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[Earlier conversation summary:\n")

	// 概览
	sb.WriteString("- Scope: ")
	sb.WriteString(strutil.Itoa(len(truncated)))
	sb.WriteString(" messages compacted, ")
	sb.WriteString(strutil.Itoa(turnCount))
	sb.WriteString(" turns\n")

	// 最近用户请求（最后 3 条）
	if len(recentUserReqs) > 0 {
		limit := len(recentUserReqs)
		if limit > 3 {
			limit = 3
		}
		start := len(recentUserReqs) - limit
		if start < 0 {
			start = 0
		}
		sb.WriteString("- Recent requests:\n")
		for _, req := range recentUserReqs[start:] {
			sb.WriteString("  - ")
			sb.WriteString(req)
			sb.WriteString("\n")
		}
	}

	// 编辑文件
	if len(filesEdited) > 0 {
		sb.WriteString("- Files modified: ")
		limit := len(filesEdited)
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			if i > 0 {
				sb.WriteString(", ")
			}
			short := filesEdited[i]
			if idx := strings.LastIndex(short, "/"); idx >= 0 {
				short = short[idx+1:]
			}
			sb.WriteString(short)
		}
		if len(filesEdited) > 8 {
			sb.WriteString(", ...")
		}
		sb.WriteString("\n")
	}

	// 工具使用
	if len(toolCounts) > 0 {
		sb.WriteString("- Tools used: ")
		type tc struct {
			name  string
			count int
		}
		var sorted []tc
		for name, count := range toolCounts {
			sorted = append(sorted, tc{name, count})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
		for i, t := range sorted {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(t.name)
			sb.WriteString("×")
			sb.WriteString(strutil.Itoa(t.count))
		}
		sb.WriteString("\n")
	}

	// 待办项
	if len(pendingItems) > 0 {
		sb.WriteString("- Pending work:\n")
		limit := len(pendingItems)
		if limit > 3 {
			limit = 3
		}
		for _, item := range pendingItems[:limit] {
			sb.WriteString("  - ")
			sb.WriteString(item)
			sb.WriteString("\n")
		}
	}

	// 关键文件
	if len(keyFiles) > 0 {
		sb.WriteString("- Key files: ")
		limit := len(keyFiles)
		if limit > 8 {
			limit = 8
		}
		for i, f := range keyFiles[:limit] {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(f)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("]")
	return sb.String()
}

// messageImportance 是对单条消息重要性的确定性评分函数。
// 返回 0.0-1.0 的分值；高于 0.35 的消息在折叠时优先保留。
// 完全确定性：相同输入 → 相同输出，不影响缓存稳定性。
func messageImportance(m provider.Message) float64 {
	score := 0.0

	// 工具结果包含错误信号 → 必须保留上下文
	if m.Role == provider.RoleTool {
		for _, kw := range []string{"FATAL", "ERROR", "panic", "PANIC", "Permission denied", "permission denied", "Access denied"} {
			if strings.Contains(m.Content, kw) {
				score += 0.5
				break
			}
		}
		// 编译/测试失败
		for _, kw := range []string{"FAIL", "exit status 1", "build failed", "test failed"} {
			if strings.Contains(m.Content, kw) {
				score += 0.3
				break
			}
		}
		// 长内容 → 略降优先级（更倾向折叠冗长成功结果）
		if len(m.Content) > 2000 {
			score -= 0.15
		}
	}

	// 用户消息包含硬约束
	if m.Role == provider.RoleUser {
		lower := strings.ToLower(m.Content)
		for _, kw := range []string{"不要", "禁止", "必须", "never", "must", "don't", "do not", "绝对不能"} {
			if strings.Contains(lower, kw) {
				score += 0.4
				break
			}
		}
	}

	// Assistant 编辑操作 → 保留关键决策
	if m.Role == provider.RoleAssistant {
		for _, tc := range m.ToolCalls {
			switch tc.Name {
			case "edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol":
				score += 0.2
			}
		}
	}

	return score
}

// keepThreshold 是 messageImportance 评分的保留阈值。
const keepThreshold = 0.35
func truncateText(s string, maxChars int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars]) + "…"
}

// extractKeyFiles 从消息中提取引用的文件路径。
func extractKeyFiles(msg provider.Message) []string {
	var texts []string
	if msg.Content != "" {
		texts = append(texts, msg.Content)
	}
	for _, tc := range msg.ToolCalls {
		texts = append(texts, tc.Arguments)
	}
	var files []string
	seen := make(map[string]bool)
	for _, text := range texts {
		for _, token := range strings.Fields(text) {
			token = strings.Trim(token, `,.:;()"'` + "`")
			if !strings.Contains(token, "/") {
				continue
			}
			// 检查是否有已知代码文件扩展名
			hasExt := false
			for _, ext := range []string{".go", ".ts", ".tsx", ".js", ".py", ".rs", ".java", ".md", ".json", ".yaml", ".yml", ".toml"} {
				if strings.HasSuffix(token, ext) {
					hasExt = true
					break
				}
			}
			if hasExt && !seen[token] {
				files = append(files, token)
				seen[token] = true
			}
		}
	}
	return files
}
