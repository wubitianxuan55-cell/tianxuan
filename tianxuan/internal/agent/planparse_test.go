package agent

import "testing"

// planparse.go 统一计划解析器的行为契约。isStepLine / extractStepTitle
// 曾被 hermes_confirm.go 与 hermes_sdd.go 各自实现（行为重复），此处
// 固定单一实现的语义，防止计划格式演进时三处不同步。

func TestIsStepLine_Chinese(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"步骤 1：写失败测试", true},
		{"步骤 2: 实现", true},
		{"步骤 10：多位数", true},
		{"步骤 1 无冒号", true},
		// 缩进由调用方 TrimSpace 处理——与历史实现一致
		{"  步骤 3：带缩进", false},
		{"步骤 abc：不是数字", false},
		{"步骤 ", false},
		{"步骤", false},
		{"- **Delta** ADDED", false},
		{"## 提案", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isStepLine(c.line); got != c.want {
			t.Errorf("isStepLine(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func TestIsStepLine_English(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"Step 1: write failing test", true},
		{"Step 2：Chinese colon", true},
		{"Step 12: multi-digit", true},
		{"Step 1 no colon", true},
		{"Step x: not a number", false},
		{"Steps 1: plural", false},
	}
	for _, c := range cases {
		if got := isStepLine(c.line); got != c.want {
			t.Errorf("isStepLine(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func TestExtractStepTitle(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"步骤 1：写失败测试", "写失败测试"},
		{"步骤 2: 实现", "实现"},
		{"步骤 1 无冒号", "无冒号"},
		{"Step 1: write failing test", "write failing test"},
		{"Step 2：Chinese colon", "Chinese colon"},
		{"步骤 12：多位数", "多位数"},
		// 非步骤行原样返回（沿用历史行为，调用方先经 isStepLine 判定）
		{"步骤 ", "步骤 "},
		{"- **Delta** ADDED", "- **Delta** ADDED"},
	}
	for _, c := range cases {
		if got := extractStepTitle(c.line); got != c.want {
			t.Errorf("extractStepTitle(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}
