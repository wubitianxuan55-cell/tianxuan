---
name: v10-68-0-project-baseline
title: V10.68.0 项目基准
description: V10.68.0 项目基准 — 当前版本、构建命令、核心变更
metadata:
  type: project
---

# V10.68.0 项目基准

## 版本

- **版本号**: V10.68.0
- **分支**: master
- **发布时间**: 2026-07-14
- **最新构建**: 2026-07-14 10:51

## 构建

```bash
cd tianxuan/desktop && wails build
```

| 项目 | 值 |
|------|-----|
| 产物路径 | `build/bin/tianxuan-desktop.exe` |
| 安装路径 | `%APPDATA%\tianxuan\bin\tianxuan.exe` |
| 文件大小 | 17,452,032 bytes (~17.5 MB) |
| SHA256 | `cfe683e1218d50f442f76098d88626e6825dd3fa41dc26453bdb16d7b4d8805f` |

## 核心变更

### Prompt 约束强化 (V10.68.0)

1. **HephaestusSystemPrompt** — 删除 `adapt` 裁量权，改为 `report as ❌ and move on`，禁止重新探索代码
2. **HermesPrompt** — 明确告知规划者执行者不会验证计划，要求路径和 Verify 命令精确

### 技术栈

- Go 后端 + Wails v2.13 CLI / v2.12 go.mod
- React 18 + TypeScript 前端
- DeepSeek API (双模型：planner + executor)
