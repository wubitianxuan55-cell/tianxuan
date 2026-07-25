---
name: v10-100-project-baseline
title: V10.100.0 项目基线
description: V10.100.0 项目基线 — 当前版本、构建命令、核心变更
---

## V10.100.0 项目基线

| 项目 | 值 |
|------|-----|
| 版本号 | V10.100.0 |
| Git tag | v10.100.0 |
| 分支 | release/v10.88.0 |
| 发布日期 | 2026-07-25 |
| SHA256 | `39a153df8966b1bd77eadb351c0591d79337ddc3efdefc6b08ed27f7ec2fe21f` |

### 构建命令

```
cd D:\AI\gaeaW\desktop && wails build
```

| 参数 | 值 |
|------|-----|
| 构建位置 | `desktop/build/bin/`（固定，禁止移动） |
| 产物名称 | `gaea-desktop.exe` |
| 构建模式 | production |
| 平台 | windows/amd64 |
| 文件大小 | 17,686,528 bytes (~16.9 MB) |

### 核心变更概要

**6 轮蒸馏**（Reasonix v1.17.21 → tianxuan）：
- Prompt 层：Task Contract 五段式、证据强制、Progress guard、研究质量、No-op 标记、角色约束、Goal 三态、Output 段补齐
- Go 代码层：ValidateSerialTodos 状态机守卫、HasFailedCommand/FailedCommands 精确错误区分、LatestTodos

**3 轮代码优化**：
- 消除冗余 FromContext 调用、toEvidenceTodos 单次复用、commandHints 安全副本
- 重复错误消歧、误导 Deprecated 注释删除、ext 查找 O(n)→O(1)

### 文件变更
```
9 files changed, +184/-36 行:
  hermes_prompt.go       — Prompt 蒸馏 + 结构重组
  evidence.go            — HasFailedCommand, FailedCommands, LatestTodos
  evidence_extra.go      — ValidateSerialTodos
  completestep.go        — 证据校验精确错误 + commandHints 安全副本
  completestep_test.go   — 测试期望适配
  todo.go                — 状态机校验集成 + 冗余消除
  compact_summary.go     — ext 查找 O(n)→O(1)
  compiler.go            — 删除误导 Deprecated 注释
  attachments.go         — 重复错误消歧
```
