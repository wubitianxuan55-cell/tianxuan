---
name: version-history
title: 版本历史
description: 版本历史汇总 — V7.6 到 V10.30 全部主要版本摘要
metadata:
  type: reference
---

## 版本历史

| 版本 | 日期 | 主题 |
|------|------|------|
| V10.30.0 | 2026-07-04 | web_fetch 代理(HTTP CONNECT+SOCKS5) + grep .gitignore 精确行走 + 启动动画重设计 |
| V10.26.0 | 2026-07-04 | Reasonix V1.15 蒸馏完成 + 双模型协调器(planner+executor) + 桌面端适配 |
| V10.25.0 | 2026-07-04 | 统计面板标题栏修复 + 构建脚本 |
| V10.24.0 | 2026-07-04 | agent 包拆分 + 子代理模型选择 + 统计面板优化 + 流式渲染修复 |
| V10.23.0 | 2026-07-04 | 测试修复 + boot 拆分 + 前端测试 + 缓存安全工具 |
| V10.22.0 | 2026-07-04 | 自动路由删除 + 子代理模型自由选择 + 统计面板重设计 + 权限修复 |
| V10.21.0 | 2026-07-04 | 计划模式彻底删除 + 系统提示词更新 — 44文件/-1943行 |
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

## V10.30.0 详情

- **关键特性**: web_fetch 代理支持 + grep .gitignore 精确行走
- web_fetch: HTTP CONNECT + SOCKS5 代理隧道，支持 Proxy-Authorization 认证
- grep: gitignoreWalker ~260行，多层级 .gitignore 解析 + 匹配引擎
- 启动动画: Zap 图标 + 双层旋转环 + 卡片独立品牌色
- StatsPanel: planner 步骤正确归入主模型统计
- SetPlannerModel: ResolveModel 校验支持 provider/model 格式

## V10.26.0 详情

- **关键特性**: Reasonix V1.15 蒸馏完成 + 双模型协调器
- 编码管线: delete_range/delete_symbol/editlines 编码感知读写
- 子代理 transcript 持久化 (SubagentStore/SubagentRun)
- 双模型协调器 (Coordinator, ~260行): planner 流式规划 → executor 执行
- 桌面端: 双视图 Planner 模型选择器 + SetPlannerModel 绑定
