---
name: v10-52-2-project-baseline
title: V10.52.2 项目基准
description: V10.52.2 项目基准 — 当前版本、构建命令、核心变更
metadata:
  type: project
---

# V10.52.2 项目基准

## 版本

- **版本号**: V10.52.2
- **分支**: release/v10.17.5
- **发布时间**: 2026-07-09

## 构建

```bash
cd tianxuan/desktop && wails build
```

产物: `C:\Users\吴比\AppData\Roaming\tianxuan\bin\tianxuan.exe`

## 核心变更

### 规划者（HermesPrompt）全面优化
- 7原则→4信条 + 5分支决策树 + 5个研究终止条件
- Zero flattery + 独立验证规则
- 从 155 行压缩到 90 行

### 执行者（HephaestusSystemPrompt 新增）
- 执行契约从 handoff 移到 L2 system prompt
- Pre-execution ritual / Step execution loop / Tool failure recovery / Parallel execution
- 与 AGENTS.md 去重零重复

### 架构变化
- formatHandoff 精简 83→16 行
- formatExecutionFeedback 结构化
- boot.go 注入 `compiler.WithInstructions(agent.HephaestusSystemPrompt)`
- persistAnswer 空实现删除

## 技术栈

- Go 后端 + Wails v2.12 桌面框架
- React 18 + TypeScript 前端
- DeepSeek API (双模型：planner + executor)
