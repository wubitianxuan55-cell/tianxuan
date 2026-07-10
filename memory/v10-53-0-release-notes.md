---
name: v10-53-0-release-notes
title: V10.53.0 发布记录
description: V10.53.0 发布记录 — 规划者进化 + 计划确认重构 + 构建桌面端
metadata:
  type: reference
---

## V10.53.0 发布记录

### 规划者（HermesPrompt）全面进化

1. **角色定义升级** — 加入可行性/必要性/信息充分性前置三检查，通过才输出计划
2. **4 信条加固**：
   - 证据原则：禁止"我认为/应该是/通常是"断言 + 第三方 API 过时警告
   - KISS 原则：追加可逆性优先——10 行能解决不设计 3 步计划
3. **分类决策树 → 通用推理循环**（5 步：理解意图→收集证据→评估可行性→决策→处理结果），保留执行结果处理，追加结果矛盾时更新理解模型
4. **新增 Intent check 小节** — 检测隐藏意图、过早请求、非代码方案
5. **新增 Engineering judgment 小节** — blast radius / trade-off / scope discipline / priority
6. **新增"Your errors are executed blindly"警示** — Heph 无质疑执行，无安全网

### 计划确认弹窗重构

7. **planParser 工具** — 从 Markdown 计划文本解析结构化步骤数据，10 个测试用例覆盖全部边界
8. **PlanCard 重写** — 摘要栏（步骤数+文件数+文件 chips）+ 可折叠步骤卡片（编号圆形标记/标题/文件标签/成功标准/风险恢复），解析失败降级到 Markdown

### 后端改进

9. **`<!--plan-->` 前缀剥离** — `strings.Cut` 分割后只传递计划正文给确认弹窗和执行者，前言分析不再进入 UI

### 构建

- 分支: `release/v10.17.5`
- 构建命令: `cd tianxuan/desktop && wails build`
- 产物: `build/bin/tianxuan-desktop.exe`
- SHA256: `0fb758fcb4637f022582d005f8d08c492400cd7699eb3b2cac2758ccc5d73d76`
- 变更: 15 文件，+497/-605 行（含 11 个旧记忆文件清理）
