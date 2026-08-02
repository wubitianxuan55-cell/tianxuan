package builtin

import (
	"encoding/json"
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

// TestCompactSchemaKeepsConstraints 系统性防线（V10.148）：V10.146 只修了
// edit_lines 的 minimum:1，但审计发现同根因还有 18 处、13 个工具的
// enum/minimum 约束在 compact schema 中系统性丢失——手写压缩没有自动化
// 校验。此测试对每个 CompactDescriptor 工具递归对比完整 Schema 与
// CompactSchema，断言 required/enum/minimum（含嵌套 items[].prop）全部保留，
// 防止未来任何工具新增约束时再次丢失。
func TestCompactSchemaKeepsConstraints(t *testing.T) {
	for _, tl := range tool.Builtins() {
		cd, ok := tl.(tool.CompactDescriptor)
		if !ok {
			continue
		}
		name := tl.Name()
		full := analyzeSchemaForTest(tl.Schema())
		compact := analyzeSchemaForTest(cd.CompactSchema())

		for _, r := range full.required {
			if !stringInSlice(compact.required, r) {
				t.Errorf("%s: compact schema 丢失 required %q（完整 schema 有）", name, r)
			}
		}
		for key, en := range full.enums {
			ce, ok := compact.enums[key]
			if !ok {
				t.Errorf("%s: compact schema 完全缺失属性 %q（完整有 enum %v）", name, key, en)
				continue
			}
			if !sameEnum(ce, en) {
				t.Errorf("%s: 属性 %q enum 不一致：full=%v compact=%v", name, key, en, ce)
			}
		}
		for key, m := range full.mins {
			cm, ok := compact.mins[key]
			if !ok {
				t.Errorf("%s: compact schema 完全缺失属性 %q（完整有 minimum %d）", name, key, m)
				continue
			}
			if cm != m {
				t.Errorf("%s: 属性 %q minimum 不一致：full=%d compact=%d", name, key, m, cm)
			}
		}
	}
}

type schemaAudit struct {
	required []string
	enums    map[string][]json.RawMessage
	mins     map[string]int64
}

// analyzeSchemaForTest 将 schema 拍平为路径键：顶层 "kind"、数组项
// "evidence[].kind"、嵌套对象 "evidence.kind"。enum 用 JSON 值比较，
// 整数 enum（如 todos[].level 的 [0,1]）也能正确对比。
func analyzeSchemaForTest(raw json.RawMessage) schemaAudit {
	audit := schemaAudit{enums: map[string][]json.RawMessage{}, mins: map[string]int64{}}
	var walk func(raw json.RawMessage, prefix string, isRoot bool)
	walk = func(raw json.RawMessage, prefix string, isRoot bool) {
		var s struct {
			Required   []string                   `json:"required"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			return
		}
		if isRoot {
			audit.required = s.Required
		}
		for name, p := range s.Properties {
			key := name
			if prefix != "" {
				key = prefix + "." + name
			}
			var ps struct {
				Enum       []json.RawMessage `json:"enum"`
				Minimum    *int64            `json:"minimum"`
				Type       string            `json:"type"`
				Items      *json.RawMessage  `json:"items"`
				Properties json.RawMessage   `json:"properties"`
			}
			if err := json.Unmarshal(p, &ps); err != nil {
				continue
			}
			if len(ps.Enum) > 0 {
				audit.enums[key] = ps.Enum
			}
			if ps.Minimum != nil {
				audit.mins[key] = *ps.Minimum
			}
			if ps.Type == "array" && ps.Items != nil {
				walk(*ps.Items, key+"[]", false)
			} else if ps.Type == "object" && len(ps.Properties) > 0 {
				walk(p, key, false)
			}
		}
	}
	walk(raw, "", true)
	return audit
}

func stringInSlice(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func sameEnum(a, b []json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for _, av := range a {
		found := false
		for _, bv := range b {
			if string(av) == string(bv) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
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
