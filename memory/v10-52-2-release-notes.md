---
name: v10-52-2-release-notes
title: V10.52.2 发布记录
description: V10.52.2 发布记录 — 双模型 Prompt 全面重写 + 执行契约 L2 化 + 整体优化
metadata:
  type: reference
---

## V10.52.2 发布记录

### HermesPrompt 全面重写（~155→90行）

- 7条原则合并为 **4条信条**（Evidence / Push back / Clarify / KISS+design quality）
- 新增 **5分支任务决策树**：纯操作/只读/需澄清/需规划/执行反馈
- 研究阶段 **5个精确终止条件**（文件/签名/影响/测试/探索超越用户提及）
- **Zero flattery** 防献媚声明
- **独立验证** 规则：不轻信用户提供的路径和函数名
- **3-8步粒度约束** + Success 字段强制精确命令
- UI 设计触发条件精确化

### HephaestusSystemPrompt 新增（L2 系统层）

- **执行契约从 handoff 移到 L2 system prompt**，利用 DeepSeek prefix cache 只计费一次
- **6段结构**：Pre-execution ritual / Hermes 契约+偏离规则 / Step execution loop / Tool failure recovery 3级策略 / Failure strategy / Reporting+Instructions
- **Parallel execution** 段：Identify→Dispatch→Collect 三步 + 禁止规则
- 与 AGENTS.md 去重：编码铁律全部由 AGENTS.md 统一提供

### formatHandoff 精简（83→16行）

- 删除重复的执行契约内容，仅保留 handoff 标记 + 任务 + 计划 + 备注
- 与 L2 system prompt 配合减少每轮 token 消耗

### formatExecutionFeedback 结构化

- 从自由文本改为 **Markdown 列表格式**
- 文件名用反引号包裹，所有字段始终存在
- 支持 Hermes 逐字段解析

### 其他清理

- `persistAnswer` 空实现删除（-25行）
- `confirmPlan` default 注释明确语义
- `parallel_tasks` 工具调整 + 品牌图标统一

### 构建

- 构建命令: `cd tianxuan/desktop && wails build`
- 产物: `C:\Users\吴比\AppData\Roaming\tianxuan\bin\tianxuan.exe`
