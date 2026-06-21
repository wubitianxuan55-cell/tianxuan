package agent

import "testing"

func TestClassifyMode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// explore cases
		{
			name:  "simple question",
			input: "how does the cache system work?",
			want:  "explore",
		},
		{
			name:  "chinese simple question",
			input: "怎么使用这个工具",
			want:  "explore",
		},
		{
			name:  "explain request",
			input: "explain the architecture of the agent package",
			want:  "explore",
		},
		{
			name:  "short non-action input",
			input: "what files are in the internal directory?",
			want:  "explore",
		},
		{
			name:  "code review request",
			input: "code review this file for security issues",
			want:  "explore",
		},
		{
			name:  "understanding question",
			input: "i want to understand how compact works",
			want:  "explore",
		},

		// orchestrate cases
		{
			name:  "complex multi-step",
			input: "implement a new authentication system with JWT tokens across the frontend and backend. This involves multiple files including the API layer, middleware, and database schema changes.",
			want:  "orchestrate",
		},
		{
			name:  "refactor across files",
			input: "refactor the error handling across all handler files. There are several files in the API package that need updating, and we also need to update the frontend error display.",
			want:  "orchestrate",
		},
		{
			name:  "chinese complex task",
			input: "重构整个缓存系统，涉及 internal/cache/ 下的多个文件，还需要更新 agent 和 provider 层的集成代码",
			want:  "orchestrate",
		},
		{
			name: "numbered multi-step request",
			input: `Please help me with the following:
1. Add a new API endpoint for user profiles
2. Update the frontend to use the new endpoint
3. Add tests for the new endpoint
4. Update the API documentation`,
			want: "orchestrate",
		},

		// develop cases (default)
		{
			name:  "single file edit",
			input: "fix the typo in agent.go line 42",
			want:  "develop",
		},
		{
			name:  "add a feature",
			input: "add a new tool for reading JSON files",
			want:  "develop",
		},
		{
			name:  "simple fix",
			input: "修复 compact.go 中的编译错误",
			want:  "develop",
		},
		{
			name:  "run command",
			input: "run go test ./internal/agent/...",
			want:  "develop",
		},
		{
			name:  "empty input",
			input: "",
			want:  "develop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyMode(tt.input)
			if got != tt.want {
				t.Errorf("ClassifyMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestClassifyModeWithScore(t *testing.T) {
	// 验证评分函数不panic且返回有效模式
	inputs := []string{
		"how does this work?",
		"implement a complex multi-file refactor across the entire codebase with frontend and backend changes",
		"fix the bug in agent.go",
		"",
	}
	for _, input := range inputs {
		ms := ClassifyModeWithScore(input)
		if ms.Mode != "explore" && ms.Mode != "develop" && ms.Mode != "orchestrate" {
			t.Errorf("ClassifyModeWithScore(%q) returned invalid mode %q", input, ms.Mode)
		}
		if len(ms.Reasons) == 0 {
			t.Errorf("ClassifyModeWithScore(%q) returned empty reasons", input)
		}
	}
}

func TestClassifyModeDeterministic(t *testing.T) {
	// 验证纯确定性：相同输入 → 相同输出
	input := "refactor the cache system across multiple files in the backend"
	first := ClassifyMode(input)
	for i := 0; i < 100; i++ {
		if got := ClassifyMode(input); got != first {
			t.Fatalf("非确定性：第 %d 次调用 ClassifyMode(%q) = %q, want %q", i+1, input, got, first)
		}
	}
}
