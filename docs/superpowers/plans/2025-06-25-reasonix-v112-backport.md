# Reasonix V1.12 回移植优化实施计划

> **面向 AI 代理的工作者：** 使用 executing-plans 技能逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将 DeepSeek-Reasonix V1.12 的 8 项优化回移植到 tianxuan，涵盖性能（Schema预缓存、FastPath）、健壮性（双层重试、智能重试）、功能（并行子代理、Grace Round）和运维（Storm Breaker、SuspendPrefix）

**架构：** 逐文件修改，每项优化独立可测。P0（性能/功能）优先，P1（健壮性/运维）其次。所有改动遵循项目已有的接口和命名约定，不破坏现有测试。

**技术栈：** Go 1.22+, DeepSeek API, OpenAI-compatible provider

**源参考：** `/tmp/DeepSeek-Reasonix-v112/internal/`
**目标：** `tianxuan/internal/`

---

## P0-1：Schema 预归一化缓存

**文件：**
- 修改：`tianxuan/internal/tool/tool.go`

**背景：** 当前 `Schemas()` / `FilteredSchemas()` 每次调用时对每个工具 schema 执行 `provider.CanonicalizeSchema()`。这在每轮 LLM 调用时重复进行，产生不必要的 CPU 和 GC 压力。Reasonix 在 `Registry.Add()` 时一次性 canonicalize 并缓存到 `canon` map。

### 任务 1：添加 canon 缓存字段和初始化

**文件：** `tianxuan/internal/tool/tool.go`

- [ ] **步骤 1：修改 Registry 结构体，添加 canon 缓存字段**

```go
// Registry is a per-run set of tools: enabled built-ins plus plugin tools.
// V6.0 P8: supports hiding tools from the model schema while keeping them callable.
type Registry struct {
	tools     map[string]Tool
	order     []string
	hidden    map[string]bool // V6.0 P8: hidden from schema but still callable
	canon     map[string]json.RawMessage // V10.0: schema canonicalized once on Add, reused by Schemas()
}
```

- [ ] **步骤 2：修改 NewRegistry() 初始化 canon map**

```go
func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}, hidden: map[string]bool{}, canon: map[string]json.RawMessage{}}
}
```

- [ ] **步骤 3：修改 Add() 方法，添加时即 canonicalize 并缓存**

在 `r.tools[name] = t` 之后（或之前）添加：

```go
r.canon[name] = provider.CanonicalizeSchema(t.Schema())
```

- [ ] **步骤 4：修改 FilteredSchemas() 使用缓存**

将 `Parameters: provider.CanonicalizeSchema(schema)` 替换为从 `r.canon[name]` 读取。但保留 CompactDescriptor 的判断逻辑（compact schema 不会被缓存，按需计算）：

```go
// 在 FilteredSchemas 的 for 循环中，将现有的 CanonicalizeSchema 调用替换为：
desc := t.Description()
schema := t.Schema()
if cd, ok := t.(CompactDescriptor); ok {
	desc = cd.CompactDescription()
	schema = cd.CompactSchema()
	// Compact schema 不缓存（可能在不同上下文中返回不同结果）
	out = append(out, provider.ToolSchema{
		Name:        t.Name(),
		Description: desc,
		Parameters:  provider.CanonicalizeSchema(schema),
	})
} else {
	// 标准 schema 从缓存读取
	out = append(out, provider.ToolSchema{
		Name:        t.Name(),
		Description: desc,
		Parameters:  r.canon[name],
	})
}
```

- [ ] **步骤 5：RemovePrefix 也需要清理 canon 缓存**

```go
func (r *Registry) RemovePrefix(prefix string) int {
	kept := r.order[:0]
	removed := 0
	for _, name := range r.order {
		if strings.HasPrefix(name, prefix) {
			delete(r.tools, name)
			delete(r.canon, name)  // 新增
			removed++
			continue
		}
		kept = append(kept, name)
	}
	r.order = kept
	return removed
}
```

- [ ] **步骤 6：运行现有测试验证**

```bash
cd tianxuan && go test ./internal/tool/... -v
```

预期：全部 PASS，无新增失败

- [ ] **步骤 7：Commit**

---

## P0-2：SanitizeToolPairing Fast Path

**文件：**
- 修改：`tianxuan/internal/provider/provider.go`

**背景：** 当前 `SanitizeToolPairing`（即 `NormalizeMessages`）每次调用都完整扫描整个消息历史，进行工具调用配对修复。Reasonix 增加了 `tryNormalizeFastPath` 检查：当历史"健康"（所有 tool 配对正确、无 broken args）时直接返回原切片，零分配，O(n) 检查。

### 任务 2：添加 Fast Path 检查

**文件：** `tianxuan/internal/provider/provider.go`

- [ ] **步骤 1：阅读当前 NormalizeMessages 的入口点**

找到 `func SanitizeToolPairing` 和 `func NormalizeMessages` 的实现。

- [ ] **步骤 2：在 NormalizeMessages 函数开头添加 Fast Path 检查**

在 `func NormalizeMessages(msgs []Message) []Message {` 之后立即添加：

```go
// Fast path: well-formed histories pass through with zero allocation.
if fast, ok := tryNormalizeFastPath(msgs, true); ok {
	return fast
}
```

- [ ] **步骤 3：实现 tryNormalizeFastPath 辅助函数**

在 `NormalizeMessages` 之前添加：

```go
// tryNormalizeFastPath checks whether the message history is already well-formed
// (every assistant tool_calls has matching, correctly-ordered tool results; no
// orphan tool messages; no broken tool-call JSON arguments). When the history is
// healthy it returns the original slice and true — zero allocation.
func tryNormalizeFastPath(msgs []Message, dropOrphanTools bool) ([]Message, bool) {
	for i := 0; i < len(msgs); {
		m := msgs[i]
		if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
			j := i + 1
			for j < len(msgs) && msgs[j].Role == RoleTool {
				j++
			}
			if !toolTurnWellFormed(m.ToolCalls, msgs[i+1:j]) || needsToolCallArgRepair(m.ToolCalls) {
				return nil, false
			}
			i = j
			continue
		}
		if m.Role == RoleTool && dropOrphanTools {
			return nil, false
		}
		i++
	}
	return msgs, true
}

// toolTurnWellFormed checks that tool results match their calls in count and ID order.
func toolTurnWellFormed(calls []ToolCall, results []Message) bool {
	if len(calls) != len(results) {
		return false
	}
	for _, tc := range calls {
		if tc.Name == "" {
			return false
		}
	}
	for k, tc := range calls {
		if results[k].ToolCallID != tc.ID {
			return false
		}
	}
	return true
}

// needsToolCallArgRepair returns true if any call has broken (non-valid-JSON) arguments.
func needsToolCallArgRepair(calls []ToolCall) bool {
	for _, tc := range calls {
		if tc.Arguments != "" && !json.Valid([]byte(tc.Arguments)) {
			return true
		}
	}
	return false
}
```

需要添加 `"encoding/json"` 到 import。

- [ ] **步骤 4：运行现有测试验证**

```bash
cd tianxuan && go test ./internal/provider/... -v
```

预期：全部 PASS

- [ ] **步骤 5：Commit**

---

## P0-3：parallel_tasks 通用并行子代理

**文件：**
- 创建：`tianxuan/internal/agent/parallel_tasks.go`
- 修改：`tianxuan/internal/agent/task.go`（提取可复用部分）
- 修改：`tianxuan/internal/tool/builtin/`（注册新工具）

**背景：** Reasonix 的 `parallel_tasks` 工具支持依赖图（`depends_on`）的并行子代理调度，含 DFS 三色标记循环检测。tianxuan 当前只有 `task`（单子代理），无并行调度能力。

### 任务 3：实现 parallel_tasks 工具

**文件：** 创建 `tianxuan/internal/agent/parallel_tasks.go`

- [ ] **步骤 1：创建 parallel_tasks.go，定义核心类型和工具结构**

```go
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// parallelTaskItem is a single sub-task in a parallel dispatch.
type parallelTaskItem struct {
	Skill     string   `json:"skill"`
	Arguments string   `json:"arguments"`
	DependsOn []int    `json:"depends_on,omitempty"` // indices of tasks this depends on (0-based)
	ID        string   `json:"id,omitempty"`         // optional identifier for depends_on reference
}

// ParallelTasksTool dispatches multiple sub-agent tasks in parallel with DAG
// dependency resolution. Tasks with no cross-dependencies run concurrently;
// those that depend on others wait for their predecessors to complete.
type ParallelTasksTool struct {
	taskTool   *TaskTool
	registry   *tool.Registry
	maxSteps   int
	temperature float64
	workspaceRoot string
}

func NewParallelTasksTool(tt *TaskTool, reg *tool.Registry, maxSteps int, temp float64, workspaceRoot string) *ParallelTasksTool {
	return &ParallelTasksTool{
		taskTool:   tt,
		registry:   reg,
		maxSteps:   maxSteps,
		temperature: temp,
		workspaceRoot: workspaceRoot,
	}
}

func (p *ParallelTasksTool) Name() string        { return "parallel_tasks" }
func (p *ParallelTasksTool) ReadOnly() bool       { return false } // spawns sub-agents that may write

func (p *ParallelTasksTool) Description() string {
	return "并行派发多个子代理技能同时执行，完成后汇总结果。适用于 2+ 个独立任务（如并行探索多模块）。有依赖时请分次调用 run_skill。"
}

func (p *ParallelTasksTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "tasks": {
      "type": "array",
      "description": "要执行的任务列表。无 depends_on 的任务并行执行；有 depends_on 的任务等待依赖完成后执行并接收其结果。",
      "items": {
        "type": "object",
        "properties": {
          "skill": {"type": "string", "description": "技能名称，如 explore、review"},
          "arguments": {"type": "string", "description": "传给技能的任务描述"},
          "depends_on": {
            "type": "array",
            "items": {"type": "string"},
            "description": "依赖的任务 id 列表——这些任务完成后才执行本任务，且其结果会作为上下文注入"
          },
          "id": {"type": "string", "description": "可选标识，用于 depends_on 引用"}
        },
        "required": ["arguments", "skill"]
      }
    }
  },
  "required": ["tasks"]
}`)
}
```

- [ ] **步骤 2：实现依赖环检测（DFS 三色标记）**

```go
// validateParallelTaskDeps checks for circular dependencies using DFS three-color
// marking (0=unvisited, 1=visiting, 2=done). Returns nil if the dependency graph is acyclic.
func validateParallelTaskDeps(tasks []parallelTaskItem) error {
	if len(tasks) == 0 {
		return fmt.Errorf("tasks array must not be empty")
	}
	// Build adjacency list: each task depends on its DependsOn indices.
	color := make([]int, len(tasks))
	var dfs func(i int) error
	dfs = func(i int) error {
		if color[i] == 1 {
			return fmt.Errorf("circular dependency detected involving task index %d", i)
		}
		if color[i] == 2 {
			return nil
		}
		color[i] = 1
		for _, dep := range tasks[i].DependsOn {
			if dep < 0 || dep >= len(tasks) {
				// Also check by ID string
				found := false
				for j, t := range tasks {
					if t.ID != "" && t.ID == fmt.Sprint(dep) {
						dep = j
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("task %d depends on unknown task %d", i, dep)
				}
			}
			if err := dfs(dep); err != nil {
				return err
			}
		}
		color[i] = 2
		return nil
	}
	// Resolve string IDs to indices first, then run DFS from each unvisited node.
	for i := range tasks {
		resolved := make([]int, 0, len(tasks[i].DependsOn))
		for _, dep := range tasks[i].DependsOn {
			resolved = append(resolved, dep)
		}
		tasks[i].DependsOn = resolved
	}
	for i := range tasks {
		if err := dfs(i); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **步骤 3：实现 Execute 方法（核心调度逻辑）**

```go
func (p *ParallelTasksTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Tasks []parallelTaskItem `json:"tasks"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("parallel_tasks: invalid arguments: %w", err)
	}
	if len(input.Tasks) == 0 {
		return "", fmt.Errorf("parallel_tasks: tasks array must not be empty")
	}

	// Validate DAG — check for cycles.
	if err := validateParallelTaskDeps(input.Tasks); err != nil {
		return "", fmt.Errorf("parallel_tasks: %w", err)
	}

	// Create wisdom directory for cross-task result sharing.
	wisdomDir, err := os.MkdirTemp("", "parallel-wisdom-*")
	if err != nil {
		return "", fmt.Errorf("parallel_tasks: wisdom dir: %w", err)
	}
	defer os.RemoveAll(wisdomDir)

	// Run tasks respecting dependencies.
	n := len(input.Tasks)
	results := make([]string, n)
	var mu sync.Mutex
	done := make([]chan struct{}, n)
	for i := range done {
		done[i] = make(chan struct{})
	}

	// remaining counts how many of a task's dependencies are still incomplete.
	remaining := make([]int, n)
	for i, t := range input.Tasks {
		remaining[i] = len(t.DependsOn)
	}

	var wg sync.WaitGroup

	// Kick off tasks whose dependencies are already satisfied (remaining==0).
	for i := range input.Tasks {
		if remaining[i] == 0 {
			wg.Add(1)
			go p.runTask(ctx, i, input.Tasks, results, wisdomDir, &mu, done, remaining, &wg)
		}
	}

	wg.Wait()

	// Build summary markdown.
	var sb strings.Builder
	sb.WriteString("# Parallel Tasks Results\n\n")
	for i, t := range input.Tasks {
		label := t.Skill
		if t.ID != "" {
			label = t.ID
		}
		fmt.Fprintf(&sb, "## Task %d: %s\n\n", i, label)
		if results[i] != "" {
			sb.WriteString(results[i])
		} else {
			sb.WriteString("(cancelled or skipped)\n")
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}
```

- [ ] **步骤 4：实现 runTask 辅助方法（单个任务执行+依赖唤醒）**

```go
func (p *ParallelTasksTool) runTask(
	ctx context.Context,
	idx int,
	tasks []parallelTaskItem,
	results []string,
	wisdomDir string,
	mu *sync.Mutex,
	done []chan struct{},
	remaining []int,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	// Wait for all dependencies to complete.
	for _, dep := range tasks[idx].DependsOn {
		select {
		case <-done[dep]:
		case <-ctx.Done():
			// Mark as cancelled and wake my dependents.
			close(done[idx])
			return
		}
	}

	// Read predecessor results from wisdom directory.
	var contextBuilder strings.Builder
	for _, dep := range tasks[idx].DependsOn {
		resultFile := filepath.Join(wisdomDir, fmt.Sprintf("task-%d.md", dep))
		if data, err := os.ReadFile(resultFile); err == nil {
			fmt.Fprintf(&contextBuilder, "\n---\n## Result from task %d (%s)\n\n%s\n", dep, tasks[dep].Skill, string(data))
		}
	}
	// Inject predecessor results into the task arguments.
	augmentedArgs := tasks[idx].Arguments
	if contextBuilder.Len() > 0 {
		augmentedArgs = contextBuilder.String() + "\n\n## Your Task\n\n" + augmentedArgs
	}

	// Execute the skill as a sub-agent using taskTool.
	taskResult, err := p.taskTool.Execute(ctx, json.RawMessage(fmt.Sprintf(
		`{"skill":"%s","arguments":"%s","max_steps":%d}`,
		escapeJSON(tasks[idx].Skill), escapeJSON(augmentedArgs), p.maxSteps,
	)))
	if err != nil {
		mu.Lock()
		results[idx] = fmt.Sprintf("**FAILED:** %s", err.Error())
		mu.Unlock()
	} else {
		mu.Lock()
		results[idx] = taskResult
		mu.Unlock()
		// Write result to wisdom directory for dependents.
		os.WriteFile(filepath.Join(wisdomDir, fmt.Sprintf("task-%d.md", idx)), []byte(taskResult), 0644)
	}

	// Signal completion.
	close(done[idx])

	// Kick off any task whose last dependency just completed.
	mu.Lock()
	defer mu.Unlock()
	for j := range tasks {
		if remaining[j] > 0 {
			// Check if task idx is a dependency of task j.
			for _, dep := range tasks[j].DependsOn {
				if dep == idx {
					remaining[j]--
					if remaining[j] == 0 {
						wg.Add(1)
						go p.runTask(ctx, j, tasks, results, wisdomDir, mu, done, remaining, wg)
					}
					break
				}
			}
		}
	}
}

// escapeJSON escapes a string for safe JSON embedding.
func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}
```

- [ ] **步骤 5：在 tianxuan/internal/tool/builtin/ 目录中注册 parallel_tasks 工具**

找到 `builtin` 包（如 `all.go`），添加：

```go
// 在 boot 或 init 函数中添加：
// 注意：parallel_tasks 需要 TaskTool 实例，所以它在 boot 层构造而非全局 init()
// parallelTasks = agent.NewParallelTasksTool(taskTool, registry, maxSteps, temperature, workspaceRoot)
```

具体注册位置在 `tianxuan/internal/boot/` 中，在创建 TaskTool 之后添加。

- [ ] **步骤 6：运行编译验证**

```bash
cd tianxuan && go build ./...
```

预期：编译通过

- [ ] **步骤 7：Commit**

---

## P1-4：双层重试（mid-stream reconnect + idle timeout）

**文件：**
- 修改：`tianxuan/internal/provider/openai/openai.go`
- 修改：`tianxuan/internal/provider/provider.go`（添加 `StreamInterruptedError` 和 `IsConnReset`）

**背景：** tianxuan 当前有 idle timeout（60s/120s）但没有 mid-stream reconnect。当代理/VPN 在 SSE 流中途断开连接时（空闲超时断开），如果还没 emit 任何输出，应该 replay 请求而非失败。Reasonix 的 `streamWithReconnect` 支持最多 3 次 replay。

### 任务 4：添加 mid-stream reconnect 和增强 idle watchdog

**文件：** `tianxuan/internal/provider/provider.go` — 添加辅助类型

- [ ] **步骤 1：在 provider.go 添加 StreamInterruptedError 类型**

```go
// StreamInterruptedError is returned when a streaming response disconnects
// after the model has already emitted output. The agent can decide whether
// to recover (e.g. inject a tail-recovery prompt) or fail the turn.
type StreamInterruptedError struct {
	Err error
}

func (e *StreamInterruptedError) Error() string {
	return fmt.Sprintf("stream interrupted after output already emitted: %s", e.Err.Error())
}

// IsConnReset reports whether an error is a connection-reset/abort or
// unexpected-EOF (typical of proxy/VPN idle disconnects on long SSE streams).
func IsConnReset(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "connection reset") ||
		strings.Contains(s, "connection aborted") ||
		strings.Contains(s, "unexpected EOF") ||
		strings.Contains(s, "broken pipe")
}
```

需要添加 `"io"` 和 `"net"` 到 import。

**文件：** `tianxuan/internal/provider/openai/openai.go` — 修改 Stream 方法

- [ ] **步骤 2：在 openai.go 添加 maxStreamReconnects 常量**

```go
// maxStreamReconnects bounds how many mid-stream reconnection attempts.
const maxStreamReconnects = 3
```

- [ ] **步骤 3：修改 Stream 方法，使用 streamWithReconnect goroutine**

当前的 `go c.readStream(...)` 替换为 `go c.streamWithReconnect(ctx, resp, newReq, out)`。

需要将 `buildRequest` / `body` 提取为可重用的形式（当前 `Stream` 方法中 body 在 `sendWithRetry` 内部构建）。重构为：

```go
func (c *client) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	body, err := c.marshalRequest(req)
	if err != nil {
		return nil, err
	}

	newReq := func(ctx context.Context) (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if c.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		}
		httpReq.Header.Set("Accept", "text/event-stream")
		return httpReq, nil
	}

	resp, err := c.sendWithRetry(ctx, body)
	if err != nil {
		return nil, err
	}

	out := make(chan provider.Chunk, 16)
	go c.streamWithReconnect(ctx, resp, newReq, out)
	return out, nil
}

// marshalRequest serializes the chat completion request body.
func (c *client) marshalRequest(req provider.Request) ([]byte, error) {
	// ... existing build logic ...
}
```

- [ ] **步骤 4：实现 streamWithReconnect 方法**

```go
func (c *client) streamWithReconnect(ctx context.Context, resp *http.Response, newReq func(context.Context) (*http.Request, error), out chan<- provider.Chunk) {
	defer close(out)
	for attempt := 0; ; attempt++ {
		emitted, err := c.readStream(ctx, resp, out)
		if err == nil {
			return
		}
		if !provider.IsConnReset(err) {
			sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkError, Err: err})
			return
		}
		if emitted {
			sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkError, Err: &provider.StreamInterruptedError{Err: err}})
			return
		}
		if attempt >= maxStreamReconnects {
			sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkError, Err: err})
			return
		}
		next, rerr := c.sendWithRetry(ctx, body) // need to store body for replay
		if rerr != nil {
			sendChunk(ctx, out, provider.Chunk{Type: provider.ChunkError, Err: rerr})
			return
		}
		resp = next
	}
}
```

但有个问题：`sendWithRetry` 需要 body bytes。需要重构让 `sendWithRetry` 接受 `newReq` callback 而非 body。或者让 `streamWithReconnect` 使用同一个 `newReq`。

- [ ] **步骤 5：修改 sendWithRetry 接口，接受 newReq callback**

当前 `sendWithRetry(ctx, body)` 内部构建 request。改为与 Reasonix 一致的 `sendWithRetry(ctx, httpClient, sendOpts, newReq)` 模式：

```go
// SendOptions configures a send.
type sendOpts struct {
	MaxRetries int
	Provider   string
	KeyEnv     string
	RetryAuth  bool
}

func (c *client) sendOpts() sendOpts {
	return sendOpts{
		MaxRetries: 5,
		Provider:   c.name,
		KeyEnv:     c.keyEnv,
		RetryAuth:  c.authed.Load(),
	}
}
```

然后 `sendWithRetry` 使用 `newReq(ctx)` 生成每次重试的 HTTP request。修改 readStream 返回 `(emitted bool, err error)`：

```go
func (c *client) readStream(ctx context.Context, resp *http.Response, out chan<- provider.Chunk) (emitted bool, err error) {
	defer resp.Body.Close()
	// ... existing logic, but track whether any chunk was emitted ...
	var emittedFlag bool
	// ... in case ChunkText/ChunkReasoning/ChunkToolCallStart/ChunkToolCall:
	emittedFlag = true
	// ...
	return emittedFlag, nil
}
```

- [ ] **步骤 6：编译验证**

```bash
cd tianxuan && go build ./...
```

预期：编译通过

- [ ] **步骤 7：Commit**

---

## P1-5：401/403 智能重试

**文件：**
- 修改：`tianxuan/internal/provider/openai/openai.go`

**背景：** MiMo/中转站等网关在负载高时会返回 transient 401。对于已验证成功过的 key（`authed atomic.Bool`），应该额外重试 2 次而非立即失败。Reasonix 用 `authed atomic.Bool` + `maxAuthRetries=2` 实现。

### 任务 5：添加 401/403 智能重试

**文件：** `tianxuan/internal/provider/openai/openai.go`

- [ ] **步骤 1：在 client 结构体添加 authed 字段**

```go
type client struct {
	// ... existing fields ...
	authed atomic.Bool // a request has succeeded — gate transient-401 retry
}
```

- [ ] **步骤 2：添加 maxAuthRetries 常量**

```go
const maxAuthRetries = 2
```

- [ ] **步骤 3：修改 sendWithRetry，支持 401/403 智能重试**

在 `sendWithRetry` 方法的错误处理中，将 401/403 的直接返回改为：

```go
if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
	authErr := &provider.AuthError{Provider: c.name, KeyEnv: c.keyEnv, Status: resp.StatusCode}
	if c.authed.Load() && authRetries < maxAuthRetries {
		authRetries++
		lastErr = authErr
		continue
	}
	return nil, authErr
}
```

- [ ] **步骤 4：在成功认证后设置 authed=true**

在 `sendWithRetry` 的 `resp.StatusCode == http.StatusOK` 分支中：

```go
if resp.StatusCode == http.StatusOK {
	c.authed.Store(true)
	return resp, nil
}
```

或者在 `Stream` 方法中 `sendWithRetry` 成功后设置。

- [ ] **步骤 5：运行编译和现有测试**

```bash
cd tianxuan && go build ./... && go test ./internal/provider/... -v
```

预期：编译通过，测试通过

- [ ] **步骤 6：Commit**

---

## P1-6：Storm Breaker 增强 — (name, error) 签名

**文件：**
- 修改：`tianxuan/internal/agent/agent.go`（应用签名变更和注入 nudge）
- 修改：`tianxuan/internal/agent/agent_run.go`（调用 applyStormBreaker）

**背景：** tianxuan 当前使用 `(toolName, args, result)` 三联签名检测重复（`repeat_detect.go`）。Reasonix 的 Storm Breaker 使用 `(toolName, errorMsg)` 双联签名——因为模型会变着花样改参数但同样的错误反复出现。两者互补：`detectRepeatedSteps` 检测重复成功行为，`applyStormBreaker` 检测重复失败行为。

### 任务 6：增强 Storm Breaker 检测

**文件：** `tianxuan/internal/agent/agent.go`

- [ ] **步骤 1：修改 StormBreaker 结构体，使用 (name, error) 签名**

当前 `StormBreaker` 结构体（agent.go:750-753）有 `Sig string` 和 `Count int`。确认 `executeOne` 返回的 `toolOutcome` 包含 `errMsg` 字段。如果是，则修改 `extractSignature` 逻辑：

在 `agent.go` 中确认 `toolOutcome` 结构体有 `errMsg` 字段：

```go
type toolOutcome struct {
	output    string
	blocked   bool
	errMsg    string
	truncMsg  string
}
```

- [ ] **步骤 2：添加 applyStormBreaker 方法（agent.go 末尾或 agent_run.go）**

```go
// applyStormBreaker detects a run of identically-failing turns and, past the
// threshold, appends a directive to change approach to the first result.
// It keys on each call's (tool, error) — not its args — because a stuck model
// reworks the arguments cosmetically while failing identically.
func (a *AgentRunner) applyStormBreaker(calls []provider.ToolCall, outcomes []toolOutcome, results []string) {
	sig, ok := batchStormSignature(calls, outcomes)
	if !ok {
		a.stormSig, a.stormCount = "", 0
		return
	}
	if sig != a.stormSig {
		a.stormSig, a.stormCount = sig, 1
		return
	}
	a.stormCount++
	if a.stormCount < 3 { // stormBreakThreshold
		return
	}
	subject := fmt.Sprintf("%q", calls[0].Name)
	short := calls[0].Name
	if len(calls) > 1 {
		subject = fmt.Sprintf("this batch of %d tool calls", len(calls))
		short = fmt.Sprintf("a batch of %d calls", len(calls))
	}
	results[0] = outcomes[0].output + fmt.Sprintf(
		"\n\n[loop guard] %s has now failed %d times in a row with the same error. Re-sending it — even with the wording changed — will not help. Change approach: fix the arguments, use a different tool, or explain the blocker in your final answer.",
		subject, a.stormCount)
	a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: fmt.Sprintf(
		"storm breaker: %s failed %d× with the same error — nudging model to change approach",
		short, a.stormCount)})
}

// batchStormSignature returns a per-turn fixation signature — each call's
// (name, error) in order — and ok=true only when every call errored and none was
// merely blocked.
func batchStormSignature(calls []provider.ToolCall, outcomes []toolOutcome) (string, bool) {
	if len(calls) == 0 {
		return "", false
	}
	var sb strings.Builder
	for i := range calls {
		if outcomes[i].errMsg == "" || outcomes[i].blocked {
			return "", false
		}
		sb.WriteString(calls[i].Name)
		sb.WriteByte(0)
		sb.WriteString(outcomes[i].errMsg)
		sb.WriteByte(0)
	}
	return sb.String(), true
}
```

- [ ] **步骤 3：在 executeBatch 中调用 applyStormBreaker（agent_run.go）**

在 `executeBatch` 方法的末尾（所有 tool 执行完成后），收集 outcomes 并调用：

```go
// V10.0: Storm Breaker — detect repeated failure patterns.
if len(calls) > 0 {
	// Build outcomes slice from results (extract error messages).
	outcomes := make([]toolOutcome, len(calls))
	for i := range calls {
		outcomes[i] = a.extractOutcome(results[i])
	}
	a.applyStormBreaker(calls, outcomes, results)
}
```

需要添加 `extractOutcome` 辅助函数，从 result string 中提取是否有错误。

- [ ] **步骤 4：编译验证**

```bash
cd tianxuan && go build ./...
```

预期：编译通过

- [ ] **步骤 5：Commit**

---

## P1-7：Grace Round 机制

**文件：**
- 修改：`tianxuan/internal/agent/agent_run.go`

**背景：** 当 `maxSteps` 达到限制时，tianxuan 直接返回错误。Reasonix 在步骤用尽后给模型一轮"免工具调用"机会——注入 nudge 消息让模型从已完成工作中总结最终答案。

### 任务 7：实现 Grace Round

**文件：** `tianxuan/internal/agent/agent_run.go`

- [ ] **步骤 1：在 runDirect 方法中添加 graceRound 变量**

在 `for step := 0; ...` 之前添加：

```go
graceRound := false
```

修改循环条件：

```go
for step := 0; a.maxSteps <= 0 || step < a.maxSteps || graceRound; step++ {
```

- [ ] **步骤 2：在步骤用尽时注入 nudge 并设置 graceRound**

在 `executeBatch` 完成后（或在 `detectRepeatedSteps` 之后），添加 Grace Round 检查：

```go
// V10.0: Grace Round — when maxSteps is reached, give the model one
// extra turn to produce a final answer from completed work.
if a.maxSteps > 0 && step+1 >= a.maxSteps && !graceRound {
	graceRound = true
	nudge := fmt.Sprintf(
		"Do not call any more tools — your tool-call round limit (agent.max_steps = %d) has been reached. Instead, synthesize a final answer from all the work already completed: summarize what was accomplished, what remains to be done, and any decisions the user should make. The user can increase max_steps or continue in the next turn if more work is needed.",
		a.maxSteps)
	a.session.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: nudge,
	})
	continue
}
```

- [ ] **步骤 3：在 Grace Round 无工具调用时优雅退出**

修改现有的 `len(calls) == 0` 门检查，如果在 graceRound 中且无工具调用，正常返回 nil：

```go
if len(calls) == 0 {
	if graceRound {
		return nil // Grace Round: model produced summary, done.
	}
	// ... existing gates ...
	return nil
}
```

- [ ] **步骤 4：在 Grace Round 仍有工具调用时返回错误**

```go
// 在 step++ 循环末尾（for 循环内部），如果 graceRound 为 true：
if graceRound {
	return fmt.Errorf("paused after %d tool-call rounds (agent.max_steps) — the model attempted to call more tools during the grace round; the work so far is saved. Send another message to continue, or set max_steps higher or to 0 for no limit", a.maxSteps)
}
```

- [ ] **步骤 5：编译验证**

```bash
cd tianxuan && go build ./...
```

预期：编译通过

- [ ] **步骤 6：Commit**

---

## P1-8：SuspendPrefix / ResumePrefix

**文件：**
- 修改：`tianxuan/internal/tool/tool.go`

**背景：** 当用户临时关闭 MCP server 时，tianxuan 用 `RemovePrefix` 彻底删除工具。但后台 MCP 握手线程可能在之后重新添加工具，导致前端关闭无效。Reasonix 的 `SuspendPrefix` 在 `suspended` map 中标记，`Add` 会检查并拒绝被 suspend 的前缀。

### 任务 8：添加 SuspendPrefix / ResumePrefix

**文件：** `tianxuan/internal/tool/tool.go`

- [ ] **步骤 1：在 Registry 添加 suspended 字段**

```go
type Registry struct {
	tools     map[string]Tool
	order     []string
	hidden    map[string]bool
	canon     map[string]json.RawMessage
	suspended map[string]bool // V10.0: MCP prefixes temporarily disabled per-session
}
```

- [ ] **步骤 2：修改 NewRegistry 初始化 suspended**

```go
func NewRegistry() *Registry {
	return &Registry{
		tools:     map[string]Tool{},
		hidden:    map[string]bool{},
		canon:     map[string]json.RawMessage{},
		suspended: map[string]bool{},
	}
}
```

- [ ] **步骤 3：修改 Add() 检查 suspended 前缀**

在 `Add` 方法开头添加：

```go
func (r *Registry) Add(t Tool) {
	name := t.Name()
	for prefix := range r.suspended {
		if strings.HasPrefix(name, prefix) {
			return // silently reject — prefix is suspended
		}
	}
	// ... existing logic ...
}
```

- [ ] **步骤 4：添加 SuspendPrefix 方法**

```go
// SuspendPrefix unregisters every tool whose name starts with prefix, and
// prevents future Add calls for that prefix until ResumePrefix is called.
// Used for per-session MCP disables — an in-flight background handshake
// may attempt to re-add tools for the suspended prefix, and SuspendPrefix
// blocks it silently. Returns the count of tools removed.
func (r *Registry) SuspendPrefix(prefix string) int {
	r.suspended[prefix] = true
	kept := r.order[:0]
	removed := 0
	for _, name := range r.order {
		if strings.HasPrefix(name, prefix) {
			delete(r.tools, name)
			delete(r.canon, name)
			removed++
			continue
		}
		kept = append(kept, name)
	}
	r.order = kept
	return removed
}
```

- [ ] **步骤 5：添加 ResumePrefix 方法**

```go
// ResumePrefix allows future Add calls for a previously suspended prefix.
func (r *Registry) ResumePrefix(prefix string) {
	delete(r.suspended, prefix)
}
```

- [ ] **步骤 6：在 control 层使用 SuspendPrefix 替代 RemovePrefix**

找到 `tianxuan/internal/control/` 中处理 MCP server disconnect 的代码，将 `RemovePrefix` 改为 `SuspendPrefix`；对应的 reconnect 逻辑使用 `ResumePrefix`。

（如果 control 层尚未使用 MCP 的 per-session 启停，则此改动推迟到后续集成。当前先完成 Registry 层 API。）

- [ ] **步骤 7：运行工具注册表测试**

```bash
cd tianxuan && go test ./internal/tool/... -v
```

预期：全部 PASS

- [ ] **步骤 8：Commit**

---

## 最终验证

- [ ] **全量测试**

```bash
cd tianxuan && go build ./... && go test ./internal/... -count=1 -timeout 120s 2>&1
```

预期：编译通过，测试全部 PASS（无新增失败）

- [ ] **构建桌面端验证**

```bash
cd tianxuan && bash build-desktop.bat
```

预期：构建成功，产物可正常运行
