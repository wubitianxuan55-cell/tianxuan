# Project memory

## Notes

- 思考输出说中文
- 记得使用skill
- **当前版本**: V8.21.0 (2026-06-21)
- V8.21.0: 设计系统落地 — 32 组件令牌迁移 + 配色统一 + DrawerHeader 动画类化 + UI 优化
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

## 🧬 技能系统架构

### 分层设计

```
工具 (Tool) = 原子操作，AI 无法自己完成
  ├─ 文件: read_file, write_file, edit_file, glob, grep, ls...
  ├─ 命令: bash, bash_output, wait, kill_shell
  ├─ 版本: git_diff, git_log, git_status, git_commit
  ├─ 网络: web_fetch, web_search
  └─ MCP:  codegraph_*, gitnexus_*, context7_*, lsp_*

技能 (Skill) = 领域知识或工作流编排
  ├─ 知识型 (inline): 设计系统、中文规范、工具参考
  └─ 流程型 (subagent): 深度探索、代码审查、安全审查
```

### 命名空间规范

| 范围 | 前缀 | 示例 | 说明 |
|------|------|------|------|
| 全局技能 | 无前缀 | `brainstorming`, `chinese-code-review` | 跨项目通用，存于 `~/.tianxuan/skills/` |
| 项目技能 | 领域描述性 | `ui-ux-pro-max`, `gitnexus-guide` | 项目特定，存于 `.claude/skills/` |

### 技能 vs 工具的判断原则

- **能用工具完成的 → 不要包装成技能**。技能不应是工具的 alias
- **有领域数据/知识的 → 做成知识型技能 (inline)**。如 UI 设计数据、规范文档
- **需要多步编排的 → 做成流程型技能 (subagent)**。如 debug、review、explore
- **功能重复时 → 保留内置技能，删除项目/全局副本**

### 当前技能清单 (8 项目 + 18 全局 + 9 内置 = 35)

**项目技能 (8):** banner-design, brand, design, design-system, gitnexus-guide, slides, ui-styling, ui-ux-pro-max
**全局技能 (18):** brainstorming, chinese-code-review, chinese-commit-conventions, chinese-documentation, chinese-git-workflow, dispatching-parallel-agents, executing-plans, finishing-a-development-branch, mcp-builder, receiving-code-review, requesting-code-review, subagent-driven-development, using-git-worktrees, using-superpowers, verification-before-completion, workflow-runner, writing-plans, writing-skills
**内置技能 (9):** init, explore, research, review, security-review, tdd, lsp, context7, debug

### 反模式

- ❌ 不要创建包装 MCP 工具的技能（如 `gitnexus-debugging` — 直接用 `mcp__gitnexus__*` + 内置 `debug`）
- ❌ 不要创建包装内置技能的项目/全局副本（如 `test-driven-development` — 直接用内置 `tdd`）
- ❌ 不要嵌套技能目录（如 `gitnexus/gitnexus-cli/SKILL.md` — 扫描只到 1 层）
- ✅ 技能名用 kebab-case，描述用中文
