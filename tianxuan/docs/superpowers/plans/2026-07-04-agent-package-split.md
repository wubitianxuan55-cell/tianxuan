# P2: agent 包按关注点拆分 — 设计文档

> 2026-07-04 · V10.23.0 后续改进计划

## 目标

将 `internal/agent` 包从单层 44 文件 god-object 重构为按关注点分层的子包结构，每个子包只暴露纯函数和独立结构体，不引入接口间接调用层，确保编译后行为完全一致。

## 当前状态

| 维度 | 数值 |
|------|------|
| 源文件 | 44 个 .go 文件 |
| 总代码量 | ~280 KB（不含测试） |
| 测试文件 | 30 个 `*_test.go`，~180 KB |
| 核心结构体 `AgentRunner` | ~100 字段，~65 方法 |
| 外部消费者 | 6 个包（desktop, boot, cli, control, serve, acp），25 个文件 |

### 关键约束

1. **缓存前缀不变性**（AGENTS.md 最高优先级）：`filteredSchemas`、`activeSchemas`、`verifyPrefixAndShape`、`CaptureShape` 等方法必须保持字节级行为一致
2. **25 个外部消费者**需要同步迁移 import 路径

## 目标架构

```
internal/agent/                    ← 保留为核心包（AgentRunner + 核心流程）
├── agent.go                       ← 缩减到 ~350 行（仅结构体定义 + New + setter/getter）
├── agent_run.go                   ← 主运行循环（不变）
├── agent_stream.go                ← LLM streaming（不变）
├── execute_one.go                 ← 单工具执行（不变）
├── batch_executor.go              ← 批量执行（不变）
├── stop_gate.go + judge.go        ← 停止门控（不变）
├── canonical_todo.go              ← 任务状态（不变）
├── reasoning_language.go          ← 语言偏好（不变）
├── recall_reminder.go             ← 记忆提醒（不变）
├── trace.go                       ← Trace ID（不变）
│
├── session/                       ← 会话管理子包（Phase 1）
│   ├── session.go                 ← Session 结构体 + 并发安全容器
│   ├── save.go                    ← 序列化/反序列化
│   └── branch.go                  ← 分支元数据
│
├── budget/                        ← 预算与模型配置（Phase 1）
│   ├── budget_gate.go             ← BudgetGate 结构体
│   └── model_profile.go           ← ModelProfile 查找
│
├── textutils/                     ← 文本工具（Phase 1）
│   ├── text_utils.go              ← truncateToolOutput + normalizeText + compressRepeatedLines
│   └── width.go                   ← visibleWidth + streamedRows
│
├── loopguard/                     ← 循环/风暴防护（Phase 2）
│   ├── repeat_detect.go           ← detectRepeatedSteps
│   ├── param_storm.go             ← ParamStormBreaker
│   ├── detector.go                ← Detector（失败模式检测）
│   └── output_continue.go         ← maybeContinueOutputLength / maybeRetryInvalidOutput
│
├── render/                        ← 输出渲染（Phase 2）
│   ├── textsink.go                ← TextSink ANSI 渲染
│   └── stream_batcher.go          ← streamBatcher
│
├── cache/                         ← 缓存诊断（Phase 2）
│   ├── cache_shape.go             ← PrefixShape + CaptureShape
│   ├── cache_guard.go             ← verifyPrefixAndShape
│   ├── tool_fingerprint.go        ← BuildToolCatalogFingerprint
│   └── toolcache.go              ← toolCache（只读工具缓存）
│
├── toolguard/                     ← 工具守卫（Phase 2）
│   ├── tool_dispatch.go           ← ToolDispatcher
│   ├── tool_precheck.go           ← precheckTool
│   ├── tool_coherence.go          ← postBatchCoherenceCheck
│   └── tool_call_repair.go        ← RepairDispatchToolArguments
│
├── compaction/                    ← 上下文压缩（Phase 3）
│   ├── compact.go                 ← 核心压缩逻辑（可能拆为 compact_plan + compact_exec）
│   ├── compact_summary.go         ← BuildCompactSummary
│   ├── compress.go                ← compressGrep + 工具输出压缩
│   ├── prune.go                   ← PruneStaleToolResults
│   └── checkpoint.go              ← WriteCheckpoint
│
└── tasks/                         ← 子代理 + ask 工具（Phase 3）
    ├── task.go                    ← RunSubAgent + NewTaskTool + FilterRegistry
    └── ask.go                     ← AskTool
```

## 各子包设计

### Phase 1（低风险，零耦合）

#### 1. `session/` — 会话管理

**移出文件**：`session.go` + `save.go` + `branch.go`

**暴露符号**：
- `Session`、`NewSession()`、`Add()`/`Replace()`/`Snapshot()` 等方法
- `Save()`、`LoadSession()`、`NewSessionPath()`、`ListSessions()`
- `BranchMeta`、`BranchInfo`、`BranchID()`、`SaveBranchMeta()`、`LoadBranchMeta()`、`ListBranches()`

**耦合分析**：Session 是 `provider.Message` 的容器，外部消费者（desktop/cli/control）直接使用 `*agent.Session`。迁移方案：`agent.Session` → `agent/session.Session`。

#### 2. `budget/` — 预算与模型

**移出文件**：`budget_gate.go` + `model_profile.go`

**暴露符号**：
- `BudgetGate` 结构体（含 `Record()`、`Blocked()`、`Warned()` 方法）
- `ModelProfile` 结构体、`LookupModelProfile()`、`ApplyModelProfile()`

**耦合分析**：`BudgetGate` 是独立结构体，只在 `agent.go` 中作为字段持有。纯搬移零风险。

#### 3. `textutils/` — 文本工具

**移出文件**：`text_utils.go` + `width.go`

**暴露符号**：
- `TruncateToolOutput()`、`NormalizeText()`、`CompressRepeatedLines()`、`CompactArgs()`、`IsToolError()`
- `VisibleWidth()`、`StreamedRows()`、`FormatUsageLine()`

**耦合分析**：纯函数，被 agent 内多处引用。外部消费者 `cli` 使用 `CompactArgs()` 和 `FormatUsageLine()`。

### Phase 2（中风险，弱耦合）

#### 4. `loopguard/` — 循环检测

**移出文件**：`repeat_detect.go` + `param_storm.go` + `detector.go` + `output_continue.go`

**暴露符号**：
- `detectRepeatedSteps()` — 仍通过 `*AgentRunner` 接收者调用内部字段
- `ParamStormBreaker` 结构体
- `Detector` 结构体
- `MaybeContinueOutputLength()`/`MaybeRetryInvalidOutput()`

**耦合分析**：`detectRepeatedSteps` 和 `MaybeContinueOutputLength` 需要访问 `AgentRunner` 的 `repeatSig`/`repeatCount`/`lenContCount` 等字段。保留在 agent 包内但逻辑移到子包：子包提供纯函数，`AgentRunner` 的方法只是薄包装调用子包函数并传入必要参数。

⚠️ 这是一个微妙点。用户选择**不允许接口化**。所以我们要将这些检测器保留在 agent 包内（以 `(a *AgentRunner)` 方法形式），但检测逻辑的核心纯函数移到子包。

#### 5. `render/` — 输出渲染

**移出文件**：`textsink.go` + `stream_batcher.go`

**暴露符号**：
- `TextSink` 结构体 + `NewTextSink()`
- `StreamBatcher` 结构体

**耦合分析**：`TextSink` 是完全独立的，只消费 `event.Sink` 接口。`streamBatcher` 也是独立结构体。零耦合迁移。

#### 6. `cache/` — 缓存诊断

⚡ **高敏感度**：直接涉及前缀缓存不变性（AGENTS.md 核心约束）。

**移出文件**：`cache_shape.go` + `cache_guard.go` + `tool_fingerprint.go` + `toolcache.go`

**暴露符号**：
- `PrefixShape` 结构体、`CaptureShape()`
- `verifyPrefixAndShape()` — **保留在 agent 包**（因为它访问 `AgentRunner` 的 `sink`/`prefixFingerprintSet`/`lastPrefixShape` 字段）
- `ToolCatalogFingerprint`、`BuildToolCatalogFingerprint()`
- `ToolCache` 结构体

**风险控制**：
- `CaptureShape()` 是纯函数，可以安全移到子包
- `verifyPrefixAndShape()` 保留在 agent 包作为 `AgentRunner` 方法
- `cache_guard.go` 移到子包后，`verifyPrefixAndShape()` 调用 `cache.CaptureShape()` 而非 `a.CaptureShape()`

⚠️ 关键检查：`CaptureShape` 当前是 `(a *AgentRunner)` 方法，访问 `a.session.Messages` 和 `a.activeSchemas`。移到子包后需改为接收参数。

#### 7. `toolguard/` — 工具守卫

**移出文件**：`tool_dispatch.go` + `tool_precheck.go` + `tool_coherence.go` + `tool_call_repair.go`

**暴露符号**：
- `ToolDispatcher` 结构体 + `NewToolDispatcher()` + `Dispatch()`
- `PreCheckToolResult()`（纯函数）
- `PostBatchCoherenceCheck()`（纯函数）
- `RepairDispatchToolArguments()`（纯函数）

**耦合分析**：`ToolDispatcher` 已是独立结构体，在 `boot.go` 中通过 `agent.NewToolDispatcher()` 创建。大部分逻辑是纯函数。

### Phase 3（高耦合，需小心处理）

#### 8. `compaction/` — 上下文压缩

**移出文件**：`compact.go` + `compact_summary.go` + `compress.go` + `prune.go` + `checkpoint.go`

**暴露符号**：
- `CompactionConfig` 结构体（从 agent.go 移出）
- `MaybeCompact()`、`Compact()`（保留在 agent 包，作为轻量包装）
- `BuildCompactSummary()`
- `CompressGrep()`、`CompressBash()`、`CompressFileList()`
- `PruneStaleToolResults()`
- `WriteCheckpoint()`

**风险控制**：
- `compact.go` 中的核心函数（`planCompaction`/`partitionFold`/`summarize`）移入子包
- agent 包保留 `maybeCompact()` 和 `compact()` 方法，调用子包函数
- **`CompactionConfig` 移出后**，`agent.Options` 需要更新字段类型

#### 9. `tasks/` — 子代理 + ask

**移出文件**：`task.go` + `ask.go`

**暴露符号**：
- `RunSubAgent()`、`NewTaskTool()`、`FilterRegistry()`、`SubagentMetaTools()`、`NestedSink()`
- `AskTool` 结构体 + `NewAskTool()`

**耦合分析**：这些是相对独立的功能，task.go 依赖 `AgentRunner` 创建子代理。ask.go 依赖 `Asker` 接口。通过参数化可移出。

## 风险分析

| 风险项 | 等级 | 缓解措施 |
|--------|------|----------|
| 缓存前缀漂移 | 🔴 高 | `verifyPrefixAndShape` 保留在 agent 包；`CaptureShape` 改为纯函数参数化；Phase 2 merge 后运行 cachehit e2e 测试 |
| 外部编译失败 | 🟡 中 | 分阶段迁移，每阶段独立编译验证 |
| 循环 import | 🟡 中 | 子包零依赖 agent 包，只依赖 provider/event/tool 等底层包 |
| compact.go 拆分引入 bug | 🟡 中 | compact_test.go 已有充分测试覆盖 |
| 测试文件迁移遗漏 | 🟢 低 | grep 检查全部 `package agent` 引用 |

## 迁移路径

### 外部消费者迁移清单

| 消费者包 | 受影响符号 | Phase |
|----------|-----------|-------|
| `desktop/app.go` | `NewSessionPath`, `ListSessions`, `LoadSession` | P1 |
| `desktop/app_meta.go` | `Session`, `NewSessionPath` | P1 |
| `desktop/app_session.go` | `ListSessions`, `LoadSession` | P1 |
| `desktop/app_workspace.go` | `NewSessionPath` | P1 |
| `desktop/settings_app.go` | `NewSessionPath`, `Session` | P1 |
| `desktop/memory_suggestions.go` | `ListSessions`, `LoadSession` | P1 |
| `boot/boot.go` | `NewTaskTool`, `NewAskTool`, `FilterRegistry`, `SubagentMetaTools`, `RunSubAgent`, `NestedSink`, `NewToolDispatcher`, `NewSession`, `New`, `Options` | P1+P2+P3 |
| `control/controller.go` | `Runer`, `Agent`, `Asker`, `NewSessionPath`, `NewSession`, `CacheShape` | P1+P2+P3 |
| `control/controller_checkpoint.go` | `BranchID`, `NewSession`, `NewSessionPath`, `SaveBranchMeta`, `BranchInfo`, `BranchMeta`, `ListBranches`, `LoadSession` | P1 |
| `control/branches.go` | `BranchID`, `BranchInfo` | P1 |
| `control/dream.go` | （indirect via controller） | — |
| `cli/cli.go` | `Renderer`, `NewTextSink`, `LoadSession`, `NewSessionPath`, `ListSessions`, `Session` | P1+P2 |
| `cli/chat_tui.go` | `FormatUsageLine`, `CompactArgs` | P1 |
| `cli/branch.go` | `BranchID` | P1 |
| `cli/acp.go` | `NewTaskTool`, `New`, `NewSession`, `Options` | P1+P2+P3 |
| `cli/resume.go` | `LoadSession` | P1 |
| `serve/serve.go` | （indirect） | — |
| `serve/serve_handlers.go` | （indirect） | — |
| `acp/service.go` | `LoadSession` | P1 |
| `acp/e2e_test.go` | `New`, `NewSession`, `Options` | P1+P2+P3 |

## 验证策略

每阶段完成后运行：
```bash
# 1. 编译检查
go build ./...

# 2. 全部测试
go test ./internal/agent/... -count=1

# 3. 缓存 e2e 测试（Phase 2+ 必须通过）
go test ./internal/agent/ -run CacheHit -count=1 -v

# 4. 外部消费者编译
go build ./internal/boot/... ./internal/control/... ./internal/cli/... ./desktop/...

# 5. 竞态检测
go test -race ./internal/agent/... -count=1
```

## 预计工作量

| 阶段 | 子包 | 预计时间 | 风险 |
|------|------|----------|------|
| Phase 1 | session + budget + textutils | 2h | 🟢 低 |
| Phase 2 | loopguard + render + cache + toolguard | 4h | 🟡 中 |
| Phase 3 | compaction + tasks + agent.go 精简 | 2h | 🔴 高 |
| **总计** | **7 个子包 + agent 精简** | **8h** | — |

## 不做的事情

- ❌ 不引入接口抽象层（用户明确拒绝）
- ❌ 不创建循环 import（子包不 import agent 包）
- ❌ 不改变 `AgentRunner` 的并发安全语义（所有 mutex/atomic 行为不变）
- ❌ 不修改测试逻辑（只更新 import 和类型引用）
