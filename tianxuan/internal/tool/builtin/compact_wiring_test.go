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

// TestEditLinesCompactSchemaMinimum 回归：模型看到的是 CompactSchema（每轮省
// ~75% token），它必须保留 start_line/end_line 的 minimum:1 约束——否则模型不
// 认为行号必须 >=1，会漏传或传 0（真实事故：模型调用 edit_lines 只传
// end_line/new_content，start_line 反序列化为 0 → "start_line must be >= 1"）。
// 完整 Schema 有 minimum，压缩版丢失正是根因。
func TestEditLinesCompactSchemaMinimum(t *testing.T) {
	tl, ok := tool.LookupBuiltin("edit_lines")
	if !ok {
		t.Fatal("edit_lines not found")
	}
	cd, ok := tl.(tool.CompactDescriptor)
	if !ok {
		t.Fatal("edit_lines must implement CompactDescriptor")
	}
	schema := string(cd.CompactSchema())
	for _, want := range []string{`"start_line":{"type":"integer","minimum":1}`, `"end_line":{"type":"integer","minimum":1}`} {
		if !strings.Contains(schema, want) {
			t.Errorf("compact schema missing %s (full schema has it; model sees compact → 漏传/传 0 根因)\nfull: %s", want, schema)
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
