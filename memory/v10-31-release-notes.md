---
name: v10-31-release-notes
title: V10.31.0 发布记录
description: V10.31.0 发布记录 — 双模型弹性降级 + 统计面板重设计 + 子代理冷启动优化
metadata:
  type: reference
---

## V10.31.0 发布记录

### 双模型弹性降级
- `!` 前缀快速模式跳过 planner，启发式自动检测简单任务
- Phase 显示 `模型名 · 快速执行`

### 统计面板规划/执行拆分
- 会话级和本轮级拆分规划/执行/子代理三组统计
- 汇总行移至表格下方，命中率大字高亮
- 规划/执行/子代理分别使用各自的 modelPrice

### 子代理冷启动优化
- 简单查询优先用 lsp_definition/codegraph 直接工具
- explore 子代理 Fast Path 指令

### 记忆债务清理
- 旧基准归档，版本历史补全至 V10.30

### 构建
- SHA256: a688bb0b3744cdb61c48da48b4556b1047466b88a5cd81a9fcbebae79fa79d91
- 构建命令: cd tianxuan/desktop && wails build
