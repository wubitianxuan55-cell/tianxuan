---
name: v10-88-project-baseline
title: V10.88.0 项目基线
description: V10.88.0 项目基线 — 旧版本（已归档，当前 V10.88.1）、构建命令、核心变更
---

## V10.88.0 项目基线

| 项目 | 值 |
|------|-----|
| 版本号 | V10.88.0 |
| Git tag | v10.88.0 |
| 分支 | release/v10.88.0 |
| 发布日期 | 2026-07-15 |
| SHA256 | `0c9033dffdf6f9b9a1bf4ae4bcaa3482a3c3a80388b35285be225a43976ebcb8` |

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

### 三重门（solo 模式）

```
taskGate (todo未完成) → goalGate (目标未达成) → verifyGate (运行测试)
   ↓                       ↓                       ↓
plannerMode跳过         plannerMode跳过         disableVerify跳过
重入上限3              重入上限3                每轮1次
```

### 关键代码位置

| 功能 | 文件 | 行 |
|------|------|-----|
| Hermes 结构体 | hermes.go | 27-55 |
| planWithConfirmation | hermes.go | ~450 |
| executePlanWithRetry | hermes.go | ~237 |
| planFix | hermes.go | ~328 |
| executePlan | hermes.go | ~571 |
| formatSummary | hermes.go | ~712 |
| shouldSkipPlanner | hermes.go | ~880 |
| taskGate | stop_gate.go | ~30 |
| goalGate | stop_gate.go | ~62 |
| verifyGate | stop_gate.go | ~80 |
| shouldAutoConfirm | hermes_confirm.go | ~73 |
| wrapExecutorSink | hermes.go | ~513 |

### 关键约束

1. **缓存前缀不变性**（最高优先级）：任何修改必须保持双模型各自的 session prefix 不变
2. **wails build 固定位置**：桌面端必须用 `cd tianxuan/desktop && wails build`，产物固定在 `desktop/build/bin/`
3. **禁止 hard reset 已发布版本**：`git tag` 后不可 reset，永远用 revert
4. **三重门仅 solo 模式**：plannerMode 下所有 stop gates 跳过
