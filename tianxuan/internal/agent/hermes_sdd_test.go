package agent

import (
	"fmt"
	"strings"
	"testing"
)

// ── SDD 蒸馏测试 — OpenSpec 方法论映射到 Hermes ──────────────────────────
//
// 这些测试验证 Hermes 的规划者是否遵循 Spec-Driven Development (SDD)
// 方法论。每个测试对应 OpenSpec 的一个核心概念：
//
//   Delta Specs  — parseStepDeltas (ADDED/MODIFIED/REMOVED)
//   Proposal     — hasProposal (why+what 在 plan 之前)
//   Coherence    — checkCoherence (执行结果 vs 计划)
//   Specs First  — HermesPrompt 关键词验证
//   Verify 三维  — formatExecutionFeedback 完整性检查

// ── 1. Delta Specs：步骤变更类型标记 ────────────────────────────────────

func TestParseStepDeltas_AllThreeTypes(t *testing.T) {
	plan := `<!--plan-->

步骤 1：新增日志模块
- **Delta**：ADDED
- **File(s)**：[NEW] internal/log/logger.go
- **Change**：创建新的日志接口
- **Depends on**：无
- **Verify**：go build ./internal/log/

步骤 2：修改入口函数
- **Delta**：MODIFIED
- **File(s)**：cmd/main.go
- **Change**：注入日志模块
- **Depends on**：1
- **Verify**：go run . 2>&1 | grep "logger"

步骤 3：删除旧日志代码
- **Delta**：REMOVED
- **File(s)**：internal/oldlog/legacy.go
- **Change**：删除废弃的日志实现
- **Depends on**：2
- **Verify**：go build ./...`

	deltas := parseStepDeltas(plan)
	if len(deltas) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(deltas))
	}
	if deltas["新增日志模块"] != "ADDED" {
		t.Errorf("步骤1 expected ADDED, got %q", deltas["新增日志模块"])
	}
	if deltas["修改入口函数"] != "MODIFIED" {
		t.Errorf("步骤2 expected MODIFIED, got %q", deltas["修改入口函数"])
	}
	if deltas["删除旧日志代码"] != "REMOVED" {
		t.Errorf("步骤3 expected REMOVED, got %q", deltas["删除旧日志代码"])
	}
}

func TestParseStepDeltas_NoDeltaMarker(t *testing.T) {
	// V10.88 格式：无 Delta 标记的旧 plan 应返回空 map
	plan := `<!--plan-->

步骤 1：Fix typo
- **File(s)**：foo.go
- **Change**：fix
- **Depends on**：无
- **Verify**：test`

	deltas := parseStepDeltas(plan)
	if len(deltas) != 0 {
		t.Fatalf("old format without Delta should return empty, got %d entries", len(deltas))
	}
}

func TestParseStepDeltas_NoPlan(t *testing.T) {
	deltas := parseStepDeltas("这是纯文本回复，无计划")
	if len(deltas) != 0 {
		t.Fatalf("no plan should return empty, got %d entries", len(deltas))
	}
}

func TestParseStepDeltas_PartialDelta(t *testing.T) {
	// 混合：部分步骤有 Delta，部分没有
	plan := `<!--plan-->

步骤 1：Add feature
- **Delta**：ADDED
- **File(s)**：[NEW] a.go
- **Change**：new file
- **Depends on**：无
- **Verify**：test

步骤 2：Fix old code
- **File(s)**：b.go
- **Change**：fix
- **Depends on**：1
- **Verify**：test`

	deltas := parseStepDeltas(plan)
	if deltas["Add feature"] != "ADDED" {
		t.Errorf("步骤1 expected ADDED, got %q", deltas["Add feature"])
	}
	if _, ok := deltas["Fix old code"]; ok {
		t.Error("步骤2 without Delta should not appear in map")
	}
}

func TestParseStepDeltas_ChineseTitle(t *testing.T) {
	plan := `<!--plan-->

步骤 1：重构认证模块 — 提取 JWT 验证逻辑
- **Delta**：MODIFIED
- **File(s)**：internal/auth/jwt.go
- **Change**：extract validation
- **Depends on**：无
- **Verify**：go test ./internal/auth/`

	deltas := parseStepDeltas(plan)
	if deltas["重构认证模块 — 提取 JWT 验证逻辑"] != "MODIFIED" {
		t.Errorf("Chinese title with em-dash should parse, got %q",
			deltas["重构认证模块 — 提取 JWT 验证逻辑"])
	}
}

// ── 2. Proposal：提案层检测 ──────────────────────────────────────────────

func TestHasProposal_WithProposal(t *testing.T) {
	// OpenSpec 风格：plan 前有 proposal 段落（why + what）
	plan := `## Proposal
需要为用户添加深色模式支持。当前所有页面使用硬编码浅色主题，
导致夜间使用体验差。方案：添加 CSS 变量 + React Context 实现主题切换。

<!--plan-->
步骤 1：Add CSS variables
- **Delta**：ADDED
- **File(s)**：[NEW] styles/theme.css
- **Change**：define light/dark variables
- **Depends on**：无
- **Verify**：npm run build`

	if !hasProposal(plan) {
		t.Fatal("plan with ## Proposal section before <!--plan--> should be detected")
	}
}

func TestHasProposal_WithoutProposal(t *testing.T) {
	plan := `<!--plan-->
步骤 1：Fix typo
- **Delta**：MODIFIED
- **File(s)**：foo.go
- **Change**：fix
- **Depends on**：无
- **Verify**：go build`

	if hasProposal(plan) {
		t.Fatal("plan starting directly with <!--plan--> should NOT have proposal")
	}
}

func TestHasProposal_ProposalAsWhy(t *testing.T) {
	// 最小化提案：只有一行 why
	plan := `**Why**: 构建脚本缺少 Windows 兼容性检查。

<!--plan-->
步骤 1：Add OS check
- **Delta**：ADDED
- **File(s)**：build.sh
- **Change**：add platform guard
- **Depends on**：无
- **Verify**：bash build.sh`

	if !hasProposal(plan) {
		t.Fatal("plan with **Why** before <!--plan--> should be detected as proposal")
	}
}

func TestHasProposal_AnalysisNotProposal(t *testing.T) {
	// Hermes 的分析/调查叙述不是 proposal
	plan := `我检查了 internal/auth/ 目录，发现 JWT 验证逻辑分散在 3 个文件中。
建议合并到单一模块。README 也需要更新。

<!--plan-->
步骤 1：Merge JWT logic
- **Delta**：MODIFIED
- **File(s)**：internal/auth/jwt.go
- **Change**：consolidate from 3 files
- **Depends on**：无
- **Verify**：go test ./internal/auth/`

	if hasProposal(plan) {
		t.Fatal("analysis/investigation text without structured proposal header " +
			"should NOT count as a proposal — only ## Proposal, **Why**, or ## 提案 trigger detection")
	}
}

func TestHasProposal_NoPlan(t *testing.T) {
	if hasProposal("纯文本回复，无 plan") {
		t.Fatal("text without <!--plan--> should not have proposal")
	}
}

// ── 3. Coherence：执行结果与计划一致性检查 ──────────────────────────

func TestCheckCoherence_AllFilesMatch(t *testing.T) {
	plan := `<!--plan-->
步骤 1：Add logger
- **File(s)**：[NEW] internal/log/logger.go
- **Delta**：ADDED
- **Change**：create
- **Depends on**：无
- **Verify**：go build

步骤 2：Update main
- **File(s)**：cmd/main.go
- **Delta**：MODIFIED
- **Change**：wire logger
- **Depends on**：1
- **Verify**：go build`

	result := &TurnResult{
		FilesCreated:  []string{"internal/log/logger.go"},
		FilesModified: []string{"cmd/main.go"},
	}

	warnings := checkCoherence(plan, result)
	if len(warnings) > 0 {
		t.Errorf("all files in plan, expected no warnings, got: %v", warnings)
	}
}

func TestCheckCoherence_UnplannedFile(t *testing.T) {
	plan := `<!--plan-->
步骤 1：Fix bug
- **File(s)**：a.go
- **Delta**：MODIFIED
- **Change**：fix
- **Depends on**：无
- **Verify**：go build`

	result := &TurnResult{
		FilesModified: []string{"a.go", "b.go", "c.go"},
	}

	warnings := checkCoherence(plan, result)
	if len(warnings) == 0 {
		t.Fatal("b.go and c.go modified but not in plan, should warn")
	}
	for _, f := range []string{"b.go", "c.go"} {
		found := false
		for _, w := range warnings {
			if strings.Contains(w, f) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("warning should mention unplanned file %s", f)
		}
	}
}

func TestCheckCoherence_PlanFileNotTouched(t *testing.T) {
	plan := `<!--plan-->
步骤 1：Add test
- **File(s)**：[NEW] a_test.go
- **Delta**：ADDED
- **Change**：test
- **Depends on**：无
- **Verify**：go test

步骤 2：Update impl
- **File(s)**：a.go
- **Delta**：MODIFIED
- **Change**：fix
- **Depends on**：1
- **Verify**：go build`

	result := &TurnResult{
		FilesCreated: []string{"a_test.go"},
		// a.go not modified — step 2 partially skipped
	}

	warnings := checkCoherence(plan, result)
	// a.go is in plan but not in result — coherence gap
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "a.go") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about a.go: plan mentions it but not in result")
	}
}

func TestCheckCoherence_NilResult(t *testing.T) {
	warnings := checkCoherence("<!--plan-->\n步骤 1：do\n- **File(s)**：x.go\n- **Delta**：MODIFIED\n- **Change**：fix\n- **Depends on**：无\n- **Verify**：test", nil)
	if len(warnings) != 0 {
		t.Fatalf("nil result should return no warnings, got %v", warnings)
	}
}

func TestCheckCoherence_EmptyFiles(t *testing.T) {
	plan := `<!--plan-->
步骤 1：Research only
- **Depends on**：无
- **Change**：investigate
- **Verify**：none`

	result := &TurnResult{}
	warnings := checkCoherence(plan, result)
	if len(warnings) > 0 {
		t.Errorf("plan with no File(s) entries should produce no warnings, got: %v", warnings)
	}
}

// ── 4. Prompt 关键词验证 — SDD 方法论已注入 ──────────────────────────

func TestHermesPrompt_SDDKeywords(t *testing.T) {
	p := HermesPrompt
	required := []string{
		// Proposal layer
		"Proposal",
		"why",
		// Delta marking
		"Delta",
		"ADDED",
		"MODIFIED",
		"REMOVED",
		// Specs first
		"openspec/specs",
		"现有规范",
		// Verify dimensions
		"completeness",
		"correctness",
		"coherence",
		// Enablers not gates
		"Enablers",
		"活文档",
	}
	var missing []string
	for _, kw := range required {
		if !strings.Contains(p, kw) {
			missing = append(missing, kw)
		}
	}
	if len(missing) > 0 {
		// TDD: deliberately failing — 这些关键词尚未注入 HermesPrompt
		t.Errorf("HermesPrompt 缺少 SDD 关键词（待注入）: %v", missing)
	}
}

func TestHermesPrompt_ContainsDeltaFormat(t *testing.T) {
	// 验证步骤格式中包含 Delta 字段说明
	if !strings.Contains(HermesPrompt, "**Delta**") {
		t.Error("HermesPrompt missing Delta field in step format")
	}
	if !strings.Contains(HermesPrompt, "ADDED | MODIFIED | REMOVED") {
		t.Error("HermesPrompt missing ADDED/MODIFIED/REMOVED explanation")
	}
}

func TestHermesPrompt_ContainsProposalGuidance(t *testing.T) {
	// 验证 prompt 引导 Hermes 在复杂任务时先写提案
	p := HermesPrompt
	if !strings.Contains(p, "先写简短提案") && !strings.Contains(p, "提案先行") {
		t.Error("HermesPrompt missing proposal-first guidance")
	}
}

// ── 5. formatExecutionFeedback 增强 — OpenSpec 风格汇报 ──────────────

func TestFormatExecutionFeedback_ContainsDeltaSummary(t *testing.T) {
	r := &TurnResult{
		Success:       true,
		Summary:       "完成",
		FilesCreated:  []string{"new.go"},
		FilesModified: []string{"old.go"},
	}
	out := formatExecutionFeedbackEnhanced(r, "")

	// 增强版应包含 Delta 分类信息
	if !strings.Contains(out, "ADDED") {
		t.Error("enhanced feedback should mention ADDED files")
	}
	if !strings.Contains(out, "MODIFIED") {
		t.Error("enhanced feedback should mention MODIFIED files")
	}
}

func TestFormatExecutionFeedbackEnhanced_VerifyDimensions(t *testing.T) {
	r := &TurnResult{
		Success: true,
		StepResults: []StepResult{
			{Step: "步骤1", Status: "success"},
			{Step: "步骤2", Status: "success"},
		},
	}
	out := formatExecutionFeedbackEnhanced(r, "")

	// 增强版应包含三维检查
	if !strings.Contains(out, "completeness") {
		t.Error("enhanced feedback should mention completeness")
	}
	if !strings.Contains(out, "correctness") {
		t.Error("enhanced feedback should mention correctness")
	}
	if !strings.Contains(out, "coherence") {
		t.Error("enhanced feedback should mention coherence")
	}
}

func TestFormatExecutionFeedbackEnhanced_BackwardCompatible(t *testing.T) {
	// 增强版必须是旧版的超集：包含所有旧版的关键信息
	r := &TurnResult{
		Success:       true,
		Summary:       "all done",
		FilesCreated:  []string{"a.go"},
		FilesModified: []string{"b.go"},
	}
	out := formatExecutionFeedbackEnhanced(r, "")

	// 旧版关键字段必须在增强版中保留
	legacyKeys := []string{
		"[上一轮执行结果]",
		"`a.go`",
		"`b.go`",
		"任务已完成",
	}
	for _, key := range legacyKeys {
		if !strings.Contains(out, key) {
			t.Errorf("enhanced feedback missing legacy key: %q", key)
		}
	}
}

// ── 6. Plan 文本中的 proposal 提取 ─────────────────────────────────────

func TestExtractProposal_WithProposal(t *testing.T) {
	plan := `## Proposal
添加深色模式支持。

<!--plan-->
步骤 1：Add CSS variables`

	proposal := extractProposal(plan)
	if proposal == "" {
		t.Fatal("should extract proposal text before <!--plan-->")
	}
	if !strings.Contains(proposal, "深色模式") {
		t.Errorf("proposal should contain the why/what text, got: %s", proposal)
	}
	if strings.Contains(proposal, "<!--plan-->") {
		t.Error("proposal should NOT contain <!--plan--> marker")
	}
}

func TestExtractProposal_WithoutProposal(t *testing.T) {
	plan := `<!--plan-->
步骤 1：Do thing`

	proposal := extractProposal(plan)
	if proposal != "" {
		t.Errorf("no proposal section should return empty, got: %s", proposal)
	}
}

func TestExtractProposal_WhyKeyword(t *testing.T) {
	plan := `**Why**: 当前登录流程在 Safari 上不工作。

<!--plan-->
步骤 1：Fix Safari login`

	proposal := extractProposal(plan)
	if !strings.Contains(proposal, "Safari") {
		t.Errorf("proposal should contain the Why text, got: %s", proposal)
	}
}

// ── 7. Delta 计数统计 ───────────────────────────────────────────────────

func TestCountDeltas_MixedPlan(t *testing.T) {
	plan := `<!--plan-->

步骤 1：New file
- **Delta**：ADDED
- **File(s)**：[NEW] a.go
- **Change**：create
- **Depends on**：无
- **Verify**：test

步骤 2：Edit file
- **Delta**：MODIFIED
- **File(s)**：b.go
- **Change**：update
- **Depends on**：1
- **Verify**：test

步骤 3：Delete code
- **Delta**：REMOVED
- **File(s)**：c.go
- **Change**：remove
- **Depends on**：2
- **Verify**：test

步骤 4：Another new file
- **Delta**：ADDED
- **File(s)**：[NEW] d.go
- **Change**：create
- **Depends on**：1
- **Verify**：test`

	counts := countDeltas(plan)
	if counts["ADDED"] != 2 {
		t.Errorf("expected 2 ADDED, got %d", counts["ADDED"])
	}
	if counts["MODIFIED"] != 1 {
		t.Errorf("expected 1 MODIFIED, got %d", counts["MODIFIED"])
	}
	if counts["REMOVED"] != 1 {
		t.Errorf("expected 1 REMOVED, got %d", counts["REMOVED"])
	}
}

func TestCountDeltas_EmptyPlan(t *testing.T) {
	counts := countDeltas("")
	if len(counts) != 0 {
		t.Fatalf("empty plan should return empty counts, got %v", counts)
	}
}

// ── helper: 构建多步骤 plan 文本 ───────────────────────────────────────

func makePlanWithDeltas(steps []struct{ title, delta, files string }) string {
	var b strings.Builder
	b.WriteString("<!--plan-->\n")
	for i, s := range steps {
		deps := "无"
		if i > 0 {
			deps = fmt.Sprintf("%d", i)
		}
		b.WriteString(fmt.Sprintf(`
步骤 %d：%s
- **Delta**：%s
- **File(s)**：%s
- **Change**：do thing
- **Depends on**：%s
- **Verify**：test
`, i+1, s.title, s.delta, s.files, deps))
	}
	return b.String()
}
