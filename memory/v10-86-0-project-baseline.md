---
name: v10-86-0-project-baseline
title: V10.86.0 项目基准
description: V10.86.0 项目基准 — 旧版本（已归档，当前 V10.87.0）
metadata:
  type: project
---

# V10.86.0 项目基准

**版本**：V10.86.0
**标签**：`v10.86.0`
**提交**：`3d6f637`
**构建命令**：`cd tianxuan/desktop && wails build -ldflags "-s -w -H windowsgui" -o tianxuan-desktop.exe`
**构建产物位置**：`tianxuan/desktop/build/bin/tianxuan-desktop.exe` → 复制到 `release/tianxuan-v10.86.0-desktop.exe`
**核心变更**：8 项代码审查修复 — ProcessCard 图标不可见、CSS 变量未定义、finalReadinessCheck plannerMode 守卫、3 处内存泄漏、LSP pipe fd 泄漏、inline DOM 操作修复
