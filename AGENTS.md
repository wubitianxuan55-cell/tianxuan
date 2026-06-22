# Project memory

## Notes

- 思考输出说中文
- **技能优先原则**（Reasonix 模式）：在开始任何工作之前，必须先检查 Skills 索引中是否有匹配的技能。技能 = 领域知识 + 工作流编排，使用技能比从零开始更高效、更正确。具体规则：
  - 在任何创造性工作（创建功能、构建组件、添加功能、修改行为）之前，必须先调用 brainstorming 技能探索用户意图和设计
  - 在理解不熟悉的代码之前，先使用 explore 技能进行深度探索
  - 在修改代码前，使用 review 或 security_review 技能进行审查
  - 在编写实现代码之前，通过 Plan 模式制定方案——Plan 模式确保只读探索
  - 涉及 GitNexus 代码图谱操作时，使用 gitnexus-guide 技能获取完整工具参考
  - 涉及 UI/UX 设计时，使用 ui-ux-pro-max 或 ui-styling 技能获取设计系统数据
- **当前版本**: V8.23.0 (2026-06-22)
- V8.23.0: 桌面端 agentMode 自由切换 — Go 桥接 + TS 全栈 + Composer/StatusBar UI + i18n
- V8.22.0: 技能触发策略强化 + L1 Token 优化 — Reasonix push-based 五层机制 + 描述精简 ~2,700 tok/会话
- V8.18.0: 缓存架构重构 — compact 从纯截断改为 LLM 摘要 digest 累积 + PrefixShape 诊断统一
- V8.17.0: 跨项目吸收 — DeepSeek thinking 注入 + retrieval/sysproxy/netclient/proc 4 模块
- V8.16.2: 仓库公开 — 首次 git push + README（简略）

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
