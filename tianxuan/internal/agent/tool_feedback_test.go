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

func TestMaybeInjectToolFeedback_TwoErrors(t *testing.T) {
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

	preLen := len(s.Messages)
	if !a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("should inject feedback for 2+ errors")
	}
	if len(s.Messages) != preLen+1 {
		t.Fatalf("expected 1 message added, got %d", len(s.Messages)-preLen)
	}

	msg := s.Messages[len(s.Messages)-1]
	if msg.Role != provider.RoleUser {
		t.Fatalf("expected user role, got %s", msg.Role)
	}
	if !strings.Contains(msg.Content, "2 个失败") {
		t.Fatalf("feedback should mention 2 failures, got: %s", msg.Content)
	}
	if !strings.Contains(msg.Content, "[system]") {
		t.Fatal("feedback should have [system] prefix")
	}
}

func TestMaybeInjectToolFeedback_CapAtThree(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{{Name: "bash"}, {Name: "read_file"}}
	results := []string{"error: fail", "error: also fail"}

	// First 3 injections should fire.
	for i := 0; i < ToolFeedbackCap; i++ {
		if !a.maybeInjectToolFeedback(calls, results) {
			t.Fatalf("injection %d should have fired", i+1)
		}
	}

	// 4th call should be capped.
	if a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("4th consecutive injection should be capped")
	}
}

func TestMaybeInjectToolFeedback_ResetOnSuccess(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	badCalls := []provider.ToolCall{{Name: "bash"}}
	badResults := []string{"error: fail"}

	// 1 error — below threshold, should reset counter (stays 0).
	a.maybeInjectToolFeedback(badCalls, badResults)
	if a.toolFeedbackCount != 0 {
		t.Fatalf("counter should be 0 after below-threshold round, got %d", a.toolFeedbackCount)
	}

	// Now 2 errors — should inject.
	badCalls2 := []provider.ToolCall{{Name: "bash"}, {Name: "read_file"}}
	badResults2 := []string{"error: fail", "error: fail2"}
	if !a.maybeInjectToolFeedback(badCalls2, badResults2) {
		t.Fatal("should inject for 2 errors")
	}
	if a.toolFeedbackCount != 1 {
		t.Fatalf("counter should be 1, got %d", a.toolFeedbackCount)
	}

	// Now 0 errors — should reset.
	goodResults := []string{"success"}
	goodCalls := []provider.ToolCall{{Name: "bash"}}
	a.maybeInjectToolFeedback(goodCalls, goodResults)
	if a.toolFeedbackCount != 0 {
		t.Fatalf("counter should reset to 0 on success, got %d", a.toolFeedbackCount)
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

func TestMaybeInjectToolFeedback_BlockedResults(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{
		{Name: "write_file"},
		{Name: "edit_file"},
	}
	results := []string{
		"error: permission denied",
		"blocked: [loop guard] already succeeded",
	}

	if !a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("should inject feedback for blocked + error")
	}
	msg := s.Messages[len(s.Messages)-1]
	if !strings.Contains(msg.Content, "blocked") || !strings.Contains(msg.Content, "error") {
		t.Fatalf("feedback should mention both blocked and error, got: %s", msg.Content)
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

// TestToolFeedback_RealFailuresTrigger 移植自 steer_test.go。
func TestToolFeedback_RealFailuresTrigger(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{
		{Name: "read_file"}, {Name: "read_file"},
	}
	results := []string{
		"error: no such file",
		"error: permission denied",
	}
	if !a.maybeInjectToolFeedback(calls, results) {
		t.Error("real failures should trigger feedback")
	}
}

// TestToolFeedback_MixedBlockedAndFailed 移植自 steer_test.go。
func TestToolFeedback_MixedBlockedAndFailed(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{
		{Name: "write_file"}, {Name: "read_file"}, {Name: "read_file"},
	}
	results := []string{
		"blocked: writer denied",
		"error: no such file",
		"error: permission denied",
	}
	if !a.maybeInjectToolFeedback(calls, results) {
		t.Error("mixed blocked+real failures (2 real) should trigger")
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
// 全部非 blocked 调用失败 + 连续 >=3 轮 → "停下来重新评估方案"
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

	// Round 1 — gentle: "尝试不同方法"
	a.maybeInjectToolFeedback(calls, results)
	msg1 := s.Messages[len(s.Messages)-1]
	if !strings.Contains(msg1.Content, "尝试不同方法") {
		t.Errorf("round 1 (all-fail, count=1) should say 尝试不同方法, got: %s", msg1.Content)
	}

	// Round 2 — standard (non-all-fail condition not met? No — it IS all-fail but count=2, not 1 or >=3)
	a.maybeInjectToolFeedback(calls, results)
	msg2 := s.Messages[len(s.Messages)-1]
	// count=2 falls through to standard feedback (not all-fail gentle, not firm)
	if strings.Contains(msg2.Content, "停下来重新评估") {
		t.Error("round 2 (count=2) should NOT trigger firm steer")
	}

	// Round 3 — firm: "停下来重新评估方案"
	a.maybeInjectToolFeedback(calls, results)
	msg3 := s.Messages[len(s.Messages)-1]
	if !strings.Contains(msg3.Content, "停下来重新评估") {
		t.Errorf("round 3 (count=3) should trigger firm steer, got: %s", msg3.Content)
	}
}

// TestToolFeedback_NonAllFailUsesStandard 验证非全失败时用标准消息（不触发强硬/温和模式）。
func TestToolFeedback_NonAllFailUsesStandard(t *testing.T) {
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

	// 2 errors out of 3 — ≥2 but not all-fail.
	if !a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("should trigger for 2 errors")
	}
	msg := s.Messages[len(s.Messages)-1]
	// Should use standard message — no all-fail prefix.
	if strings.Contains(msg.Content, "全部操作都失败") || strings.Contains(msg.Content, "停下来重新评估") {
		t.Errorf("mixed success+fail should use standard message, got: %s", msg.Content)
	}
	if !strings.Contains(msg.Content, "2 个失败") {
		t.Error("standard message should mention error count")
	}
}

// TestToolFeedbackResetsPerTurn 验证新 turn 开始时 toolFeedbackCount 重置为 0。
// 防止 turn 1 耗尽 cap 后 turn 2 被错误抑制。
func TestToolFeedbackResetsPerTurn(t *testing.T) {
	s := NewSession("")
	a := &AgentRunner{session: s}

	calls := []provider.ToolCall{{Name: "bash"}, {Name: "read_file"}}
	results := []string{"error: fail", "error: also fail"}

	// Simulate turn 1: exhaust the cap.
	for i := 0; i < ToolFeedbackCap; i++ {
		if !a.maybeInjectToolFeedback(calls, results) {
			t.Fatalf("injection %d should have fired", i+1)
		}
	}
	if a.toolFeedbackCount != ToolFeedbackCap {
		t.Fatalf("counter should be %d after exhausting cap, got %d", ToolFeedbackCap, a.toolFeedbackCount)
	}

	// Simulate turn 2 start: explicitly reset (what runDirect does).
	a.toolFeedbackCount = 0

	// Turn 2 should get fresh feedback.
	if !a.maybeInjectToolFeedback(calls, results) {
		t.Fatal("after reset, turn 2 should get fresh feedback")
	}
	if a.toolFeedbackCount != 1 {
		t.Fatalf("after reset, counter should be 1, got %d", a.toolFeedbackCount)
	}
}
