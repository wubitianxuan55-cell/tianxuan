package control

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"tianxuan/internal/agent"
	"tianxuan/internal/event"
)

const (
	autoPlanOff = "off"
	autoPlanAsk = "ask"
	autoPlanOn  = "on"
)

var numberedListRE = regexp.MustCompile(`(?m)^\s*(?:[-*]|\d+[.)])\s+\S`)

type autoPlanClassifier interface {
	NeedsPlan(ctx context.Context, input string, score int) (bool, string, error)
}

func normalizeAutoPlan(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", autoPlanAsk:
		return autoPlanAsk
	case autoPlanOn:
		return autoPlanOn
	case autoPlanOff:
		return autoPlanOff
	default:
		return autoPlanAsk
	}
}

func (c *Controller) maybeAutoPlan(ctx context.Context, input string) {
	c.mu.Lock()
	mode := c.autoPlan
	am := c.agentMode
	classifier := c.classifier
	c.mu.Unlock()

	// V9.0: If already in a planning mode (explore/orchestrate), skip.
	// Only auto-switch from develop mode.
	if am != "" && am != "develop" {
		return
	}
	if mode == autoPlanOff {
		return
	}

	score := autoPlanScore(input)
	if score <= 0 {
		return
	}
	// V10.16: 区分 "on"（始终规划，旧 PlanModeMarker）和 "ask"（自动检测，
	// 切换 orchestrate 模式）。V10.13 错误地将两者统一为 orchestrate，导致
	// AutoPlan="on" 的测试和行为回归。
	if mode == autoPlanOn {
		if score >= 1 {
			c.SetPlanMode(true)
			c.notice("auto plan: complex task detected, entering plan mode")
		}
		return
	}
	// "ask" mode: classifier + heuristic → orchestrate
	if classifier != nil && score <= 2 {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		needsPlan, reason, err := classifier.NeedsPlan(ctx, input, score)
		if err == nil {
			if needsPlan {
				if reason != "" {
					c.notice("auto plan classifier: " + reason)
				}
				c.SetAgentMode("orchestrate")
				c.notice("auto mode: switched to orchestrate (multi-step task detected)")
				return
			}
			return
		}
		c.notice("auto plan classifier failed; falling back to heuristic: " + err.Error())
	}
	if score >= 1 {
		c.SetAgentMode("orchestrate")
		c.notice("auto mode: switched to orchestrate (multi-step task detected)")
	}
}

func autoPlanScore(input string) int {
	text := strings.TrimSpace(input)
	if text == "" || strings.HasPrefix(text, "/") || strings.HasPrefix(text, PlanModeMarker) {
		return 0
	}
	lower := strings.ToLower(text)
	if isLowRiskQuestion(lower) {
		return 0
	}

	score := 0
	if utf8.RuneCountInString(text) >= 160 {
		score++
	}
	if numberedListRE.MatchString(text) {
		score++
	}
	if strings.Count(text, "\n") >= 2 {
		score++
	}
	if containsAny(lower, complexIntentTerms) {
		score++
	}
	if containsAny(lower, multiSurfaceTerms) {
		score++
	}
	if containsAny(lower, docsAndIssueTerms) {
		score++
	}
	if strings.Count(text, "@") >= 2 || strings.Count(lower, ".go")+
		strings.Count(lower, ".ts")+strings.Count(lower, ".tsx")+strings.Count(lower, ".js") >= 2 {
		score++
	}
	return score
}

func isLowRiskQuestion(lower string) bool {
	lower = strings.TrimSpace(lower)
	if strings.HasPrefix(lower, "解释") || strings.HasPrefix(lower, "说明") ||
		strings.HasPrefix(lower, "怎么看") || strings.HasPrefix(lower, "查一下") ||
		strings.HasPrefix(lower, "运行") || strings.HasPrefix(lower, "run ") ||
		strings.HasPrefix(lower, "show ") || strings.HasPrefix(lower, "what ") ||
		strings.HasPrefix(lower, "why ") || strings.HasPrefix(lower, "how ") {
		return !containsAny(lower, complexIntentTerms)
	}
	return false
}

func containsAny(s string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}

var complexIntentTerms = []string{
	"implement", "add support", "refactor", "migrate", "redesign", "end-to-end",
	"e2e", "wire up", "integration", "fix the issue", "build a",
	"实现", "新增", "支持", "重构", "迁移", "改造", "端到端", "联调", "接入",
	"修复这个问题", "修一下这个问题", "补齐", "设计",
}

var multiSurfaceTerms = []string{
	"multiple files", "several files", "across", "frontend", "backend", "config",
	"tests", "docs", "ui", "api", "database", "schema",
	"多个文件", "多处", "前端", "后端", "配置", "测试", "文档", "接口", "数据库",
}

var docsAndIssueTerms = []string{
	"prd", "issue", "requirements", "spec", "proposal", "roadmap",
	"需求", "产品文档", "接口文档", "方案", "规划",
}

// isPlanMode reports whether the controller is currently in plan mode.
func (c *Controller) isPlanMode() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.planMode
}

// maybeClarifyVagueInput checks if the user input is too vague for plan mode
// and emits a clarifying question. Returns true if a question was emitted.
// V8.0 P1-4: plan mode smart clarification.
func (c *Controller) maybeClarifyVagueInput(input string) bool {
	if len(input) >= 30 {
		return false
	}
	goalVerbs := []string{"implement", "fix", "refactor", "add", "create",
		"write", "build", "deploy", "optimize", "remove", "delete",
		"实现", "修复", "重构", "创建", "添加", "删除", "优化", "构建"}
	lower := strings.ToLower(input)
	for _, v := range goalVerbs {
		if strings.Contains(lower, v) {
			return false
		}
	}
	c.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
		Text: "[Clarify] Your request is brief. What exactly do you want to achieve? " +
			"You can use read_file/ls/grep to explore — currently in read-only mode."})
	return true
}

// maybeAutoMode classifies the user input and sets the agent mode automatically
// when no mode has been manually selected via /mode. Classification is heuristic
// and deterministic — same input always yields the same mode, preserving cache
// stability. Only fires on the first unclassified turn; subsequent turns reuse
// the locked-in mode. Manual /mode always takes precedence.
// V8.19: auto-mode classification.
func (c *Controller) maybeAutoMode(input string) {
	c.mu.Lock()
	mode := c.agentMode
	c.mu.Unlock()
	if mode != "" {
		return // manually set via /mode or already classified
	}
	classified := agent.ClassifyMode(input)
	c.SetAgentMode(classified)
	if classified != "develop" {
		c.notice("auto mode: " + classified + " (classified from input)")
	}
}
