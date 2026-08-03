package skill

import "strings"

// maxAutoSkillBodyChars caps how much of a skill body is auto-injected.
// Every injected rune lands in the current turn's new messages and is billed
// as a cache-miss token, so oversized playbooks are truncated (deterministically)
// to keep the per-turn cost bounded.
const maxAutoSkillBodyChars = 2000

// AutoTriggerRule 定义一条技能自动触发规则：输入包含任一关键词即命中。
type AutoTriggerRule struct {
	SkillName string
	Keywords  []string
}

// autoTriggerRules 内置自动触发规则，按优先级排序（先命中先得）。
// 只覆盖核心编程工作流（inline 技能）；subagent 技能（explore/review 等）
// 已暴露为专用工具，不需要自动注入正文；设计类技能不自动触发，
// 避免误命中与正文过大。
var autoTriggerRules = []AutoTriggerRule{
	// V10.140: 扩展常见编程措辞——"修复缓存问题"/"这个功能不工作"/"构建失败"
	// 等最普遍的任务描述此前完全不触发（只有"修 bug/调试/排查"等窄词）。
	{SkillName: "systematic-debugging", Keywords: []string{
		"崩溃", "报错", "测试失败", "修 bug", "修bug", "调试", "排查", "定位问题", "异常",
		"修复", "bug", "故障", "出错", "不工作", "失败", "修一下",
	}},
	{SkillName: "tdd", Keywords: []string{
		"tdd", "测试驱动", "先写测试", "红绿", "写测试用例", "补测试", "测试用例", "写个测试",
	}},
	{SkillName: "requesting-code-review", Keywords: []string{"code review", "代码审查", "审查一下", "review 一下", "合并前审查"}},
	{SkillName: "receiving-code-review", Keywords: []string{"审查意见", "审查反馈", "review 反馈", "review 意见", "按反馈修改"}},
	{SkillName: "finish-development-branch", Keywords: []string{"收尾", "合并分支", "清理 worktree", "开发分支收尾"}},
	{SkillName: "release", Keywords: []string{"发布", "打版", "打包发布", "出个新版本", "release 新版本"}},
}

// MatchSkill 返回输入匹配的第一个技能名；无匹配返回空串。
// 确定性纯函数：同输入 → 同结果（DeepSeek 前缀缓存安全前提）。
func MatchSkill(input string) string {
	return matchSkill(input, autoTriggerRules)
}

func matchSkill(input string, rules []AutoTriggerRule) string {
	lower := strings.ToLower(input)
	for _, rule := range rules {
		for _, kw := range rule.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return rule.SkillName
			}
		}
	}
	return ""
}

// InjectAutoSkill 在匹配技能时把技能正文确定性注入到输入前部，包装为
// <auto-skill> 块（前端 StripTransientBlocks 可剥离）。
// 缓存安全：只修改 user 消息字节；不触碰 L1/L2/tools。同输入+同技能 → 同字节。
func InjectAutoSkill(input string, store *Store) string {
	return injectAutoSkill(input, store, autoTriggerRules)
}

func injectAutoSkill(input string, store *Store, rules []AutoTriggerRule) string {
	if store == nil || input == "" {
		return input
	}
	name := matchSkill(input, rules)
	if name == "" {
		return input
	}
	sk, ok := store.Read(name)
	if !ok {
		return input
	}
	// 只自动注入 inline 技能；subagent 技能已工具化，注入正文反而冗余。
	if sk.RunAs != RunInline {
		return input
	}
	body := sk.Body
	if r := []rune(body); len(r) > maxAutoSkillBodyChars {
		body = string(r[:maxAutoSkillBodyChars]) + "\n…(正文过长已截断，如需全文请 run_skill)"
	}
	var b strings.Builder
	b.WriteString("<auto-skill>\n")
	b.WriteString("[自动加载技能] 你的输入匹配技能 " + name + "，执行前请通读并遵守其 playbook：\n\n")
	b.WriteString(body)
	b.WriteString("\n</auto-skill>\n\n")
	b.WriteString(input)
	return b.String()
}
