# Project memory

## Notes

- 思考输出说中文
- **工具优先原则**：tianxuan 是专业编程 agent，所有方法论已融入工具描述和系统宪法，自动生效，无需显式调用。
- **编码铁律**（自动生效）：
  - 🔴 **设计优先**：编码前必须用 `design_session` 探索需求。禁止跳过设计直接编码。
  - 🔴 **TDD**：无失败测试不写产品代码。先写测试→确认失败→最小实现→确认通过。提前写的代码必须删除。
  - 🔴 **验证强制**：声称“已修复/已完成”前必须运行 `verify`。`complete_step` 拒绝纯 manual 证据。
  - 🔴 **无根因不修复**：先重现→隔离根因→再修复。3+修复失败=架构问题。
  - 🔴 **无占位符**：禁止 TODO/TBD/“add error handling”。每步必须含完整代码。
  - 🔴 **持续执行**：计划执行中不中途汇报。仅在 BLOCKED/模糊/完成时停止。
  - 🔴 **拒绝谄媚**：审查反馈时禁止表演性同意。技术正确性优先于社交舒适。
- **子代理隔离**：复杂任务通过 `task` 工具派发独立子代理，两阶段审查。
- **计划粒度**：`todo_write` 每步 2-5 分钟，含精确文件路径和测试代码。
## 🔴 核心约束：禁止损害缓存命中率（完整消息前缀不变性）

DeepSeek 前缀缓存是项目成本命脉。**缓存匹配的是整个 API 请求消息数组的连续前缀**——即
`[L1 system | L2 runtime | user msg | assistant msg | tool_result msg | ...]`
中任意位置的任何字节变化，都会导致该位置之后的所有缓存断裂。

**任何修改如果会导致以下情况，绝对不允许：**

1. **L1 Identity 字节变化** — 系统提示词任何字符不可改。由 `verifyPrefixAndShape` 守卫（漂移 → Warning Notice）
2. **tools 列表 session 中途变化** — 不能按输入动态增删工具。V8.0.2 `filteredSchemas` 致命事故：命中率从 99% 降到 0%
3. **L2 Runtime 首轮后变化** — 运行时上下文（`compactSystemPrompt` 输出）锁定后不可变
4. **动态系统提示词注入** — 不能在 user 消息前插入可变文本，会破坏 [L1+L2+user...] 前缀
5. **工具描述热更新** — session 中途不能改 CompactDescriptor
6. **L4 流中任何消息的字节变化** — compaction 摘要、grep 压缩、diff 输出、tool_result 等**所有进入 API 消息的内容**，修改后必须逐字节验证与旧版一致

原理：DeepSeek prefix cache 按"前缀连续匹配"计费。1 字节差异 = 整轮 cache miss = 2.5 倍费用。**不是仅 L1+L2，而是整个消息数组前缀。**

修改涉及以下任何文件/机制前，必须先确认不会破坏前缀不变性：
- `internal/cache/` 四域管理（含 `runtime.go:compactSystemPrompt`）
- `internal/context/` TCCA 内核
- `internal/agent/agent.go` 中的 `filteredSchemas`/`activeSchemas`/`verifyPrefix`
- `internal/agent/compact.go` 中的 `BuildCompactSummary`（注入 user 消息）
- `internal/agent/compress.go` 中的 `compressTree`/`formatGrepOutput`（注入 user 消息）
- `internal/diff/diff.go` 中的 diff 输出（作为 tool_result 进入消息流）
- `internal/boot/boot.go` 中的系统提示词构建
- 工具注册表 (`internal/tool/`) 的热加载路径

## 🔗 Git 仓库

- **远程**: `git@github.com:wubitianxuan55-cell/tianxuan.git`
- **默认分支**: `main`（本地 `master` 推送至远程 `main`）
- **SSH 密钥**: `~/.ssh/id_ed25519` (ed25519, esengine@tianxuan.dev)
- **首次推送**: 2026-06-21 · 46 提交

## 📐 架构文档

- **当前架构**: [V8.0-ARCHITECTURE.md](tianxuan/V8.0-ARCHITECTURE.md) — V8.0.6 完整架构 (2026-06-18)
- 历史: [_archive/V5.0-ARCHITECTURE.md](tianxuan/_archive/V5.0-ARCHITECTURE.md) (已过时) | [_archive/V6.0-ARCHITECTURE.md](tianxuan/_archive/V6.0-ARCHITECTURE.md) (设计草稿)
- 设计愿景: [.tianxuan/specs/2026-06-02-tianxuan-design.md](.tianxuan/specs/2026-06-02-tianxuan-design.md)

## 🧬 技能系统

工具 (Tool) = 原子操作；技能 (Skill) = 领域知识/工作流编排。技能索引由系统提示注入（见 Skills 列表），此处仅记录架构原则。
- 能用工具完成的 → 不包装成技能；有领域数据的 → inline 技能；需多步编排的 → subagent 技能
- 反模式: ❌ 包装MCP工具 ❌ 包装内置技能 ❌ 嵌套技能目录 ✅ kebab-case命名+中文描述

## 🦸 Superpowers 开发方法论（内置工具）

以下法则来自 [obra/superpowers](https://github.com/obra/superpowers)，已内置为 tianxuan 一等工具，无需调用技能即可使用：

### 1. 设计优先法则 → `design_session` 工具
编码前必须先探索需求、提出方案、获得批准。使用 `design_session` 工具进入结构化设计流程：
- **explore** → 探索项目上下文（文件、文档、git 历史）
- **clarify** → 逐轮提问，每次一个问题，优先选择题
- **design** → 提出 2-3 个方案（含利弊），推荐最佳方案
- **review** → 展示设计文档分节，每节获得批准
- **done** → 写入 `docs/specs/YYYY-MM-DD-<topic>-design.md`，提交到 git
- 禁止在用户批准设计前开始编码。即使是"简单"任务也必须走流程——简单任务的设计可以很短，但不能跳过。
- 反模式："这个太简单不需要设计"——简单项目是未检查假设造成浪费最多的场景。

### 2. TDD 铁律 → `verify` + `complete_step` 工具
先写失败测试 → 运行验证失败 → 最小实现 → 运行验证通过 → 提交。
- 严禁在测试通过前写多于测试要求的代码（YAGNI）
- 使用 `verify` 工具运行测试命令并结构化检查结果
- `complete_step` 要求至少 1 条 verifiable 证据（verification/diff/files），纯 manual 不通过

### 3. 子代理隔离 → `task` / `parallel_skills` 工具
每个任务用独立子代理执行，两阶段审查（规格合规 → 代码质量）。
- 子代理没有父会话上下文，必须写自包含任务提示
- 审查按严重性分级：必须修复 / 建议修改 / 仅供参考

### 4. 验证强制 → `verify` 工具
完成任何步骤前必须运行验证命令并提供证据。
- `complete_step` 拒绝无 verifiable 证据的完成声明
- `verify` 工具运行命令并返回结构化结果（退出码/stdout/stderr/耗时）
- 声称"已修复"或"已完成"前必须展示验证输出

### 5. 计划粒度 → `todo_write` 工具
每个步骤 2-5 分钟，带精确文件路径和完整代码。禁止 TODO/TBD/占位符。
- 步骤必须是可独立测试的最小单元
- 每个步骤含：文件路径、接口定义、测试代码、验证命令

### 6. 无占位符 → 全局约束
禁止 "TODO"、"TBD"、"implement later"、"add error handling"、"add validation"、"handle edge cases"。
- 每个步骤必须包含完整代码，不能引用"类似第N步"——子代理可能乱序执行
- 类型/签名必须在首次出现时定义，后续步骤引用必须一致
- **计划失败模式**（写入计划时禁止）：
  - "TBD"/"TODO"/"implement later"/"fill in details"
  - "Add appropriate error handling" / "add validation" / "handle edge cases"
  - "Write tests for the above"（无实际测试代码）
  - "Similar to Task N"（重复代码——工程师可能乱序读任务）
  - 引用未在任何任务中定义的类型/函数/方法

### 7. 持续执行 → 全局约束
执行计划时，不要在各任务之间停下来向用户汇报进度。“Should I continue?”和进度摘要浪费用户时间——用户要求执行计划，就执行到底。
仅在以下情况停止：BLOCKED 状态无法自行解决、真正阻碍进展的模糊性、所有任务完成。

### 8. 常见推辞识别（来自 Superpowers）

当出现以下想法时——停下来，这是理性化借口：

| 推辞 | 现实 |
|------|------|
| "这只是个简单问题" | 问题就是任务。检查是否有匹配的技能/工具。 |
| "我需要先了解上下文" | 技能/工具告诉你如何了解。先用 design_session。 |
| "让我先探索代码库" | 技能告诉你怎么探索。先用 explore。 |
| "这个不需要走正式流程" | 如果存在技能/工具，就用它。 |
| "我记得这个技能的内容" | 技能在进化。读当前版本。 |
| "技能/工具太小题大做了" | 简单事情变复杂。用它。 |
| "我先快速做一件事" | 先检查技能/工具再做任何事。 |
| "这个太简单不需要设计" | 简单项目是未检查假设造成浪费最多的场景。 |
