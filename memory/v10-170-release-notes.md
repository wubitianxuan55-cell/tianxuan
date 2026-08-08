---
name: v10-170-release-notes
title: V10.170.0 版本发布
description: V10.170.0 版本发布 — Codex 蒸馏 P2 + 技能并发修复 + bash 结构化头（承载 V10.167/168/169 三项累积变更）
---

## V10.170.0 版本发布

| 项 | 值 |
|------|------|
| 版本 | V10.170.0 |
| 日期 | 2026-08-08 |
| 范围 | internal/tool + internal/agent + internal/skill + internal/boot + internal/cli + desktop |
| 主题 | Codex 蒸馏 P2 + 技能并发修复 + bash 结构化头 |
| 产物 | `release/v10.170.0/tianxuan-windows-amd64-installer.exe` |
| 大小 | 10.2 MB（10733176 bytes） |
| SHA256 | `bcb01e53b32fd4fcf20b7d524b9e20a110724e63d025707315bcfb983884ded8` |

### 主要变更（承载 V10.167/168/169）

| 版本 | 模块 | 变更 |
|------|------|------|
| 167 | internal/tool + agent | **get_context_remaining 工具**：模型主动查询上下文剩余 token，executeOne 注入 tokensLeft 闭包（Window − chars×tokPerChar），长任务窗口溢出前规划收敛 |
| 167 | internal/tool + agent + cli | **ToolDispatchTrace 落盘**：TraceStore JSONL 追加每次工具调用（ts/session/trace/call_id/tool/args/outcome/error/duration），参数截断 500/错误 300，懒打开不持句柄；CLI `tianxuan tools trace [-n N]` |
| 168 | internal/agent | **修复技能工具不并发**：getConflictKey registry 感知，ReadOnly 技能工具（explore/research/review 等）返回 ro: 共享键互相并行，与写工具 file:* 互斥保住写后读顺序；根因 V10.124→V10.147 技能工具化回归 |
| 169 | internal/tool/builtin | **bash 失败输出结构化头**（对标 codex format_exec_output_for_model）：plain 模式失败输出 Exit code / Wall time / [Total output lines] / Output，成功与 JSON 模式契约不变 |

### 验证

- `go test ./...` 48 包全绿（仅 5 个预存 API 认证类失败：4 个真实 API 401 + 1 个 zen provider 环境变量）
- `go vet` 干净 · `go build ./...` EXIT 0
- 技能并发端到端：3 只读工具 0.30s 并行（修复前 0.90s 串行）
- bash 失败输出真实核对：`Exit code: 9 / Wall time: 0.6s / Total output lines: 3000`

### 下一步 5 件事

- 用真实会话积累 tool-trace.jsonl 数据，驱动下一轮错误率优化
- 评估 codex freeform 编辑工具（P2-7 遗留，Lark 语法）
- 评估 codex tool_search（BM25 动态工具发现）
- 桌面端重启后用新版本验证 get_context_remaining 在实际会话中的调用
- 观察技能工具并行是否改善多技能任务的实际体验
