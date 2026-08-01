---
name: v10-143-release-notes
title: V10.143.0 发布记录
description: V10.143.0 发布记录 — 记忆提取质量修复（控制块泄漏 + 重复候选堆积）
---

## V10.143.0 发布记录

| 项目 | 值 |
|------|-----|
| 版本号 | V10.143.0 |
| 发布日期 | 2026-08-01 |
| 代码变更 | 4 文件（internal/memory/extract.go + test、desktop/memory_suggestions.go + test）|
| 主题 | 记忆提取质量修复 |
| 构建产物 | `tianxuan-desktop.exe` |
| 文件大小 | 21,249,024 bytes (~20.3 MB) |
| SHA256 | `c50b6f14016a07cc0ef439379a8eb72e596f9f3e9c72c3ff4d62f96c61a8c9c1` |

### 本发布包含的提交

| 提交 | 主题 |
|------|------|
| 56c3add | fix(tool): git 工具在 PATH 无 git 时探测常见安装路径（本发布纳入） |
| (本轮) | 记忆提取质量修复 — 控制块泄漏 + 重复候选堆积 |

### 根因（5 级追溯）

- 表象：pending 待确认区堆积 10 个候选，含 `<memory-update>` 控制块泄漏、go-build
  重复 3 次、半截噪声。
- 直接原因：`isTransientBlock` 只做 `HasPrefix` 前缀匹配——宿主在用户消息尾部追加
  `<memory-update>` 块时（"默认是不是中文 <memory-update>..."），前缀不是控制块
  标签，逃过过滤；"默认" 又命中 memoryMarkers，整个块被提取成候选。
- 本地根因：去重基准 `existingCovers` 只查 active memory 且只做包含式匹配——同一
  规则换措辞（"执行" vs "需" vs "="）不互相包含，跨 session 重复堆积。
- 系统根因：自动提取的 transient 过滤与去重都是"前缀/包含"精确匹配，未覆盖
  "嵌入块"与"换措辞重复"两类真实输入。
- 过程根因：V10.142 引入自动提取时测试只覆盖"块在开头 + 完全相同重复"。

### 修复

- `memory/extract.go`：尖括号控制块任意位置匹配；去重基准合入 pending 候选；
  `sharesCorePhrase` ≥10 runes 公共子串检测
- `desktop/memory_suggestions.go`：同源修复（suggestMemories 补 transient 过滤 +
  公共子串去重）

### 测试

- `memory/extract_test.go`：`TestExtractSkipsEmbeddedTransientBlocks`（RED→GREEN）、
  `TestExtractDedupAgainstPending`、`TestExtractCursorAdvance` 语义更新
- `desktop/memory_suggestions_test.go`：`TestIsTransientBlockEmbedded`、
  `TestSharesCorePhrase`

### 验证

- `go build ./...` EXIT 0；`go vet ./internal/memory/` 无告警
- `go test ./internal/memory/ ./internal/control/` 全 ok
- desktop `go test ./...` 全 ok（tianxuan/desktop module）
