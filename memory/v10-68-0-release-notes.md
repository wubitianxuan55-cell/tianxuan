---
name: v10-68-0-release-notes
title: V10.68.0 发布记录
description: V10.68.0 发布记录 — 修复执行者重新探索代码推翻计划的 prompt 约束
metadata:
  type: reference
---

## V10.68.0 发布记录

### BUG 修复 — Prompt 约束强化

**根因**：HephaestusSystemPrompt 中 `adapt` 一词给执行者过多自由裁量权；Hephaestus 拥有完整的 codegraph/grep/glob 探索工具，在执行前会自行重新探索代码再推翻计划。

**HephaestusSystemPrompt 变更**：
- `report deviation in complete_step evidence and adapt` → `report deviation in complete_step as ❌ and move to the next step. Do NOT search for the correct file or fix the plan — Hermes handles replanning`
- 新增 `NEVER re-explore or re-investigate the codebase — Hermes already did all code investigation`
- 明确约束 `Your codegraph/grep/glob tools are ONLY for finding exact edit anchors (old_string in edit_file)`
- `not summaries` → `not investigations`（更精确描述禁止的行为）

**HermesPrompt 变更**：
- `Hephaestus has zero judgment` → `Hephaestus has zero judgment and will NOT re-explore or verify your plan — she trusts it blindly`
- 追加 `make file paths and Verify commands exact`（提醒规划者路径和验证命令必须精确）

### 验证

- `go test ./internal/agent/... -count=1` — 全部通过
- `go vet ./internal/agent/...` — 零警告
- `go build ./...` — 编译通过
