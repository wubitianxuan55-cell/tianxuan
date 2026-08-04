---
name: v10-154-release-notes
title: V10.154.0 版本发布
description: V10.154.0 版本发布 — Codex CLI 工具蒸馏（执行前 schema 校验/参数描述保留/错误统计/learning 修复）
---

## V10.154.0 版本发布

| 项 | 值 |
|------|-----|
| 版本 | V10.154.0 |
| 日期 | 2026-08-04 |
| 范围 | internal/tool + internal/agent + internal/learning + internal/provider + internal/cli + internal/boot |
| 主题 | Codex CLI 工具蒸馏：降低工具出错率 |
| 产物 | `release/v10.154.0/tianxuan-desktop.exe` |
| 大小 | 25,054,720 bytes (~23.9 MB) |
| SHA256 | `90a0b4b9ceb6fcbbb8115f3a445a640d68b99c0b4909f39ce71f10ad909ea218` |

### 主要变更

| 模块 | 变更 |
|------|------|
| 执行前校验 (P0) | 内建工具必填/类型/枚举错误以 validation_error 大声失败 + 附带完整 schema 提示；别名兼容并满足必填 |
| 参数描述 (P0) | 30 个工具 compactSchema 补精简英文描述；CanonicalizeSchemaVerbose 修复描述被二次剥离 |
| 错误统计 (P0) | tool × error_kind × count 落盘 .tianxuan/tool-stats.json；`tianxuan tools stats` 查询 |
| learning 修复 | Observe（提取+合并+持久化）替代从未 merge 的 Extract+SaveStore；validation_error 通配分类 |
| 错误反馈 | delete_range/delete_symbol not found/not unique 补恢复 hint |

### 验证

- `go test ./...` 全绿（新增 20+ 用例：校验器/stats/接线/描述保留/learning merge）
- `go vet` 干净 · `wails build` EXIT 0（24.7s）
- SHA256 已核对：`90a0b4b9…` 与 exe 实际一致

### 下一步（数据驱动）

- 跑真实任务后用 `tianxuan tools stats` 量化错误率，验证 schema 校验与描述保留的效果
- 若 old_string_not_found 仍占大头，评估 P2 freeform 编辑工具（受 DeepSeek API 限制需适配）
