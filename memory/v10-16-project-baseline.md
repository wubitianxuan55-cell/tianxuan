---
name: v10-16-project-baseline
title: V10.16.0 项目基准
description: V10.16.0 项目基准 — 旧版本（已归档，当前 V10.30.0）
metadata:
  type: project
---

## 当前版本
- **版本号**: V10.16.0
- **发布日期**: 2026-06-30
- **构建产物**: `release/v10.16.0/tianxuan-desktop.exe`
- **SHA256**: `938d0da7d395175c0de85979eef7a74d15aca5b621d9f60b482076374c3df3c2`
- **构建命令**: `cd tianxuan/desktop && wails build`

## 核心变更 — Bug修复 + 设计加固 + 性能优化 + 测试恢复

10 次提交，30+ 文件变更：

### 🐛 Bug 修复（5 项）
1. **SubagentStop 重复调用** — `execute_one.go` 删除复制粘贴导致的钩子双重触发
2. **bash plain 模式输出截断** — 加入 48KB truncateStream，防止上下文爆炸
3. **git amend 不生效** — main/master 分支拦截改为 `!p.Amend` 时触发
4. **readfile 大文件扫描** — 用单行 `scanner.Scan()` 替代全量 drain（O(n)→O(1)）
5. **store.ts 跨轮次覆盖** — 旧轮 streaming=false 后新轮创建新项而非追加

### ⚠️ 设计缺陷修复（9 项）
6. **Grace round preWG 泄漏** — 错误退出前调用 `preWG.Wait()` 排空 goroutine
7. **Grace round nudge 残留** — 成功/失败双路径清理临时标记
8. **delete_range CR 损坏** — `ReplaceAll("\r","")` → `ReplaceAll("\r\n","\n")`
9. **truncateStream UTF-8 边界** — 字节截断改为 rune 边界对齐
10. **readskill 错误返回** — 3 处 `("error:...",nil)` → proper error
11. **editfile/multiedit 权限保留** — `os.Stat` 取原权限替代固定 0644
12. **readfile 参数上限** — Limit 上限 10000 防 OOM
13. **memory_search nil 索引** — 返回 proper error
14. **repeatSuccessBreakThreshold 命名** → `repeatSuccessAllowed`

### ⚡ 性能优化（3 项）
15. **grep O(n²) 去重** → O(1)：`emittedLines` map 替代每次全量扫描
16. **readfile ctx 取消** — 长扫描前检查 `ctx.Done()`
17. **multiedit 权限保留** — 与 editfile 一致

### 🔧 架构优化
18. **partitionToolCalls 统一冲突键算法** — 消除读写人为隔离，典型轮次延迟降 30-50%，代码 45→25 行，删除 `parallelisable()`

### 🧪 测试恢复（10 项）
19. **control 6 个 plan-mode 测试** — `PlanModeMarker`→`OrchestrateModeMarker`，notice 文本同步
20. **serve 1 个测试** — `"Plan mode"`→`"Read-only mode"`
21. **acp 2 个测试** — dispatch 层解包 ToolEnvelope JSON
22. **autoPlan="on" 回归修复** — `maybeAutoPlan` 区分 on/ask 模式

### 🎨 前端优化（7 项）
23. **流式 Markdown 纯文本** — 流式期间用 `<pre>` 替代 ReactMarkdown，消除闪烁
24. **Transcript useMemo** — `subcallsByParent` + `userTurn` 缓存
25. **ToolCard JSON.parse 缓存** — `diffsFor`/`subjectOf` 包装 useMemo
26. **store.ts 死状态清理** — 移除 `subAgentActive`
27. **流式推理可见** — 默认展示思考卡片，结束后折叠
28. **tool_result 代码整理** — 双路径格式化为多行
29. **scheduleMeasure 耦合注释** — 标注 250ms 与 GSAP 动画关系

### 🔒 安全加固
30. **writefile/delete_range/delete_symbol/notebookedit/editlines** — 5 个写工具保留原文件权限

### 🧹 代码清理
31. **agent_run.go** — 移除重复 canonical todo 注释
32. **ToolCard** — 移除 dead variable `effectiveOpen`

## 不变
TCCA 四层架构、事件驱动管线、缓存前缀不变性约束、自我进化原则均不变。

## 测试
- **Go**: 36 包全绿，零失败
- **TypeScript**: 零错误
- **前端**: npx tsc --noEmit 通过

**How to apply:** `cd tianxuan/desktop && wails build`
