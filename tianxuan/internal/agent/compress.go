package agent

import (
	"sort"
	"strings"

	"tianxuan/internal/strutil"
)

// grepMatch 表示一条 grep 匹配行。
type grepMatch struct {
	file    string
	line    string // 原始行号字符串
	content string // 匹配文本
	score   float64
	index   int // 原文件内序号（0-based）
}

const (
	grepMaxMatches   = 30 // 全局匹配数上限
	grepMaxFiles     = 15 // 全局文件数上限
	grepPerFile      = 5  // 每文件最多保留
	grepNoCap        = 30 // 不超过此数不压缩
)

// 错误模式关键词——匹配这些的行加权保留。
var errorKeywords = []string{
	"FATAL", "ERROR", "panic", "exception", "fail",
	"Fail", "Error", "PANIC", "Exception",
}

// compressGrep 压缩 grep/search_content 工具的输出结果。
func compressGrep(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	lines := strings.Split(raw, "\n")

	// 解析每一行（同时过滤空行）
	matches := parseGrepLines(lines)
	if len(matches) <= grepNoCap {
		// 少量结果：重新组装（过滤空行，保持格式一致）
		return formatGrepPassthrough(matches)
	}

	// 按文件分组
	fileGroups := groupByFile(matches)

	// 计算分数
	scoreMatches(fileGroups)

	// 选择保留项
	selected := selectGrepMatches(fileGroups)

	// 格式化输出
	return formatGrepOutput(selected, fileGroups)
}

// parseGrepLines 解析 path:line:text 格式的行。
func parseGrepLines(lines []string) []grepMatch {
	matches := make([]grepMatch, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		m := parseOneGrepLine(line)
		if m.file != "" {
			matches = append(matches, m)
		}
	}
	return matches
}

// parseOneGrepLine 解析单行。格式: path:line:text
// 适配 Windows 盘符 (C:\...) 和文件名中的特殊字符。
func parseOneGrepLine(line string) grepMatch {
	// 找第二个冒号（文件路径中可能包含冒号，如 Windows C:\）
	// 策略：从后往前找 "数字:非数字" 边界作为行号分隔符
	// 简化为：找最后一个符合 ":数字:" 模式的位置
	rest := line
	var file string

	// 尝试按 ":\d+:" 模式分割——这是 riplgrep 行号标记
	for i := 0; i < len(rest)-1; i++ {
		if rest[i] == ':' && i+1 < len(rest) && rest[i+1] >= '0' && rest[i+1] <= '9' {
			// 检查这是否是行号开始
			j := i + 1
			for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
				j++
			}
			if j < len(rest) && rest[j] == ':' {
				// 找到: file[:i] = 路径, i+1:j = 行号, j+1: = 内容
				file = rest[:i]
				lineNo := rest[i+1 : j]
				content := rest[j+1:]
				// 去除内容前导空格
				content = strings.TrimLeft(content, " ")
				return grepMatch{file: file, line: lineNo, content: content}
			}
		}
	}

	// 回退：按第一个冒号和最后一个冒号分割
	// 路径可能包含冒号（Windows），但行号标记一定在最后
	lastColon := strings.LastIndex(line, ":")
	if lastColon < 0 {
		return grepMatch{}
	}
	secondLast := strings.LastIndex(line[:lastColon], ":")
	if secondLast < 0 {
		return grepMatch{}
	}

	file = line[:secondLast]
	lineNo := line[secondLast+1 : lastColon]
	content := line[lastColon+1:]
	content = strings.TrimLeft(content, " ")
	return grepMatch{file: file, line: lineNo, content: content}
}

// groupByFile 按文件名分组。
func groupByFile(matches []grepMatch) map[string][]grepMatch {
	groups := make(map[string][]grepMatch)
	for i := range matches {
		m := matches[i]
		m.index = len(groups[m.file])
		groups[m.file] = append(groups[m.file], m)
	}
	return groups
}

// scoreMatches 给每条匹配打分。
// 首条 +0.4, 末条 +0.3, 错误行 +0.5。
func scoreMatches(groups map[string][]grepMatch) {
	for _, ms := range groups {
		n := len(ms)
		for i := range ms {
			score := 0.0
			// 首条加权
			if i == 0 {
				score += 0.4
			}
			// 末条加权
			if i == n-1 {
				score += 0.3
			}
			// 错误行加权
			for _, kw := range errorKeywords {
				if strings.Contains(ms[i].content, kw) {
					score += 0.5
					break
				}
			}
			ms[i].score = score
		}
	}
}

// selectGrepMatches 选择要保留的匹配行。
func selectGrepMatches(groups map[string][]grepMatch) map[string][]grepMatch {
	// 按文件匹配数降序排列
	type fileEntry struct {
		name  string
		count int
	}
	var files []fileEntry
	for f, ms := range groups {
		files = append(files, fileEntry{f, len(ms)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].count > files[j].count })

	// 限制文件数
	if len(files) > grepMaxFiles {
		files = files[:grepMaxFiles]
	}

	selected := make(map[string][]grepMatch)
	total := 0

	for _, fe := range files {
		ms := groups[fe.name]
		n := len(ms)

		// 少于等于每文件上限：全部保留
		if n <= grepPerFile {
			if total+n <= grepMaxMatches {
				selected[fe.name] = ms
				total += n
			} else {
				// 空间不够，只取前几个
				take := grepMaxMatches - total
				if take > 0 {
					selected[fe.name] = ms[:take]
					total += take
				}
			}
			continue
		}

		// 需要选择：首条+末条 + 高分中间行
		kept := make([]grepMatch, 0, grepPerFile)
		used := make(map[int]bool)

		// 首条必选
		if total < grepMaxMatches {
			kept = append(kept, ms[0])
			used[0] = true
			total++
		}

		// 末条必选（如果不与首条重合）
		if n > 1 && total < grepMaxMatches {
			kept = append(kept, ms[n-1])
			used[n-1] = true
			total++
		}

		// 中间行按分数降序选择
		type scored struct {
			idx   int
			score float64
		}
		var middle []scored
		for i := 1; i < n-1; i++ {
			middle = append(middle, scored{i, ms[i].score})
		}
		sort.Slice(middle, func(i, j int) bool { return middle[i].score > middle[j].score })

		for _, s := range middle {
			if len(kept) >= grepPerFile || total >= grepMaxMatches {
				break
			}
			kept = append(kept, ms[s.idx])
			total++
		}

		// 按原始行号排序
		sort.Slice(kept, func(i, j int) bool {
			return kept[i].index < kept[j].index
		})
		selected[fe.name] = kept
	}

	return selected
}

// formatGrepLine 格式化单行为 path:line: text。
func formatGrepLine(m grepMatch) string {
	return m.file + ":" + m.line + ": " + m.content
}

// formatGrepPassthrough 少量结果时直接重组（过滤空行）。
func formatGrepPassthrough(matches []grepMatch) string {
	var out strings.Builder
	for i, m := range matches {
		if i > 0 {
			out.WriteString("\n")
		}
		out.WriteString(formatGrepLine(m))
	}
	return out.String()
}

// ─── tree 压缩器 ────────────────────────────────────────────────

// 已知噪声目录——这些在树输出中应被折叠。
var noiseDirs = []string{
	"node_modules", ".git", "dist", "build", "target",
	"__pycache__", ".next", ".nuxt", ".cache", ".venv",
	"venv", "coverage", "out", ".turbo", ".devenv",
}

// compressTree 压缩目录树输出。
// 折叠已知噪声目录，其余内容原样保留。
func compressTree(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	lines := strings.Split(raw, "\n")

	// 检测是否有噪声目录
	hasNoise := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// 去除尾部斜杠
		name := strings.TrimRight(trimmed, "/")
		for _, nd := range noiseDirs {
			if name == nd {
				hasNoise = true
				break
			}
		}
		if hasNoise {
			break
		}
	}

	if !hasNoise {
		return raw
	}

	// 折叠噪声目录：保留目录名，隐藏其子内容
	var out strings.Builder
	skipDepth := -1
	for _, line := range lines {
		if skipDepth >= 0 {
			// 计算当前行缩进深度（2空格一级）
			depth := indentDepth(line)
			if depth > skipDepth {
				continue // 跳过噪声目录的子内容
			}
			skipDepth = -1 // 退出噪声目录
		}

		trimmed := strings.TrimSpace(line)
		name := strings.TrimRight(trimmed, "/")

		isNoise := false
		for _, nd := range noiseDirs {
			if name == nd {
				isNoise = true
				break
			}
		}

		if isNoise {
			depth := indentDepth(line)
			skipDepth = depth
			out.WriteString(line)
			// 计算子条目数量
			childCount := countChildren(lines, depth)
			out.WriteString(" [")
			out.WriteString(strutil.Itoa(childCount))
			out.WriteString(" hidden — 依赖/构建目录]\n")
		} else {
			out.WriteString(line)
			out.WriteString("\n")
		}
	}

	return strings.TrimRight(out.String(), "\n")
}

// indentDepth 返回行首缩进级别（每2个空格为一级）。
func indentDepth(line string) int {
	count := 0
	for _, c := range line {
		if c == ' ' {
			count++
		} else {
			break
		}
	}
	return count / 2
}

// countChildren 统计某缩进级别下的子条目数。
func countChildren(lines []string, parentDepth int) int {
	count := 0
	inChildren := false
	for _, line := range lines {
		depth := indentDepth(line)
		if depth == parentDepth {
			if inChildren {
				break
			}
			continue
		}
		if depth == parentDepth+1 {
			inChildren = true
			count++
		} else if inChildren && depth <= parentDepth {
			break
		}
	}
	return count
}

// ─── SmartCompress 路由 ──────────────────────────────────────────

// ─── V5.10: KUN 风格按工具分策略压缩 ─────────────────────────────

// toolCompressLimits 定义各工具的输出上限（行数+字节数）。
// 数值来自 Kun token-economy.ts 的实测调优。
type toolCompressLimit struct {
	maxLines int
	maxBytes int
}

var toolCompressLimits = map[string]toolCompressLimit{
	"bash":            {180, 24 * 1024},  // shell 输出
	"run_command":     {180, 24 * 1024},
	"run_background":  {180, 24 * 1024},
	"read_file":       {320, 32 * 1024},  // 文件内容
	"glob":            {160, 24 * 1024},  // 文件列表
	"search_files":    {160, 24 * 1024},
	"ls":              {120, 24 * 1024},  // 目录列表
	"list_directory":  {120, 24 * 1024},
	"grep":            {0, 0},            // 走 compressGrep
	"search_content":  {0, 0},            // 走 compressGrep
	"directory_tree":  {0, 0},            // 走 compressTree
}

// SmartCompress V5.10: 根据工具名称路由到对应的压缩策略。
// grep/search_content → compressGrep（错误行加权+全局30条上限）
// directory_tree → compressTree（折叠噪声目录）
// bash/read_file/glob/ls → 行数+字节数二维上限
// 其余 → 原样返回（由 truncateToolOutput 统一处理）
func SmartCompress(toolName, raw string) string {
	switch toolName {
	case "grep", "search_content":
		return compressGrep(raw)
	case "directory_tree", "list_directory":
		return compressTree(raw)
	default:
		if limit, ok := toolCompressLimits[toolName]; ok {
			return compressByLimit(raw, limit.maxLines, limit.maxBytes)
		}
		return raw
	}
}

// compressByLimit 按行数+字节数上限压缩文本。保留信号行（error/fatal等）。
func compressByLimit(raw string, maxLines, maxBytes int) string {
	if maxLines <= 0 || maxBytes <= 0 {
		return raw
	}
	// 复用 truncateToolOutputWith 的逻辑
	result, _ := truncateToolOutputWith(raw, maxLines, maxBytes)
	return result
}

// ─── 输出格式化 ──────────────────────────────────────────────────
func formatGrepOutput(selected map[string][]grepMatch, original map[string][]grepMatch) string {
	var out strings.Builder

	// 按文件名排序输出
	var fileNames []string
	for f := range selected {
		fileNames = append(fileNames, f)
	}
	sort.Strings(fileNames)

	firstFile := true
	for _, f := range fileNames {
		ms := selected[f]
		for _, m := range ms {
			if !firstFile || out.Len() > 0 {
				out.WriteString("\n")
			}
			out.WriteString(formatGrepLine(m))
			firstFile = false
		}
		// 如果有省略，添加摘要
		orig := original[f]
		if len(orig) > len(ms) {
			omitted := len(orig) - len(ms)
			out.WriteString("[... and ")
			out.WriteString(strutil.Itoa(omitted))
			out.WriteString(" more matches in ")
			out.WriteString(f)
			out.WriteString("]\n")
		}
	}

	return strings.TrimRight(out.String(), "\n")
}
