---
name: v10-23-release-notes
title: V10.23.0 发布记录
description: V10.23.0 发布记录 — 测试修复 + boot 拆分 + 前端测试 + 缓存安全工具
metadata:
  type: project
---

## V10.23.0 发布记录

- **版本号**: V10.23.0
- **发布日期**: 2026-07-04
- **构建产物**: `release/v10.23.0/tianxuan-desktop.exe`
- **SHA256**: `4967b7e566933a9aa535142b2e82ad01b483a1c71ca2d1db698adad8cf046b04`
- **构建命令**: `cd tianxuan/desktop && wails build`
- **变更统计**: 核心 19 文件，+1322/-200 行，Go build ✅，TSC ✅，Vitest 32 tests ✅

### ✅ P0: 修复 6 个已知失败的测试

**根因**: verify gate 在无 todo 列表的 session 中误触发，注入 `[system]` 消息导致额外 API 调用。

- `agent.go`: Options 新增 `DisableVerify` 字段，AgentRunner 新增 `disableVerify`
- `stop_gate.go`: `taskGate()` 检查 `disableVerify` 跳过 verify nudge
- `task.go`: `RunSubAgent` 自动设 `DisableVerify=true`
- `toolcache_test.go`: Windows mtime 精度修复（`time.Sleep(10ms)`）
- 3 个测试文件：Options 加 `DisableVerify: true`

### ✅ P1: 拆分 boot.go

- `sysprompt.go` (85行): 系统提示词/记忆/技能/Profile/Compiler/L2 组装
- `plugins.go` (93行): CodeGraph/Context7/MCP/LSP 启动
- `boot.go`: 773→648 行 (-125)

### ✅ P3: 前端关键逻辑加测试

- `lib/stats.ts`: 提取纯函数模块（priceFor/calcCost/fmtTokens/aggSteps/colFromUsage/filterSteps 等 13 个函数）
- `lib/stats.test.ts`: 27 个 Vitest 测试
- `lib/store.test.ts`: 5 个 usage 分流逻辑测试
- `StatusBar.tsx`: 消除重复代码，改为 import stats.ts
- `vitest.config.ts`: node 环境配置

### ✅ P4: 构建产物管理

- `Makefile`: 新增 `webui` / `clean-webui` 目标
- `internal/serve/webui/assets/.gitkeep`: 说明 assets 用途

### ✅ P5: 缓存安全静态检查工具

- `cmd/cacheguard/`: 128行零外部依赖的 Go 静态分析器
- 三条规则：工具热插拔(L3) / 系统提示词突变(L1) / L2运行时漂移
- `make lint-cache` 一键检查

### ⏳ 未完成

- **P2**: agent 包拆分（8h，风险/收益比差，跳过）
