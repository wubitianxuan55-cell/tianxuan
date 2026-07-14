---
name: v10-66-0-project-baseline
title: V10.66.0 项目基准
description: V10.66.0 项目基准 — 当前版本、构建命令、核心变更
metadata:
  type: project
---

# V10.66.0 项目基准

## 版本

- **版本号**: V10.66.0
- **分支**: master
- **发布时间**: 2026-07-14

## 构建

```bash
cd tianxuan/desktop && wails build
```

产物: `C:\Users\吴比\AppData\Roaming\tianxuan\bin\tianxuan.exe`

## 核心变更

### BUG 修复

1. **anthropic readStream** — 添加 ctx 参数 + 缓冲通道 + idleDone body-close-on-cancel goroutine，防止 goroutine 泄漏
2. **controller_submit 孤儿 goroutine** — 新增 bgCtx/bgCancel，Close() 时取消 4 个后台 goroutine
3. **kill_windows 句柄错误日志** — 所有 CloseHandle/ResumeThread 失败改为 slog.Warn
4. **ACP server 写入错误日志** — c.write 失败改为 slog.Warn
5. **task.go SaveFailed 错误日志** — SaveFailed 错误改为 slog.Warn

## 技术栈

- Go 后端 + Wails v2.12 桌面框架
- React 18 + TypeScript 前端
- DeepSeek API (双模型：planner + executor)
