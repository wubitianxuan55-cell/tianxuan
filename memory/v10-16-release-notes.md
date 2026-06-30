---
name: v10-16-release-notes
title: V10.16.0 发布记录
description: V10.16.0 发布记录 — Bug修复+设计加固+性能优化+测试恢复
metadata:
  type: project
---

## V10.16.0 发布记录

### 📦 构建信息
- **版本号**: V10.16.0
- **发布日期**: 2026-06-30
- **构建产物**: `release/v10.16.0/tianxuan-desktop.exe`
- **SHA256**: `938d0da7d395175c0de85979eef7a74d15aca5b621d9f60b482076374c3df3c2`
- **构建命令**: `cd tianxuan/desktop && wails build`

### 🐛 Bug 修复（5 项）
| 位置 | 问题 | 影响 |
|------|------|------|
| `execute_one.go:241-248` | SubagentStop 复制粘贴导致重复调用 | 功能异常 |
| `bash.go:224-233` | plain 模式输出无截断 | 上下文爆炸 |
| `git.go:249-250` | amend=true 被分支拦截忽略 | 功能不可用 |
| `readfile.go:152-154` | 大文件全量 drain 确定剩余行数 | 性能退化 |
| `store.ts:58-78` | 跨轮次 text/reasoning 覆盖旧 assistant | 显示错乱 |

### ⚠️ 设计缺陷修复（9 项）
- Grace round preWG goroutine 泄漏 → 退出前 drain
- Grace round nudge 永久残留 → 双路径清理
- delete_range `ReplaceAll("\r","")` 损坏合法 CR → `ReplaceAll("\r\n","\n")`
- truncateStream 字节截断破坏 UTF-8 → rune 边界调整
- readskill 3 处静默错误 → proper error
- editfile/multiedit 固定 0644 → 保留原权限
- readfile Offset/Limit 无上限 → max 10000
- memory_search nil 索引 → proper error
- `repeatSuccessBreakThreshold` 命名 → `repeatSuccessAllowed`

### ⚡ 性能优化
- **grep O(n²)→O(1)**：`emittedLines` map 替代每匹配全量扫描
- **readfile ctx 取消**：长扫描前检查 `ctx.Done()`
- **partitionToolCalls 重构**：统一冲突键贪心分区，消除读写隔离

### 🧪 测试恢复
- **9→0 失败**：plan-mode 6 项 + serve 1 项 + acp 2 项全部修复
- **autoPlan="on" 回归**：`maybeAutoPlan` 区分 on/ask 模式

### 🎨 前端优化
- **流式 Markdown 纯文本**：消除闪烁根源
- **useMemo 缓存**：Transcript + ToolCard
- **流式推理可见**：默认展示思考卡片
- **死状态清理**：移除 `subAgentActive`

### 🔒 权限修复
writefile/delete_range/delete_symbol/notebookedit/editlines 全部保留原文件权限

### 📊 质量
- Go 36 包全绿 · TypeScript 零错误
- 10 commits · 30+ 文件变更
