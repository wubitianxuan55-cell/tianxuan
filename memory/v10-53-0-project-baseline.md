---
name: v10-53-0-project-baseline
title: V10.53.0 项目基准
description: V10.53.0 项目基准 — 当前版本、构建命令、核心变更
metadata:
  type: project
---

# V10.53.0 项目基准

## 版本

- **版本号**: V10.53.0
- **分支**: release/v10.17.5
- **发布时间**: 2026-07-10

## 构建

```bash
cd tianxuan/desktop && wails build
```

产物: `build/bin/tianxuan-desktop.exe`
SHA256: `0fb758fcb4637f022582d005f8d08c492400cd7699eb3b2cac2758ccc5d73d76`

## 核心变更

### 规划者（HermesPrompt）全面进化
- 角色定义：可行性/必要性/信息充分性前置评估
- 4 信条加固：证据自检 + API 过时警告 + 可逆性优先
- 分类决策树 → 通用 5 步推理循环
- 新增 Intent check / Engineering judgment / Errors executed blindly

### 计划确认弹窗重构
- planParser 工具 + 10 个测试用例
- PlanCard 重写：摘要栏 + 可折叠步骤卡片 + 降级方案
- `<!--plan-->` 前缀剥离仅传计划正文

### 架构变化
- `hermes.go`：HermesPrompt 常量 ~85→~130 行；Run 方法追加 `strings.Cut` 拆分
- `planParser.ts`：新增解析工具
- `PlanCard.tsx`：Markdown 文本块 → 结构化组件

## 技术栈

- Go 后端 + Wails v2.12 桌面框架
- React 18 + TypeScript 前端
- DeepSeek API (双模型：planner + executor)
