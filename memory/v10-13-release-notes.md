---
name: v10-13-release-notes
title: V10.13.0 发布记录
description: V10.13.0 发布记录 — 7 项改进：体验打磨迭代
metadata:
  type: project
---

## V10.13.0 发布记录

### 1. 删除流式输出闪烁光标（MemoMarkdown + tailwind.css）
- 移除流式文本末尾的 `<span>` 闪烁光标方块
- 清理 `@keyframes cursor-blink` 定义

### 2. 修复流式文本布局抖动（MemoMarkdown）
- 流式容器添加 `contain: layout style` 防止文本增长触发全页重排
- 消除每 token 到达时的视觉闪烁

### 3. 修复同一阶段多思考卡（store.ts）
- `reasoning` 事件改为同步 dispatch，不再与 `tool_dispatch` 交错执行
- 根因：`setTimeout(0)` 异步导致 reasoning 在 tool_dispatch 后执行，`currentAssistant` 已被清除
- 修复后同一次 API 调用的 reasoning 始终合并到同一张思考卡

### 4. 清除"计划模式"概念（input.go, controller.go, execute_one.go, tool_dispatch.go, plan_bash.go, auto_plan.go）
- `PlanModeMarker`：`[Plan mode — ...]` → `[Read-only mode — ...]`
- `planApprovedMessage`：去掉 "plan mode is off"
- 工具拒绝/bahs 限制消息：`"plan mode"` 全部改为 `"read-only mode"`（7+ 处）
- **模型和用户再也看不到"plan mode"这个词**

### 5. 底栏模式子状态（StatusBar + App）
| 显示 | 含义 | 颜色 |
|------|------|------|
| `探索·只读` | 只能研究 | 蓝色 |
| `开发·可写` | 直接动手 | 绿色 |
| `开发·YOLO` | 开发+自动批准 | 绿色 |
| `编排·规划中` | 等待审批计划 | 橙色 |
| `编排·执行中` | 已审批，执行中 | 绿色 |

- YOLO 在开发模式下合并到主 badge，其他模式独立显示

### 6. 思考卡/工具卡默认折叠（Message + ToolCard）
- 思考卡：`reasoningOpen` 默认为 false，不再随流式自动展开
- 工具卡：延续默认折叠，无变化

### 7. 工具卡空间紧缩（ToolCard）
| 项目 | 之前 | 现在 |
|------|------|------|
| 标题行 padding | py-1 px-2 | py-0.5 px-1.5 |
| 字体 | 12px | 11px |
| 卡片间距 | my-0.5 | my-px |
| 圆角 | rounded-lg | rounded-md |
| 参数 maxHeight | 100px | 60px |
| 输出 maxHeight | 240px → 切换显示 | 160px（展开即显示） |

紧凑模式同步缩小。每个工具卡节省约 30-40% 垂直空间。

### 附带（继承自 V10.12.0 未提交变更）
- CodeViewer 代码块头部栏：语言 badge + 行数 + 复制
- hljs LRU 缓存（djb2 哈希，200 条目，防碰撞二次校验）
- 语言支持 11→22 种 + 别名映射大幅扩展
- 主题兼容：midnight/neon/mono → dark
- BEM 消息语义层 + 思考面板重设计（大脑 SVG + shimmer + 实时计时）
- GSAP 入场动画（stagger, 340ms）
- 移除 @tanstack/react-virtual → 纯 DOM + rAF
- bash truncateStream 边界修复（ceil 除法 + 防止反向膨胀）
