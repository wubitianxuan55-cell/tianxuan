---
name: v10-31-project-baseline
title: V10.31.0 项目基准
description: V10.31.0 项目基准 — 当前版本、构建命令、核心变更
metadata:
  type: project
---

# V10.31.0 项目基准

## 版本

- **版本号**: V10.31.0
- **分支**: release/v10.17.5
- **发布时间**: 2026-07-04

## 构建

```bash
cd tianxuan/desktop && wails build
```

产物: `tianxuan/desktop/build/bin/tianxuan-desktop.exe`

## 核心变更

### 双模型弹性降级
- `!` 前缀快速模式：跳过 planner 直接执行
- 启发式检测：短任务 + 简单操作关键词自动跳过
- 文件：`coordinator.go`

### 统计面板重设计
- 规划/执行/子代理三组拆分统计
- 汇总行移至表格下方
- 命中率大字高亮
- 文件：`app.go`, `app_meta.go`, `store.ts`, `StatsPanel.tsx`, `App.tsx`, `types.ts`

### 子代理冷启动优化
- AGENTS.md 工具直用优先规则
- explore body Fast Path 指令
- 文件：`AGENTS.md`, `builtins.go`

### 记忆债务清理
- 旧基准归档 + 版本历史补全
- 文件：`memory/` 目录

## 技术栈

- Go 后端 + Wails v2.12 桌面框架
- React 18 + TypeScript 前端
- DeepSeek API (双模型：planner + executor)
