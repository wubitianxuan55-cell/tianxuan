---
name: v10-142-project-baseline
title: V10.142.0 项目基线
description: V10.142.0 项目基线 — 当前版本、构建命令、发布文件
---

## V10.142.0 项目基线

| 项目 | 值 |
|------|-----|
| 版本号 | V10.142.0 |
| Git tag | v10.142.0 |
| 分支 | release/v10.110.0 |
| 发布日期 | 2026-08-01 |
| SHA256 | `65534973E430E10C9CF60F4694E411563709516F89774932C286524E90EE6065` |

### 构建命令

```
cd D:\AI\tianxuanX && build-desktop.bat
```

（V10.138 起脚本自动补齐 PATH：探测 `tools\go\bin` / `tools\node` / Wails CLI，
无需手动设置环境；bat 注释必须纯 ASCII。）

`build-desktop.bat`：同步 `.tianxuan/skills` → `internal/skill/bundled`
（robocopy /E）→ `cd desktop && wails build -ldflags "-s -w -H windowsgui"
-o tianxuan-desktop.exe`

| 参数 | 值 |
|------|-----|
| 构建位置 | `desktop/build/bin/tianxuan-desktop.exe`（固定，禁止移动） |
| 发布位置 | `release/v10.142.0/tianxuan-desktop.exe` |
| 校验文件 | `release/v10.142.0/SHA256SUMS` |

### 发布文件清单

| 文件 | 用途 |
|------|------|
| `release/v10.142.0/tianxuan-desktop.exe` | 桌面端安装/运行产物（20.3 MB） |
| `release/v10.142.0/SHA256SUMS` | 产物校验值 |
