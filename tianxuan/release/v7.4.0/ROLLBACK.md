# V7.4.0 回退指南

## 快速回退到 V7.2.0

### 方式 1: Git 硬回退（丢失所有 V7.4 变更）
```bash
cd tianxuan
git checkout 0f680167
go build -ldflags "-s -w" -o bin/tianxuan.exe ./cmd/tianxuan
```
⚠️ 这会丢失 Git 工具、LSP 扩展、诊断注入、Constitution、/undo、controller.go 拆分等全部 V7.4 功能。

### 方式 2: 仅删除新增文件（保留 V7.2 已有功能不变）
```batch
cd tianxuan

REM 删除新增文件
del internal\tool\builtin\git.go
del internal\boot\constitution.go
del .tianxuan\constitution.toml
del internal\control\controller_submit.go
del internal\control\controller_memory.go
del internal\control\controller_approval.go
del internal\cli\chat_tui_styles.go
del internal\cli\chat_tui_view.go
del internal\lsp\client.go
del internal\lsp\manager.go
del internal\lsp\results.go
del internal\lsp\tool.go
del _split_chat_tui.py _edit_agent_*.py _fix_*.py _add_*.py _analyze_tui.py

REM 恢复被修改的原始文件
git checkout -- internal\agent\agent.go
git checkout -- internal\boot\boot.go
git checkout -- internal\cli\chat_tui.go
git checkout -- internal\cli\branch.go
git checkout -- internal\control\controller.go
git checkout -- internal\tool\builtin\compact.go
git checkout -- internal\jobs\jobs.go

REM 恢复文档（可选）
git checkout -- TIANXUAN.md CHANGELOG.md
```

### 方式 3: 保留功能但禁用某个模块
```bash
# 禁用 Constitution（不影响其他功能）
mv .tianxuan/constitution.toml .tianxuan/constitution.toml.off

# 禁用 Git 工具（不影响其他功能）
# 在 tianxuan.toml 的 [tools] 段添加: disabled = ["git_status","git_diff","git_commit","git_log"]

# 禁用 LSP 诊断自动注入
# 在 tianxuan.toml 设置: [lsp] enabled = false
```

## 发布时间线

| 版本 | 日期 | 基版本 | 说明 |
|------|------|--------|------|
| V7.2.0 | 2026-06-13 | - | DSR + 三闸门 + 归档 + 6 Bug 修复 |
| V7.3.0 | 2026-06-14 | V7.2.0 | 统计面板修复 + DSR 收敛 + 清理 |
| V7.4.0 | 2026-06-16 | V7.2.0 | 智能增强版：Git/LSP/Constitution/重构 |
