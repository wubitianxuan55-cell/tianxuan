# 桌面端全面优化设计

> 2026-06-08 | V5.22 → V5.23

## 范围

Wails 桌面端 (`tianxuan/desktop/`) 全面优化，不涉及 Go 内核 (`internal/`)。

## Phase 1 — 代码架构优化

### App.tsx 拆分 (976→~200行)

提取为自定义 hooks，每个 hook 单一职责：

| Hook | 职责 | 来源 |
|------|------|------|
| `useLayoutSizes` | 布局常量 + clamp 函数 | App.tsx L1-100 |
| `useLayoutResize` | 侧边栏/面板拖拽 resize | App.tsx 拖拽逻辑 |
| `useGlobalShortcuts` | Ctrl+N/K/,/B/J/M/H + Esc | App.tsx 快捷键 |
| `useSessionManager` | new/list/resume/delete/rename | App.tsx 会话逻辑 |
| `useModeManager` | normal/plan/yolo + thinkLevel | App.tsx 模式逻辑 |
| `useMemoryDrawer` | 记忆面板 CRUD | App.tsx 记忆逻辑 |
| `useHistoryDrawer` | 历史面板 | App.tsx 历史逻辑 |
| `useWorkspaceSwitcher` | 工作空间切换 | App.tsx 空间逻辑 |
| `usePlanExtractor` | 计划内容提取 | App.tsx useMemo |
| `useTodoExtractor` | 待办提取 | App.tsx useMemo |
| `useToolStats` | 工具/技能统计 | App.tsx 统计计算 |

### 大组件拆分

| 组件 | 行数 | 拆分 |
|------|------|------|
| Composer.tsx | 765 | 提取 ModePill、ViewToggle 子组件 |
| SettingsPanel.tsx | 663 | 按标签页拆分 |
| CapabilitiesPanel.tsx | 593 | 按列表类型拆分 |
| WorkspacePanel.tsx | 559 | 按视图模式拆分 |
| MemoryPanel.tsx | 471 | 按功能区域拆分 |

### 原则
- 纯提取，不改逻辑
- 每步可单独构建验证
- Props 接口不变

## Phase 2 — 样式系统现代化

### CSS 模块化

`styles.css` (5296行) → CSS Modules：

```
styles/
├── tokens.css          # CSS 变量
├── global.css          # reset + body
├── layout.css          # Grid 布局
└── components/*.module.css  # 每个组件一个
```

### 策略
1. 先跑 `find_unused_css.ts` 清理死规则
2. 按组件逐个迁移
3. 迁移完一个删一个原 CSS 段
4. 保留 tokens.css 为全局注入

## Phase 3 — 渲染性能

| 优化 | 手段 |
|------|------|
| 大组件 memo | React.memo + useMemo |
| 流式节流 | rAF 批量更新 |
| KaTeX 懒加载 | React.lazy |
| 虚拟滚动调优 | overscan 参数 |

## Phase 4 — 包体积

| 依赖 | 行动 |
|------|------|
| highlight.js | 按需注册语言 |
| katex | 懒加载 |

## 约束

- 桌面端构建命令：`cd desktop && wails build`
- 前端 dev：`cd desktop/frontend && npm run dev`
- 不修改 `internal/` Go 内核
- 不修改 Wails 绑定接口 (`app.go` 导出的方法)
