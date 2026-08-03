---
name: v10-153-release-notes
title: V10.153.0 版本发布
description: V10.153.0 版本发布 — P0-P5 工程改进（编辑安全/Windows 适配/发布流水线/Git 工作流/环境预警）
---

## V10.153.0 版本发布

| 项 | 值 |
|------|-----|
| 版本 | V10.153.0 |
| 日期 | 2026-08-03 |
| 范围 | internal/tool/builtin + internal/boot + internal/skill + release 产物 |
| 主题 | P0-P5 工程改进：编辑安全、Windows 适配、发布流水线、Git 工作流、环境预警 |
| 产物 | `release/v10.153.0/tianxuan-desktop.exe` |
| 大小 | 25,020,928 bytes (~23.9 MB) |
| SHA256 | `957ecce4a2357e63bd68a0e596a2fcf81e38e71476769c83265ba8b947c0924e` |

### 主要变更

| 模块 | 变更 |
|------|------|
| edit_lines (P0) | start_anchor/end_anchor 锚点校验（不匹配拒绝）+ 编辑后自动语法检查回滚（.go→gofmt -e，.ts/.tsx→tsc --noEmit，60s 限时，validate 可关） |
| bash 适配 (P1) | heredoc 转 here-string（cat/文件写/python/node/tsx stdin）；npm/npx/pnpm .cmd shim 绕执行策略；git 路径自动注入 |
| release 技能 (P3) | 版本号 4 处同步、package.ps1 打包+SHA256、CHANGELOG/记忆模板、autotrigger 触发词 |
| boot 提示 (P4) | L1 静态 Git 工作流约定：开始前建功能分支、完成按单元提交（缓存安全：不注入实时状态） |
| 环境预警 (P5) | 数据库类命令执行前比对用户级 DATABASE_URL 与项目 .env，不一致注入 env-warning |

### 验证

- `go test ./...` 全绿；`go vet` 干净；`wails build` EXIT 0（35s）
- SHA256 已核对：`957ecce4…` 与 exe 实际一致

### 下一步 5 件事

- P2 前端数据契约校验（需前端项目路径）：探索/开发时自动比对前端字段引用 vs 后端返回字段
- 用 release 技能沉淀发布流程：下次发布直接触发"打包发布"关键词
- P4 落地观察：多任务开始是否主动建功能分支，避免变更堆积
- P5 扩展：将预警从 DATABASE_URL 扩展到其他高危变量（API 密钥、环境级覆盖）
- 桌面端实际验证 v10.153.0：WebView2 构建产物在真实环境跑通编辑/发布流程
