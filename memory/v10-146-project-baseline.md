---
name: v10-146-project-baseline
title: V10.146.0 项目基线
description: V10.146.0 项目基线 — 当前版本、构建命令、发布文件
---

## V10.146.0 项目基线

| 项目 | 值 |
|------|-----|
| 版本号 | V10.146.0 |
| Git tag | v10.146.0 |
| 分支 | release/v10.110.0 |
| 发布日期 | 2026-08-01 |
| SHA256 | 待构建后填充 |

### 构建命令

```
cd D:\AI\tianxuanX && build-desktop.bat
```

（V10.138 起脚本自动补齐 PATH：探测 `tools\go\bin` / `tools\node` / Wails CLI；
无需手动设置环境；bat 注释必须纯 ASCII。`tools/` 已在 .gitignore 中。）

`build-desktop.bat`：同步 `.tianxuan/skills` → `internal/skill/bundled`
（robocopy /E）→ `cd desktop && wails build -ldflags "-s -w -H windowsgui"
-o tianxuan-desktop.exe`

| 参数 | 值 |
|------|-----|
| 构建位置 | `desktop/build/bin/tianxuan-desktop.exe`（固定，禁止移动）|
| 发布位置 | `release/v10.146.0/tianxuan-desktop.exe` |
| 校验文件 | `release/v10.146.0/SHA256SUMS` |

### 发布文件清单

| 文件 | 用途 |
|------|------|
| `release/v10.146.0/tianxuan-desktop.exe` | 桌面端安装运行产物 |
| `release/v10.146.0/SHA256SUMS` | 产物校验值 |
