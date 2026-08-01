---
name: v10-137-release-notes
title: V10.137.0 发布记录
description: V10.137.0 发布记录 — V10.128~137 累积 10 版本，44 文件 +2179/-219，双模型架构优化 + 单模型 Adaptive Execution 工作流
---

## V10.137.0 发布记录

| 项目 | 值 |
|------|-----|
| 版本号 | V10.137.0 |
| 发布日期 | 2026-08-01 |
| 构建产物 | `tianxuan-desktop.exe` |
| 文件大小 | 21,161,472 bytes (~20.2 MB) |
| SHA256 | `426407380ABB65660F01CD7AB618DF76D18913928E9531698C4F8E70899E3390` |
| 代码变更 | 10 版本累积（V10.128~137），44 文件，+2179/-219 |

### 本发布包含的版本

| 版本 | 主题 |
|------|------|
| V10.128 | 双模型架构优化 — 规划者压缩保留 / 完成判定以计划为准 / 计划解析统一 |
| V10.129 | 双模型架构优化 II — 漏标记补偿 / 路由三值化 / 规划者步数默认上限 |
| V10.130 | 双模型工作流闭环 — 直接执行回灌规划者 / 修正轮次标识 / 修正计划可见 |
| V10.131 | 双模型工作流闭环 II — 步骤覆盖度对照 / 修正计划补全漏签收步骤 |
| V10.132 | 修复执行时代办进度落后 — complete_step 步骤匹配容错 |
| V10.133 | 修复技能/子代理使用统计恒为 0 — 自动注入可观测 + Solo 子代理指引 |
| V10.134 | 单模型架构优化 — AutoPlan 接线（V10.135 已回退，方向修正） |
| V10.135 | 单模型工作流重定义 — Adaptive Execution（回退 AutoPlan 计划确认） |
| V10.136 | Adaptive Execution 细节打磨 — 同步骤失败宿主信号 + Solo 进度保护 |
| V10.137 | 单模型中途纠偏 — 运行中输入即时注入当前任务（steer） |

### 架构里程碑（本发布）

- **双模型（Hermes+Hephaestus）**：规划者压缩 digest 不再被快照恢复丢弃（每轮
  重复 summarizer 成本消除）；allStepsPassed 以计划步数为 ground truth；
  直接执行结果回灌规划者（"继续"类多轮请求有上下文）；verify 四元组新增
  coverage 维度；complete_step 步骤匹配容错（"步骤 2：标题"/"Step 3" 均可命中）
- **单模型（Solo）**：工作流重定义为 **Adaptive Execution**——todo 是活文档
  而非合同、无计划确认往返、失败分级自适应（重试→根因诊断→换方案→收敛）、
  进度保护（8/16 轮）、同步骤持续失败宿主信号、运行中 steer 中途纠偏
- **修复**：技能/子代理统计恒 0（自动注入可观测）；代办进度落后（匹配容错）

### 构建环境（2026-08-01）

| 组件 | 版本/路径 |
|------|-----------|
| Go | 1.26.5（`D:\AI\tianxuanX\tools\go\bin`） |
| Wails CLI | v2.13.0（`C:\Users\Administrator\go\bin`） |
| Node | v26.5.1（`D:\AI\tianxuanX\tools\node`） |
| pnpm | v11.9.0（tools/node） |

### 构建说明

`build-desktop.bat` 需要 PATH 包含 Go / Wails / Node；前端编译依赖 node 命令，
缺 node 会报 `'node' is not recognized`（V10.127 后首次遇到，需 `tools\node`
加入 PATH）。
