---
name: v10-19-project-baseline
title: V10.19.0 项目基准
description: V10.19.0 项目基准 — 当前版本、构建命令、核心变更
metadata:
  type: project
---

## 当前版本
- **版本号**: V10.19.0
- **发布日期**: 2026-07-03
- **构建产物**: `release/v10.19.0/tianxuan-desktop.exe`
- **SHA256**: `6d4bb02d779b6e75d44a49fc77b2803008dc5a4019fa7738ff36b0d96fae2164`
- **构建命令**: `cd tianxuan/desktop && wails build`

## 核心变更 — 系统模式重构 + 代码清理 + 前端优化
71 文件变更，+1891/-1704 行，TypeScript 零错误。

### 系统模式重构 (AgentMode → PermLevel)
- 删除 mode_classifier.go（NLP 分类器）
- AgentMode(explore/develop/orchestrate) → PermLevel(ask/auto/yolo)
- 新增 /perm 命令快速切换权限级别
- 移除 deprecated SetBypass/Bypass（全调用侧迁移）
- Compose() 简化：移除 3 个 mode marker 常量
- 前端 Composer/StatusBar 统一为 permLevel

### System Prompt 优化
- DefaultSystemPrompt 精简 ~43行→~33行（去除英文冗余，整合 Batch Execution）
- 移除 boot.go 冗余追加行

### Go 后端清理
- 删除 L2Dir 死字段（7处只写不读）
- 删除 stopGate() 遗留代码（~72行 + 2个重复常量）
- 重写 gate 测试（14个新测试）
- 移除 boot.go 多余赋值

### 前端优化
- App.tsx 使用 usePaletteItems + useGlobalShortcuts hook
- sessionTitle 3处重复 → 统一 import
- ProcessCard 删除 3 个死代码 icon
- 5 个组件添加 memo
- KaTeX CSS 延迟加载
- 流式预览增强 + AskCard 键盘交互

## 不变
TCCA四层架构、事件驱动管线、缓存前缀不变性约束、自我进化原则均不变。
