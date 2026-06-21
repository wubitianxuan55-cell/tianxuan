# Project memory

## Notes

- 思考输出说中文
-记得使用skill
- 2026-06-21: 首次 git push — 46提交推至 github.com/wubitianxuan55-cell/tianxuan · SSH 密钥配置 · 根目录 README.md
- 2026-06-21: V8.16.1 发布 — 缓存命中率趋势图 Y轴修复
- 2026-06-21: V8.4.1 发布 — Token成本优化+统计修复+7组件UI优化 · 24文件 +451/-275行 · release/v8.4.1/
- 2026-06-20: V8.2.3 发布 — 6项对话窗口核心修复 + 2组件重设计 + 颜色系统修复
- 2026-06-20: V8.2.2 发布 — 10轮 UI 优化：CSS -58.5%, gzip -14.3%, 桌面端构建
- 2026-06-19: V8.2.1 发布 — Tailwind CSS 全量迁移 (24组件) + 6 Go 修复
- 2026-06-13: V7.2.0 发布 — DSR + 三闸门 + 归档 + 6 Bug 修复

## 🔴 核心约束：禁止损害缓存命中率（完整消息前缀不变性）

DeepSeek 前缀缓存是项目成本命脉。**缓存匹配的是整个 API 请求消息数组的连续前缀**——即
`[L1 system | L2 runtime | user msg | assistant msg | tool_result msg | ...]`
中任意位置的任何字节变化，都会导致该位置之后的所有缓存断裂。

**任何修改如果会导致以下情况，绝对不允许：**

1. **L1 Identity 字节变化** — 系统提示词任何字符不可改。由 `verifyPrefix` SHA-256 守卫（漂移 → panic）
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
