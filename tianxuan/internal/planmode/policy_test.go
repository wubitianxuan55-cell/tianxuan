package planmode

import "testing"

// Marker 契约：计划模式指令必须指向真实存在的只读调查方式，不能指引
// 不存在的工具（V10.135 修正 read_only_task/read_only_skill 引用）。
func TestMarker_NamesRealReadOnlyTools(t *testing.T) {
	for _, kw := range []string{"read_file", "grep", "glob", "todo_write", "ask"} {
		if !containsAny(Marker, kw) {
			t.Errorf("Marker should name real read-only tool %q", kw)
		}
	}
	for _, ghost := range []string{"read_only_task", "read_only_skill"} {
		if containsAny(Marker, ghost) {
			t.Errorf("Marker must not reference non-existent tool %q", ghost)
		}
	}
}

func containsAny(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
