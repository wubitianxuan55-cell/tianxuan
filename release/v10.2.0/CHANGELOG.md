# V10.2.0 发布记录

**发布日期**: 2026-06-26
**构建产物**: `release/v10.2.0/tianxuan-desktop.exe` (16.2 MB)
**类型**: 功能增强 + 代码重构 + 空间清理

---

## 一、项目空间清理 (-62%)

- 删除已淘汰的 Electron 目录 (557 MB)
- 删除重复的 release 构建产物 (156 MB)
- 删除旧版本构建产物 (430 MB + 89 MB)
- 清理 node_modules 缓存 (490 MB)
- **项目总大小**: 2.15 GB → 820 MB

## 二、Go 后端 — app.go 拆分

- `app.go` 1,645 行 → **214 行**（-87%），仅保留核心生命周期
- 新增 5 个职责单一文件:
  - `app_submit.go` (134 行) — 命令提交
  - `app_session.go` (228 行) — 会话管理
  - `app_mcp.go` (306 行) — MCP 管理
  - `app_workspace.go` (369 行) — 工作区/文件
  - `app_meta.go` (442 行) — 元信息/模型/记忆
- `go build` / `go vet` / `go test` 全部通过

## 三、前端 UI 优化

### 记忆面板重设计
- 弹窗 → 右侧抽屉 (640px)，对话可见
- 快速添加栏固定顶部
- 三标签分离: 记忆 | 文档 | 建议
- 记忆卡片默认 3 行正文预览
- 内联删除确认，取消二次弹窗
- 新增 `FactCard.tsx`、`DocEditor.tsx`

### 左侧边栏优化
- 解除文件预览强制互斥锁定
- 收起态显示当前会话指示器
- 会话列表分页加载（10 → "Show more..."）
- 搜索框始终显示

### 底栏优化
- 所有数值加文字标注（缓存/Tokens/余额/任务）
- 连接状态始终显示文字标签
- 上下文进度条加宽到 60px + "上下文"标签
- 紧凑模式放宽 (h-6 → h-7)

### 统计面板
- 新建会话自动清零统计
- 子代理缓存命中率独立展示
- `WireUsage` 新增 `source` 字段区分主模型/子代理

## 四、前端工具模块

- `hooks/useGlobalShortcuts.ts` — 全局快捷键 hook
- `hooks/usePaletteItems.tsx` — 命令面板 hook
- `lib/session.ts` — 会话工具函数

## 五、构建命令

```sh
cd tianxuan/desktop
pnpm --dir frontend build     # 前端构建
wails build                   # 桌面端打包
```

## 六、变更文件统计

- 修改: 14 文件
- 新增: 10 文件
- 删除: 大量旧构建产物
- 净增: +391 行 / -2,160 行

---
