package agent

import (
	"strings"
	"testing"

	"tianxuan/internal/provider"
)

// ── maybeInjectToolFeedback ─────────────────────────────────────────

func TestMaybeInjectToolFeedback_NoErrors(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{{Name: "bash"}, {Name: "read_file"}}
	results := []string{"success output", "file content here"}

	if a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("should not inject feedback when all calls succeed")
	}
}

func TestMaybeInjectToolFeedback_OneError(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{{Name: "bash"}, {Name: "read_file"}}
	results := []string{"success output", "error: file not found"}

	if a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("should not inject feedback for a single error (below threshold)")
	}
}

func TestMaybeInjectToolFeedback_PartialErrorsNoInject(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{
		{Name: "bash"},
		{Name: "read_file"},
		{Name: "write_file"},
	}
	results := []string{
		"error: command not found",
		"error: no such file",
		"success",
	}

	if a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("partial failures must not inject the soft feedback (V10.152)")
	}
	if len(s.Messages) != 0 {
		t.Fatalf("no message should be added, got %d", len(s.Messages))
	}
}

func TestMaybeInjectToolFeedback_AllFailedThreeRounds(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{{Name: "bash"}, {Name: "read_file"}}
	results := []string{"error: fail", "error: also fail"}

	for i := 0; i < ToolFeedbackCap-1; i++ {
		if a.maybeInjectToolFeedback(calls, results) {
			t.Fatalf("round %d must not inject before the streak threshold", i+1)
		}
	}
	if !a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("full-failure streak at threshold must inject the STOP steer")
	}
	msg := s.Messages[len(s.Messages)-1]
	if !strings.Contains(msg.Content, "[system]") || !strings.Contains(msg.Content, "2 个失败") {
		t.Fatalf("STOP steer should summarize failures, got: %s", msg.Content)
	}
	// After injection the counter resets; the next full-failure round restarts counting.
	if a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("injection resets the counter; immediate next round must not re-inject")
	}
}

func TestMaybeInjectToolFeedback_ResetOnPartialSuccess(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	fail := []provider.ToolCall{{Name: "bash"}, {Name: "read_file"}}
	failRes := []string{"error: fail", "error: fail2"}
	ok := []provider.ToolCall{{Name: "bash"}}
	okRes := []string{"success"}

	a.maybeInjectToolFeedback(fail, failRes)
	if a.toolFeedbackCount != 1 {
		t.Fatalf("streak counter should be 1, got %d", a.toolFeedbackCount)
	}
	a.maybeInjectToolFeedback(ok, okRes)
	if a.toolFeedbackCount != 0 {
		t.Fatalf("a round with any success must reset the streak, got %d", a.toolFeedbackCount)
	}
}

func TestMaybeInjectToolFeedback_AllBlockedNoInject(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{{Name: "write_file"}, {Name: "edit_file"}}
	results := []string{"blocked: permission denied", "blocked: [loop guard] already succeeded"}

	if a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("all-blocked (permission gating) must not inject")
	}
}

func TestMaybeInjectToolFeedback_PlannerModeSkip(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s, plannerMode: true}

	calls := []provider.ToolCall{{Name: "bash"}, {Name: "read_file"}}
	results := []string{"error: fail", "error: fail"}

	if a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("should skip in plannerMode")
	}
}

// ── buildToolFeedbackMessage ────────────────────────────────────────

func TestBuildToolFeedbackMessage_Basic(t *testing.T) {
	details := []string{"  bash → error: command not found", "  read_file → error: no such file"}
	msg := buildToolFeedbackMessage(2, 3, details)
	if !strings.Contains(msg, "2 个失败") {
		t.Fatalf("expected '2 个失败', got: %s", msg)
	}
	if !strings.Contains(msg, "bash") {
		t.Fatalf("expected 'bash' detail, got: %s", msg)
	}
	if !strings.Contains(msg, "不要重复相同操作") {
		t.Fatal("expected generic advice")
	}
}

func TestBuildToolFeedbackMessage_AllErrors(t *testing.T) {
	details := []string{"  bash → error", "  read_file → error", "  write_file → error"}
	msg := buildToolFeedbackMessage(3, 3, details)
	if !strings.Contains(msg, "3 个失败") {
		t.Fatalf("expected '3 个失败', got: %s", msg)
	}
}

func TestBuildToolFeedbackMessage_ErrorCategories(t *testing.T) {
	details := []string{
		"  read_file → error: no such file or directory",
		"  bash → error: go build: undefined: Foo",
		"  write_file → Error: permission denied",
	}
	msg := buildToolFeedbackMessage(3, 3, details)

	// Should contain category labels
	if !strings.Contains(msg, "文件缺失") {
		t.Fatal("should have 文件缺失 category")
	}
	if !strings.Contains(msg, "编译错误") {
		t.Fatal("should have 编译错误 category")
	}
	if !strings.Contains(msg, "权限错误") {
		t.Fatal("should have 权限错误 category")
	}
}

// ── categorizeErrors ────────────────────────────────────────────────

func TestCategorizeErrors_FileMissing(t *testing.T) {
	details := []string{"  read_file → error: no such file or directory: foo.go"}
	cats := categorizeErrors(details)
	if len(cats) < 1 || cats[0].label != "文件缺失" {
		t.Fatalf("expected 文件缺失, got %+v", cats)
	}
}

func TestCategorizeErrors_CompileError(t *testing.T) {
	details := []string{"  bash → error: ./main.go:10:2: undefined: Foo"}
	cats := categorizeErrors(details)
	if len(cats) < 1 || cats[0].label != "编译错误" {
		t.Fatalf("expected 编译错误, got %+v", cats)
	}
}

func TestCategorizeErrors_Permission(t *testing.T) {
	details := []string{"  write_file → Error: permission denied"}
	cats := categorizeErrors(details)
	if len(cats) < 1 || cats[0].label != "权限错误" {
		t.Fatalf("expected 权限错误, got %+v", cats)
	}
}

func TestCategorizeErrors_Timeout(t *testing.T) {
	details := []string{"  bash → error: context deadline exceeded"}
	cats := categorizeErrors(details)
	if len(cats) < 1 || cats[0].label != "超时" {
		t.Fatalf("expected 超时, got %+v", cats)
	}
}

func TestCategorizeErrors_Blocked(t *testing.T) {
	details := []string{"  write_file → blocked: [loop guard] repeated success"}
	cats := categorizeErrors(details)
	if len(cats) < 1 || cats[0].label != "被阻止" {
		t.Fatalf("expected 被阻止, got %+v", cats)
	}
}

func TestCategorizeErrors_GenericFallback(t *testing.T) {
	details := []string{"  bash → error: something unexpected happened"}
	cats := categorizeErrors(details)
	if len(cats) < 1 || cats[0].label != "通用错误" {
		t.Fatalf("expected 通用错误 fallback, got %+v", cats)
	}
}

func TestCategorizeErrors_Dedup(t *testing.T) {
	// Two file_missing errors should only produce one category entry
	details := []string{
		"  read_file → error: no such file: a.go",
		"  read_file → error: no such file: b.go",
	}
	cats := categorizeErrors(details)
	if len(cats) < 1 || cats[0].label != "文件缺失" {
		t.Fatalf("expected 文件缺失, got %+v", cats)
	}
	// Should not have duplicates
	seen := map[string]bool{}
	for _, c := range cats {
		if seen[c.label] {
			t.Fatalf("duplicate category: %s", c.label)
		}
		seen[c.label] = true
	}
}

// ── 合并自 steer_test.go：全失败批次的两级机制 ─────────────────────

// TestToolFeedback_AllBlockedIsNotFailure 移植自 steer_test.go。
// 全部 blocked → 不是失败，不触发反馈。
func TestToolFeedback_AllBlockedIsNotFailure(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{
		{Name: "write_file"}, {Name: "edit_file"},
	}
	results := []string{
		"blocked: write_file denied",
		"blocked: edit_file denied",
	}
	if a.maybeInjectToolFeedback(calls, results) {
		t.Error("all-blocked should NOT trigger feedback")
	}
}

// TestToolFeedback_SingleFailureNotTriggered 移植自 steer_test.go。
func TestToolFeedback_SingleFailureNotTriggered(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{
		{Name: "read_file"}, {Name: "read_file"},
	}
	results := []string{
		"error: no such file",
		"file content here",
	}
	if a.maybeInjectToolFeedback(calls, results) {
		t.Error("single failure should NOT trigger feedback")
	}
}

// TestToolFeedback_FirmSteerAfterThreeAllFail 验证合并后的强硬模式：
// 全部非 blocked 调用失败 + 连续 >=3 轮 → “停下来重新评估方案”
// （V10.152：温和模式已删除，前两轮不注入，第三轮才注入一次）。
func TestToolFeedback_FirmSteerAfterThreeAllFail(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{
		{Name: "read_file"}, {Name: "read_file"},
	}
	results := []string{
		"error: fail1",
		"error: fail2",
	}

	// Rounds 1-2: no injection (soft mode removed).
	if a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("round 1 must not inject (streak below threshold)")
	}
	if a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("round 2 must not inject (streak below threshold)")
	}
	if len(s.Messages) != 0 {
		t.Fatalf("no message should be added before the threshold, got %d", len(s.Messages))
	}

	// Round 3: firm STOP steer.
	if !a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("round 3 all-fail streak must inject the STOP steer")
	}
	msg := s.Messages[len(s.Messages)-1]
	if !strings.Contains(msg.Content, "停下来重新评估方案") {
		t.Errorf("round 3 should say 停下来重新评估方案, got: %s", msg.Content)
	}
}

// TestToolFeedback_NonAllFailNoInject 验证非全败（混合成功+失败）不注入：
// 温和模式已删除（V10.152），只有全部失败连续 3 轮才发宿主 STOP 引导。
func TestToolFeedback_NonAllFailNoInject(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{
		{Name: "bash"}, {Name: "read_file"}, {Name: "write_file"},
	}
	results := []string{
		"error: fail1",
		"error: fail2",
		"success output",
	}

	if a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("mixed success+fail must not inject (soft mode removed)")
	}
	if len(s.Messages) != 0 {
		t.Fatalf("no message should be added, got %d", len(s.Messages))
	}
}

// TestToolFeedbackResetsPerTurn 验证新 turn 开始时 toolFeedbackCount 重置为 0：
// 连击计数是 per-turn 的，新 turn 从零开始重新累计到 3 轮才注入。
func TestToolFeedbackResetsPerTurn(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{{Name: "bash"}, {Name: "read_file"}}
	results := []string{"error: fail", "error: also fail"}

	// Turn 1: streak reaches the threshold at round 3 and injects once.
	for i := 0; i < ToolFeedbackCap-1; i++ {
		if a.maybeInjectToolFeedback(calls, results) {
			t.Fatalf("round %d must not inject before threshold", i+1)
		}
	}
	if !a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("round 3 must inject")
	}
	if a.toolFeedbackCount != 0 {
		t.Fatalf("counter resets after injection, got %d", a.toolFeedbackCount)
	}

	// Simulate turn 2 start: explicitly reset (what runDirect does).
	a.toolFeedbackCount = 0

	// Turn 2 restarts the streak from zero.
	if a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("turn 2 round 1 must not inject (fresh streak)")
	}
	if a.toolFeedbackCount != 1 {
		t.Fatalf("after reset, counter should be 1, got %d", a.toolFeedbackCount)
	}
}
