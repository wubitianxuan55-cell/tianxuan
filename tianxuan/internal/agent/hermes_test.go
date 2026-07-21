package agent

import (
	"fmt"
	"strings"
	"testing"
)

// ── shouldSkipPlanner ──────────────────────────────────────

func TestShouldSkipPlanner_BangPrefix(t *testing.T) {
	s, ok := shouldSkipPlanner("!build desktop")
	if !ok {
		t.Fatal("expected true for ! prefix")
	}
	if s != "build desktop" {
		t.Fatalf("got %q, want %q", s, "build desktop")
	}
}

func TestShouldSkipPlanner_NoBang(t *testing.T) {
	_, ok := shouldSkipPlanner("build desktop")
	if ok {
		t.Fatal("expected false without ! prefix")
	}
}

func TestShouldSkipPlanner_Empty(t *testing.T) {
	_, ok := shouldSkipPlanner("")
	if ok {
		t.Fatal("expected false for empty input")
	}
}

func TestShouldSkipPlanner_BangOnly(t *testing.T) {
	s, ok := shouldSkipPlanner("!")
	if !ok {
		t.Fatal("expected true for bare !")
	}
	if s != "" {
		t.Fatalf("got %q, want empty", s)
	}
}

func TestShouldSkipPlanner_BangWithSpaces(t *testing.T) {
	s, ok := shouldSkipPlanner("!  run tests  ")
	if !ok {
		t.Fatal("expected true for ! with spaces")
	}
	if s != "run tests" {
		t.Fatalf("got %q, want %q", s, "run tests")
	}
}

func TestShouldSkipPlanner_WithTrailingBlocks(t *testing.T) {
	// Compose() now appends blocks AFTER user input, so ! always at position 0.
	input := "!build desktop\n\n<memory-update>\n- Saved memory \"foo\"\n</memory-update>"
	s, ok := shouldSkipPlanner(input)
	if !ok {
		t.Fatal("expected true for ! prefix with trailing blocks")
	}
	if s != "build desktop" {
		t.Fatalf("got %q, want %q", s, "build desktop")
	}
}

func TestShouldSkipPlanner_NoBangWithBlocks(t *testing.T) {
	// "!" in middle of text should NOT trigger, even with trailing Compose blocks.
	input := "help me fix the !important CSS bug\n\n<memory-update>\nsomething\n</memory-update>"
	_, ok := shouldSkipPlanner(input)
	if ok {
		t.Fatal("should NOT trigger on ! in middle of text")
	}
}

// ── isAnswerNotAction ──────────────────────────────────────

func TestIsAnswerNotAction_Short(t *testing.T) {
	if !isAnswerNotAction("ok") {
		t.Fatal("short text should be treated as answer")
	}
}

func TestIsAnswerNotAction_Empty(t *testing.T) {
	if !isAnswerNotAction("") {
		t.Fatal("empty text should be treated as answer")
	}
}

func TestIsAnswerNotAction_WhitespaceOnly(t *testing.T) {
	if !isAnswerNotAction("   ") {
		t.Fatal("whitespace-only should be treated as answer")
	}
}

func TestIsAnswerNotAction_WithPlan(t *testing.T) {
	// Need > 100 chars to avoid the short-circuit for short texts
	prefix := strings.Repeat("x", 120)
	plan := "<!--plan-->\n" + prefix
	if isAnswerNotAction(plan) {
		t.Fatal("text containing <!--plan--> should NOT be treated as answer")
	}
}

func TestIsAnswerNotAction_LongWithoutPlan(t *testing.T) {
	long := strings.Repeat("x", 150)
	// Long text without <!--plan--> IS a direct answer (Hermes answered directly)
	if !isAnswerNotAction(long) {
		t.Fatal("long text without <!--plan--> should be treated as a direct answer")
	}
}

// ── formatHandoff ─────────────────────────────────────────

func TestFormatHandoff_Normal(t *testing.T) {
	out := formatHandoff("build", "run wails build", "")
	if !strings.Contains(out, hephaestusHandoffMarker) {
		t.Fatal("handoff missing marker")
	}
	if !strings.Contains(out, "任务:\nbuild") {
		t.Fatal("handoff missing original task")
	}
	if !strings.Contains(out, "计划:\nrun wails build") {
		t.Fatal("handoff missing Hermes output")
	}
	if strings.Contains(out, "📌 用户备注") {
	}
	if strings.Contains(out, "📌 User note (written during plan confirmation)") {
		t.Fatal("should not contain user note section when empty")
	}
}

func TestFormatHandoff_WithUserNote(t *testing.T) {
	out := formatHandoff("build", "run wails build", "also run tests first")
	if !strings.Contains(out, "📌 用户备注:\nalso run tests first") {
		t.Fatal("handoff missing user note")
	}
}

func TestFormatHandoff_EmptyPlan(t *testing.T) {
	out := formatHandoff("build", "", "")
	if !strings.Contains(out, "计划:\n") {
		t.Fatal("handoff should still have Hermes output section")
	}
}

func TestFormatHandoff_SpecialChars(t *testing.T) {
	out := formatHandoff(`test "quotes"`, `plan with <angle>`, "")
	if !strings.Contains(out, `test "quotes"`) {
		t.Fatal("handoff should preserve special chars in task")
	}
	if !strings.Contains(out, `plan with <angle>`) {
		t.Fatal("handoff should preserve special chars in plan")
	}
}

// ── HandoffTask ───────────────────────────────────────────

func TestHandoffTask_ExtractsTask(t *testing.T) {
	handoff := formatHandoff("build the app", "run wails build", "")
	extracted := HandoffTask(handoff)
	if extracted != "build the app" {
		t.Fatalf("got %q, want %q", extracted, "build the app")
	}
}

func TestHandoffTask_NonHandoffPassthrough(t *testing.T) {
	plain := "just a regular message"
	if HandoffTask(plain) != plain {
		t.Fatal("non-handoff should pass through unchanged")
	}
}

func TestHandoffTask_Empty(t *testing.T) {
	if HandoffTask("") != "" {
		t.Fatal("empty should return empty")
	}
}

func TestHandoffTask_ShortMessage(t *testing.T) {
	s := "hi"
	if HandoffTask(s) != s {
		t.Fatal("short message should pass through")
	}
}

// ── Prompt constants validation ────────────────────────────

func TestSoloSystemPrompt_ContainsEssentials(t *testing.T) {
	p := SoloSystemPrompt
	required := []string{
		"Tianxuan",
		"plans and executes",
		"todo_write",
		"complete_step",
		"TDD",
		"Design first",
		"Surgical",
		"Defensive",
		"No placeholders",
	}
	for _, kw := range required {
		if !strings.Contains(p, kw) {
			t.Errorf("SoloSystemPrompt missing keyword: %q", kw)
		}
	}
}

func TestHephaestusSystemPrompt_ContainsEssentials(t *testing.T) {
	p := HephaestusSystemPrompt
	required := []string{
		"Hephaestus",
		"Hermes",
		"complete_step",
		"todo_write",
		"parallel_tasks",
		"Surgical Changes",
		"Goal-Driven Execution",
		"NEVER re-explore",
		"not plans, not confirmations, not investigations",
		"Hermes handles replanning",
	}
	for _, kw := range required {
		if !strings.Contains(p, kw) {
			t.Errorf("HephaestusSystemPrompt missing keyword: %q", kw)
		}
	}
}

func TestHermesPrompt_ContainsEssentials(t *testing.T) {
	p := HermesPrompt
	required := []string{
		"Hermes",
		"planner",
		"Hephaestus",
		"<!--plan-->",
		"read-only",
		"3–8 steps",
		"will NOT re-explore",
	}
	for _, kw := range required {
		if !strings.Contains(p, kw) {
			t.Errorf("HermesPrompt missing keyword: %q", kw)
		}
	}
}

func TestPromptsAreDistinct(t *testing.T) {
	if SoloSystemPrompt == HephaestusSystemPrompt {
		t.Fatal("SoloSystemPrompt and HephaestusSystemPrompt must differ")
	}
	if SoloSystemPrompt == HermesPrompt {
		t.Fatal("SoloSystemPrompt and HermesPrompt must differ")
	}
	if HephaestusSystemPrompt == HermesPrompt {
		t.Fatal("HephaestusSystemPrompt and HermesPrompt must differ")
	}
}

func TestPromptsAreNonEmpty(t *testing.T) {
	if len(SoloSystemPrompt) == 0 {
		t.Fatal("SoloSystemPrompt is empty")
	}
	if len(HephaestusSystemPrompt) == 0 {
		t.Fatal("HephaestusSystemPrompt is empty")
	}
	if len(HermesPrompt) == 0 {
		t.Fatal("HermesPrompt is empty")
	}
}

func TestSoloPromptContainsTDD(t *testing.T) {
	if !strings.Contains(SoloSystemPrompt, "TDD") {
		t.Fatal("SoloSystemPrompt must reference TDD")
	}
	if !strings.Contains(SoloSystemPrompt, "todo_write") {
		t.Fatal("SoloSystemPrompt must reference todo_write")
	}
	if !strings.Contains(SoloSystemPrompt, "complete_step") {
		t.Fatal("SoloSystemPrompt must reference complete_step")
	}
}

// ── formatExecutionFeedback ────────────────────────────────

func TestFormatExecutionFeedback_Success(t *testing.T) {
	r := &TurnResult{
		Success:       true,
		Summary:       "all steps done",
		FilesCreated:  []string{"a.go"},
		FilesModified: []string{"b.go"},
	}
	out := formatExecutionFeedback(r)
	if !strings.Contains(out, "success") {
		t.Fatal("success feedback should say success")
	}
	if !strings.Contains(out, "任务已完成") {
		t.Fatal("success feedback should say 任务已完成")
	}
	if !strings.Contains(out, "`a.go`") {
		t.Fatal("success feedback should mention created file")
	}
}

func TestFormatExecutionFeedback_Failure(t *testing.T) {
	r := &TurnResult{
		Success: false,
		Errors:  []string{"something broke"},
		Summary: "step 3 failed",
	}
	out := formatExecutionFeedback(r)
	if !strings.Contains(out, "errors") {
		t.Fatal("failure feedback should say errors")
	}
	if !strings.Contains(out, "任务未完成") {
		t.Fatal("failure feedback should say 任务未完成")
	}
	if !strings.Contains(out, "something broke") {
		t.Fatal("failure feedback should include error text")
	}
}

func TestFormatExecutionFeedback_EmptyResult(t *testing.T) {
	r := &TurnResult{Success: true}
	out := formatExecutionFeedback(r)
	if !strings.Contains(out, "(none)") {
		t.Fatal("empty result should show (none) for errors")
	}
	if !strings.Contains(out, "execution produced no summary") {
		t.Fatal("empty result should show actionable no-summary message")
	}
}

// ── hasStructuralChange ────────────────────────────────────

func TestHasStructuralChange_GoMod(t *testing.T) {
	if !hasStructuralChange([]string{"go.mod"}, nil) {
		t.Fatal("go.mod should be structural change")
	}
}

func TestHasStructuralChange_PackageJSON(t *testing.T) {
	if !hasStructuralChange(nil, []string{"package.json"}) {
		t.Fatal("package.json should be structural change")
	}
}

func TestHasStructuralChange_InternalGoFile(t *testing.T) {
	if !hasStructuralChange([]string{"internal/agent/new.go"}, nil) {
		t.Fatal("internal/*.go should be structural change")
	}
}

func TestHasStructuralChange_NoChange(t *testing.T) {
	if hasStructuralChange([]string{"main.go", "README.md"}, []string{"config.toml"}) {
		t.Fatal("non-structural files should not trigger structural change")
	}
}

func TestHasStructuralChange_Empty(t *testing.T) {
	if hasStructuralChange(nil, nil) {
		t.Fatal("empty should not trigger structural change")
	}
}

// ── shouldAutoConfirm ────────────────────────────────────

func TestShouldAutoConfirm_SimplePlan(t *testing.T) {
	plan := `<!--plan-->

步骤 1：Update greeting
- **File(s)**：internal/foo.go
- **Change**：fix greeting text
- **Depends on**：无
- **Verify**：go test

步骤 2：Update tests
- **File(s)**：internal/foo_test.go
- **Change**：update expected value
- **Depends on**：1
- **Verify**：go test`
	if !shouldAutoConfirm(plan) {
		t.Fatal("simple plan with 2 steps, no new files should auto-confirm")
	}
}

func TestShouldAutoConfirm_TooManySteps(t *testing.T) {
	var plan string
	for i := 1; i <= 5; i++ {
		plan += fmt.Sprintf("步骤 %d：Step %d\n- **File(s)**：a.go\n- **Change**：do thing\n- **Depends on**：无\n- **Verify**：test\n", i, i)
	}
	if shouldAutoConfirm(plan) {
		t.Fatal("plan with 5 steps should NOT auto-confirm")
	}
}

func TestShouldAutoConfirm_NewFile(t *testing.T) {
	plan := `<!--plan-->

步骤 1：Create new file
- **File(s)**：internal/new.go [NEW]
- **Change**：add helper
- **Depends on**：无
- **Verify**：go build`
	if shouldAutoConfirm(plan) {
		t.Fatal("plan with [NEW] file should NOT auto-confirm")
	}
}

func TestShouldAutoConfirm_Empty(t *testing.T) {
	if !shouldAutoConfirm("") {
		t.Fatal("empty plan should auto-confirm (no steps = trivial)")
	}
}

func TestShouldAutoConfirm_ThreeSteps(t *testing.T) {
	plan := `<!--plan-->

步骤 1：Fix typo
- **File(s)**：a.go
- **Change**：fix typo
- **Depends on**：无
- **Verify**：test

步骤 2：Update docs
- **File(s)**：README.md
- **Change**：update
- **Depends on**：无
- **Verify**：test

步骤 3：Run tests
- **File(s)**：a_test.go
- **Change**：update
- **Depends on**：1
- **Verify**：test`
	if !shouldAutoConfirm(plan) {
		t.Fatal("plan with exactly 3 steps and no new files should auto-confirm")
	}
}

// ── HermesPrompt tool alignment ──────────────────────────

// TestHermesPromptToolsExist checks that every tool name mentioned in
// HermesPrompt matches a known tool. When a tool is renamed in the registry,
// this test catches the stale prompt entry by checking substring presence.
func TestHermesPromptToolsExist(t *testing.T) {
	tools := []struct {
		name string
		frag string // substring to search in HermesPrompt
	}{
		{"read_file", "read_file"},
		{"grep", "grep"},
		{"glob", "glob"},
		{"ls", "ls"},
		{"code_index", "code_index"},
		{"lsp_definition", "lsp_definition"},
		{"lsp_hover", "lsp_hover"},
		{"lsp_references", "lsp_references"},
		{"lsp_diagnostics", "lsp_diagnostics"},
		{"mcp__codegraph__*", "mcp__codegraph__*"}, // wildcard in prompt
		{"git_status", "git_status"},
		{"git_diff", "git_diff"},
		{"git_log", "git_log"},
		{"web_search", "web_search"},
		{"web_fetch", "web_fetch"},
		{"memory_search", "memory_search"},
		{"read_skill", "read_skill"},
		{"explore", "explore"},
		{"research", "research"},
		{"review", "review"},
		{"security_review", "security_review"},
	}
	prompt := HermesPrompt
	var missing []string
	for _, tt := range tools {
		if !strings.Contains(prompt, tt.frag) {
			missing = append(missing, tt.name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("HermesPrompt missing references to these tools (may have been renamed): %v\n"+
			"Update HermesPrompt or the tools list in TestHermesPromptToolsExist.", missing)
	}
}

// ── formatSummary ──────────────────────────────────────────

func TestFormatSummary_NilResultWithError(t *testing.T) {
	out := (&Hermes{}).formatSummary(nil, fmt.Errorf("timeout"), false)
	if !strings.Contains(out, "❌ 执行失败") {
		t.Fatalf("expected failure prefix, got: %s", out)
	}
	if !strings.Contains(out, "timeout") {
		t.Fatalf("expected error text, got: %s", out)
	}
}

func TestFormatSummary_NilResultNoError(t *testing.T) {
	out := (&Hermes{}).formatSummary(nil, nil, false)
	if out != "" {
		t.Fatalf("expected empty string for nil result + nil error, got: %s", out)
	}
}

func TestFormatSummary_SuccessNoDetails(t *testing.T) {
	r := &TurnResult{Success: true}
	out := (&Hermes{}).formatSummary(r, nil, false)
	if !strings.Contains(out, "✅ 任务完成") {
		t.Fatalf("expected success prefix, got: %s", out)
	}
	if !strings.Contains(out, "未记录步骤详情") {
		t.Fatalf("expected no-details note, got: %s", out)
	}
}

func TestFormatSummary_SuccessWithFiles(t *testing.T) {
	r := &TurnResult{
		Success:       true,
		FilesCreated:  []string{"a.go", "b.go"},
		FilesModified: []string{"c.go"},
	}
	out := (&Hermes{}).formatSummary(r, nil, false)
	if !strings.Contains(out, "新建 2 个文件") {
		t.Fatalf("expected file creation count, got: %s", out)
	}
	if !strings.Contains(out, "修改 1 个文件") {
		t.Fatalf("expected file modification count, got: %s", out)
	}
}

func TestFormatSummary_PartialSuccess(t *testing.T) {
	r := &TurnResult{
		Success: false,
		Errors:  []string{"e1", "e2", "e3"},
	}
	out := (&Hermes{}).formatSummary(r, nil, false)
	if !strings.Contains(out, "⚠️ 任务部分完成") {
		t.Fatalf("expected partial prefix, got: %s", out)
	}
	if !strings.Contains(out, "3 个错误") {
		t.Fatalf("expected error count, got: %s", out)
	}
}

func TestFormatSummary_WithStepResults(t *testing.T) {
	r := &TurnResult{
		Success: true,
		StepResults: []StepResult{
			{Step: "步骤1", Status: "success"},
			{Step: "步骤2", Status: "error"},
			{Step: "步骤3", Status: "success"},
		},
	}
	out := (&Hermes{}).formatSummary(r, nil, false)
	if !strings.Contains(out, "✅ 步骤1") {
		t.Fatalf("expected step1 success, got: %s", out)
	}
	if !strings.Contains(out, "❌ 步骤2") {
		t.Fatalf("expected step2 failure, got: %s", out)
	}
}

func TestFormatSummary_RetriesExhausted(t *testing.T) {
	r := &TurnResult{Success: false, Errors: []string{"e1"}}
	out := (&Hermes{}).formatSummary(r, nil, true)
	if !strings.Contains(out, "已尝试多轮自动修正") {
		t.Fatalf("expected retries-exhausted note, got: %s", out)
	}
}

// ── allStepsPassed ─────────────────────────────────────────

func TestAllStepsPassed_Nil(t *testing.T) {
	if (&Hermes{}).allStepsPassed(nil) {
		t.Fatal("nil TurnResult should not pass")
	}
}

func TestAllStepsPassed_NotSuccess(t *testing.T) {
	r := &TurnResult{Success: false}
	if (&Hermes{}).allStepsPassed(r) {
		t.Fatal("!Success should not pass")
	}
}

func TestAllStepsPassed_FailedStep(t *testing.T) {
	r := &TurnResult{
		Success: true,
		StepResults: []StepResult{
			{Step: "step1", Status: "success"},
			{Step: "step2", Status: "error"},
		},
	}
	if (&Hermes{}).allStepsPassed(r) {
		t.Fatal("result with failed step should not pass")
	}
}

func TestAllStepsPassed_BlockedStep(t *testing.T) {
	r := &TurnResult{
		Success: true,
		StepResults: []StepResult{
			{Step: "step1", Status: "blocked"},
		},
	}
	if (&Hermes{}).allStepsPassed(r) {
		t.Fatal("result with blocked step should not pass")
	}
}

func TestAllStepsPassed_AllSuccess(t *testing.T) {
	r := &TurnResult{
		Success: true,
		StepResults: []StepResult{
			{Step: "step1", Status: "success"},
			{Step: "step2", Status: "success"},
		},
	}
	if !(&Hermes{}).allStepsPassed(r) {
		t.Fatal("result with all-success steps should pass")
	}
}

func TestAllStepsPassed_NoStepResults(t *testing.T) {
	r := &TurnResult{Success: true}
	// Success=true but no step results — model declared done without complete_step.
	// allStepsPassed treats this as passing (no contradiction to Success).
	if !(&Hermes{}).allStepsPassed(r) {
		t.Fatal("Success=true with no step results should pass")
	}
}

// ── planFix prompt construction ────────────────────────────

func TestPlanFixPrompt_Round2(t *testing.T) {
	failed := &TurnResult{
		Success: false,
		Errors:  []string{"build error: undefined: Foo"},
		StepResults: []StepResult{
			{Step: "步骤1", Status: "success"},
			{Step: "步骤2", Status: "error", Result: "go build 失败"},
		},
	}
	prompt := buildFixPrompt("修复所有bug", "步骤1: fix\n步骤2: test", failed, 2, nil)

	// Round 2: targeted fix — should reference original task and plan.
	if !strings.Contains(prompt, "修复所有bug") {
		t.Fatal("round 2 prompt should include original task")
	}
	if !strings.Contains(prompt, "步骤1: fix") {
		t.Fatal("round 2 prompt should include original plan")
	}
	if !strings.Contains(prompt, "步骤2") {
		t.Fatal("round 2 prompt should mention failed step")
	}
	if !strings.Contains(prompt, "<!--plan-->") {
		t.Fatal("round 2 prompt should request <!--plan--> marker")
	}
	if !strings.Contains(prompt, "仅修复标记") {
		t.Fatal("round 2 prompt should say 仅修复")
	}
	if strings.Contains(prompt, "反思") {
		t.Fatal("round 2 prompt should NOT request reflection")
	}
}

func TestPlanFixPrompt_Round3(t *testing.T) {
	failed := &TurnResult{
		Success: false,
		Errors:  []string{"still broken"},
		StepResults: []StepResult{
			{Step: "步骤1", Status: "error", Result: "test still fails"},
		},
	}
	fixHistory := []fixAttempt{
		{round: 2, fixPlan: "fix plan v2", feedback: "still failed"},
	}
	prompt := buildFixPrompt("原始任务", "原始计划", failed, 3, fixHistory)

	// Round 3: reflection mode — should request rethinking.
	if !strings.Contains(prompt, "反思") {
		t.Fatal("round 3 prompt should request reflection")
	}
	if !strings.Contains(prompt, "根本方向") {
		t.Fatal("round 3 prompt should ask about fundamental approach")
	}
	if !strings.Contains(prompt, "fix plan v2") {
		t.Fatal("round 3 prompt should include fix history")
	}
	if !strings.Contains(prompt, "<!--plan-->") {
		t.Fatal("round 3 prompt should request <!--plan--> marker")
	}
}

func TestPlanFixPrompt_NoStepResults(t *testing.T) {
	// Defensive: TurnResult.Errors is non-empty but StepResults is empty.
	failed := &TurnResult{Success: false, Errors: []string{"unknown error"}}
	prompt := buildFixPrompt("task", "plan", failed, 2, nil)

	if !strings.Contains(prompt, "unknown error") {
		t.Fatal("prompt should include error text even without step results")
	}
}

// ── displayPlan ────────────────────────────────────────────

func TestDisplayPlan_WithMarker(t *testing.T) {
	full := `这是分析前言，包含了一些内部记忆和推理过程。
用户提到……
<!--plan-->
步骤 1：修复 bug
- **File(s)**：foo.go
- **Change**：修正逻辑`

	displayed := displayPlan(full)
	if strings.Contains(displayed, "分析前言") {
		t.Fatal("displayPlan should strip preamble analysis")
	}
	if strings.Contains(displayed, "内部记忆") {
		t.Fatal("displayPlan should strip memory references in preamble")
	}
	if !strings.Contains(displayed, "<!--plan-->") {
		t.Fatal("displayPlan should keep the <!--plan--> marker")
	}
	if !strings.Contains(displayed, "步骤 1") {
		t.Fatal("displayPlan should keep plan content after marker")
	}
}

func TestDisplayPlan_NoMarker(t *testing.T) {
	text := "这是纯文本回复，没有计划标记。"
	displayed := displayPlan(text)
	if displayed != text {
		t.Fatalf("displayPlan without marker should return full text unchanged, got: %s", displayed)
	}
}

func TestDisplayPlan_MarkerAtStart(t *testing.T) {
	text := "<!--plan-->\n步骤 1：do thing"
	displayed := displayPlan(text)
	if !strings.HasPrefix(displayed, "<!--plan-->") {
		t.Fatal("displayPlan with marker at start should keep it")
	}
}

func TestDisplayPlan_BlankPreamble(t *testing.T) {
	text := "\n\n  <!--plan-->\n步骤 1：do thing"
	displayed := displayPlan(text)
	if !strings.HasPrefix(displayed, "<!--plan-->") {
		t.Fatalf("displayPlan should trim whitespace and start at marker, got: %s", displayed)
	}
}

func TestDisplayPlan_Empty(t *testing.T) {
	if displayPlan("") != "" {
		t.Fatal("displayPlan of empty string should return empty")
	}
}

// ── resolveConfirmChoice ───────────────────────────────────

func TestResolveConfirmChoice_Submit(t *testing.T) {
	note, chatOnly, revise, err := resolveConfirmChoice("提交执行", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chatOnly || revise || note != "" {
		t.Fatal("提交执行 should return zero values")
	}
}

func TestResolveConfirmChoice_ChatOnly(t *testing.T) {
	_, chatOnly, revise, err := resolveConfirmChoice("仅聊天", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !chatOnly || revise {
		t.Fatal("仅聊天 should set chatOnly=true, revise=false")
	}
}

func TestResolveConfirmChoice_Revise(t *testing.T) {
	note, chatOnly, revise, err := resolveConfirmChoice("按用户意见修改计划", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !revise || chatOnly {
		t.Fatal("按用户意见修改计划 should set revise=true")
	}
	if note != "" {
		t.Fatalf("revise without feedback should return empty note, got %q", note)
	}
}

func TestResolveConfirmChoice_ReviseWithFeedback(t *testing.T) {
	note, chatOnly, revise, err := resolveConfirmChoice("按用户意见修改计划", []string{"请改用 Go 1.21 语法"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !revise {
		t.Fatal("should set revise=true")
	}
	if note != "请改用 Go 1.21 语法" {
		t.Fatalf("should extract feedback from extra[0], got %q", note)
	}
	if chatOnly {
		t.Fatal("should not set chatOnly")
	}
}

func TestResolveConfirmChoice_Cancel(t *testing.T) {
	_, _, _, err := resolveConfirmChoice("取消", nil)
	if err == nil {
		t.Fatal("取消 should return an error")
	}
	if !strings.Contains(err.Error(), "取消") {
		t.Fatalf("expected cancel error, got: %v", err)
	}
}

func TestResolveConfirmChoice_FreeText(t *testing.T) {
	note, chatOnly, revise, err := resolveConfirmChoice("请添加更多测试", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chatOnly || revise {
		t.Fatal("free text should not set chatOnly or revise")
	}
	if note != "请添加更多测试" {
		t.Fatalf("free text should become note, got %q", note)
	}
}
