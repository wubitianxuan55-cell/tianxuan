---
name: v10-141-project-baseline
title: V10.141.0 项目基线
description: V10.141.0 项目基线 — 当前版本、构建命令、发布文件
---

## V10.141.0 项目基线

| 项目 | 值 |
|------|-----|
| 版本号 | V10.141.0 |
| Git tag | v10.141.0 |
| 分支 | release/v10.110.0 |
| 发布日期 | 2026-08-01 |
| SHA256 | `98976515197B5BA509D45AFC3D5568559C1CA026DCA318BE1E30B7EBD8BCEBBA` |

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
| 发布位置 | `release/v10.141.0/tianxuan-desktop.exe` |
| 校验文件 | `release/v10.141.0/SHA256SUMS` |

### 发布文件清单

| 文件 | 用途 |
|------|------|
| `release/v10.141.0/tianxuan-desktop.exe` | 桌面端安装/运行产物（20.2 MB） |
| `release/v10.141.0/SHA256SUMS` | 产物校验和 |
| `CHANGELOG.md` | 版本变更记录（V10.138~141 全量条目） |
| `memory/v10-141-release-notes.md` | 本版本发布记录 |
| `memory/v10-141-project-baseline.md` | 本版本项目基线（本文件） |

### 架构速览（V10.141）

- **双模型**：Hermes 规划者 + Hephaestus 执行者，3 轮自动修复，verify 四元组
- **单模型**：Adaptive Execution（todo 活文档/失败分级/steer 纠偏/进度保护）
- **横切**：子代理并行优先（调查外包）、技能自动触发 + 主动调用、验证分级
  （Go/前端/文档/跨模块四档）
