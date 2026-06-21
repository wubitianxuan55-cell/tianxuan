package skill

import (
	"fmt"
	"strings"
)

// IndexMaxChars caps the pinned skills-index block so it can't bloat the
// cache-stable system-prompt prefix; bodies never enter the prefix.
const IndexMaxChars = 4000

const missingDescPlaceholder = `(no description — frontmatter is missing a "description:" line; tell the user to add one)`

// indexHeader introduces the skills block in the system prompt: how to invoke a
// skill, and the inline-vs-subagent distinction. V8.22: changed from permissive
// "you can invoke" to prescriptive "you MUST consult before acting" to push the
// model toward skill usage (Reasonix pattern).
const indexHeader = "# Skills — playbooks you MUST consult before acting\n\n" +
	"技能是领域知识或工作流编排——使用技能比从零开始更高效、更正确。" +
	"在开始任何工作之前，先检查是否有匹配的技能。" +
	"各技能的 description 说明了何时必须使用它" +
	"（如 \"在任何创造性工作之前必须使用此技能\"）。\n\n" +
	"每个条目是内置或用户编写的 playbook。" +
	"调用方式: `run_skill({ name: \"<skill-name>\", arguments: \"<task>\" })` " +
	"— `name` 只需标识符（如 `\"explore\"`），不要含 `[🧬 subagent]` 标签。" +
	"标记 `[🧬 subagent]` 的技能启动隔离子代理——其工具调用和推理不会进入你的上下文，" +
	"仅返回最终结论；用于上下文繁重的工作（深度探索、多步研究），你只需结论。" +
	"未标记的技能为 inline：正文成为工具结果供你直接阅读执行。" +
	"用户也可通过 `/<name>` 调用技能。"

// ApplyIndex appends the skills index to basePrompt, or returns it unchanged
// when there are no skills. Only names + descriptions (+ a subagent tag) are
// listed; bodies load on demand via run_skill.
func ApplyIndex(basePrompt string, skills []Skill) string {
	if len(skills) == 0 {
		return basePrompt
	}
	lines := make([]string, 0, len(skills))
	for _, sk := range skills {
		lines = append(lines, indexLine(sk))
	}
	joined := strings.Join(lines, "\n")
	if r := []rune(joined); len(r) > IndexMaxChars {
		joined = string(r[:IndexMaxChars]) + fmt.Sprintf("\n… (truncated %d chars)", len(r)-IndexMaxChars)
	}
	return basePrompt + "\n\n" + indexHeader + "\n\n```\n" + joined + "\n```"
}

// indexLine renders one skill as "- name [tag] — description", clipped to a
// stable width. The subagent tag goes after the name so a model copying the line
// into run_skill's `name` arg still yields a clean identifier.
func indexLine(sk Skill) string {
	desc := strings.TrimSpace(strings.ReplaceAll(sk.Description, "\n", " "))
	if desc == "" {
		desc = missingDescPlaceholder
	}
	tag := ""
	if sk.RunAs == RunSubagent {
		tag = " [🧬 subagent]"
	} else if sk.RunAs == RunPipeline {
		tag = " [🔗 pipeline]"
	}
	max := 130 - len([]rune(sk.Name)) - len([]rune(tag))
	clipped := clipRunes(desc, max)
	if clipped == "" {
		return "- " + sk.Name + tag
	}
	return "- " + sk.Name + tag + " — " + clipped
}

// clipRunes truncates s to at most max runes (ellipsis included), never
// splitting a multi-byte rune.
func clipRunes(s string, max int) string {
	if max < 1 {
		max = 1
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max-1 < 1 {
		return string(r[:1])
	}
	return string(r[:max-1]) + "…"
}
