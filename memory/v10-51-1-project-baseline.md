---
name: v10-51-1-project-baseline
title: V10.51.1 项目基准
description: V10.51.1 项目基准 — 当前版本、构建命令、核心变更
metadata:
  type: project
---

# V10.51.1 项目基准

## 版本

- **版本号**: V10.51.1
- **分支**: release/v10.17.5
- **发布时间**: 2026-07-09

## 构建

```bash
cd tianxuan/desktop && wails build
```

产物: `tianxuan/desktop/build/bin/tianxuan-desktop.exe`

## 核心变更

### 重启后历史会话中文输入显示修复
- 双模型模式下 handoff prompt 覆盖原始中文输入的 Bug 修复
- `Hermes.Run` 先注入 origInput，再调用 formatHandoff
- `History()` 提取 handoff 消息中的原始任务文本并显示
- 文件：`agent.go`, `agent_run.go`, `hermes.go`, `app_session.go`

### 历史显示补齐
- `History()` 增加 `StripTransientBlocks` 调用
- handoff 前缀检测 + extractOriginalTask 提取
- 文件：`app_session.go`

## 技术栈

- Go 后端 + Wails v2.12 桌面框架
- React 18 + TypeScript 前端
- DeepSeek API (双模型：planner + executor)
