package agent

import (
	"strings"
	"unicode/utf8"

	"tianxuan/internal/provider"
)

// ─── V5.14: 启发式自动模型路由 (Kun auto-model-router.ts 移植) ──────────

// AutoRouteModel 是路由结果中的模型名称。
type AutoRouteModel string

const (
	AutoRouteFlash AutoRouteModel = "deepseek-v4-flash"
	AutoRoutePro   AutoRouteModel = "deepseek-v4-pro"
)

// complexKeywords — 这些关键词暗示任务需要深度推理，路由到 pro。
// 可通过 Options.RouterKeywords 追加自定义关键词（V10.12）。
var complexKeywords = []string{
	"refactor", "architecture", "design", "debug", "security",
	"review", "audit", "migrate", "optimize", "rewrite",
	"implement", "analyze", "漏洞", "安全", "重构", "架构",
	"设计", "调试", "审查", "迁移", "优化", "重写", "实现", "分析",
}

// SetRouterKeywords merges custom keywords with the built-in list.
// Call once during agent construction. Subsequent calls replace, not append.
// A nil or empty slice leaves the built-in defaults unchanged.
func SetRouterKeywords(custom []string) {
	if len(custom) == 0 {
		return
	}
	merged := make([]string, 0, len(complexKeywords)+len(custom))
	seen := make(map[string]bool, len(complexKeywords)+len(custom))
	for _, kw := range complexKeywords {
		lower := strings.ToLower(kw)
		if !seen[lower] {
			seen[lower] = true
			merged = append(merged, kw)
		}
	}
	for _, kw := range custom {
		lower := strings.ToLower(kw)
		if !seen[lower] {
			seen[lower] = true
			merged = append(merged, kw)
		}
	}
	complexKeywords = merged
}

// AutoRoute 根据输入内容启发式选择模型。
// 规则（优先级递减）：
//  1. 包含复杂关键词 → pro
//  2. 输入 < 100 字符 → flash（简单问题）
//  3. 输入 > 500 字符 → pro（复杂任务）
//  4. 默认 → flash
//
// 这是 Kun auto-model-router.ts 启发式回退的 Go 移植。
func AutoRoute(input string) AutoRouteModel {
	lower := strings.ToLower(input)

	// 复杂关键词 → pro
	for _, kw := range complexKeywords {
		if strings.Contains(lower, kw) {
			return AutoRoutePro
		}
	}

	// 长度启发式
	runes := utf8.RuneCountInString(input)
	if runes < 100 {
		return AutoRouteFlash
	}
	if runes > 500 {
		return AutoRoutePro
	}

	return AutoRouteFlash
}

// AutoRouteProvider 根据路由结果选择 provider。
// flashProv 为 nil 时，始终返回默认 provider（禁用自动路由）。
func AutoRouteProvider(input string, defaultProv, flashProv provider.Provider) provider.Provider {
	if flashProv == nil {
		return defaultProv
	}
	route := AutoRoute(input)
	if route == AutoRouteFlash {
		return flashProv
	}
	return defaultProv
}
