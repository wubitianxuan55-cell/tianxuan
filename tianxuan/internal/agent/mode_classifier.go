package agent

import (
	"strings"
	"unicode/utf8"
)

// ClassifyMode 根据用户输入启发式返回推荐的 agent 运行模式。
// 返回值: "explore"（只读研究）、"develop"（全工具开发）、"orchestrate"（规划→执行→验证）。
// 该函数是纯确定性的——相同输入始终返回相同结果，不影响缓存稳定性。
//
// 分类规则（优先级递减）：
//  1. 纯探索意图（提问/了解/查找） → explore
//  2. 复杂多阶段任务（长输入+多文件+关键词） → orchestrate
//  3. 默认 → develop
func ClassifyMode(input string) string {
	text := strings.TrimSpace(input)
	if text == "" {
		return "develop"
	}

	lower := strings.ToLower(text)
	runes := utf8.RuneCountInString(text)

	// ── 规则 1: 纯探索意图 → explore ──
	if isExploreIntent(lower, runes) {
		return "explore"
	}

	// ── 规则 2: 复杂多阶段任务 → orchestrate ──
	if isOrchestrateIntent(text, lower, runes) {
		return "orchestrate"
	}

	// ── 规则 3: 默认 → develop ──
	return "develop"
}

// ModeScore 记录分类过程中的评分详情，用于诊断。
type ModeScore struct {
	Mode             string // 最终分类结果
	ExploreScore     int    // explore 倾向得分
	OrchestrateScore int    // orchestrate 倾向得分
	Reasons          []string
}

// ClassifyModeWithScore 返回分类结果及其评分详情。
func ClassifyModeWithScore(input string) ModeScore {
	ms := ModeScore{Mode: ClassifyMode(input)}
	// 反向计算分数用于诊断
	text := strings.TrimSpace(input)
	lower := strings.ToLower(text)
	runes := utf8.RuneCountInString(text)

	if isExploreIntent(lower, runes) {
		ms.ExploreScore = 1
		ms.Reasons = append(ms.Reasons, "纯探索意图")
	}
	if isOrchestrateIntent(text, lower, runes) {
		ms.OrchestrateScore = 1
		ms.Reasons = append(ms.Reasons, "复杂多阶段任务")
	}
	if ms.ExploreScore == 0 && ms.OrchestrateScore == 0 {
		ms.Reasons = append(ms.Reasons, "默认开发模式")
	}
	return ms
}

// ─── 探索意图检测 ───

var exploreQuestionStarts = []string{
	"what", "how", "why", "when", "where", "who", "which",
	"explain", "describe", "show", "list", "find", "search",
	"tell me", "can you explain", "i want to understand",
	"什么是", "为什么", "怎么", "如何", "解释", "说明",
	"查找", "搜索", "列出", "显示", "告诉我", "了解一下",
	"怎么看", "查一下", "帮我看看", "分析一下",
}

var exploreOnlyKeywords = []string{
	"code review", "review this", "understand", "overview",
	"codebase", "architecture", "how does",
	"代码审查", "理解", "架构", "概览", "梳理",
}

func isExploreIntent(lower string, runes int) bool {
	// 以提问词开头
	for _, start := range exploreQuestionStarts {
		if strings.HasPrefix(lower, start) {
			// 确认没有修改意图
			if !containsAny(lower, actionVerbs) {
				return true
			}
		}
	}

	// 短输入（< 80 字符）且不含操作动词 → 很可能是问题
	if runes < 80 && !containsAny(lower, actionVerbs) {
		return true
	}

	// 包含纯探索关键词
	for _, kw := range exploreOnlyKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}

	return false
}

// ─── Orchestrate 意图检测 ───

var orchestrateKeywords = []string{
	"implement", "refactor", "migrate", "redesign", "rewrite",
	"build a", "create a", "add support for", "end-to-end",
	"full stack", "multi-step", "multi-phase", "step by step",
	"实现", "重构", "迁移", "重新设计", "重写",
	"端到端", "全栈", "多步骤", "多阶段", "逐步",
	"搭建", "构建", "新增功能",
}

var multiFileIndicators = []string{
	"multiple files", "several files", "across",
	"frontend and backend", "api and ui",
	"多个文件", "多处", "前后端", "跨模块",
}

func isOrchestrateIntent(text, lower string, runes int) bool {
	score := 0

	// 长输入（> 400 字符）→ 强信号
	if runes > 400 {
		score += 3
	} else if runes > 200 {
		score += 1
	}

	// 包含编号列表（用户已列出步骤）
	if strings.Count(text, "\n") >= 3 {
		lines := strings.Split(text, "\n")
		numberedCount := 0
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if len(trimmed) >= 2 && (trimmed[0] >= '1' && trimmed[0] <= '9') &&
				(trimmed[1] == '.' || trimmed[1] == ')' || strings.HasPrefix(trimmed[1:], "、")) {
				numberedCount++
			}
		}
		if numberedCount >= 3 {
			score += 3
		}
	}

	// 编排关键词
	if containsAny(lower, orchestrateKeywords) {
		score += 2
	}

	// 多文件指标
	if containsAny(lower, multiFileIndicators) {
		score += 2
	}

	// 引用多个代码文件
	codeFileCount := strings.Count(lower, ".go") + strings.Count(lower, ".ts") +
		strings.Count(lower, ".tsx") + strings.Count(lower, ".js") +
		strings.Count(lower, ".py") + strings.Count(lower, ".rs")
	if codeFileCount >= 3 {
		score += 2
	}

	return score >= 5
}

// ─── 操作动词（与探索意图互斥） ───

var actionVerbs = []string{
	"implement", "fix", "refactor", "add", "create",
	"write", "build", "deploy", "optimize", "remove", "delete",
	"change", "modify", "update", "replace", "rename",
	"实现", "修复", "重构", "创建", "添加", "删除", "优化",
	"修改", "更新", "替换", "重命名", "新增", "去掉",
}

// ─── 辅助函数 ───

func containsAny(s string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}
