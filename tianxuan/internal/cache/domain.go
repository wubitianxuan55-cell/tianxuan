package cache

import "strings"

// TaskKind classifies the type of task for tool filtering and context injection.
// Originally in router.go (V3.0); kept for skill/spawn/runtime compatibility.
type TaskKind string

const (
	KindFixBug      TaskKind = "fix_bug"
	KindWriteFeature TaskKind = "write_feature"
	KindReview      TaskKind = "review"
	KindExplain     TaskKind = "explain"
	KindResearch    TaskKind = "research"
	KindDefault     TaskKind = "default"
	// V4.0: non-code task kinds
	KindDataAnalysis TaskKind = "data_analysis"
	KindWriting      TaskKind = "writing"
	KindGeneral      TaskKind = "general"
)

// DomainConfig holds the routing result: classified kind and tool filter set.
// V6.0: simplified from original router.go DomainConfig.
type DomainConfig struct {
	Kind         TaskKind
	SkillFilter  []string
	ContextFocus []string
}

// Tool lists for skill layer classification (originally in router.go).
var readTools = []string{
	"read_file", "grep", "glob", "ls",
	"web_search", "web_fetch", "preview",
}

var editTools = []string{
	"edit_file", "multi_edit", "write_file",
	"delete_range", "delete_symbol", "notebook_edit",
}

var shellTools = []string{
	"bash", "bash_output", "kill_shell", "wait",
}

var metaTools = []string{
	"todo_write", "complete_step",
	"remember", "forget", "memory",
}

var subagentTools = []string{
	"explore", "research", "review", "security_review",
	"run_skill", "install_skill", "task",
}

// merge concatenates multiple string slices.
func merge(slices ...[]string) []string {
	var out []string
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}

// mergeUnique appends unique elements from src to dst.
func mergeUnique(dst, src []string) []string {
	seen := make(map[string]bool, len(dst))
	for _, t := range dst {
		seen[t] = true
	}
	for _, t := range src {
		if !seen[t] {
			dst = append(dst, t)
			seen[t] = true
		}
	}
	return dst
}

// matchAnyWord reports whether s contains any keyword as a whole word.
func matchAnyWord(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if containsWord(s, kw) {
			return true
		}
	}
	return false
}

// containsWord checks whether needle appears as a word-boundary delimited
// substring in haystack (substring match like "api" inside "rapid" is ignored).
func containsWord(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	needleLower := strings.ToLower(needle)
	haystackLower := strings.ToLower(haystack)
	for i := 0; i <= len(haystackLower)-len(needleLower); i++ {
		if haystackLower[i:i+len(needleLower)] == needleLower {
			prev := byte(' ')
			if i > 0 {
				prev = haystackLower[i-1]
			}
			next := byte(' ')
			if i+len(needleLower) < len(haystackLower) {
				next = haystackLower[i+len(needleLower)]
			}
			if !isAlphaNum(prev) && !isAlphaNum(next) {
				return true
			}
		}
	}
	return false
}

func isAlphaNum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}
