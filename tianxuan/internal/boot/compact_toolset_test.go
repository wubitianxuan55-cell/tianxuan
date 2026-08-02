package boot

import (
	"testing"

	"tianxuan/internal/tool"
	_ "tianxuan/internal/tool/builtin"
)

// TestApplyCompactToolsetHidesRedundant locks the reduced-toolset behavior:
// applyCompactToolset must remove redundant tools from the model-visible schema
// list (glob behind ls, delete_range behind edit_file, ...) while keeping them
// callable. The hidden set is decided once at boot — never mid-session — so the
// per-request tools list stays byte-stable (prefix-cache safe).
func TestApplyCompactToolsetHidesRedundant(t *testing.T) {
	reg := tool.NewRegistry()
	for _, tl := range tool.Builtins() {
		reg.Add(tl)
	}
	applyCompactToolset(reg)

	visible := map[string]bool{}
	for _, s := range reg.Schemas() {
		visible[s.Name] = true
	}
	for _, hidden := range []string{"glob", "delete_range", "delete_symbol", "kill_shell", "wait"} {
		if visible[hidden] {
			t.Errorf("compact toolset must hide %q from the model-visible schema", hidden)
		}
		if _, ok := reg.Get(hidden); !ok {
			t.Errorf("hidden tool %q must stay callable (only schema-hidden)", hidden)
		}
	}
	for _, keep := range []string{"ls", "edit_file", "multi_edit", "bash", "bash_output"} {
		if !visible[keep] {
			t.Errorf("compact toolset must keep %q visible", keep)
		}
	}
}
