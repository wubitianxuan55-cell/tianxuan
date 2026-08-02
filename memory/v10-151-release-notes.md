---
name: v10-151-release-notes
title: V10.151.0 发布记录
description: V10.151.0 发布记录 — 浏览器右侧分栏可拖拽调整宽度（修复宽度写死 CSS、无 resizer）
---

## V10.151.0 发布记录

| 项目 | 值 |
|------|-----|
| 版本号 | V10.151.0 |
| 发布日期 | 2026-08-02 |
| 代码变更 | 7 文件（useBrowserPanel 新 hook + useLayoutSizes/App/i18n×3 + 测试） |
| 主题 | 修复浏览器右侧分栏无法拖动变宽 |
| 构建产物 | `release/v10.151.0/tianxuan-desktop.exe` |
| 文件大小 | 24,999,936 bytes (~23.8 MB) |
| SHA256 | `1ac93074fb67e48d05326b59f0775573bc0b7c3039c77ed513167ad1e144ad53` |

### 本次发布包含的改动
| 模块 | 主题 |
|------|------|
| useBrowserPanel（新） | 浏览器分栏宽度 state + 拖拽调整（pointer/键盘/双击复位），与 workspace 面板同模式，宽度持久化 |
| useLayoutSizes | `clampBrowserPanelWidth`（360 下限/1080 上限/62% 比例/对话区 200px 保护）+ load/save |
| App.tsx | 浏览器打开时 `--workspace-width` 轨道跟随浏览器宽度；渲染浏览器 resizer（复用 workspace-panel-resizer 样式） |
| i18n | en/zh/zh-TW 新增 `browser.resizePanel` |

### 关键决策（5 级追溯）

- 表象：浏览器打开在右侧面板，无法拖动变宽
- 直接原因：浏览器分栏 div 宽度写死 CSS `clamp(360px, 38vw, 760px)`，且没有自己的 resizer
- 本地根因：浏览器分栏是独立于 workspace 面板的 grid 第三列，workspace 的 resizer 只服务工作树面板，浏览器打开时工作树自动折叠，resizer 随之消失
- 方案：浏览器宽度独立 state + 复用 workspace 拖拽模式 + `--workspace-width` 轨道切换，最小侵入（无新 CSS，复用现成 resizer 样式）

### 验证

- `pnpm test` 135/135（新增 7 例 clamp 测试）· `pnpm typecheck` 0 错误 · `vite build` EXIT 0
- `wails build -ldflags "-X main.version=v10.151.0"` EXIT 0（23s）
- SHA256 归档前后一致
