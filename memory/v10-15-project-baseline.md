---
name: v10-15-project-baseline
title: V10.15.0 项目基准
description: V10.15.0 项目基准 — 旧版本（已归档，当前 V10.16.0）
metadata:
  type: project
---

## 当前版本
- **版本号**: V10.15.0
- **发布日期**: 2026-06-29
- **构建产物**: `release/v10.15.0/tianxuan-desktop.exe`
- **SHA256**: `01ac5f6a0397a62959a36b915606f94ebf71b37214a458c48cdd0332b0e0132c`
- **构建命令**: `cd tianxuan/desktop && wails build`
- **发布位置**: `release/vX.Y.Z/tianxuan-desktop.exe`

## 核心变更 — 启动黑屏热修复 + 会话记忆升级 + 前端优化

23 个文件变更（Go 15 + 前端 8）：

### 🔥 关键修复：启动黑屏
1. **react-virtuoso 不兼容回退** — react-virtuoso v4.12~v4.18 在 Wails WebView2 中触发 React error #321，回退到 DOM 原生滚动方案

### 🧠 会话记忆升级
2. **promote_session_facts 工具** — 模型可将临时 session 记忆一键提升为永久存储（新文件 `memory/promote.go`）
3. **sessionFacts 跨轮次存活** — remember(session=true) 记忆跨轮次保持，promote 后落盘

### 🎨 前端体验优化
4. **JumpBar/MessageNavigator 解耦** — 移除 threadEl DOM 引用，改用函数式 scrollToTurn 接口
5. **store.ts 流式优化** — 移除 ensureAssistant，text/reasoning 反向查找避免工具事件清空后创建错误卡片
6. **ToolCard 紧凑化** — 间距/字号/图标/边距全面收缩，节省 30-40% 垂直空间

## 构建位置固定规则
- `wails build` **不加 `-o`**（GitHub Actions），本地构建可用 `-o`
- 发布时拷贝：`cp desktop/build/bin/tianxuan-desktop.exe release/vX.Y.Z/tianxuan-desktop.exe`

## 不变
TCCA 四层架构、事件驱动管线、缓存前缀不变性约束、自我进化原则均不变。

**How to apply:** `cd tianxuan/desktop && wails build`
