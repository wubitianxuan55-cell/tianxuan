---
name: v10-146-release-notes
title: V10.146.0 发布记录
description: V10.146.0 发布记录 — 修复 edit_lines compact schema 丢失 minimum 约束
---

## V10.146.0 发布记录

| 项目 | 值 |
|------|-----|
| 版本号 | V10.146.0 |
| 发布日期 | 2026-08-01 |
| 代码变更 | 2 文件（compact.go +1 行、compact_wiring_test.go +测试）|
| 主题 | edit_lines 报错修复 |
| 构建产物 | `tianxuan-desktop.exe` |

### 本发布包含的提交

| 提交 | 主题 |
|------|------|
| (本轮) | fix(tool): edit_lines compact schema 补 minimum:1 约束 |

### 根因（5 级追溯）

- 表象：edit_lines 反复报 `start_line must be >= 1`。
- 直接原因：模型参数漏传 start_line（或传 0）。
- 本地根因：模型看到的 CompactSchema 缺 `minimum:1`（完整 Schema 有）。
- 系统根因：compact schema 手写压缩丢失约束——模型无法感知行号 ≥1。
- 过程根因：compact.go 按省 token 精简，未对照完整 Schema 保留约束。

### 修复

- `tool/builtin/compact.go`：edit_lines 的 start_line/end_line 补 `"minimum":1`

### 测试

- `TestEditLinesCompactSchemaMinimum`（新增，RED→GREEN）：断言 compact schema
  必须含 minimum:1，防回归

### 验证

- `go test ./internal/tool/builtin/ -count=1` 全 ok；vet 无告警；build EXIT 0
