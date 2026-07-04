---
name: v10-17-project-baseline
title: V10.17.0 项目基准
description: V10.17.0 项目基准 — 旧版本（已归档，当前 V10.30.0）
metadata:
  type: project
---

## 当前版本
- **版本号**: V10.17.0
- **发布日期**: 2026-06-30
- **构建产物**: `release/v10.17.0/tianxuan-desktop.exe`
- **SHA256**: `134144907b8d3fb672bc00fe587db3588c4db26a7d8c2e7d0619d352686bee87`
- **构建命令**: `cd tianxuan/desktop && wails build`

## 核心变更 — 编码修复 + 前端全面重设计
6 个发布分支，22 文件变更，36 包全绿，零测试失败。

### 编码修复（v10.17.0）：63 处 U+FFFD → em-dash/中文
### CSS 冲突消除（v10.17.0）：删除重复 light/warm 块，统一 @media 值
### 记忆面板优化（v10.17.1）：骨架屏 + 自动加载 + FactCard memo
### 变更面板（v10.17.2）：顶栏 GitBranch 按钮 + 变更视图增强
### 设置面板（v10.17.3）：字体选择器修复 + 紧凑模式开关
### 工具卡重设计（v10.17.4）：卡片式布局 + 彩色左边框 + 状态徽章 + 工具栏
### 文本输出重设计（v10.17.5）：聊天式布局 + 代码块增强 + 流式光标

## 前端架构变更
- **Message.tsx**: 聊天式布局（用户右对齐气泡 + 助手左对齐 AI 头像）
- **ToolCard.tsx**: 卡片式布局 + 彩色左边框 + pill 状态徽章 + 工具栏（复制/链接）
- **Markdown.tsx**: 代码块顶部栏（语言标签+复制）+ 表格/引用块样式增强
- **MemoMarkdown.tsx**: 流式末尾闪烁光标
- **MemoryPanel.tsx**: 骨架屏 + 自动加载 + 快捷键增强
- **SettingsPanel.tsx**: 字体选择器（CSS data-* 联动）+ 紧凑模式开关
- **StatusBar.tsx**: GitBranch 变更按钮
- **WorkspacePanel.tsx**: 变更视图增强（统计摘要+自动加载+initialViewMode）
- **FactCard.tsx**: React.memo 包装
- **styles.css**: 主题统一 + ds-pulse 动画

## 不变
TCCA四层架构、事件驱动管线、缓存前缀不变性约束、自我进化原则均不变。

**How to apply:** `cd tianxuan/desktop && wails build`
