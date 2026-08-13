package boot

import (
	"strings"

	"tianxuan/internal/cache"
	"tianxuan/internal/config"
	"tianxuan/internal/control"
	"tianxuan/internal/provider"
	"tianxuan/internal/skill"
	"tianxuan/internal/tool"
)

func forkContextOf(c *control.Controller) string {
	ex := c.Executor()
	if ex == nil {
		return ""
	}
	s := ex.Session()
	if s == nil {
		return ""
	}
	return forkContextText(s.Snapshot())
}

// forkContextText renders a compact snapshot of a conversation for
// inherit_context: the last user input and the turns that follow, truncated to
// a fixed budget. Pure so the extraction is unit-tested without a controller.
func forkContextText(msgs []provider.Message) string {
	start := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleUser {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	const budget = 4000
	// Reserve a floor for the turns after the user input so a very long user
	// message cannot starve the follow-up context (assistant/tool turns).
	const followBudget = 800
	var follows []string
	followUsed := 0
	for i := start + 1; i < len(msgs) && followUsed < followBudget; i++ {
		text := strings.TrimSpace(msgs[i].Content)
		if text == "" {
			continue
		}
		if len(text) > followBudget-followUsed {
			text = text[:followBudget-followUsed]
		}
		follows = append(follows, renderForkRole(msgs[i].Role)+": "+text)
		followUsed += len(text) + 3
	}
	userText := strings.TrimSpace(msgs[start].Content)
	if len(userText) > budget-followUsed {
		userText = userText[:budget-followUsed]
	}
	var b strings.Builder
	b.WriteString("user: " + userText + "\n")
	for _, f := range follows {
		b.WriteString(f + "\n")
	}
	return strings.TrimSpace(b.String())
}

// renderForkRole maps a message role to its label in the fork snapshot.
func renderForkRole(r provider.Role) string {
	switch r {
	case provider.RoleAssistant:
		return "assistant"
	case provider.RoleTool:
		return "tool"
	default:
		return "user"
	}
}

func subagentModelRef(cfg *config.Config, sk skill.Skill) string {
	if cfg != nil {
		for _, key := range subagentModelKeys(sk.Name) {
			if m := strings.TrimSpace(cfg.Agent.SubagentModels[key]); m != "" {
				return m
			}
		}
	}
	if m := strings.TrimSpace(sk.Model); m != "" {
		return m
	}
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Agent.SubagentModel)
}

func subagentModelKeys(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	keys := []string{name}
	for _, alias := range []string{
		strings.ReplaceAll(name, "-", "_"),
		strings.ReplaceAll(name, "_", "-"),
	} {
		if alias == "" {
			continue
		}
		seen := false
		for _, key := range keys {
			if key == alias {
				seen = true
				break
			}
		}
		if !seen {
			keys = append(keys, alias)
		}
	}
	return keys
}

// taskCompilerAdapter wraps *cache.Compiler to satisfy agent.TaskCompiler,
// bridging the return-type mismatch between Fork() *cache.Compiler (concrete)
// and Fork() interface{SystemPrompt() string} (interface).
type taskCompilerAdapter struct {
	c *cache.Compiler
}

func (a *taskCompilerAdapter) Fork() interface{ SystemPrompt() string } { return a.c.Fork() }
func (a *taskCompilerAdapter) SystemPrompt() string                     { return a.c.SystemPrompt() }

func orDefault(val, def string) string {
	if val != "" {
		return val
	}
	return def
}

func subagentSkillToTemplateKind(skillName string) cache.TaskKind {
	switch skillName {
	case "explore":
		return "subagent_explore"
	case "research":
		return "subagent_research"
	case "review", "code-review":
		return "subagent_review"
	case "security-review", "security_review":
		return "subagent_security"
	default:
		return ""
	}
}

// lookupSubagentTemplatePrefix 根据技能名称查找子代理模板前缀（V5.30）。
// 同类子代理共享相同模板前缀 → DeepSeek 前缀缓存命中。
func lookupSubagentTemplatePrefix(skillName string) string {
	kind := subagentSkillToTemplateKind(skillName)
	if kind == "" {
		return ""
	}
	tmpl, ok := cache.LookupSpawnTemplate(kind)
	if !ok || tmpl.Prefix == "" {
		return ""
	}
	return tmpl.Prefix
}

// subagentSkillToolDescs holds the model-facing descriptions for the built-in
// subagent skill tools (ordered). They are first-class tools so DeepSeek's
// native tool calling sees a dedicated entry instead of routing through
// run_skill. Keys match the built-in skill identifiers (security-review uses
// the hyphen form; the model-visible tool name is normalised to underscores).
var subagentSkillToolDescs = []struct {
	name        string
	desc        string
	compactDesc string
}{
	{
		name:        "explore",
		desc:        "Explore the codebase in an isolated read-only sub-agent. 在隔离的只读子代理中深入调查代码库：架构、调用链、符号定义、影响面、跨文件综合问题；返回带 file:line 引用的精炼结论。适合宽泛调查、多处调用、理解系统结构——不要在主线上下文里反复 grep/read_file 堆砌探索。",
		compactDesc: "在隔离只读子代理中调查代码库（架构/调用链/影响面），返回带引用的精炼结论",
	},
	{
		name:        "research",
		desc:        "Research a question combining web + code in an isolated read-only sub-agent. 在隔离的只读子代理中结合网页资料与本地代码调研（web_search/web_fetch + 代码检索），返回带代码与 URL 引用的综合结论。适合“某库是否支持 X”、外部方案对比、版本兼容等调研。",
		compactDesc: "在隔离只读子代理中结合网页与代码调研问题，返回带引用的综合结论",
	},
	{
		name:        "review",
		desc:        "Review the current branch diff in an isolated read-only sub-agent. 在隔离的只读子代理中审查当前分支改动：正确性、安全、缺失测试、隐藏行为变化；按 file:line 给出问题与严重度，返回审查结论。只读，不提交不改文件。",
		compactDesc: "在隔离只读子代理中审查当前分支 diff，按严重度返回问题清单",
	},
	{
		name:        "security-review",
		desc:        "Security-focused review of the current branch diff in an isolated read-only sub-agent. 在隔离的只读子代理中做安全专项审查：注入、越权、密钥泄露、反序列化、路径穿越、加密误用；按严重度标注并给出 file:line。只读，不提交不改文件。",
		compactDesc: "在隔离只读子代理中做安全专项审查，按严重度返回风险清单",
	},
	{
		name:        "taste-skill",
		desc:        "Frontend aesthetic judgment in an isolated read-only sub-agent (distilled from Leonxlnx/taste-skill, MIT). 在隔离只读子代理中做前端审美判断与反模板验收：先读需求推断设计方向（Design Read），按 DESIGN_VARIANCE/MOTION_INTENSITY/VISUAL_DENSITY 三拨盘定视觉基调，映射真实设计系统，输出布局/排版/配色/动效指令与 AI Tells 反模式清单。适合 landing page、作品集、改版等界面设计的前置决策与评审；只读，返回建议，主代理实现。",
		compactDesc: "在隔离只读子代理中做前端审美判断（Design Read+三拨盘+设计系统映射+AI Tells 验收）",
	},
}

// subagentSkillToolDescsBySkill indexes the custom descriptions by skill name.
var subagentSkillToolDescsBySkill = func() map[string]struct{ desc, compactDesc string } {
	m := map[string]struct{ desc, compactDesc string }{}
	for _, d := range subagentSkillToolDescs {
		m[d.name] = struct{ desc, compactDesc string }{d.desc, d.compactDesc}
	}
	return m
}()

// applyCompactToolset hides redundant tools from the model's tool schema list
// while keeping them callable by name. V6.0 P8: reduces visible tool count
// from ~41 to ~25, lowering model cognitive load.
func applyCompactToolset(reg *tool.Registry) {
	// File deletion: merge delete_range + delete_symbol into edit_file
	// edit_file already supports delete via mode parameter
	reg.HideUnlessOnly([]string{"delete_range", "delete_symbol"}, []string{"edit_file"})

	// Background job management: merge kill_shell + wait into bash/bgjobs
	reg.HideUnlessOnly([]string{"kill_shell", "wait"}, []string{"bash", "bash_output"})

	// Notebook editing: rarely used, hide unless explicitly enabled
	reg.HideUnlessOnly([]string{"notebook_edit"}, []string{"edit_file", "write_file"})

	// CodeGraph deep-analysis tools: the parent only needs the high-signal
	// query tools; trace/explore/impact/callers/callees/status stay available
	// inside explore sub-agents (FilterRegistry copies tools without hiding
	// markers), so hiding them here only trims the parent's schema tokens.
	reg.HideUnlessOnly([]string{
		"mcp__codegraph__codegraph_trace",
		"mcp__codegraph__codegraph_explore",
		"mcp__codegraph__codegraph_impact",
		"mcp__codegraph__codegraph_callers",
		"mcp__codegraph__codegraph_callees",
		"mcp__codegraph__codegraph_status",
	}, []string{
		"mcp__codegraph__codegraph_context",
		"mcp__codegraph__codegraph_search",
		"mcp__codegraph__codegraph_node",
	})

	// Skill authoring is a user/CLI action, not a per-turn model capability;
	// run_skill covers invocation.
	reg.HideUnlessOnly([]string{"install_skill"}, []string{"run_skill"})

	// LSP rename/completion are lower-frequency mutations; the read-side
	// tools (definition/references/hover/diagnostics) cover investigation.
	reg.HideUnlessOnly([]string{"lsp_rename", "lsp_completion"}, []string{"lsp_definition", "lsp_references", "lsp_diagnostics"})

	// File listing: glob is redundant with ls (which supports patterns)
	reg.HideUnlessOnly([]string{"glob"}, []string{"ls"})
}
