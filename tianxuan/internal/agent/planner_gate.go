package agent

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// ── Feature extraction ────────────────────────────────────────────────

type planFeat struct {
	work         bool
	highRisk     bool
	multiFile    bool
	crossSurface bool
	structured   bool
	complex      bool
	atomic       bool
	readOnly     bool
	guidance     bool
	anchored     bool
	ambiguous    bool
}

var planFileRefRE = regexp.MustCompile(`(?i)(?:^|[\s@\x60"'(])(?:[\w.-]+[/\\])*[\w.-]+\.(?:go|ts|tsx|js|jsx|py|rs|java|kt|md|json|ya?ml|toml|sql|sh|css|html)(?:$|[\s,;:!?，；：！？\x60"'])`)
var planListRE = regexp.MustCompile(`(?m)^\s*(?:[-*]|\d+[.)、])\s+\S`)

func planFeatures(text, lower string) planFeat {
	fileRefs := planFileRefRE.FindAllString(text, -1)
	anchored := len(fileRefs) > 0 || strings.Contains(text, "@") || containsAny(lower, planNamedTargets)
	work := containsAny(lower, planWorkTerms)
	highRisk := containsAny(lower, planHighRiskTerms)
	multiFile := len(fileRefs) >= 2 || strings.Count(text, "@") >= 2
	crossSurface := containsAny(lower, planCrossSurfaceTerms)
	structured := utf8.RuneCountInString(text) >= 240 || planListRE.MatchString(text) || strings.Count(text, "\n") >= 2
	complex := containsAny(lower, planComplexTerms)
	guidance := isComplexGuidance(lower)
	ambiguous := work && containsAny(lower, planAmbiguousTerms)
	readOnly := work && containsAny(lower, planReadOnlyTerms) && !containsAny(lower, planMutationTerms)
	atomic := work && anchored && !highRisk && !multiFile && !crossSurface && !structured && !complex &&
		utf8.RuneCountInString(text) <= 140 && containsAny(lower, planAtomicTerms)
	return planFeat{
		work: work, highRisk: highRisk, multiFile: multiFile,
		crossSurface: crossSurface, structured: structured, complex: complex,
		atomic: atomic, readOnly: readOnly, guidance: guidance,
		anchored: anchored, ambiguous: ambiguous,
	}
}

func containsAny(lower string, terms []string) bool {
	for _, t := range terms {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

// ── Turn classifiers ──────────────────────────────────────────────────

func isShortReply(lower string) bool {
	if utf8.RuneCountInString(lower) > 16 {
		return false
	}
	_, ok := shortReplies[lower]
	return ok
}

var shortReplies = map[string]bool{
	"ok": true, "okay": true, "yes": true, "y": true, "no": true, "n": true,
	"sure": true, "go ahead": true, "proceed": true, "continue": true, "next": true,
	"sounds good": true, "好": true, "好的": true, "可以": true, "行": true, "嗯": true,
	"对": true, "是的": true, "没问题": true, "搞": true, "干": true, "来吧": true,
}

func isConversational(lower string) bool {
	for _, p := range []string{"thanks", "thank you", "got it", "明白了", "谢谢", "了解", "收到"} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func isLowRiskQuestion(lower string) bool {
	for _, p := range []string{"what is", "what does", "how does", "what are", "how is",
		"where is", "when did", "who is", "which file", "what's", "how's",
		"什么是", "怎么用", "在哪里", "哪个", "如何查看"} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func isComplexGuidance(lower string) bool {
	for _, p := range []string{"best practice", "best way", "how should i", "how to structure",
		"recommend", "suggest", "最佳实践", "推荐", "怎么做比较好", "该怎么设计"} {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// ── Word lists (Chinese + English) ───────────────────────────────────

var planWorkTerms = []string{
	"fix", "fixing", "update", "updating", "remove", "removing", "delete", "deleting",
	"edit", "editing", "write", "writing", "create", "creating", "add", "adding", "repair",
	"patch", "run", "running", "build", "building", "implement", "implementing", "refactor",
	"refactoring", "migrate", "migrating", "review", "reviewing", "audit",
	"inspect", "debug", "test", "tests", "testing", "optimize", "optimise",
	"修改", "修复", "更新", "删除", "移除", "编辑", "写入", "创建", "新增", "添加",
	"运行", "构建", "实现", "重构", "迁移", "改造", "评审", "审查", "排查", "调试", "测试",
	"优化", "加个", "加一", "补一个", "补个", "联调", "接入", "对接", "配置",
}

var planMutationTerms = []string{
	"fix", "fixing", "update", "updating", "remove", "removing", "delete", "deleting",
	"edit", "editing", "write", "writing", "create", "creating", "add", "adding", "repair",
	"patch", "build", "building", "implement", "implementing", "refactor", "refactoring",
	"migrate", "migrating",
	"修改", "修复", "更新", "删除", "移除", "编辑", "写入", "创建", "新增", "添加",
	"构建", "实现", "重构", "迁移", "改造", "加个", "加一", "补一个", "补个",
}

var planReadOnlyTerms = []string{
	"run", "running", "review", "reviewing", "audit", "inspect", "debug",
	"test", "tests", "testing",
	"运行", "评审", "审查", "排查", "调试", "测试",
}

var planHighRiskTerms = []string{
	"auth", "authentication", "authorization", "permission", "token", "secret",
	"credential", "payment", "billing", "race", "concurrency", "deadlock", "transaction",
	"encryption", "signature", "sandbox", "privilege",
	"权限", "鉴权", "认证", "令牌", "密钥", "支付", "账单", "并发", "竞态", "死锁", "事务", "加密", "签名", "沙箱", "提权",
}

var planCrossSurfaceTerms = []string{
	"multiple files", "several files", "across", "frontend and backend", "backend and frontend",
	"api and ui", "ui and api", "database and api",
	"多个文件", "多处", "前后端", "整个模块", "整个项目", "全链路", "跨模块",
}

var planComplexTerms = []string{
	"refactor", "migrate", "migration", "redesign", "end-to-end", "e2e", "wire up",
	"integration", "architecture", "release", "package",
	"重构", "迁移", "改造", "端到端", "联调", "接入", "架构", "发布", "打包",
}

var planAtomicTerms = []string{
	"typo", "wording", "copy", "readme", "changelog", "nil check", "null check",
	"log line", "one line", "rename",
	"文案", "错别字", "拼写", "空指针检查", "一行日志", "改名", "重命名",
}

var planNamedTargets = []string{"readme", "changelog", "makefile", "dockerfile"}

var planAmbiguousTerms = []string{
	"the bug", "the issue", "the problem", "performance", "everything", "whole module",
	"这个 bug", "这个bug", "这个问题", "性能", "整个模块", "全部问题",
}
