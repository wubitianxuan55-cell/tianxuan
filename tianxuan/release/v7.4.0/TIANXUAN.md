# tianxuan V7.4.0 — 智能增强版

> 发布日期: 2026-06-16 | 基于: V7.2.0 | Go 1.25 | DeepSeek V4 | 参考: CodeWhale

## 新增功能

### 原生 Git 工具
4 个原生 Git 工具替代 `bash git ...`：`git_status`（结构化状态）、`git_diff`（行级 diff）、`git_commit`（自动消息提交）、`git_log`（格式化历史）。

### LSP 扩展
`lsp_completion`（代码补全建议）+ `lsp_rename`（跨文件重命名）。

### 新功能 — Diff 注入模型上下文
`edit_file`/`write_file`/`multi_edit` 等 writer 工具执行后，自动收集文件变更的 unified diff，
作为系统消息注入下一轮对话。模型无需手动 `read_file` 确认变更结果——在下一轮回答中即可看到
自己修改了哪些文件、哪几行、具体做了什么改动。

- 新增: `AgentRunner.pendingDiffs` 收集器 + `runPostToolDiffPreview()` 方法
- 注入时机: `executeBatch` → `runPostToolDiagnostics` → `runPostToolDiffPreview`
- 位置: `internal/agent/agent.go`

### LSP 诊断自动注入
每次 writer 工具执行后自动触发 LSP 诊断...
每次 writer 工具执行后自动触发 LSP 诊断，结果作为系统消息注入下一轮。让模型无需主动请求就能看到编译错误。

### Constitution 宪法系统
`.tianxuan/constitution.toml` 声明权威链 + 保护性不变式 + 验证策略。注入到系统提示的最高优先级区块。

### 子 Agent 输出契约
子 agent 输出强制五段式：SUMMARY / CHANGES / EVIDENCE / RISKS / BLOCKERS。

### `/undo` 回滚命令
基于 Checkpoint 系统的 `/undo [N]` 用户级回滚。

## 重构

### controller.go 拆分（2041→1469 行）
拆为 `controller_submit.go` + `controller_memory.go` + `controller_approval.go`。

### chat_tui.go 样式提取（2367→2011 行）
提取 `chat_tui_styles.go`（样式变量）+ `chat_tui_view.go`（View + 渲染函数）。

## 文档清理
- 删除误导性 `design.md`（Rust tokio 架构残留）
- V5/V6 过时文档归档到 `_archive/`
- TIANXUAN.md 完整重写

## Bug 修复
- `branch.go:72`: 修复 `renderTUIBanner` 传参错误

## 文件变更清单

### 新增文件（18 个）
```
.tianxuan/constitution.toml           — 宪法配置文件
_archive/V5.0-ARCHITECTURE.md         — 过时文档备份
_archive/V5.0-RELEASE.md              — 过时文档备份
_archive/V6.0-ARCHITECTURE.md         — 过时文档备份
_archive/V6.1-RELEASE.md              — 过时文档备份
internal/boot/constitution.go         — 宪法加载器
internal/cli/chat_tui_styles.go       — 样式变量提取
internal/cli/chat_tui_view.go         — View + 渲染函数提取
internal/control/controller_submit.go — Submit + slash 分发
internal/control/controller_memory.go — Memory 操作
internal/control/controller_approval.go — 审批桥梁
internal/lsp/*                         — LSP completion/rename 扩展
internal/tool/builtin/git.go          — 4 个 Git 工具
internal/tool/builtin/time.go         — time 工具
internal/notify/                       — 通知模块
internal/crash/                        — panic 兜底
internal/update/                       — 更新模块
desktop/frontend/src/components/StreamingIndicator.tsx
desktop/frontend/src/locales/zh-TW.ts
```

### 修改文件（40+ 个）
`internal/agent/agent.go` — LSP 诊断注入 + lspManager 接口
`internal/boot/boot.go` — 宪法注入 + 子 Agent 契约
`internal/control/controller.go` — /undo 命令
`internal/lsp/client.go/manager.go/results.go/tool.go` — completion/rename
`internal/tool/builtin/compact.go` — Git 工具 compact 描述
等

### 删除文件
```
design.md → design.md.obsolete
V5.0-ARCHITECTURE.md → _archive/
internal/agent/prefix_volatility.go (V7.3)
internal/agent/pressure_flush.go (V7.3)
internal/agent/rebuild.go (V7.3)
```

## 回退指南

### 方式 1: git reset 回 V7.2.0
```bash
git checkout 0f680167
```

### 方式 2: 只回退新增文件（保留现有功能）
```bash
# 删除新增的文件
rm -f internal/tool/builtin/git.go
rm -f internal/boot/constitution.go .tianxuan/constitution.toml
rm -f internal/control/controller_submit.go
rm -f internal/control/controller_memory.go
rm -f internal/control/controller_approval.go
rm -f internal/cli/chat_tui_styles.go internal/cli/chat_tui_view.go
# 恢复原始文件
git checkout -- internal/agent/agent.go internal/boot/boot.go
git checkout -- internal/cli/chat_tui.go internal/cli/branch.go
git checkout -- internal/control/controller.go
git checkout -- internal/lsp/ $FILES
git checkout -- internal/tool/builtin/compact.go
```

### 方式 3: 保留功能但禁用 Constitution
```bash
mv .tianxuan/constitution.toml .tianxuan/constitution.toml.off
```
