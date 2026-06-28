# V10.8.0 发布记录

**发布日期**: 2026-06-28
**构建产物**: `release/v10.8.0/tianxuan-desktop.exe` (16 MB)
**SHA256**: `b9671ae8f408592bb712ac1dd6a37a4493afef31e1a64bb7fd46d8a6195b1084`
**前端版本**: `package.json` version = "10.8.0"
**Git 提交**: `e7401e5`

---

## 🔵 第四阶段：智能化（3 项）

### 1. compact 保留 todo 状态
- **问题**: compaction 后丢失 todo_write 状态，agent 重复执行已完成步骤
- **方案**: `maybeCompact` 压缩前通过 `readProgressFile()` 读取 `.tianxuan/progress.md` 注入到压缩指令
- **实现**: `readProgressFile()` 从 cwd 向上查找项目根，进度内容作为 summarizer 指令的 `## Pending & next step` 节
- **效果**: 压缩后的摘要包含任务进度，agent 恢复无需重复工作

### 2. 增强 commit message
- **问题**: 自动生成的 commit message 太简单（如 `chore: 5 file(s)`）
- **方案**: `autoCommitMessage` 解析 diff stat 文件路径 → `summarizeFiles()` 生成文件名摘要
- **规则**: ≤3 个文件列出名字（如 `grep.go + git.go + compact.go`），>3 显示计数
- **效果**: 消息从 `feat(core): 5 file(s)` 变为 `feat(core): grep.go + compact.go — 3 file(s), 24 insertions`

### 3. grep 结果相关性排序
- **参数**: `sort_by` (path/relevance)，默认 path 保持向后兼容
- **算法**: `sortByRelevance()` 按文件匹配密度降序 → 文件路径 → 行号
- **效果**: 最相关文件排最前，agent 更快找到关键代码

---

## 版本路线图总览

V10.4.0 → V10.8.0 完成了完整优化路线图 4 阶段 12 项：

| 阶段 | 版本 | 日期 | 核心成果 |
|------|------|------|----------|
| 基础 | V10.4.0 | 06-26 | Superpowers 融合 + 工具精简 28→24 + 技能 10→4 |
| 🔴 编辑体验 | V10.5.0 | 06-28 | 行尾适配 + edit_lines + read_file 无行号 |
| 🟡 可靠性 | V10.6.0 | 06-28 | web_fetch 重试 + bash stdout/stderr 分离 + 子代理部分结果 |
| 🟢 工作流 | V10.7.0 | 06-28 | git_worktree + 进度持久化 + main 分支保护 |
| 🔵 智能化 | V10.8.0 | 06-28 | compact 保留进度 + 增强 commit msg + grep 排序 |

---

## 技术统计

- **新增工具**: 3 个 (edit_lines V10.5.0, git_worktree V10.7.0, 增强 grep V10.8.0)
- **修改工具**: 8 个 (edit_file, multi_edit, read_file, web_fetch, bash, task, git_commit, todo_write)
- **新增 Go 文件**: 3 个 (editlines.go, gitworktree.go, app_*.go × 5)
- **新增前端文件**: 6 个 (DocEditor, FactCard, MessageNavigator, useGlobalShortcuts, usePaletteItems, session)
- **总提交**: 6 次（含 chore 清理）

---

## 构建

```sh
cd tianxuan/desktop
pnpm --dir frontend build
wails build
# 产物: build/bin/tianxuan-desktop.exe
```
