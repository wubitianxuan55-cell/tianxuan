package agent

import (
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

func TestShouldSkipPlanner_AfterComposeBlocks(t *testing.T) {
	// Simulates Compose() prepending <memory-update> and <background-jobs>
	// blocks before the user's "!build desktop" input.
	input := `<memory-update>
The following project-memory changes were just made and apply from now on:
- Saved memory "foo"
</memory-update>

<background-jobs>
job bash-1 finished
</background-jobs>

!build desktop`
	s, ok := shouldSkipPlanner(input)
	if !ok {
		t.Fatal("expected true for ! prefix after Compose blocks")
	}
	if s != "build desktop" {
		t.Fatalf("got %q, want %q", s, "build desktop")
	}
}

func TestShouldSkipPlanner_AfterMemoryRules(t *testing.T) {
	// Simulates Compose() prepending procedural rules.
	input := `Procedural rule line 1
Procedural rule line 2

!run tests`
	s, ok := shouldSkipPlanner(input)
	if !ok {
		t.Fatal("expected true for ! after procedural rules")
	}
	if s != "run tests" {
		t.Fatalf("got %q, want %q", s, "run tests")
	}
}

func TestShouldSkipPlanner_NoBangInMiddle(t *testing.T) {
	// "!" inside a message (not at a paragraph boundary) should NOT trigger.
	input := `<memory-update>
something
</memory-update>

help me fix the !important CSS bug`
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
	if !strings.Contains(out, "Original task:\nbuild") {
		t.Fatal("handoff missing original task")
	}
	if !strings.Contains(out, "Hermes output:\nrun wails build") {
		t.Fatal("handoff missing Hermes output")
	}
	if strings.Contains(out, "📌 User note (written during plan confirmation)") {
		t.Fatal("should not contain user note section when empty")
	}
}

func TestFormatHandoff_WithUserNote(t *testing.T) {
	out := formatHandoff("build", "run wails build", "also run tests first")
	if !strings.Contains(out, "📌 User note (written during plan confirmation):\nalso run tests first") {
		t.Fatal("handoff missing user note")
	}
}

func TestFormatHandoff_EmptyPlan(t *testing.T) {
	out := formatHandoff("build", "", "")
	if !strings.Contains(out, "Hermes output:\n") {
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
