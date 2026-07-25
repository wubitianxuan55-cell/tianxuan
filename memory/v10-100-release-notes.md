---
name: v10-100-release-notes
title: V10.100.0 发布记录
description: V10.100.0 发布记录 — Reasonix v1.17.21 蒸馏 + 代码优化，9 文件 184 行新增
---

## V10.100.0 发布记录

| 项目 | 值 |
|------|-----|
| 版本号 | V10.100.0 |
| 发布日期 | 2026-07-25 |
| 构建产物 | `gaea-desktop.exe` |
| 文件大小 | 17,686,528 bytes (~16.9 MB) |
| SHA256 | `39a153df8966b1bd77eadb351c0591d79337ddc3efdefc6b08ed27f7ec2fe21f` |
| 代码变更 | 9 文件, +184/-36 行 |

### 蒸馏（Reasonix v1.17.21 → tianxuan）

#### Prompt 层（hermes_prompt.go）
- **HermesPrompt**：Task Contract 五段式模板（Context/Request/Output/Constraints/Pause）融入 Output 段、研究质量要求（区分已验证/未验证事实）、角色约束（"你的职责是规划"）、No-op 显式标记 `[no_changes]`
- **HephaestusSystemPrompt**：complete_step 证据强制（4 种证据 + 拒绝纯 manual）、Progress guard（8 轮 nudge → 16 轮暂停）、残编号修复
- **SoloSystemPrompt**：Output 段补齐（Direct answer/Ask/Plan+Execute/No-op）、Goal 三态标记（continue/complete/blocked）、研究质量同步

#### Go 代码层
- **evidence_extra.go**：新增 `ValidateSerialTodos` — todo 状态机校验（最多一个 in_progress、有效状态值、level-1 须有 phase 头）
- **evidence.go**：新增 `LatestTodos`、`HasFailedCommand`、`FailedCommands` — 精确区分"命令失败"和"从未运行"
- **todo.go**：Execute 中集成 ValidateSerialTodos 校验、`verifyTodoCompletionTransitionsFrom`（单次转换复用）
- **completestep.go**：增强 `verifyStepEvidence` 精确错误提示（失败 vs 缺失）、`commandHints` 纳入失败命令信息 + 安全副本

### 代码优化

| 优化项 | 文件 | 效果 |
|--------|------|------|
| 消除冗余 FromContext | `todo.go` | 删除重复 context 查找 + shadow 变量 |
| toEvidenceTodos 单次复用 | `todo.go` | 减少 1 次 O(n) 切片拷贝 |
| commandHints 安全副本 | `completestep.go` | 防止调用者变更引发 bug |
| 重复错误消歧 | `attachments.go` | "not a data URL" vs "missing base64 marker" |
| 删除误导 Deprecated | `compiler.go` | Compiler 结构体并未弃用 |
| ext 查找 O(n)→O(1) | `compact_summary.go` | 预计算 codeExts map |

### 暂不蒸馏

- 主机侧 complete_step 证据校验（需 Go 架构变更）
- Final Readiness Check 10 项矩阵（需架构变更）
- YAML frontmatter 子代理 profile（与当前技能体系正交）
- 验证器白名单/Delivery 模式（需完整 delivery 概念）
