# 上下文管理三层升级 — 设计蓝图

> **蒸馏来源**: MiMoCode (OpenCode fork) 的三层上下文管理
> **目标项目**: Tianxuan (Reasonix) V10.x
> **语言**: 全部 Go（集成到 `tianxuan/internal/`）
> **日期**: 2026-07-15
> **状态**: 设计蓝图 — 待审批后编码

---

## 一、现状分析

### 1.1 Tianxuan 当前架构

```
每轮 LLM 调用后:
  maybeCompact(ctx, usage):
    ├── Tier 1: Soft notice (50%)        ← 一次性提示
    ├── Tier 2: PruneStaleToolResults     ← 依赖 planCompaction 窗口，仅 Compaction 前置
    └── Tier 3: compact()                 ← LLM 摘要 → session.Replace() 原地替换
         ├── planCompaction → 定位 head..start 区域
         ├── partitionFold  → kept(已有digest/短user turn/KeepPolicy) + fold
         ├── summarize      → LLM 生成摘要 → 注入 <compaction-summary> user 消息
         └── session.Replace(compacted)   ← 🔴 完全替换消息数组，破坏前缀缓存
```

**核心问题**：

| # | 问题 | 影响 |
|---|------|------|
| 1 | **Prune 不是独立层** — 仅在 compact 前触发，无 cache-TTL 门控，无两级剪枝，无独立触发阈值 | 工具结果在对话中途就占据大量 token，等到 compact 才清理太晚 |
| 2 | **Compaction 无增量压缩** — 每次 compact 重新扫描全历史，`tail_start_id` 不持久化 | 长会话中 compaction 越做越重，摘要质量下降 |
| 3 | **无 Checkpoint Rebuild** — WriteCheckpoint 从未被调用，CheckpointData 太简单 | 上下文溢出时只能 fallback 到有损 compaction，无法无损恢复 |
| 4 | **session.Replace() 破坏前缀缓存** — 压缩后整个消息数组被替换，L4 前缀断裂 | 每次 compaction 后首轮 cache miss，成本 2.5× |
| 5 | **无微压缩（Microcompact）** — 恢复后不清理可再生 tool result | 上下文中充斥着冗余的旧工具输出 |
| 6 | **无 Agent 隔离** — subagent 的 tool result 混入 main agent 的 compaction 窗口 | 子代理输出污染主代理摘要 |
| 7 | **无溢出文件机制** — CheckpointData 只能存一个 JSON | 大项目上下文无法拆分 |

### 1.2 当前 Compaction 已经做对的地方（保留）

- ✅ `pinnedPrefixLen` 保护 system + firstUser + 已有 digest — 前缀缓存友好
- ✅ `KeepPolicy` 位掩码（KeepErrors / KeepUserMarked / KeepProtected）
- ✅ `protectedTools` 白名单（read_skill / memory_search / remember）
- ✅ tool-call group 完整性保护（kept 的 tool result 连带保留其 assistant 消息）
- ✅ 结构化摘要 prompt（7 标题）
- ✅ `compactStuck` 检测（连续 2 次无效 → 暂停）
- ✅ archive 归档

---

## 二、目标架构：三层升级

```
每轮 LLM 调用后:

┌────────────────────────────────────────────────────────────────┐
│ Step 1: fireCheckpoints()                                      │
│   基于 token 数 vs 配置的阈值表                                  │
│   跨越阈值 → spawn background checkpoint-writer 子代理           │
│   跨越 max 阈值 → 设 maxCrossed 标志                             │
├────────────────────────────────────────────────────────────────┤
│ Step 2: maybePrune()          ← [新] 独立 Prune 层              │
│   触发: cache-TTL (5min) + pressureLevel ≥ 50%                 │
│   两级: soft-trim (首尾保留) + hard-compact (占位符替换)          │
├────────────────────────────────────────────────────────────────┤
│ Step 3: maybeCompact()        ← [改进] 增量 Compaction          │
│   触发: prompt ≥ window * ratio (默认 80%)                      │
│   增量: 从上次 compaction boundary 之后开始折叠                   │
│   overflow: maxCrossed || prompt ≥ forceRatio                  │
│      ├── 有 checkpoint → rebuildFromCheckpoint() ← [新]        │
│      └── 无 checkpoint → compaction (现有路径)                   │
├────────────────────────────────────────────────────────────────┤
│ Step 4: 下一轮继续（Step 1 重新评估）                             │
└────────────────────────────────────────────────────────────────┘
```

### 2.1 三层协作决策树

```
Token 压力检测（每轮结束时）

├── 压力 0-50%:
│   ├── fireCheckpoints: 按阈值表触发
│   └── maybePrune:      cache-TTL 冷却后执行 soft-trim
│
├── 压力 50-80%:
│   ├── fireCheckpoints: 按阈值表触发
│   └── maybePrune:      Level 1 soft-trim + 可能的 Level 2 hard-compact
│
├── 压力 80-90% (overflow imminent):
│   ├── fireCheckpoints: 可能触发 max threshold
│   ├── maybePrune:      Level 2 hard-compact
│   └── maybeCompact:    触发 compaction (LLM 摘要)
│
└── 压力 ≥ 90% (overflow) 或 maxCrossed:
    ├── hasCheckpoint? → rebuildFromCheckpoint（无损重建）← [新]
    └── 无 checkpoint  → compaction（有损回退）
```

---

## 三、Layer 1: 独立 Prune 层

### 3.1 设计目标

将 prune 从 compaction 的前置步骤提升为**独立层**，有自己的触发条件和压力分级，在 token 压力达到 compaction 阈值之前就开始清理。

### 3.2 触发条件

```go
// PruneConfig 控制 prune 行为
type PruneConfig struct {
    Enabled        bool          // 默认 true
    CacheTTL       time.Duration // 缓存冷却时间，默认 5 分钟
    SoftThreshold  float64       // 软修剪触发比例，默认 0.5
    HardThreshold  float64       // 硬剪枝触发比例，默认 0.7
    ProtectTurns   int           // 保护最近 N 个 user turn，默认 2
    ProtectTokens  int           // 保护带 token 数，默认 40_000
    ProtectTools   []string      // 永不剪枝的工具，默认 ["skill"]
}

// pressureLevel 返回 0(无压力), 1(soft), 2(hard)
func pressureLevel(prompt int, window int) int {
    ratio := float64(prompt) / float64(window)
    switch {
    case ratio < SoftThreshold:  return 0
    case ratio < HardThreshold:  return 1
    default:                     return 2
    }
}
```

触发条件（三重门）：
1. `cfg.Enabled` 未禁用
2. `isCacheCold(model, lastAssistantTime)` — 最近一次 assistant 消息距今超过 `CacheTTL`
3. `pressureLevel(prompt, window) > 0` — token 压力 ≥ 50%

### 3.3 核心算法

```go
// PruneStats reports one prune pass.
type PruneStats struct {
    SoftTrimmed  int // 软修剪数量
    HardPruned   int // 硬剪枝数量
    Stripped     int // 媒体/推理清除数量
    SavedChars   int
    Archive      string
}

// maybePrune 检查是否需要剪枝并执行。在每轮结束时调用。
func (a *AgentRunner) maybePrune(ctx context.Context, u *provider.Usage) {
    if !a.pruneCfg.Enabled || a.compaction.Window <= 0 {
        return
    }

    prompt := effectivePrompt(u, a.compaction.LastPrompt)
    if prompt == 0 {
        return
    }

    level := a.pressureLevel(prompt, a.compaction.Window)
    if level == 0 {
        return // 无压力
    }

    // 缓存冷却检查
    if !a.isCacheCold() {
        return
    }

    msgs := a.session.Messages

    // 第 1 步：硬剪枝不可再生的工具结果
    if level >= 2 {
        a.hardPruneToolResults(msgs)
    }

    // 第 2 步：软修剪长工具输出（首尾保留）
    a.softTrimToolOutputs(msgs)

    // 第 3 步：清除非必要的媒体内容和推理文本
    a.stripNonEssential(msgs)
}
```

#### 3.3.1 hardPruneToolResults（Level 2）

从后往前扫描消息：
- 保护最近 2 个 user turn 内的所有消息
- 累计 40K token 保护带内的 tool result
- 超过保护带的旧 tool result → 设为 `prunedMarker = "[Old tool result content cleared]"`
- 遇到 `<compaction-summary>` 摘要边界立即停止
- 白名单工具（skill 等）永不剪枝

#### 3.3.2 softTrimToolOutputs（Level 1）

对 tool result 内容超过 4096 字符的：
- 保留头部 1536 字符
- 保留尾部 1536 字符
- 中间插入 `[...trimmed N bytes...]` 省略提示

#### 3.3.3 stripNonEssential

在保护边界之前的消息中：
- assistant 的 `ReasoningContent` → 清空
- 用户消息中包含 image/audio/video 引用的 → 清空 URL（tianxuan 目前无多模态，此项为将来预留）

### 3.4 需要修改/新增的文件

| 文件 | 操作 | 内容 |
|------|------|------|
| `internal/agent/prune.go` | **重写** | 实现独立 Prune 层：`PruneConfig`、`maybePrune()`、两级剪枝算法 |
| `internal/agent/agent.go` | **修改** | 添加 `pruneCfg PruneConfig` 字段 |
| `internal/agent/agent_run.go` | **修改** | 在 `maybeCompact` 之前调用 `maybePrune` |
| `internal/agent/compact.go` | **修改** | 移除 Tier 2 内嵌的 `PruneStaleToolResults` 调用 |
| `internal/config/` | **新增** | 添加 PruneConfig 的 TOML 解析 |

### 3.5 前缀缓存不变性保证

- Prune 只修改 **tool result 内容**（`Content` 字段）
- 不改变消息结构（Role/ToolCallID/Name 不变）
- 不改变 system 消息和 user 消息
- 不添加或删除消息
- Prune 后的消息数组长度不变
- → **L1 + L2 + tools 前缀完全不受影响**
- → L4 flow 层中：被 prune 的 tool result 对应的 assistant tool_call 保持不变（LLM 仍能看到执行了什么工具）

---

## 四、Layer 2: 增量 Compaction（改进现有）

### 4.1 设计目标

在现有 compaction 基础上升级为**增量压缩**，持久化 `tail_start_id` 实现增量折叠，并修复 `session.Replace()` 破坏前缀缓存的问题。

### 4.2 增量压缩改进

#### 4.2.1 持久化 tail_start_id

当前 `planCompaction` 每次从头计算 head（pinnedPrefixLen），没有记忆上次压缩到哪里。改进：

```go
type CompactionConfig struct {
    // ... 现有字段 ...
    
    // [新] 增量压缩边界：上次 compaction 摘要消息在此后的消息数组中
    // 的索引。下次压缩只折叠 lastCompactionIdx+1 到 tailStart 之间的内容，
    // 而不是重新计算整个 pinnedPrefixLen。
    LastCompactionMsgIdx int  `json:"last_compaction_idx,omitempty"`
}
```

在 `compact()` 成功后更新：
```go
a.compaction.LastCompactionMsgIdx = len(compacted) - len(msgs[start:]) - 1
// 即新摘要消息在 compacted 数组中的位置
```

`planCompaction` 改进：
```go
func (a *AgentRunner) planCompaction(msgs []provider.Message, min int) (head, start int, ok bool) {
    // 优先使用增量边界
    if a.compaction.LastCompactionMsgIdx > 0 && 
       a.compaction.LastCompactionMsgIdx < len(msgs) {
        head = a.compaction.LastCompactionMsgIdx + 1
    } else {
        head = a.pinnedPrefixLen(msgs) // 回退到从头计算
    }
    // ... 后续 tail 计算不变 ...
}
```

#### 4.2.2 修复前缀缓存破坏

当前问题：`session.Replace(compacted)` 创建全新的消息数组。

**解决方案**：改为 **append-only + compaction boundary marker** 模式。

但 tianxuan 的 `Session.Messages` 是 `[]provider.Message`，不是 append-only 的。完全改造为 append-only 模式是更大的架构变更。作为第一步，我们先在 compact 时将摘要追加到原数组末尾，然后用 `filterOldMessages` 在读取时跳过已压缩的消息：

替代方案（更简单）：在 compact 后，**不修改前缀部分**（head 之前的消息原样保留），只在中间区域插入摘要。Go 的 slice 操作天然支持——`session.Replace` 已经这么做了，关键是确保 head 之前的字节完全不变。

验证手段：在 Replace 前后对比 head 前缀的 `fmt.Sprintf("%+v", msgs[:head])` 字符串是否一致。一致 → 前缀缓存安全。

```go
func (a *AgentRunner) compact(...) error {
    // ...现有代码...
    
    // 🔒 前缀不变性验证
    if !prefixEqual(original[:head], compacted[:head]) {
        panic("compaction: prefix changed — cache invariance violated")
    }
    
    a.session.Replace(compacted)
    // ...
}
```

### 4.3 Agent 隔离改进

当前问题：主代理 compaction 时会折叠子代理产生的消息。

**改进**：在 `partitionFold` 中添加对 agent origin 的感知。但 tianxuan 的消息结构没有 `AgentID` 字段。

**方案**：不在消息层面标记 agent ID（会破坏前缀缓存），而是在 subagent 创建时将子代理消息隔离到**独立的 session**。Tianxuan 已经通过 Fork 模式创建独立会话——子代理有自己的 FlowLayer。所以这不是问题。需要确认的是：子代理的 tool result 是否进入了主代理的消息数组？答案是**没有**——子代理通过 Fork 运行在独立会话中，只有其最终回答返回给主代理。所以 agent 隔离已天然满足。

### 4.4 auto-continue（可选）

MiMoCode 的 compaction 支持可选 auto-continue——compact 后自动注入 "Continue if you have next steps" 消息使 agent 继续工作而不是停下来。

**评估**：对 tianxuan 来说，agent 的 runDirect 循环本身就是 while(step < maxSteps)，不会在 compaction 后停止。所以 auto-continue 不需要——tianxuan 的循环天然会继续。

### 4.5 需要修改的文件

| 文件 | 操作 | 内容 |
|------|------|------|
| `internal/agent/compact.go` | **修改** | 添加 `LastCompactionMsgIdx`，`planCompaction` 支持增量；添加前缀不变性验证 |
| `internal/agent/agent.go` | **修改** | `CompactionConfig` 添加新字段 |

---

## 五、Layer 3: Checkpoint Rebuild（新增）

### 5.1 设计目标

这是**最大差距**——tianxuan 完全没有这一层。MiMoCode 的 checkpoint rebuild 是三层架构的皇冠：当上下文即将溢出时，不从零开始用有损摘要，而是从持久化的 `checkpoint.md` 等文件中**无损重建**上下文。

### 5.2 整体流程

```
┌───────────────────────────────────────────────────────────────────┐
│                     Checkpoint Writer 子代理                       │
│                                                                   │
│  fireCheckpoints() 跨越阈值 → spawn writer (Fork 子代理)            │
│    ↓                                                              │
│  writer 读取 session 历史 → LLM 生成 checkpoint.md                  │
│    ↓                                                              │
│  writer 成功 → 更新 last_checkpoint_message_id (watermark)         │
│    ↓                                                              │
│  失败 → watermark 不变，下次 writer 重试                            │
└───────────────────────────────────────────────────────────────────┘

┌───────────────────────────────────────────────────────────────────┐
│                    Checkpoint Rebuild 流程                         │
│                                                                   │
│  overflow 或 maxCrossed 触发:                                      │
│    ↓                                                              │
│  hasCheckpoint? → 否 → fallback compaction (现有路径)               │
│    ↓ 是                                                           │
│  renderRebuildContext():                                          │
│    · Tasks ledger (从 todo_write 提取)                              │
│    · Session checkpoint (checkpoint.md 全文)                       │
│    · Recent user input (verbatim, FIFO)                            │
│    · Project memory (MEMORY.md)                                    │
│    · Session notes                                                 │
│    ↓                                                              │
│  insertRebuildBoundary() → 注入 checkpoint marker + rebuild text    │
│    ↓                                                              │
│  microcompact() → 清除旧的、可再生的 tool_result                    │
│    ↓                                                              │
│  reset thresholds → 下一轮循环继续                                  │
└───────────────────────────────────────────────────────────────────┘
```

### 5.3 Checkpoint Writer

#### 5.3.1 触发阈值表

每个模型的 context window 对应不同的阈值表：

```go
func defaultThresholdsFor(window int) []float64 {
    switch {
    case window < 25_000:
        return nil // 窗口太小，禁用 checkpoint
    case window <= 200_000:
        return []float64{0.20, 0.40, 0.60, 0.80}
    case window <= 500_000:
        return []float64{0.10, 0.20, 0.30, 0.40, 0.50, 0.60, 0.70, 0.80, 0.90}
    default: // > 500K
        // 18 个阈值，密度极高——确保溢出时总有新鲜 checkpoint
        thresholds := make([]float64, 18)
        for i := range thresholds {
            thresholds[i] = float64(i+1) * 0.05
        }
        return thresholds
    }
}
```

设计意图（密度设计）：checkpoint 密度足够高，**确保溢出时总能用 checkpoint 无损重建，几乎不需要回退到有损 compaction**。

#### 5.3.2 Writer 子代理

```go
// checkpointWriterAgent 是一个 Fork 子代理，复用主代理的 L1+L2 前缀。
// 它接收从 last_checkpoint_message_id 到当前 watermark 的消息 delta，
// 生成结构化的 checkpoint.md 文件。
const checkpointWriterPrompt = `You are a checkpoint writer for a coding agent.
Generate a structured checkpoint from the conversation delta below.
Use EXACTLY these sections (omit empty ones):

## §1 Active intent
The user's original request (verbatim in blockquote).

## §2 Next concrete action
The single most important next step.

## §3 Directives (this session)
Working style, constraints, preferences stated by the user.

## §4 Task tree
Current task hierarchy from the todo system.

## §5 Current work
What is being worked on right now.

## §6 Files and code sections
Active files with their key facts — signatures, locations, edits.

## §7 Discovered knowledge
Cross-task discoveries and insights.

## §8 Errors and fixes
Problems encountered and how they were resolved.

## §9 Live resources
Server processes, ports, dev servers currently running.

## §10 Design decisions
Choices made and their rationale.

## §11 Open notes
Miscellaneous items.

Rules:
- Be exhaustive from the delta; prefer over- to under-including.
- Preserve identifiers, paths, line numbers, error messages exactly.
- Do NOT invent anything not present in the messages.
- Keep each section under its budget — truncate with "..." if needed.
- Output raw markdown only — no preamble, no code fences.`
```

#### 5.3.3 Writer 生命周期

```go
type CheckpointWriterState struct {
    writing     bool          // writer 是否正在运行
    pending     *WriterInput  // 单槽队列：最新的待处理请求
    failures    int           // 连续失败计数
    maxFailures int           // 最大连续失败，默认 3
    watermark   int           // last_checkpoint_message_id
    crossed     map[int]bool  // 已跨越的阈值（去重用）
}

type WriterInput struct {
    SessionID  string
    AgentID    string
    Model      ModelInfo
    Tokens     int
}

func fireCheckpoints(sessionID, agentID string, tokens int, model ModelInfo) {
    // 1. 检查 agentID 是否 qualifies（只有 main/peer agent 触发）
    if agentID != "main" {
        return
    }

    // 2. 解析阈值表
    thresholds := defaultThresholdsFor(model.ContextWindow)
    if len(thresholds) == 0 {
        return
    }

    // 3. 计算当前压力比例
    ratio := float64(tokens) / float64(model.ContextWindow)

    // 4. 遍历阈值，检测新跨越
    for _, t := range thresholds {
        idx := int(t * 100)
        if ratio < t || state.crossed[idx] {
            continue
        }
        state.crossed[idx] = true

        // 5. 尝试启动 writer
        started := tryStartCheckpointWriter(input)
        if !started {
            state.failures++
            if state.failures < state.maxFailures {
                delete(state.crossed, idx) // 清除以便重试
            }
        } else {
            state.failures = 0
        }
    }
}
```

### 5.4 Checkpoint 文件

#### 5.4.1 文件布局

```
.mimocode/              (或 .tianxuan/)
├── memory/
│   ├── projects/<hash>/
│   │   ├── MEMORY.md           ← 项目级持久记忆（4 sections）
│   │   └── notes.md            ← 项目级笔记
│   ├── sessions/<id>/
│   │   ├── checkpoint.md       ← 会话检查点（11 sections）
│   │   ├── notes.md            ← 会话级笔记
│   │   └── checkpoint-*.md     ← 溢出文件（section 超预算时）
│   └── global/
│       └── MEMORY.md           ← 全局用户级记忆
```

#### 5.4.2 Section 预算

每个 section 有 token 预算，确保整个 checkpoint.md ≤ 15K tokens：

```go
var checkpointSectionBudgets = map[string]int{
    "§1 Active intent":        500,
    "§2 Next concrete action": 1000,
    "§3 Directives":           800,
    "§4 Task tree":            1000,
    "§5 Current work":         2000,
    "§6 Files and code":       1500,
    "§7 Discovered knowledge": 2000,
    "§8 Errors and fixes":     1500,
    "§9 Live resources":       1000,
    "§10 Design decisions":    3000,  // 最大的 section——决策是核心
    "§11 Open notes":          800,
}
```

#### 5.4.3 溢出机制

当某个 section 超过其预算时：
1. 截断主文件中的 section body，替换为 `See checkpoint-<topic>.md`
2. 将完整内容写入 `checkpoint-<topic>.md`
3. Rebuild 时自动加载溢出文件

### 5.5 Rebuild From Checkpoint

#### 5.5.1 触发条件

```go
func (a *AgentRunner) shouldRebuild(prompt int) bool {
    // 条件 1: overflow（超过窗口限制）
    overflow := prompt >= a.compaction.Window
    
    // 条件 2: maxCrossed（跨越了最高阈值）
    maxCrossed := a.maxCrossed
    
    // 条件 3: 有可用 checkpoint
    hasCP := a.hasCheckpoint()
    
    return (overflow || maxCrossed) && hasCP
}
```

#### 5.5.2 重建上下文组装

```go
func (a *AgentRunner) rebuildFromCheckpoint() error {
    // 1. 等待 writer 完成（如果有正在运行的）
    a.waitForWriter(timeout)

    // 2. 组装重建上下文
    ctx := a.renderRebuildContext()

    // 3. 插入 checkpoint boundary 消息
    a.insertRebuildBoundary(ctx)

    // 4. 微压缩（清除旧的工具结果）
    a.microcompact()

    // 5. 重置阈值和 counter
    a.resetThresholds()

    return nil
}

func (a *AgentRunner) renderRebuildContext() string {
    var parts []string
    
    parts = append(parts, "The following blocks are auto-loaded from your session memory.")
    parts = append(parts, "This is a checkpoint rebuild — you are resuming a previous session.")
    parts = append(parts, "")
    
    // 优先级顺序：
    // 1. Tasks ledger（当前任务状态 — 最重要）
    if tasks := a.extractTaskLedger(); tasks != "" {
        parts = append(parts, "## Tasks ledger")
        parts = append(parts, tasks)
    }
    
    // 2. Session checkpoint（核心 — checkpoint.md 全文）
    if cp := a.readCheckpointFile(); cp != "" {
        parts = append(parts, "## Session checkpoint")
        parts = append(parts, budgetedRead(cp, 11_000)) // cap 11K tokens
    }
    
    // 3. Recent user input（verbatim, FIFO, 最多 16K tokens）
    if recent := a.collectRecentUserInput(16_000); recent != "" {
        parts = append(parts, "## Recent user input")
        parts = append(parts, recent)
    }
    
    // 4. Project memory（跨 session 持久知识）
    if proj := a.readProjectMemory(); proj != "" {
        parts = append(parts, "## Project memory")
        parts = append(parts, budgetedRead(proj, 10_000))
    }
    
    // 5. Session notes
    if notes := a.readSessionNotes(); notes != "" {
        parts = append(parts, "## Session notes")
        parts = append(parts, budgetedRead(notes, 6_000))
    }
    
    // 6. Resumption seam framing
    parts = append(parts, "")
    parts = append(parts, "This session is being continued from a previous conversation.")
    parts = append(parts, "Resume directly. Do not acknowledge this memory dump.")
    parts = append(parts, "Continue with the next concrete action from the checkpoint.")
    
    return strings.Join(parts, "\n")
}
```

#### 5.5.3 微压缩（Microcompact）

在 rebuild boundary 注入后，清除 boundary 之后所有**可再生**的 tool_result：

```go
var microcompactTools = map[string]bool{
    "read_file":    true, // 可重新读取
    "bash":        true, // 可重新执行
    "grep":        true, // 可重新搜索
    "glob":        true, // 可重新 glob
    "ls":          true, // 可重新列出
    "web_fetch":   true, // 可重新获取
    "lsp_hover":   true, // 可重新查询
    "lsp_definition": true,
    "lsp_references": true,
    "edit_file":   true, // 工具调用本身保留，但输出可清除
    "write_file":  true,
}

// 不可清除的工具：actor/task/skill/memory — 携带 LLM 后续引用的状态
```

清除方式：将 tool result 的 Content 设为 `"[Old tool result content cleared — checkpoint rebuild]"`，但保留 tool_use（assistant 的 ToolCalls）不变——LLM 仍能看到执行了什么操作。

#### 5.5.4 与现有 session.Replace 的兼容

Rebuild 不是替换整个消息数组，而是**追加** checkpoint boundary 消息：
1. 当前消息数组不变
2. 追加一条 synthetic user 消息（包含重建上下文）
3. 追加一条 checkpoint marker 消息（标记 rebuild 边界）
4. 对 boundary 之前的消息执行微压缩

Watermark（`last_checkpoint_message_id`）标记 rebuild 之前的最后一条有效消息。后续的 `filterCompactedEffect` 自动跳过被覆盖的旧消息。

### 5.6 预算读取（Budgeted Read）

```go
// budgetedRead 在预算内返回文本，超出则截断。
func budgetedRead(text string, budgetTokens int) string {
    tokens := estimateTokens(text)
    if tokens <= budgetTokens {
        return text
    }
    // 简单截断：按比例保留前面的字符
    ratio := float64(budgetTokens) / float64(tokens)
    keepChars := int(float64(len(text)) * ratio)
    return text[:keepChars] + "\n\n⚠️ Truncated at ~" + 
           strconv.Itoa(budgetTokens) + " tokens"
}

// budgetedReadSectionAware 保留所有 ## section header，
// 只在 body 文本上截断。
func budgetedReadSectionAware(text string, budgets map[string]int) string {
    // 解析 ## sections
    // 每个 section: 保留 header + italic overflow 行（如有）
    // body 文本按 budget 截断
    // 超预算 section → 生成溢出文件 + 添加交叉引用
}
```

### 5.7 需要新增/修改的文件

| 文件 | 操作 | 内容 |
|------|------|------|
| `internal/agent/checkpoint.go` | **重写** | 新 CheckpointConfig + CheckpointData 扩展 + WriteCheckpoint/LoadCheckpoint |
| `internal/agent/checkpoint_writer.go` | **新增** | Checkpoint Writer 子代理：prompt + 文件写入 + watermark 管理 |
| `internal/agent/checkpoint_rebuild.go` | **新增** | Rebuild from checkpoint：renderRebuildContext + insertRebuildBoundary + microcompact + budgetedRead |
| `internal/agent/checkpoint_threshold.go` | **新增** | 阈值表 + fireCheckpoints 逻辑 + F40 排队机制 |
| `internal/agent/checkpoint_budget.go` | **新增** | 预算读取：budgetedRead + budgetedReadSectionAware + 溢出文件 |
| `internal/agent/agent.go` | **修改** | 添加 checkpointCfg、writerState、maxCrossed 等字段 |
| `internal/agent/agent_run.go` | **修改** | 在每轮开始时调用 fireCheckpoints，overflow 时调用 shouldRebuild |
| `internal/agent/compact.go` | **修改** | 添加 LastCompactionMsgIdx 持久化；添加前缀不变性验证 |
| `internal/config/` | **新增** | 添加 CheckpointConfig 的 TOML 解析 |
| `internal/context/flow.go` | **修改** | FlowLayer 支持 partial rebuild（注入初始消息） |

---

## 六、前缀缓存不变性总表

| 操作 | L1 (Identity) | L2 (Runtime) | Tools Schema | L4 前缀 | L4 Tail |
|------|:---:|:---:|:---:|:---:|:---:|
| **Prune (soft-trim)** | ✅ 不变 | ✅ 不变 | ✅ 不变 | ✅ 不变 | ⚠️ 内容截断 |
| **Prune (hard-compact)** | ✅ 不变 | ✅ 不变 | ✅ 不变 | ✅ 不变 | ⚠️ 占位符替换 |
| **Compaction (增量)** | ✅ 不变 | ✅ 不变 | ✅ 不变 | ⚠️ 追加 digest | ⚠️ 折叠旧消息 |
| **Checkpoint Write** | ✅ 后台子代理 | ✅ 复用父前缀 | ✅ 复用 | ✅ 不变 | ✅ 主代理不受影响 |
| **Checkpoint Rebuild** | ✅ 不变 | ✅ 不变 | ✅ 不变 | ⚠️ 追加 rebuild 消息 | ⚠️ microcompact |

图例：
- ✅ = 完全不受影响，缓存命中
- ⚠️ = 内容变化，该点之后缓存 miss，但前缀仍命中

**关键保证**：Prune 和 Compaction 都不改变 `[L1][L2][tools]` 前缀——这些操作只影响 L4 flow 层。

---

## 七、实施计划

### 7.1 分阶段实施

| 阶段 | 内容 | 估计文件数 | 优先级 |
|------|------|:---:|:---:|
| **Phase 1**: Prune 独立层 | 重写 `prune.go`，添加 `maybePrune()`，两级剪枝 | 3-4 文件 | 🔴 最高 |
| **Phase 2**: 增量 Compaction | `LastCompactionMsgIdx` + 前缀不变性验证 | 2 文件 | 🟡 中 |
| **Phase 3**: Checkpoint Writer | Writer 子代理 + 阈值表 + F40 队列 + 文件写入 | 3-4 文件 | 🟡 中 |
| **Phase 4**: Checkpoint Rebuild | Rebuild 流程 + 微压缩 + 预算读取 | 3-4 文件 | 🟢 可后推 |
| **Phase 5**: 配置和集成 | TOML 配置 + agent_run 集成 + 测试 | 2-3 文件 | 🟢 可后推 |

### 7.2 Phase 1 详细步骤

1. **设计 `PruneConfig` 结构体** — `agent/prune.go`
2. **实现 `maybePrune()`** — 三重门检查 + 两级剪枝
3. **实现 `hardPruneToolResults()`** — 保护带 + 白名单 + 摘要边界停止
4. **实现 `softTrimToolOutputs()`** — 首尾保留 + 省略提示
5. **实现 `stripNonEssential()`** — 推理清空 + 媒体清除（预留）
6. **集成到 `agent_run.go`** — 在 `maybeCompact` 之前调用 `maybePrune`
7. **TOML 配置支持** — 可选的 `[prune]` 段
8. **测试** — 验证：保护带边界、白名单豁免、前缀不变性

### 7.3 Phase 2 详细步骤

1. **在 `CompactionConfig` 中添加 `LastCompactionMsgIdx`**
2. **修改 `planCompaction`** — 支持增量边界
3. **添加前缀不变性验证** — `prefixEqual` 检查
4. **测试** — 验证增量压缩后的字节级前缀一致性

---

## 八、风险和注意事项

| 风险 | 缓解措施 |
|------|---------|
| Prune 可能清除 LLM 后续需要的数据 | 保护带（40K）+ 最近 2 turns + 白名单工具；被清除的 tool 有 tool_call 保留 |
| Checkpoint writer 子代理可能失败 | F40 队列 + watermark 不前进 = 下次重试覆盖同一 delta |
| Rebuild 后 LLM 丢失关键上下文 | 优先级排序（task ledger > checkpoint > user input）+ 微压缩只清除可再生工具 |
| session.Replace() 破坏前缀 | 前缀不变性验证（panic 如果改变）+ 增量压缩减少替换范围 |
| 文件 I/O 性能 | Writer 后台异步运行 + checkpoint.md 只写一次 + 溢出文件按需写入 |

---

## 九、未纳入范围的功能（MiMoCode 有但本次不蒸馏）

| 功能 | 原因 |
|------|------|
| 任务树系统（T1→T1.1） | 本次只做上下文管理，任务树是独立功能 |
| Goal Judge（独立目标审判） | 需要独立的 judge 模型调用，且 tianxuan 已有 goalGate |
| cron 调度器 | 独立功能，非上下文管理范畴 |
| 动态 agent 生成 | LLM 生成 agent 配置，非上下文管理 |
| 语音输入 | 硬件相关，非核心 |
| auto-continue | tianxuan 的 runDirect 循环天然支持，不需要 |
