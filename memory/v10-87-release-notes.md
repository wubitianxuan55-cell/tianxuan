---
name: v10-87-release-notes
title: V10.87.0 发布记录
description: V10.87.0 发布记录 — 双模型验证闭环 + 18 项改进
metadata:
  type: release-notes
---

# V10.87.0 发布记录

**版本**：V10.87.0
**日期**：2026-07-15
**构建产物**：`tianxuan-v10.87.0-desktop.exe`
**SHA256**：`02cd1ba9fb4fb088acc8c5821d5bfc04d235d5ffeaa207995e1976873050ee67`

## 核心变更：双模型验证闭环

Hermes/Hephaestus 从"规划→执行"一步到位，升级为"规划→执行→验证→修正→完成"智能闭环：

- **验证循环**：`executePlanWithRetry` 最多 3 轮自动修正——首轮执行→Hermes 检查 StepResults→未通过则自动生成修正计划→重新执行
- **allStepsPassed**：用 complete_step 的 StepResults 精确判断每个步骤完成状态
- **planFix**：基于失败步骤自动生成最小修正计划（不重做已通过步骤），自动确认不弹窗
- **Hermes 最终输出**：`synthesizeFinalOutput` 合成简洁完成消息，替代 Hephaestus 冗长总结

## 架构清理

- **移除 taskGate/goalGate**：Hephaestus 简化为纯执行者，不再自判断任务完成。stop_gate.go 从 160 行精简到 25 行
- **删除 judge.go**：独立 judge 模型调用（goalGate 依赖）已无用
- **verifyGate 保留并重新接线**：Hephaestus 仍负责"跑测试"验证，但不再负责"判断任务是否完成"
- **StepResults 追踪**：TurnResult 新增 `StepResults []StepResult`，complete_step 执行时自动填充

## 工作流体验优化

- **智能 auto-confirm**：≤3 步 + 无新建文件的简单计划自动确认，减少一次交互往返
- **Controller.NewSession 修复**：新建会话时重置 Hermes planner session（之前只重置了 executor，导致跨会话泄漏旧项目上下文）
- **快路径 session 注入统一**：`!` 前缀快路径与正常路径使用相同的 session 注入模式，缓存前缀一致
- **wrapExecutorSink 提取**：runFastPath 和 executePlan 中重复的 TurnStarted 抑制逻辑提取为公共方法

## 代码质量

- **planStream 路径 compaction**：当 `readonlyTools==nil` 时的后备路径也触发 planner session 压缩，消除内存泄漏
- **planWithTools 计划提取简化**：从 session 扫描改为直接使用 TurnResult.Summary
- **handoff 消息中文化**：`Original task:` → `任务:`，`Hermes output:` → `计划:`，节省 tokens
- **提示词适配**：HermesPrompt 新增"修正计划"段，HephaestusSystemPrompt 重写为 "Per-step reporting"

## 测试

- 14 个测试覆盖所有新增功能（auto-confirm 5 个 + prompt 工具对齐 1 个 + stop gate 重构 4 个 + verify loop 4 个）
- `go test ./internal/agent/ -count=1` 全部通过（~70s）

## 修改文件清单

| 文件 | 变更 |
|------|------|
| `internal/agent/agent.go` | +StepResult,+StepResults 字段,-taskGateReentry,-goalGateReentry |
| `internal/agent/agent_run.go` | +turnStepResults 收集,+verifyGate 调用,+buildTurnResult 参数 |
| `internal/agent/canonical_todo.go` | +extractStepResult |
| `internal/agent/hermes.go` | +executePlanWithRetry,+allStepsPassed,+planFix,+synthesizeFinalOutput,+wrapExecutorSink |
| `internal/agent/hermes_confirm.go` | +shouldAutoConfirm |
| `internal/agent/hermes_prompt.go` | HermesPrompt +修正计划段,HephaestusSystemPrompt → Per-step reporting |
| `internal/agent/hermes_test.go` | +auto-confirm 测试,+prompt 工具对齐测试 |
| `internal/agent/stop_gate.go` | -taskGate,-goalGate,-countIncompleteTodos (160→25行) |
| `internal/agent/stop_gate_test.go` | 删除 taskGate/goalGate 测试,保留 verifyGate 测试 |
| `internal/agent/judge.go` | 删除 (125行) |
| `internal/control/controller.go` | NewSession() 中调用 hermes.ResetSession() |
