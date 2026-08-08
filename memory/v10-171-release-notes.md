---
name: v10-171-release-notes
title: V10.171.0 版本发布
description: V10.171.0 版本发布 — Codex 蒸馏 P2-7 收尾：apply_patch 补丁编辑工具 + tools trace-report + apply_patch 错误细分
---

## V10.171.0 版本发布

| 项 | 值 |
|------|------|
| 版本 | V10.171.0 |
| 日期 | 2026-08-08 |
| 范围 | internal/tool + internal/cli |
| 主题 | Codex 蒸馏 P2-7 收尾：apply_patch 补丁编辑工具 + tools trace-report |
| 产物 | `release/v10.171.0/tianxuan-desktop.exe` |
| 大小 | 25050624 bytes (~23.9 MB) |
| SHA256 | `ab3fb76f0a20c914f8a768c2869ccc58719531319c4a8818afa9ad8c91840a57` |

### 主要变更

| 模块 | 变更 |
|------|------|
| internal/tool/builtin | **apply_patch 工具**（蒸馏 codex freeform 编辑，P2-7）：patch 文本（*** Begin Patch ... *** End Patch）支持 Add File / Delete File / Update File（@@ 锚点 + 上下文行 + 行级 -/+ 增删 + *** End of File 钉尾） |
| internal/tool/builtin | **行级模糊匹配**：忽略行首尾空白（对标 codex seek_sequence），告别 old_string 大小写/空白/CRLF 精确重现；无 @@ 锚点时删除块必须唯一，多处匹配大声报错附消歧指引 |
| internal/tool/builtin | **跨文件原子**：全部 hunk 内存校验通过才写盘；CRLF 保持、权限保留、路径 confine |
| internal/tool + cli | **tools trace-report**：SummarizeTrace 流式读 JSONL，按工具聚合 calls/success/errors/blocked/error_rate/avg_ms + top 3 错误文本，驱动数据化优化 |
| internal/tool | **apply_patch 错误细分**：ClassifyError 支持 patch_parse_error / block_not_unique / block_not_found 分类，tools stats 与 trace-report 有细粒度视图 |

### 验证

- applypatch_test.go 17 例（解析 add/delete/update/@@/EOF/边界/坏行/宽容空白/无删行报错 + 应用增删改/CRLF 保持/原子性/唯一性/@@ 消歧/未找到提示/权限保持）
- trace_report_test.go 5 例 + cli tools_trace_report_test.go 2 例全绿
- `go build ./...` EXIT 0 · `go vet` 干净 · 全量测试仅 5 个预存 API 认证类失败（HTTP 401，key 失效，与本次无关）
- 打包 EXIT 0；SHA256SUMS 与实际 exe 校验一致

### 下一步 5 件事

- 用真实会话积累 tool-trace.jsonl 数据，trace-report 定位最高频失败工具
- 评估 codex tool_search（BM25 动态工具发现，减少首屏认知负担）
- 评估 apply_patch 与 edit_file/edit_lines 的错误率对比（trace 数据验证 freeform 假设）
- 桌面端重启后用新版本验证 apply_patch 在实际会话中的调用质量
- 继续 Codex 蒸馏：对照 codex 参考仓库（6d4d944 无新提交）自审工具失败模式
