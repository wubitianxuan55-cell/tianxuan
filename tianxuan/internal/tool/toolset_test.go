package tool

import "testing"

// TestCodingToolsetComplete: coding 工具集必须包含全部核心编码工具——
// 新工具（edit_lines / verify_gate / notebook_edit / git_worktree /
// search_large_output）加入后若漏配工具集，配置 toolsets=[coding] 的
// 用户会静默丢失这些能力。
func TestCodingToolsetComplete(t *testing.T) {
	got := map[string]bool{}
	for _, n := range ResolveToolset("coding", nil, nil) {
		got[n] = true
	}
	for _, want := range []string{
		"edit_lines", "verify_gate", "notebook_edit",
		"git_worktree", "search_large_output",
	} {
		if !got[want] {
			t.Errorf("coding toolset missing %q (full set: %v)", want, keys(got))
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
