# 确定性 Checkpoint 实现计划

> **给 agentic worker：** 使用 subagent-driven-dev 或 executing-plans 实现此计划。

**目标：** 在 compact 时将会话状态确定性地持久化到磁盘，overflow 时用于重建上下文，提供比纯 compact 摘要更丰富的语义连续性。

**架构：** 复用已有的 `buildCompactSummary()` 输出 + 新增 todo 快照提取 + execution state 快照，序列化为 JSON 写入文件。Rebuild 时读回并注入为 user message。全链路确定性，零 LLM 依赖。

**技术栈：** Go stdlib `encoding/json` + `os`，无新依赖。

**缓存安全：**
- checkpoint 写入是纯文件系统操作，不影响消息流 ✅
- checkpoint 内容是确定性的（`buildCompactSummary` + 结构化提取）✅
- rebuild 后的新前缀 `[L1]+[msgs[1]]+[checkpoint user msg]+[tail]` 全部确定 ✅
- rebuild 后首轮必然 cache miss（上下文重置），后续轮次可命中 ✅

---

## 文件结构

| 文件 | 操作 | 职责 |
|------|------|------|
| `agent/checkpoint.go` | 新增 | CheckpointData 结构体 + Write/Load/Rebuild |
| `agent/checkpoint_test.go` | 新增 | 单元测试 |
| `agent/agent.go` | 修改 | CompactionConfig 增加 CheckpointDir |
| `agent/compact.go` | 修改 | maybeCompact 前调用 checkpoint.Write |

---

## 步骤

### step-1: 创建 checkpoint.go — 核心结构

```go
// agent/checkpoint.go
package agent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"tianxuan/internal/provider"
)

// CheckpointData is a deterministic snapshot of session state saved before
// compaction. It is serialized as JSON — same input always produces the same
// bytes, so the prefix after a checkpoint rebuild stays cache-stable.
type CheckpointData struct {
	// Summary is the deterministic compact summary (from buildCompactSummary).
	Summary string `json:"summary"`

	// TodoSnaps captures the current todo list from the most recent todo_write.
	Todos []checkpointTodo `json:"todos,omitempty"`

	// EditFiles lists files modified so far (extracted from tool calls).
	EditFiles []string `json:"edit_files,omitempty"`

	// Goal is the current execution goal (from L2 ExecutionState).
	Goal string `json:"goal,omitempty"`

	// TruncateCount is how many times compaction has run (carried forward).
	TruncateCount int `json:"truncate_count"`
}

type checkpointTodo struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

// extractTodos scans session messages for the last todo_write call and
// returns a snapshot of the todo items (deterministic extraction).
func extractTodos(msgs []provider.Message) []checkpointTodo {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != provider.RoleAssistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			if tc.Name != "todo_write" {
				continue
			}
			var p struct {
				Todos []checkpointTodo `json:"todos"`
			}
			if err := json.Unmarshal([]byte(tc.Arguments), &p); err != nil {
				continue
			}
			return p.Todos
		}
	}
	return nil
}

// extractEditFiles scans truncated messages for file edit operations
// (deterministic, same logic as buildCompactSummary uses internally).
func extractEditFiles(msgs []provider.Message) []string {
	seen := make(map[string]bool)
	var files []string
	for _, msg := range msgs {
		for _, tc := range msg.ToolCalls {
			switch tc.Name {
			case "edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol":
				path := extractFilePath(tc.Name, tc.Arguments)
				if path != "" && !seen[path] {
					files = append(files, path)
					seen[path] = true
				}
			}
		}
	}
	return files
}

// WriteCheckpoint saves the current session state to dir/checkpoint.json.
// Called before compaction. Returns nil when dir is empty (checkpoint disabled).
func (a *AgentRunner) WriteCheckpoint(dir string) error {
	if dir == "" {
		return nil
	}
	msgs := a.session.Messages
	keepFrom := len(msgs) - a.compaction.RecentKeep
	if keepFrom < 1 {
		keepFrom = 1
	}

	cp := CheckpointData{
		Summary:       buildCompactSummary(msgs[1:keepFrom]),
		Todos:         extractTodos(msgs),
		EditFiles:     extractEditFiles(msgs[1:keepFrom]),
		TruncateCount: a.compaction.TruncateCount + 1,
	}
	if a.ctxMgr != nil {
		cp.Goal = a.ctxMgr.CurrentGoal()
	}
	if cp.Summary == "" && len(cp.Todos) == 0 && len(cp.EditFiles) == 0 {
		return nil // nothing worth checkpointing
	}

	data, err := json.Marshal(cp)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "checkpoint.json"), data, 0644)
}

// LoadCheckpoint reads the last checkpoint from dir. Returns nil when
// no checkpoint exists or dir is empty.
func LoadCheckpoint(dir string) *CheckpointData {
	if dir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "checkpoint.json"))
	if err != nil {
		return nil
	}
	var cp CheckpointData
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil
	}
	if cp.Summary == "" && len(cp.Todos) == 0 {
		return nil
	}
	return &cp
}

// formatForLLM renders the checkpoint as a structured user message for
// injection during rebuild. Deterministic output for a given CheckpointData.
func (cp *CheckpointData) formatForLLM() string {
	var parts []string
	parts = append(parts, "[Earlier conversation — checkpoint snapshot:")

	if cp.Summary != "" {
		parts = append(parts, cp.Summary)
	}
	if len(cp.Todos) > 0 {
		parts = append(parts, "- Active todo list:")
		for _, t := range cp.Todos {
			icon := " "
			switch t.Status {
			case "completed":
				icon = "✓"
			case "in_progress":
				icon = "▶"
			case "pending":
				icon = "○"
			}
			parts = append(parts, "  "+icon+" "+t.Content)
		}
	}
	if cp.Goal != "" {
		parts = append(parts, "- Goal: "+cp.Goal)
	}
	if len(cp.EditFiles) > 0 {
		parts = append(parts, "- Files modified: "+stringsJoin(cp.EditFiles, ", "))
	}
	parts = append(parts, "]")
	return stringsJoin(parts, "\n")
}

func stringsJoin(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for _, s := range ss[1:] {
		result += sep + s
	}
	return result
}
```

### step-2: 添加 CurrentGoal 方法到 ContextManager

```go
// context/manager.go 新增方法

func (m *ContextManager) CurrentGoal() string {
    if m == nil || m.runtime == nil {
        return ""
    }
    return m.runtime.Goal()
}
```

```go
// cache/runtime.go 新增方法

func (rc *RuntimeLayer) Goal() string {
    rc.mu.Lock()
    defer rc.mu.Unlock()
    return rc.session.Execution.Goal
}
```

### step-3: CompactionConfig 增加 CheckpointDir 字段

```go
// agent/agent.go CompactionConfig 结构体添加字段：

type CompactionConfig struct {
    // ... 现有字段 ...
    
    // CheckpointDir is where deterministic checkpoint snapshots are persisted.
    // Empty means checkpoint is disabled. (V5.31)
    CheckpointDir string
}
```

### step-4: maybeCompact 中调用 WriteCheckpoint

在 `compact.go` 的 `a.session.Replace(replacement)` 之前：

```go
// Write checkpoint BEFORE replacing messages (we need the pre-compaction
// message list to extract todos and edit files).
_ = a.WriteCheckpoint(a.compaction.CheckpointDir)
```

### step-5: 编写测试

```go
// agent/checkpoint_test.go
// 测试：Write/Load round-trip, empty checkpoint, todo extraction, formatForLLM
```

### step-6: 运行测试并 vet 验证
