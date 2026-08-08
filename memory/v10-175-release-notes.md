---
name: v10-175-release-notes
title: V10.175.0 版本发布
description: V10.175.0 版本发布 — Codex 蒸馏能力级提升（write_stdin/token 预算/hook 进程树/windows_shell_guidance/search_tool/precheck nearest line）+ 过程卡工作流 + 输出简洁
---

## V10.175.0 版本发布

| 项 | 值 |
|------|------|
| 版本 | V10.175.0 |
| 日期 | 2026-08-08 |
| 范围 | internal/agent + internal/tool + internal/hook + internal/jobs + internal/config + internal/boot + desktop/frontend |
| 主题 | Codex 蒸馏能力级提升 + 过程卡工作流 + 输出简洁 |
| 产物 | `release/v10.175.0/tianxuan-desktop.exe` |
| 大小 | 25132032 bytes (~24 MB) |
| SHA256 | `27bb36ee3bef1f7adfdc677790dea34859407463d871fbdf7826d217f9f1a554` |

### 主要变更

| 模块 | 变更 |
|------|------|
| internal/jobs + tool/builtin | **write_stdin 交互式进程管理**（蒸馏 codex 核心能力）：bash interactive 参数建 stdin 管道，write_stdin 工具驱动 REPL/交互式 CLI/调试器；写后轮询 500ms 返回最新输出 |
| internal/agent + config | **会话级 token 预算渐进提醒**（蒸馏 codex rollout_budget）：跨 turn 累计用量，跨阈值注入渐进式 user 提醒；agent.token_budget_limit + token_budget_reminders 配置 |
| internal/hook | **hook 超时递归杀进程树**（蒸馏 codex dd91642）：proc.StartTracked + KillTracked 回收整棵进程树 |
| internal/tool/builtin | **windows_shell_guidance**：bash PowerShell 描述注入三条 Windows 安全规则 |
| internal/agent | **search_tool**（轻量 tool_search）：关键词搜工具目录，解决 MCP 工具名猜名问题 |
| internal/agent + tool/builtin | **precheck nearest line**：编辑失败报错补最近行提示 |
| desktop/frontend | **过程卡工作流**：每轮结束整轮折叠成大过程卡（warm-turn），只留最新轮正文；生成中文本↔过程卡交替 |
| internal/agent | **输出简洁**：SoloSystemPrompt 加强约束（3-10 行，只写 changed/verified/remains） |

### 验证

- `go test ./...` 48 包全绿（仅 5 个预存 API 认证类失败：4 个真实 API 401 + 1 个 cli 配置）
- `go vet ./...` 干净 · `wails build` EXIT 0
- 前端 `tsc --noEmit` 0 错误 · vitest 142 例全绿（15 文件）
- 新增 TDD：token_budget 4 例、write_stdin jobs 3 例 + builtin 端到端 2 例、
  hook 进程树 1 例（孙进程回收）、precheck nearest line 1 例、search_tool 11 例
- SHA256SUMS 与实际 exe 校验一致；exe 修改时间 = 本次构建时间（18:56）

### 下一步 5 件事

- 真实会话积累 trace 数据，trace-report 验证 write_stdin/search_tool 实际错误率
- 评估蒸馏 codex AGENTS.md 递归加载（子目录文档驱动）
- 评估 codex ast_search 语法感知搜索（能力级差距候选）
- 用桌面端验证过程卡折叠在长会话中的体验
- 继续对照 codex 3aae5d8 挖能力级差距（write_stdin 之后）
