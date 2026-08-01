package builtin

import (
	"strings"
	"testing"

	"tianxuan/internal/tool"
)

// TestCompactDescriptorsWired: 所有内置工具要么实现 CompactDescriptor
// （每轮省 ~75% token），要么小到无需精简。code_index/move_file 的
// compactDesc/compactSchema 条目已存在但接口未接线——锁定接线。
func TestCompactDescriptorsWired(t *testing.T) {
	for _, name := range []string{"code_index", "move_file"} {
		tl, ok := tool.LookupBuiltin(name)
		if !ok {
			t.Fatalf("builtin %q not found", name)
		}
		cd, ok := tl.(tool.CompactDescriptor)
		if !ok {
			t.Errorf("%s must implement CompactDescriptor", name)
			continue
		}
		if strings.Contains(cd.CompactDescription(), "\n") {
			t.Errorf("%s compact description must be a single line", name)
		}
		if len(cd.CompactSchema()) == 0 {
			t.Errorf("%s compact schema must not be empty", name)
		}
	}
}

// TestVerifyGateKindConsistent: verify_gate 声明 ReadOnly=true（可并行、
// 无权限拦截），Kind 却标 execute（IsMutator=true）——矛盾分类。
// 统一为 KindRead，与只读验证门控的语义一致。
func TestVerifyGateKindConsistent(t *testing.T) {
	tl, ok := tool.LookupBuiltin("verify_gate")
	if !ok {
		t.Fatal("verify_gate not found")
	}
	if !tl.ReadOnly() {
		t.Fatal("verify_gate should stay ReadOnly=true (business decision)")
	}
	if got := tool.ToolKindOf(tl); got != tool.KindRead {
		t.Errorf("verify_gate kind = %s, want read (consistent with ReadOnly)", got)
	}
}

// TestGitCommitDescriptionNoVersionMarker: 工具描述是给模型的，不应携带
// 内部版本标记（V10.6:）——只保留行为警告。
func TestGitCommitDescriptionNoVersionMarker(t *testing.T) {
	tl, _ := tool.LookupBuiltin("git_commit")
	if strings.Contains(tl.Description(), "V10.6:") {
		t.Errorf("git_commit description should not carry internal version markers: %s", tl.Description())
	}
	if !strings.Contains(tl.Description(), "main") {
		t.Errorf("git_commit description should still warn about main/master commits")
	}
}

// TestReadSkillDescriptionGuide: read_skill 描述太短（64 字节），缺少
// 何时使用的引导——模型不知道它与 run_skill 的关系。
func TestReadSkillDescriptionGuide(t *testing.T) {
	tl, _ := tool.LookupBuiltin("read_skill")
	d := tl.Description()
	if !strings.Contains(d, "run_skill") {
		t.Errorf("read_skill description should mention run_skill relationship: %q", d)
	}
}
