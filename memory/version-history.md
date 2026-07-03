---
name: version-history
title: 版本历史
description: 版本历史汇总 — V7.6 到 V10.20 全部主要版本摘要
metadata:
  type: reference
---

## 版本历史

| 版本 | 日期 | 主题 |
|------|------|------|
| V10.20.0 | 2026-07-03 | 记忆升降级 + 2阻塞修复 + 2Bug修复 + 清理 — 74文件 |
| V10.19.0 | 2026-07-03 | 系统模式重构(AgentMode→PermLevel) + 代码清理 + 前端优化 — 71文件 |
| V10.17.0 | 2026-06-30 | 编码修复+前端全面重设计 — 6分支/22文件 |
| V10.16.0 | 2026-06-30 | Bug修复+设计加固+性能优化+测试恢复 — 10 commits/30+文件 |
| V10.15.0 | 2026-06-29 | 启动黑屏热修复 + 会话记忆升级 + 前端优化 |
| V10.14.0 | 2026-06-29 | 自我进化迭代 — Reasonix 吸收 + 速度优化 |
| V10.13.0 | 2026-06-29 | 体验打磨 — 清除计划模式概念 + 流式闪烁修复 + 工具卡紧缩 |
| V10.12.0 | 2026-06-29 | 对话流式输出完整重设计 — 虚拟列表 + BEM 语义层 + 配色系统 |
| V10.11.0 | — | 体验优化迭代 |
| V10.10.0 | — | 16 项改进：Bug修复+代码清理+opencode吸收+跳转修复 |
| V10.9.0 | — | 记忆建议引擎 + 多标签页骨架 + UI 增强 |
| V10.8.1 | — | 会话体验优化 |
| V10.8.0 | — | 3 项智能化优化 |
| V7.5.0 | — | 初始提交 |

## V10.20.0 详情

- **产物**: `release/v10.20.0/tianxuan-desktop.exe`
- **SHA256**: `fde38adb2259d1eee69c41841916b2c8fe4f49866ae462a0a55f879c3cb2fc3b`
- **构建命令**: `cd tianxuan/desktop && wails build`
- **变更**: 74 文件，+1991/-1786 行

1. 🔴 阻塞修复：controller deadlock（Unlock 补回）+ permLevel 出厂 YOLO
2. 🆕 记忆 Type 升降级：Store→Controller→Wails + FactCard 按钮组
3. 🐛 StatsPanel 新会话统计不重置（skipWriteRef 守卫）
4. 🐛 消息面板点击不跳转（turnEls 清理 + items 重置清空）
5. 🧹 websearch DDG 死代码 ~150 行 + StatusBar agentMode/yolo 清理

## V10.19.0 详情

- **产物**: `release/v10.19.0/tianxuan-desktop.exe`
- **SHA256**: `6d4bb02d779b6e75d44a49fc77b2803008dc5a4019fa7738ff36b0d96fae2164`
- **变更**: 71 文件，+1891/-1704 行

1. 系统模式重构：AgentMode(explore/develop/orchestrate) → PermLevel(ask/auto/yolo)
2. 删除 mode_classifier.go + /perm 命令
3. DefaultSystemPrompt 精简 ~43→~33 行
4. 删除 L2Dir 死字段 + stopGate() ~72 行
5. 前端：usePaletteItems + useGlobalShortcuts + KaTeX 延迟 + 流式预览
