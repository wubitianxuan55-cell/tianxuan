# 同类产品蒸馏计划 — 2026-08

> 目标：从 2026 年主流 AI 编程 agent（Claude Code / Qwen Code / Codex CLI / Gemini CLI）蒸馏
> 下一代协作与自主能力到 tianxuan。
> 本文只做分析与方案设计；具体代码改动需逐项批准后按 TDD 实施。

## 0. 元信息

| 项 | 值 |
|---|---|
| 日期 | 2026-08-05 |
| 状态 | 设计稿 — 待批准 |
| 基线版本 | V10.155.0（Windows 安装包 + 自动更新链路已打通） |
| 范围 | 多 agent 协作层 + 记忆 + 自主模式 + 结构化底座；不动 L1/L2 前缀缓存 |
| 方法 | 对齐 V10.154 Codex 蒸馏：差距分析 → 来源机制 → P0/P1/P2 → 实施顺序 |

## 1. 结论速览

1. **单 agent 能力已完整**：工具层（schema 校验/描述保留/错误统计，V10.154 Codex 蒸馏完成）、
   双模型协调（Hermes 规划 + Hephaestus 执行）、子代理（task / parallel_tasks / subagent_store）、
   记忆（FTS5 + Auto-Dream）、hook（11 类事件）、sandbox、后台任务（jobs）、浏览器面板均已具备。
2. **与 2026 同类产品的差距集中在多 agent 协作层**：tianxuan 的并行子代理
   "各自独立、结果只回父级"，而 Claude Code Agent teams、Qwen Code Agent Team、
   Codex multi-agent v2 都已支持 **agent 间消息传递 + 共享任务池**。
3. **缺少会话级派生与跨重启自主**：Qwen Code 的 `/fork`（中途派发背景 agent，主线程继续）
   与 durable `/loop`（自主模式跨重启恢复）在 tianxuan 没有闭环；现有 jobs 只能跑命令，
   Auto-Dream 只做记忆提取。
4. **V10.154 遗留两个 P2 评估项**（freeform 编辑、ToolDispatchTrace）与本计划 P2 合并评估，
   避免重复立项。
5. 蒸馏候选 6 项：P0×2（背景会话 fork、跨项目记忆）、P1×2（Agent 消息总线、durable 自主模式）、
   P2×2（freeform 编辑、ToolDispatchTrace + 沙箱动态扩展）。预计增量 ~2,500-3,500 行，全部 TDD。
6. **⚠️ 2026-08-05 需求驱动复查**：同类产品有 ≠ tianxuan 需要。按"只吸收能解决 tianxuan
   真实痛点的部分"校准——**P1-1 Agent 消息总线砍掉**（有依赖的任务父代理串行协调更简单可靠，
   子代理互发消息徒增独立 API 调用与成本，违背缓存命脉）；**P1-2 durable 自主模式搁置**
   （交互式编程助手的主场景是用户在场，无人值守推进不是核心需求）；**P0-1 背景会话 fork 保留**
   （撞上真实痛点：子代理看不到父会话上下文，改动小）；**P0-2 跨项目记忆不实施**
   （已被现有 GlobalDir/ScopeUser 覆盖）。后续蒸馏一律先问"tianxuan 需要吗"，再问"同类产品有吗"。

## 2. 同类产品全景与蒸馏历史

| 产品 | 2026 关键能力 | tianxuan 蒸馏历史 | 本次角色 |
|---|---|---|---|
| Claude Code | Agent teams（多会话协作+peer messaging）、Hooks（script/HTTP/prompt/subagent）、Plugins、背景会话、stream-json | superpowers 方法论、receiving-code-review、finish-development-branch（V10.50） | P1-1 Agent 消息总线（文档蒸馏） |
| Qwen Code | Agent Team（并行协作+互发消息）、`/fork` 背景 agent、durable `/loop`、跨项目记忆、`/learn` 技能、@引用历史会话 | —（未蒸馏） | P0-1 fork、P1-2 loop、P0-2 跨项目记忆 |
| Codex CLI | multi-agent v2（leaf models）、review、skills、automations；工具层 json_schema/apply_patch/ToolDispatchTrace | 工具蒸馏全量（V10.154） | P2-5 freeform 编辑、P2-6 ToolDispatchTrace（遗留评估） |
| Gemini CLI | 沙箱（容器/macOS/Windows 动态扩展）、Trusted Folders、工具审批 diff 预览 | Kind 分类/tailToolCall/SubagentDefinition（V10.93） | P2-7 沙箱动态扩展（评估） |
| OpenCode | TUI、75+ 模型、MCP、插件 | task-result 标签（V10.x） | 低优先（已覆盖） |
| Aider | repo map 引用排名 | 已蒸馏（V10.x CoreTypes 排名） | — |
| OpenClaw / Kun / Reasonix | failover / retry_until / 编码管线 / 双模型 | 已多次蒸馏 | — |

## 3. 能力差距矩阵

| 能力域 | tianxuan 现状 | 同类标杆 | 差距 | 优先级 |
|---|---|---|---|---|
| 并行子代理 | parallel_tasks：并发派发、结果合并 | 同 | 无 agent 间通信，无法分工协作 | P1-1 |
| 会话派生 | subagent_store（continue_from）+ jobs（后台命令） | `/fork` 继承上下文的独立 agent 会话 | 无"fork 独立 agent + 主线程继续"闭环 | P0-1 |
| 自主推进 | Auto-Dream（记忆提取）+ jobs（会话内） | durable `/loop` 跨重启恢复 | 无跨重启自主执行 | P1-2 |
| 记忆 | TIANXUAN.md + FTS5（项目级） | 跨项目 memory home | 项目间惯例不可见 | P0-2 |
| Hooks | 11 类事件（含 Pre/PostToolUse、SubagentStop） | 同级别 | 无 | — |
| 沙箱 | seatbelt（macOS）+ bash 包装 | Gemini 动态扩展 | Windows/Linux 弱；动态授权缺失 | P2-7 |
| 工具调用审计 | tool stats（计数落盘） | ToolDispatchTrace（参数/结果/成败） | 无结构化 trace | P2-6 |
| 编辑工具 | edit_file/edit_lines JSON 参数 | Codex apply_patch（freeform+Lark） | old_string 类错误仍是高频 | P2-5 |
| git 工作流 | git 工具 + finish-development-branch 技能 | swarm/Stagewise 原生编排 | 技能已覆盖，无需蒸馏 | — |

## 4. 蒸馏候选清单

### P0-1 背景会话 fork（Qwen Code `/fork` + Claude Code background sessions）

**来源**：Qwen Code `/fork`（2026-06：中途派发背景 agent，继承完整上下文/工具/模型配置，
主线程继续工作，结果可随时取回）。

**差距证据**：`internal/jobs` 能启动/杀死后台命令并输出文本；`internal/agent/subagent_store.go`
支持 PrepareFresh/PrepareContinue/SaveCompleted；但没有"从当前会话 fork 一个独立 agent
会话（继承上下文摘要 + 工具集）、后台持续执行、主线程不阻塞、完成后归档可查"的完整闭环。

**方案**：
- 新增 `internal/agent/fork.go`：`background_fork` 工具，参数 `prompt` / `inherit_tools`
  （默认继承父级过滤后的工具注册表）；内部复用 subagent_store 建独立 session，
  经 jobs.Manager 后台运行，结果写 `archiveDir`。
- `internal/jobs/jobs.go` 增加 `Kind = "agent"` 标签语义（已有 kind 字段，仅约定扩展）。
- desktop 端 `app_agent.go` 暴露 `ListBackgroundAgents` / `ReadBackgroundResult`（事件流已有
  `job:*`，复用）。

**测试计划**：
- fork 生命周期单测：Start → 主线程可继续 → 完成后状态 done → 结果可读（TDD RED→GREEN）。
- 上下文继承测试：父级注入的 L2 信息不进子会话 API 流（缓存不变性断言）。

**验证**：`go test ./internal/agent/...` + `go test ./internal/jobs/...`；`go vet` 干净。

**状态：✅ 已实施（V10.156，2026-08-05）**。落地为 `task` 工具新增 `inherit_context` 参数
（Qwen `/fork` 语义：子代理以父会话最近上下文快照作为首条 user 消息，任务 prompt 保持末条；
无 provider 时大声失败），boot 注入 `forkContextOf`（只读 Snapshot，不触碰父会话前缀缓存）。
新增 5 个测试：task 注入 ×2、fork 上下文提取 ×3，全量 `go test ./...` 通过。
`run_in_background:true` 与 `inherit_context:true` 组合即完整的"带上下文的后台 fork"。

### P0-2 跨项目记忆 home（Qwen Code cross-project memory）

**来源**：Qwen Code 跨项目记忆（2026-06：项目外共享记忆，跨项目惯例/偏好可见）。

**差距证据**：`internal/memory` 的 FTS5 索引与 TIANXUAN.md 均为项目级（工作区根）。
同一开发者换项目后，模型偏好/惯例（"中文输出"、"测试先行"）需要重新建立。

**方案**：
- `config` 新增 `memory_home` 选项（默认 `~/.tianxuan/memory/`，可关闭）。
- `internal/memory` 索引合并：boot 时先建 home 索引（只读），项目索引覆盖同键（项目优先）。
- 检索时两库合并返回，注入路径标注来源（`home:` / `project:`），避免混淆。

**测试计划**：
- 合并优先级单测：同键冲突项目优先；home 独有的键可检索。
- 关闭开关单测：memory_home="" 时不读 home。

**验证**：`go test ./internal/memory/...`；`go vet` 干净。

**状态：❌ 不实施（调研发现已覆盖）**。现有实现已具备跨项目记忆：`Store.GlobalDir`
（`~/.config/tianxuan/memories/`，user/feedback 类型事实落全局目录）、用户全局 `TIANXUAN.md`
（`ScopeUser`）、boot 已注入 `config.MemoryUserDir()`，且有 `store_roots_test.go` 等测试覆盖。
按极简原则不重复蒸馏。

### P1-1 Agent 消息总线（Claude Code Agent teams / Qwen Code Agent Team / Codex multi-agent v2）

**来源**：Claude Code Agent teams（多会话 peer messaging + 共享任务）、Qwen Code Agent Team
（并行 agent 互发消息）、Codex multi-agent v2（子 agent 协作）。

**差距证据**：`parallel_tasks` 的子代理各自独立 session，只能向父级返回最终结果；
无中间消息通道，分工型任务（A 改接口、B 改调用方、互相同步）只能靠父级串行协调。

**方案**：
- 新增 `internal/agent/agentbus.go`：内存消息总线（`send` / `poll` / `list`），
  可选落盘 `.tianxuan/agents/<agent-id>/inbox.jsonl`（跨进程/崩溃可恢复）。
- 工具：`agent_send`（发消息给指定 agent 或广播）、`agent_recv`（拉取自己的消息）、
  `agent_tasks`（列出共享任务池）。注册进子代理工具集（不进父级核心白名单，控制认知负担）。
- **缓存安全**：消息走独立通道，绝不注入 API 消息流；父级只看到最终 `<task-result>`。

**测试计划**：
- agentbus 并发单测：多写者多读者顺序/去重/超时。
- 消息不注入会话断言：子代理收发消息后，父会话消息数组字节不变（复用 cache_diag 断言）。

**验证**：`go test ./internal/agent/...`；新增缓存不变性测试通过。

**状态：❌ 砍掉（2026-08-05 需求驱动复查）**。需要子代理互相通信的任务本质是有依赖
关系的任务——父代理串行协调更简单、更可靠、更省钱（一个会话内解决，而非多个独立 API
会话互相传话）。tianxuan 命脉是 DeepSeek 缓存命中与低成本，子代理已是缓存驱逐者，
再引入 agent 间通信只会放大独立调用与 token 成本。属"同类产品有所以我们也做"的
蒸馏陷阱，YAGNI。

### P1-2 Durable 自主模式（Qwen Code `/loop`）

**来源**：Qwen Code durable `/loop`（2026-06/07：自主推进既有工作，跨重启恢复，
不发明新任务、不做不可逆操作）。

**差距证据**：Auto-Dream 是"事后记忆提取"；jobs 是会话内后台。没有"把循环状态持久化、
重启后从断点恢复推进"的机制。

**方案**：
- 新增 `internal/agent/loop.go`：loop 任务持久化到 `.tianxuan/loops/<id>.json`
  （目标、已完成步、下一步、状态），`tianxuan loop resume <id>` 恢复。
- 安全约束：默认只读工具集（grep/read/code_index）+ 有限步数（默认 20）+ 任何写操作
  前 Ask 审批；`loop stop` 优雅停止并落盘断点。

**测试计划**：
- 持久化恢复单测：模拟执行 3 步后"崩溃"，resume 从第 4 步继续。
- 安全约束单测：写工具被拒绝/要求审批；步数上限触发暂停。

**验证**：`go test ./internal/agent/...`；`go build ./cmd/tianxuan` 通过。

**状态：⏸ 搁置（2026-08-05 需求驱动复查）**。tianxuan 是交互式编程助手，用户在场对话
是主场景；"用户离开、agent 自主推进、重启后恢复"是 Qwen/Claude 无人值守工具的场景。
持久化、恢复、安全约束（只读+审批）投入不小而使用频率低，等出现真实需求再立项。

### P2-5 freeform 编辑工具评估（Codex P2 遗留）

**来源**：Codex `apply_patch`（freeform + Lark 语法，模型直接输出补丁文本，无 JSON 包裹）。

**差距证据**：V10.154 蒸馏报告 §P2-7 遗留；`tianxuan tools stats` 数据驱动决定。
**前置条件**：先跑真实任务采集 stats，确认 `old_string_not_found` 占比仍高再立项。
**风险**：DeepSeek 对 freeform 输出格式的遵从度需实测（比 GPT 系低），需先做 prompt 实验。

### P2-6 ToolDispatchTrace 落盘（Codex P2 遗留）

**来源**：Codex `tool_dispatch_trace.rs`。
**差距证据**：tool stats 已有 tool×error_kind×count 计数；缺参数/结果/耗时的结构化 trace，
无法做错误率仪表与回归对比。**方案**：在 batch_executor 增加可选 trace 落盘
（`.tianxuan/traces/`，JSONL，开关默认关，防体积膨胀）。

### P2-7 沙箱动态扩展（Gemini CLI）

**来源**：Gemini CLI 沙箱（macOS Seatbelt 动态扩展 + worktree 支持、Windows 动态权限）。
**差距**：Windows/Linux 弱；macOS 动态授权缺失。**评估**：先做 macOS worktree +
按需路径授权（对齐 Gemini PR #23301），Windows 动态权限依赖 OS 能力，成本高，暂缓。

## 5. 实施阶段与验收

### 阶段一（P0）：会话派生 + 跨项目记忆
1. P0-2 跨项目记忆（改动独立、最小，先做）。
2. P0-1 背景会话 fork（复用 subagent_store + jobs）。
3. 验收：`go test ./...` 全绿；`go vet` 干净；缓存不变性测试通过；release notes 记录。

### 阶段二（P1）：协作与自主
1. **已取消**：P1-1 Agent 消息总线（需求驱动复查：砍）。
2. **已搁置**：P1-2 Durable 自主模式（等真实需求）。
3. 本阶段暂停，直到 tianxuan 用户侧出现明确痛点或 `tianxuan tools stats` 数据指向
   具体改进方向。

### 阶段三（P2）：按数据立项
1. 跑真实任务采集 `tianxuan tools stats`，决定 freeform / trace 是否立项。
2. 沙箱动态扩展视跨平台资源决定。

## 6. 硬约束

- **缓存命脉**：所有新机制（agent 消息、loop 状态、fork 结果）不得进入 API 消息流；
  涉及 L1/L2/工具列表的任何改动必须逐字节验证前缀不变（verifyPrefixAndShape 守卫）。
- **TDD**：每项先写失败测试 → 最小实现 → 通过；禁止无测试产品代码。
- **手术级变更**：只改任务要求文件；新 orphan 必须清理。
- **无占位符**：每步含完整代码与测试。
- **DeepSeek 适配**：freeform 类方案先做模型输出格式实验，确认遵从度再实施。

## 7. 参照源

| 源 | 位置 | 用途 |
|---|---|---|
| Codex CLI | `D:\AI\refs\codex`（main@6d4d944） | P2-5/P2-6（已蒸馏工具层，遗留评估） |
| Qwen Code | 在线文档（qwenlm.github.io/qwen-code-docs），待克隆 `refs/qwen-code` | P0-1 fork、P1-2 loop、P0-2 跨项目记忆 |
| Claude Code | 官方文档（闭源） | P1-1 Agent teams 消息模型 |
| Gemini CLI | 在线文档 + PR #23301/#23691 | P2-7 沙箱动态扩展 |
| OpenClaw / Kun / Reasonix | `D:\AI\refs\` | 历史蒸馏（已完成，不再重复） |
