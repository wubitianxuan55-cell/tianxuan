package cache

import (
	"strings"
	"testing"
)

func TestRuntimeLayerEmptyByDefault(t *testing.T) {
	rc := NewRuntimeLayer()
	if rc.SystemPrompt() != "" {
		t.Errorf("empty RuntimeLayer SystemPrompt = %q, want empty", rc.SystemPrompt())
	}
	if rc.IsLocked() {
		t.Error("new RuntimeLayer should not be locked")
	}
}

func TestRuntimeLayerApplyRoute(t *testing.T) {
	rc := NewRuntimeLayer()
	cfg := DomainConfig{
		Kind:         KindFixBug,
		ContextFocus: []string{"internal/cache/", "cmd/main.go"},
	}
	rc.ApplyRoute(cfg)

	sys := rc.SystemPrompt()
	// V3.0: ApplyRoute stores kind in Execution.Goal and focus in ActiveModule
	if !strings.Contains(sys, "Goal: fix_bug") {
		t.Error("SystemPrompt missing Goal (was Task type)")
	}
	if !strings.Contains(sys, "Active module: internal/cache/, cmd/main.go") {
		t.Error("SystemPrompt missing Active module (was Focus Areas)")
	}
}

func TestRuntimeLayerLockPreventsMutation(t *testing.T) {
	rc := NewRuntimeLayer()
	rc.ApplyRoute(DomainConfig{Kind: KindFixBug, ContextFocus: []string{"internal/"}})
	first := rc.SystemPrompt()
	rc.Lock()

	// Second ApplyRoute should be ignored
	rc.ApplyRoute(DomainConfig{Kind: KindExplain, ContextFocus: []string{"cmd/"}})
	sys := rc.SystemPrompt()
	if sys != first {
		t.Errorf("locked RuntimeLayer should ignore ApplyRoute: got %q, want %q", sys, first)
	}
	if strings.Contains(sys, "cmd/") {
		t.Error("locked RuntimeLayer should not contain second ApplyRoute content")
	}
	if !strings.Contains(sys, "internal/") {
		t.Error("locked RuntimeLayer should keep first ApplyRoute content")
	}
}

func TestRuntimeLayerAppendHint(t *testing.T) {
	rc := NewRuntimeLayer()
	rc.AppendHint("避免使用 bash 读取文件")

	// V5.25: hints 不再出现在 SystemPrompt() 中——通过 TurnTailHints() 获取
	sys := rc.SystemPrompt()
	if strings.Contains(sys, "避免使用 bash") {
		t.Errorf("SystemPrompt must NOT contain hints (breaks cache), got: %q", sys)
	}

	hints := rc.TurnTailHints()
	if len(hints) != 1 || hints[0] != "避免使用 bash 读取文件" {
		t.Errorf("TurnTailHints() = %v, want [避免使用 bash 读取文件]", hints)
	}
}

func TestRuntimeLayerIsLockedAfterLock(t *testing.T) {
	rc := NewRuntimeLayer()
	if rc.IsLocked() {
		t.Error("should not be locked initially")
	}
	rc.Lock()
	if !rc.IsLocked() {
		t.Error("should be locked after Lock()")
	}
}

func TestRuntimeLayerSystemPromptEmptyWhenNoContent(t *testing.T) {
	rc := NewRuntimeLayer()
	rc.Lock() // locking empty context should still produce empty prompt
	if rc.SystemPrompt() != "" {
		t.Errorf("empty locked RuntimeLayer SystemPrompt = %q, want empty", rc.SystemPrompt())
	}
}

// TestSystemPromptExcludesRecentEdits 验证 L2 SystemPrompt 不包含可变 RecentEdits。
// V3.0 修复回归检查：RecentEdits 每轮变化会破坏 DeepSeek 前缀缓存，
// 必须移到 turn-tail 注入，不能出现在缓存稳定的 L2 系统消息中。
func TestSystemPromptExcludesRecentEdits(t *testing.T) {
	rc := NewRuntimeLayer()
	rc.TrackEdit("internal/cache/runtime.go")
	rc.TrackEdit("cmd/main.go")

	sys := rc.SystemPrompt()
	if strings.Contains(sys, "Recent edits") {
		t.Errorf("SystemPrompt must NOT contain 'Recent edits' (breaks DeepSeek prefix cache), got: %q", sys)
	}
	if strings.Contains(sys, "internal/cache/runtime.go") {
		t.Errorf("SystemPrompt must NOT contain tracked file paths, got: %q", sys)
	}
}

// TestSystemPromptExcludesActiveFiles 验证 L2 SystemPrompt 不包含可变 ActiveFiles。
// ExecutionState.ActiveFiles 每轮可能变化，出现在 L2 中同样会破坏缓存。
func TestSystemPromptExcludesActiveFiles(t *testing.T) {
	rc := NewRuntimeLayer()
	rc.UpdateExecution(ExecutionState{
		Goal:        "fix cache bug",
		ActiveFiles: []string{"runtime.go", "runtime_test.go"},
	})

	sys := rc.SystemPrompt()
	if strings.Contains(sys, "Active files") {
		t.Errorf("SystemPrompt must NOT contain 'Active files' (breaks cache), got: %q", sys)
	}
	if strings.Contains(sys, "runtime.go") {
		t.Errorf("SystemPrompt must NOT contain active file paths, got: %q", sys)
	}
}

// TestSetPromptHint 验证 SetPromptHint 在 Lock 前可设、Lock 后忽略。
// V5.30: PromptHint 注入 L2 Guidelines 段，Lock 后锁定保证缓存稳定。
func TestSetPromptHint(t *testing.T) {
	rc := NewRuntimeLayer()
	rc.SetPromptHint("批量工具调用以节省轮次")

	sys := rc.SystemPrompt()
	if !strings.Contains(sys, "批量工具调用以节省轮次") {
		t.Errorf("SystemPrompt must contain PromptHint, got: %q", sys)
	}

	// Lock 后 SetPromptHint 应被忽略
	rc.Lock()
	first := rc.SystemPrompt()
	rc.SetPromptHint("这个提示不应出现")
	sys2 := rc.SystemPrompt()
	if sys2 != first {
		t.Errorf("locked SystemPrompt changed after SetPromptHint: %q → %q", first, sys2)
	}
}

// TestSetCompactL2 验证 SetCompactL2 切换格式，Lock 后不可改。
func TestSetCompactL2(t *testing.T) {
	rc := NewRuntimeLayer()
	rc.SetProject(ProjectState{Language: "go", Module: "test"})
	rc.ApplyRoute(DomainConfig{Kind: KindFixBug, ContextFocus: []string{"internal/cache/"}})

	// 默认 verbose 格式
	sysVerbose := rc.SystemPrompt()
	if !strings.Contains(sysVerbose, "## Project") {
		t.Error("default SystemPrompt should use verbose Markdown format")
	}

	// 切换紧凑格式
	rc.SetCompactL2(true)
	sysCompact := rc.SystemPrompt()
	if strings.Contains(sysCompact, "## Project") {
		t.Error("compact SystemPrompt must NOT contain Markdown headers")
	}
	if !strings.Contains(sysCompact, "@p") {
		t.Error("compact SystemPrompt must contain @p project line")
	}

	// Lock 后不可改回
	rc.Lock()
	rc.SetCompactL2(false)
	sysAfterLock := rc.SystemPrompt()
	if sysAfterLock != sysCompact {
		t.Errorf("locked compact SystemPrompt changed after SetCompactL2(false): %q → %q", sysCompact, sysAfterLock)
	}
}

// TestCompactL2Format 验证紧凑格式的正确输出结构。
func TestCompactL2Format(t *testing.T) {
	rc := NewRuntimeLayer()
	rc.SetProject(ProjectState{
		Module:     "tianxuan",
		Language:   "go",
		TotalFiles: 120,
	})
	rc.ApplyRoute(DomainConfig{
		Kind:         KindFixBug,
		ContextFocus: []string{"internal/cache/"},
	})
	rc.SetCompactL2(true)

	sys := rc.SystemPrompt()

	// 必须包含 @p 项目行
	if !strings.Contains(sys, "@p") || !strings.Contains(sys, "m=tianxuan") || !strings.Contains(sys, "l=go") {
		t.Errorf("compact @p line missing fields, got: %q", sys)
	}
	if !strings.Contains(sys, "files=120") {
		t.Errorf("compact @p line missing files count, got: %q", sys)
	}

	// 必须包含 @w 工作区行
	if !strings.Contains(sys, "@w") || !strings.Contains(sys, "mod=internal/cache/") {
		t.Errorf("compact @w line missing, got: %q", sys)
	}

	// 必须包含 @g 目标行
	if !strings.Contains(sys, "@g") || !strings.Contains(sys, "fix_bug") {
		t.Errorf("compact @g goal line missing, got: %q", sys)
	}
}

// TestCompactL2Idempotent 验证紧凑格式同输入同输出（缓存稳定性）。
func TestCompactL2Idempotent(t *testing.T) {
	rc1 := NewRuntimeLayer()
	rc1.SetProject(ProjectState{Module: "test", Language: "go", TotalFiles: 42})
	rc1.ApplyRoute(DomainConfig{Kind: KindWriteFeature})
	rc1.SetCompactL2(true)
	rc1.Lock()

	rc2 := NewRuntimeLayer()
	rc2.SetProject(ProjectState{Module: "test", Language: "go", TotalFiles: 42})
	rc2.ApplyRoute(DomainConfig{Kind: KindWriteFeature})
	rc2.SetCompactL2(true)
	rc2.Lock()

	if rc1.SystemPrompt() != rc2.SystemPrompt() {
		t.Errorf("compact SystemPrompt not idempotent:\n  %q\n  %q", rc1.SystemPrompt(), rc2.SystemPrompt())
	}
}

// TestCompactL2Empty 验证空 RuntimeLayer 的紧凑格式返回空字符串。
func TestCompactL2Empty(t *testing.T) {
	rc := NewRuntimeLayer()
	rc.SetCompactL2(true)
	rc.Lock()

	if rc.SystemPrompt() != "" {
		t.Errorf("empty compact SystemPrompt should be empty, got: %q", rc.SystemPrompt())
	}
}

// TestRecentEditsGetter 验证 RuntimeLayer 提供 RecentEdits 的公开读取方法，
// 供 controller 通过 turn-tail 注入到用户消息末尾。
func TestRecentEditsGetter(t *testing.T) {
	rc := NewRuntimeLayer()
	rc.TrackEdit("foo.go")
	rc.TrackEdit("bar.go")

	edits := rc.RecentEdits()
	if len(edits) != 2 {
		t.Fatalf("RecentEdits() len = %d, want 2", len(edits))
	}
	if edits[0].Path != "foo.go" || edits[1].Path != "bar.go" {
		t.Errorf("RecentEdits() paths = %v, want [foo.go bar.go]", []string{edits[0].Path, edits[1].Path})
	}
}
