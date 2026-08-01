package evidence

import "testing"

// 复现：执行者的 complete_step.step 常写成 "步骤 2：标题" / "Step 3"
// 或带编号前缀的标题，而 todo_write 的 Content 可能是标题原文。旧的
// 匹配只认纯数字索引 + 完全相等标题，导致 todo 进度落后于实际完成。

func TestMatchTodoStep_ChineseNumberPrefix(t *testing.T) {
	todos := []TodoItem{
		{Content: "写失败测试", Status: "pending"},
		{Content: "实现功能", Status: "pending"},
		{Content: "验证", Status: "pending"},
	}
	for _, step := range []string{"步骤 2：实现功能", "步骤2:实现功能", "步骤 2", "步骤2"} {
		m, ok := MatchStep(step, todos)
		if !ok || m.Index != 2 {
			t.Errorf("MatchStep(%q) should resolve to todo #2, got found=%v index=%d", step, ok, m.Index)
		}
	}
}

func TestMatchTodoStep_EnglishNumberPrefix(t *testing.T) {
	todos := []TodoItem{
		{Content: "write failing test", Status: "pending"},
		{Content: "implement", Status: "pending"},
		{Content: "verify", Status: "pending"},
	}
	for _, step := range []string{"Step 3: verify", "Step 3", "step 3"} {
		m, ok := MatchStep(step, todos)
		if !ok || m.Index != 3 {
			t.Errorf("MatchStep(%q) should resolve to todo #3, got found=%v index=%d", step, ok, m.Index)
		}
	}
}

func TestMatchTodoStep_PrefixedTitleVsPlainTitle(t *testing.T) {
	// todo Content 带编号前缀，complete_step 用纯标题（或反向）
	todos := []TodoItem{
		{Content: "步骤 1：写失败测试", Status: "pending"},
		{Content: "步骤 2：实现功能", Status: "pending"},
	}
	if m, ok := MatchStep("写失败测试", todos); !ok || m.Index != 1 {
		t.Errorf("plain title should match prefixed todo #1, got found=%v index=%d", ok, m.Index)
	}
	todos2 := []TodoItem{
		{Content: "写失败测试", Status: "pending"},
	}
	if m, ok := MatchStep("步骤 1：写失败测试", todos2); !ok || m.Index != 1 {
		t.Errorf("prefixed step should match plain todo #1, got found=%v index=%d", ok, m.Index)
	}
}

func TestMatchTodoStep_PlainNumberStillWorks(t *testing.T) {
	todos := []TodoItem{
		{Content: "a", Status: "pending"},
		{Content: "b", Status: "pending"},
	}
	if m, ok := MatchStep("2.", todos); !ok || m.Index != 2 {
		t.Errorf("plain number should still resolve, got found=%v index=%d", ok, m.Index)
	}
	if m, ok := MatchStep("2", todos); !ok || m.Index != 2 {
		t.Errorf("plain number should still resolve, got found=%v index=%d", ok, m.Index)
	}
}

func TestMatchTodoStep_NoOvermatching(t *testing.T) {
	// 不引入包含匹配：不同标题不得匹配，避免误推进
	todos := []TodoItem{
		{Content: "实现用户认证模块", Status: "pending"},
		{Content: "实现", Status: "pending"},
	}
	if m, ok := MatchStep("实现用户认证模块", todos); !ok || m.Index != 1 {
		t.Fatalf("exact title should match #1, got found=%v index=%d", ok, m.Index)
	}
	// "认证" 不是任何 todo 的标题（无包含匹配）
	if m, ok := MatchStep("认证", todos); ok {
		t.Errorf("substring must not match any todo (got index=%d)", m.Index)
	}
	// 数字出现在标题中间（file2.txt）不得被当作索引
	if m, ok := MatchStep("更新 file2.txt", todos); ok {
		t.Errorf("number inside a title must not be treated as an index (got index=%d)", m.Index)
	}
}
