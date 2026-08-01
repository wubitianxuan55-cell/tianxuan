---
name: v10-127-project-baseline
title: V10.127.0 项目基线
description: V10.127.0 项目基线 — 当前版本、构建命令、发布文件
---

## V10.127.0 项目基线

| 项目 | 值 |
|------|-----|
| 版本号 | V10.127.0 |
| Git tag | v10.127.0 |
| 分支 | release/v10.110.0 |
| 发布日期 | 2026-08-01 |
| SHA256 | `757D636D57FAE1AA7D88CDFEE3447CDBF129F5672AC9E88834280B44B8927A32` |

### 构建命令

```
cd D:\AI\tianxuanX && build-desktop.bat
```

`build-desktop.bat`：同步 `.tianxuan/skills` → `internal/skill/bundled`（robocopy /MIR）→ `cd desktop && wails build -ldflags "-s -w -H windowsgui" -o tianxuan-desktop.exe`

| 参数 | 值 |
|------|-----|
| 构建位置 | `desktop/build/bin/tianxuan-desktop.exe`（固定，禁止移动） |
| 发布位置 | `release/v10.127.0/tianxuan-desktop.exe` |
| 校验文件 | `release/v10.127.0/SHA256SUMS` |

### 发布文件清单

| 文件 | 用途 |
|------|------|
| `release/v10.127.0/tianxuan-desktop.exe` | 桌面端安装/运行产物（20.1 MB） |
| `release/v10.127.0/SHA256SUMS` | 产物校验和 |
| `CHANGELOG.md` | 版本变更记录（V10.112~127 全量条目） |
| `memory/v10-127-release-notes.md` | 本版本发布记录 |
| `memory/v10-127-project-baseline.md` | 本版本项目基线（本文件） |

### 发布环境（2026-08-01）

| 组件 | 版本/路径 |
|------|-----------|
| Go | 1.26.5（`tools/go/bin`） |
| Wails CLI | v2.13.0（`C:\Users\Administrator\go\bin`） |
| Node | v26.5.1（`tools/node`） |
| pnpm | v11.9.0（`tools/node`） |
