# Reasonix 优点蒸馏实施计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将 DeepSeek-Reasonix-V1.12 的 6 个核心韧性机制蒸馏到 tianxuan，提升 Agent 稳定性和用户体验。

**架构：** 最小修改原则——不改动 tianxuan 现有架构，仅添加缺失的守卫机制。所有机制在 `internal/agent/` 中实现，通过 `AgentRunner` 结构体新增字段和方法接入。

**技术栈：** Go 1.21+, tianxuan/internal/agent, tianxuan/internal/provider

---

## 批次 1：P0 紧急修复 — 流中断恢复 + 空回答检测

### 任务 1.1：添加 StreamInterruptedError 到 provider 包

**文件：**
- 修改：`tianxuan/internal/provider/provider.go`

**目标：** 定义可恢复的流中断错误类型，让 agent 层能区分"致命错误"和"可恢复中断"。

- [ ] **步骤 1：在 provider.go 末尾添加 StreamInterruptedError**

```go
// StreamInterruptedError marks a recoverable transport cut that happened after
// the caller had already received model output. Providers must not replay these
// requests themselves because doing so could duplicate visible text or tool
// calls; the agent can append a tail recovery prompt instead.
type StreamInterruptedError struct {
	Err error
}

func (e *StreamInterruptedError) Error() string {
	if e == nil || e.Err == nil {
		return "stream interrupted"
	}
	return e.Err.Error()
}

func (e *StreamInterruptedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsStreamInterrupted checks whether an error is a recoverable stream interruption.
func IsStreamInterrupted(err error) bool {
	var interrupted *StreamInterruptedError
	return errors.As(err, &interrupted)
}
```

- [ ] **步骤 2：运行编译验证**

```bash
cd D:\AI\tianxuanX\tianxuan && go build ./internal/provider/...
```

### 任务 1.2：修改 agent_stream.go 支持中断检测

**文件：**
- 修改：`tianxuan/internal/agent/agent_stream.go:12-134`

**目标：** stream() 遇到 ChunkError 时，判断是否为可恢复中断，返回 interrupted 标志。

- [ ] **步骤 1：修改 stream() 返回值签名为 7 个返回值**

```go
func (a *AgentRunner) stream(ctx context.Context, turn int) (string, string, string, []provider.ToolCall, *provider.Usage, bool, error) {
```

最后新增 `bool` = interrupted。

- [ ] **步骤 2：在 ChunkError 分支检查 IsStreamInterrupted**

```go
case provider.ChunkError:
    if provider.IsStreamInterrupted(chunk.Err) {
        return text.String(), reasoning.String(), signature, calls, usage, true, chunk.Err
    }
    return "", "", "", nil, nil, false, chunk.Err
```

非中断错误保持不变。其余分支 return 末尾加 `false`。

- [ ] **步骤 3：运行编译验证**

```bash
cd D:\AI\tianxuanX\tianxuan && go build ./internal/agent/...
```

- [ ] **步骤 4：修复所有 stream() 调用者**

`agent_run.go:86` 中 `a.stream(ctx, step+1)` 返回值接收需加 `interrupted` 变量。

### 任务 1.3：在 agent_run.go 添加流中断恢复

**文件：**
- 修改：`tianxuan/internal/agent/agent_run.go:18-250`

**目标：** 流中断时保存已有输出，注入恢复提示语，不消耗 step。

- [ ] **步骤 1：在 runDirect 顶部添加流恢复计数器**

```go
streamRecoveries := 0
const maxStreamRecoveries = 3
```

- [ ] **步骤 2：修改 stream() 调用后的错误处理**

```go
text, reasoning, signature, calls, usage, interrupted, err := a.stream(ctx, step+1)
if err != nil {
    if interrupted && streamRecoveries < maxStreamRecoveries {
        streamRecoveries++
        if strings.TrimSpace(text) != "" {
            a.session.Add(provider.Message{
                Role:             provider.RoleAssistant,
                Content:          text,
                ReasoningContent: reasoning,
                ReasoningSignature: signature,
            })
        }
        a.session.Add(provider.Message{
            Role:    provider.RoleUser,
            Content: streamRecoveryMessage(strings.TrimSpace(text) != ""),
        })
        a.sink.Emit(event.Event{Kind: event.Retrying, RetryAttempt: streamRecoveries, RetryMax: maxStreamRecoveries})
        step-- // recovery retries do not consume the tool-round budget
        continue
    }
    a.preWG.Wait()
    return err
}
streamRecoveries = 0
```

- [ ] **步骤 3：添加 streamRecoveryMessage 辅助函数到 agent.go**

```go
func streamRecoveryMessage(hasPartialText bool) string {
    if hasPartialText {
        return "The previous assistant response was interrupted during streaming. Continue the same task from immediately after the partial assistant message above. Do not repeat text that is already visible."
    }
    return "The previous assistant response was interrupted during streaming before visible answer text was completed. Continue the same task now and provide the next useful response."
}
```

### 任务 1.4：添加空回答检测

**文件：**
- 修改：`tianxuan/internal/agent/agent_run.go:158-177`
- 修改：`tianxuan/internal/agent/agent.go`

**目标：** 模型返回 0 tool call 且无可见文本时，注入重试提示，最多 3 次。

- [ ] **步骤 1：在 runDirect 顶部添加空回答计数器**

```go
emptyFinalBlocks := 0
const maxEmptyFinalBlocks = 3
```

- [ ] **步骤 2：修改 len(calls) == 0 分支，在三闸门之前插入空回答检测**

在 `if len(calls) == 0 {` 之后、`if graceRound` 之前插入：

```go
if len(calls) == 0 {
    if graceRound {
        return nil
    }

    // Empty final detection: model returned no tool calls and no visible text.
    if strings.TrimSpace(text) == "" {
        emptyFinalBlocks++
        if emptyFinalBlocks >= maxEmptyFinalBlocks {
            return fmt.Errorf("model finished without a visible final answer %d times", emptyFinalBlocks)
        }
        a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
            Text: fmt.Sprintf("empty final answer blocked: retrying (%d/%d)", emptyFinalBlocks, maxEmptyFinalBlocks)})
        a.session.Add(provider.Message{Role: provider.RoleUser, Content: emptyFinalRetryMessage()})
        a.maybeCompact(ctx, usage)
        continue
    }

    // ... 三闸门 ...
}
```

- [ ] **步骤 3：添加辅助函数到 agent.go**

```go
func emptyFinalRetryMessage() string {
    return "The previous assistant response finished without any visible answer text. Continue the same task now and provide a concise visible answer to the user. Do not send reasoning only."
}
```

- [ ] **步骤 4：在工具调用后重置计数器**

在 `len(calls) > 0` 分支开头添加：`emptyFinalBlocks = 0`

### 任务 1.5：OpenAI provider 在流中断时使用 StreamInterruptedError

**文件：**
- 修改：`tianxuan/internal/provider/openai/openai.go`

**目标：** 让 provider 层在遇到可恢复的流中断（如连接重置、超时）时返回 StreamInterruptedError。

- [ ] **步骤 1：在 openai.go 中找到 ChunkError 发送位置（行 155 附近及 403、407、460），对读流中断错误包裹 StreamInterruptedError**

```go
// 在 read stream 错误处：
out <- provider.Chunk{Type: provider.ChunkError, Err: &provider.StreamInterruptedError{Err: fmt.Errorf("%s: read stream: %w", c.name, err)}}
```

注意：行 155 注释已说明 "surface as ChunkError without retry, since the model has already started"，这些位置适合使用 StreamInterruptedError。

### 任务 1.6：运行完整编译和测试

```bash
cd D:\AI\tianxuanX\tianxuan && go build ./...
cd D:\AI\tianxuanX\tianxuan && go test ./internal/agent/... -count=1 -timeout 60s
```

---

## 批次 2：P1 短期优化 — 最终就绪闸门 + 用户中途转向 + 语言偏好分离

### 任务 2.1：语言偏好分离 (reasoning_language.go)

**文件：**
- 创建：`tianxuan/internal/agent/reasoning_language.go`

**目标：** 将推理语言和回答语言偏好通过 turn-tail 瞬态块注入，不破坏缓存前缀。

- 从 DeepSeek-Reasonix-V1.12/internal/agent/reasoning_language.go 完整拷贝
- 将包名从 `agent` 改为与 tianxuan 一致
- 在 AgentRunner 中添加 `responseLanguage` 和 `reasoningLanguage` 两个 `atomic.Value` 字段
- 添加 `SetResponseLanguage` / `SetReasoningLanguage` 方法
- 添加 `withTurnPreferences()` 方法（从 Reasonix 拷贝并适配）
- 在 `runDirect` 中将 `input = a.withTurnPreferences(input)` 包裹用户输入

### 任务 2.2：用户中途转向队列

**文件：**
- 修改：`tianxuan/internal/agent/agent.go`
- 修改：`tianxuan/internal/agent/agent_run.go`

**目标：** 支持用户在 Agent 运行中注入引导消息，每轮循环消费一条。

- 在 AgentRunner 中添加 `steerMu sync.Mutex`、`steerQueue []string`、`steerConsumed bool`
- 添加 `Steer(text string)`、`consumeSteer()`、`clearSteerQueue()`、`steerQueueLen()` 方法
- 在 `runDirect` 循环顶部调用 `consumeSteer()`，若存在则注入到 session
- 在 `runDirect` defer 中调用 `clearSteerQueue()`
- 在 `len(calls)==0 && 所有门通过` 后检查 `steerQueueLen() > 0` → continue

### 任务 2.3：最终就绪闸门

**文件：**
- 修改：`tianxuan/internal/agent/agent.go`
- 修改：`tianxuan/internal/agent/agent_run.go`

**目标：** 模型声称完成时验证 todo_write 所有项已完成 + project_checks 有收据。

- 添加 `finalReadinessCheck()` 方法（依赖已有的 evidence.Ledger）
- 添加 `projectChecks` 字段
- 在 `len(calls)==0` 且三闸门全部通过后，执行 `finalReadinessCheck()`
- 若未通过 → 注入修正提示语，最多阻塞 3 次
- 需在 evidence.Ledger 中添加 `IncompleteLatestTodos()`、`HasSuccessfulTodoProgressReceipt()`、`HasSuccessfulCommandAfter()` 等查询方法

---

## 批次 3：P2 中期规划 — 规范 Todo 主机追踪

### 任务 3.1：添加 TodoState 到 AgentRunner

**文件：**
- 修改：`tianxuan/internal/agent/agent.go`
- 修改：`tianxuan/internal/evidence/`

**目标：** 主机侧维护规范 todo 列表，自动推进完成状态。

- 在 AgentRunner 添加 `todoMu sync.Mutex`、`todoState []evidence.TodoItem`、`hostAdvanceSeq atomic.Int64`
- 实现 `rebuildTodoState(snapshot)` → 从 session 重建 todo 状态
- 在 `complete_step` 工具结果处理后自动推进：completed → 下一个 pending → in_progress
- 合成 `todo_write` receipt 和 ToolDispatch/ToolResult event 保持 UI 同步
- 在 `finalReadinessCheck()` 中查询 `incompleteCanonicalTodos()`

---

## 注意事项

- 所有新增代码使用中文注释
- 保持与现有代码风格一致（字段使用中文注释，函数签名保持一致）
- 每个批次完成后运行 `go build ./...` 和 `go test ./internal/agent/...` 验证
- 每个批次提交一个 commit
