---
name: v10-137-project-baseline
title: V10.137.0 项目基线
description: V10.137.0 项目基线 — 当前版本、构建命令、发布文件
---

## V10.137.0 项目基线

| 项目 | 值 |
|------|-----|
| 版本号 | V10.137.0 |
| Git tag | v10.137.0 |
| 分支 | release/v10.110.0 |
| 发布日期 | 2026-08-01 |
| SHA256 | `426407380ABB65660F01CD7AB618DF76D18913928E9531698C4F8E70899E3390` |

### 构建命令

```
set PATH=D:\AI\tianxuanX\tools\go\bin;C:\Users\Administrator\go\bin;D:\AI\tianxuanX\tools\node;%PATH%
cd D:\AI\tianxuanX && build-desktop.bat
```

`build-desktop.bat`：同步 `.tianxuan/skills` → `internal/skill/bundled`
（robocopy /E，V10.127 修复 /MIR 误删）→ `cd desktop && wails build
-ldflags "-s -w -H windowsgui" -o tianxuan-desktop.exe`

| 参数 | 值 |
|------|-----|
| 构建位置 | `desktop/build/bin/tianxuan-desktop.exe`（固定，禁止移动） |
| 发布位置 | `release/v10.137.0/tianxuan-desktop.exe` |
| 校验文件 | `release/v10.137.0/SHA256SUMS` |

### 发布文件清单

| 文件 | 用途 |
|------|------|
| `release/v10.137.0/tianxuan-desktop.exe` | 桌面端安装/运行产物（20.2 MB） |
| `release/v10.137.0/SHA256SUMS` | 产物校验和 |
| `CHANGELOG.md` | 版本变更记录（V10.128~137 全量条目） |
| `memory/v10-137-release-notes.md` | 本版本发布记录 |
| `memory/v10-137-project-baseline.md` | 本版本项目基线（本文件） |

### 架构速览（V10.137）

- **双模型**：Hermes 规划者（只读调查 + 项目地图 + 计划确认）+ Hephaestus
  执行者（todo/complete_step 宿主推进 + verify 门），3 轮自动修复，verify
  四元组（completeness/correctness/coherence/coverage）
- **单模型**：Adaptive Execution——todo 活文档、无计划确认、失败分级自适应、
  同步骤失败宿主 nudge、steer 中途纠偏、进度保护
