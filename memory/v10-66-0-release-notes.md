---
name: v10-66-0-release-notes
title: V10.66.0 发布记录
description: V10.66.0 发布记录 — 5 项后端 BUG 清理
metadata:
  type: reference
---

## V10.66.0 发布记录

### BUG 修复

1. **anthropic readStream goroutine 泄漏修复** — readStream 添加 `ctx` 参数、缓冲通道(16)、idleDone body-close-on-cancel goroutine（参照 OpenAI 实现）。网络中断或取消对话时 goroutine 不再泄漏

2. **controller_submit 孤儿 goroutine 修复** — Controller 新增 `bgCtx/bgCancel`，4 个 fire-and-forget goroutine（/compact, /dream, /distill, /new）从 `context.Background()` 改为 `c.bgCtx`，`Close()` 时调用 `bgCancel()` 确保关闭过程中不再有孤儿 goroutine

3. **kill_windows 句柄错误日志** — 所有 `_ = windows.CloseHandle()` 和 `_, _ = windows.ResumeThread(th)` 从静默忽略改为 `slog.Warn` 日志，便于调试长期运行后的句柄泄漏

4. **ACP server 写入错误日志** — `c.write()` 写入响应失败从 `_ =` 改为 `slog.Warn`

5. **task.go SaveFailed 错误日志** — 子代理失败时 `SaveFailed` 错误从 `_ =` 改为 `slog.Warn`

### 文件变更

| 文件 | 插入 | 删除 | 变更 |
|------|------|------|------|
| internal/provider/anthropic/anthropic.go | 18 | 3 | readStream ctx 参数 + idleDone 模式 |
| internal/provider/anthropic/anthropic_test.go | 3 | 3 | 测试适配新签名 |
| internal/control/controller.go | 11 | 0 | bgCtx/bgCancel 字段 + 初始化 + Close 取消 |
| internal/control/controller_submit.go | 3 | 3 | context.Background() → c.bgCtx |
| internal/proc/kill_windows.go | 33 | 8 | 所有 CloseHandle/ResumeThread 错误日志 |
| internal/acp/server.go | 4 | 1 | c.write 错误日志 |
| internal/agent/task.go | 3 | 1 | SaveFailed 错误日志 |

### 验证

- `go test ./... -count=1` — 全部通过
- `go vet ./...` — 零警告
- `go test -race ./internal/control/... ./internal/provider/anthropic/... ./internal/acp/... ./internal/agent/...` — 无 data race
- `npx tsc --noEmit` — 零错误
