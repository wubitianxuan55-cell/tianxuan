---
name: v10-88-1-project-baseline
title: V10.88.1 项目基线
description: V10.88.1 项目基线 — 当前版本、构建命令、核心变更
---

## V10.88.1 项目基线

| 项目 | 值 |
|------|-----|
| 版本号 | V10.88.1 |
| Git tag | v10.88.1 |
| 分支 | release/v10.88.0 |
| 发布日期 | 2026-07-15 |
| SHA256 | `439f01ec91c7dc950632cada013c878a7d2244f44cb0a64fabbb69cb26949a31` |

### 构建命令

```
cd tianxuan/desktop && wails build
```

| 参数 | 值 |
|------|-----|
| 构建位置 | `tianxuan/desktop/build/bin/` (固定，禁止移动) |
| 产物名称 | `tianxuan-desktop.exe` |
| 构建模式 | production |
| 平台 | windows/amd64 |
| Wails CLI | v2.13.0 |

### 核心架构

同 V10.88.0，未变更：

```
用户输入 → Hermes (规划者)
              │
     ┌───────┼────────┐
     ▼       ▼        ▼
  planStream  planWithTools  shouldSkipPlanner("!")
     │              │              │
     │         planFix ←── fail ──► runFastPath
     │         (R2最小/R3反思)       │
     │              │              │
     └──────────────┼──────────────┘
                    ▼
            executePlan (Hephaestus 执行)
                    │
                    ▼
            executePlanWithRetry (最多3轮)
                    │
                    ▼
            TurnResultEvent (单次emit)
```

### V10.88.1 变更（相对 V10.88.0）

| # | 变更 | 文件 | 说明 |
|---|------|------|------|
| 1 | isErrorResult 移出 switch | agent_run.go:293 | 从 case edit_file 内移到 switch 后，覆盖全部工具调用 |
| 2 | gofmt 格式化 | agent_run.go | 158 行缩进规整 |
| 3 | 重复注释删除 | agent_run.go:338 | advance canonical todo state 去重 |

### 关键代码位置

| 功能 | 文件 | 行 |
|------|------|-----|
| isErrorResult 检查 | agent_run.go | 293-299 |
| Hermes 结构体 | hermes.go | 27-55 |
| planWithConfirmation | hermes.go | ~450 |
| executePlanWithRetry | hermes.go | ~237 |
| taskGate | stop_gate.go | ~30 |
| goalGate | stop_gate.go | ~62 |
| verifyGate | stop_gate.go | ~80 |
| shouldAutoConfirm | hermes_confirm.go | ~73 |

### 关键约束

1. **缓存前缀不变性**（最高优先级）：任何修改必须保持双模型各自的 session prefix 不变
2. **wails build 固定位置**：桌面端必须用 `cd tianxuan/desktop && wails build`
3. **禁止 hard reset 已发布版本**：`git tag` 后不可 reset，永远用 revert
4. **三重门仅 solo 模式**：plannerMode 下所有 stop gates 跳过
5. **isErrorResult 必须在 switch 外**：对所有工具调用（含 write_file）统一收集错误
