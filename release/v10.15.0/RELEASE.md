# V10.15.0 发布记录

> 启动黑屏热修复 + 会话记忆升级 + 前端组件优化 · 2026-06-29

---

## 🔥 关键修复：启动黑屏

### 根因
`react-virtuoso` v4.12~v4.18 在 Wails WebView2 环境中与 React 18.3 不兼容，触发 React error #321（Invalid hook call），导致 React 根被卸载 → 窗口黑屏。

### 修复
- Transcript 虚拟列表从 `react-virtuoso` 回退到已验证的 DOM 原生滚动方案
- `@tanstack/react-virtual` 保留为构建依赖（vite manualChunk）
- 前端组件优化（App/JumpBar/MessageNavigator）已适配 `onThreadEl` 移除，无需回退
- **文件**：`Transcript.tsx`, `vite.config.ts`, `package.json`, `pnpm-lock.yaml`

---

## 🧠 会话记忆升级

### 6. `promote_session_facts` 工具
- 新增 `promote_session_facts` 工具，模型可将临时会话记忆提升为永久存储
- `remember(session=true)` 保存的记忆跨轮次存活，`promote_session_facts` 一次性全部落盘
- 内存降重：同名永久记忆自动更新而非重复创建
- **文件**：`memory/promote.go` (+69 新文件), `controller.go` (+7), `controller_memory.go` (+30), `boot.go` (+3), `agent.go` (+6)

---

## 🎨 前端体验优化

### 7. 对话跳转解耦
- JumpBar / MessageNavigator 不再依赖 `threadEl` DOM 引用
- 改用 Transcript 暴露的 `scrollToTurn` 函数式接口
- 移除 App.tsx 中 `threadEl` 状态管理
- **文件**：`App.tsx` (-5), `JumpBar.tsx` (-15), `MessageNavigator.tsx` (-15)

### 8. 流式渲染优化
- `ensureAssistant()` 移除，text/reasoning 事件改为反向查找最后一个 assistant 项
- 避免 `tool_dispatch` 清空 `currentAssistant` 后错误创建新卡片
- `streaming: true` 显式设置在增量更新中
- **文件**：`store.ts` (+20/-15)

### 9. ToolCard 紧凑化
- 间距/字号/图标/边距全面收缩，节省 30-40% 垂直空间
- **文件**：`ToolCard.tsx`

---

## 📋 完整文件变更

| 文件 | 改动 |
|------|------|
| `Transcript.tsx` | 🔥 回退 react-virtuoso → DOM 原生滚动 |
| `vite.config.ts` | 回退 `react-virtuoso` → `@tanstack/react-virtual` |
| `package.json` | 移除 `react-virtuoso`，恢复 `@tanstack/react-virtual` |
| `pnpm-lock.yaml` | 重新生成 |
| `App.tsx` | -5（onThreadEl 移除） |
| `JumpBar.tsx` | -15（threadEl 解耦） |
| `MessageNavigator.tsx` | -15（threadEl 解耦） |
| `ToolCard.tsx` | 紧凑化调整 |
| `ToolGroup.tsx` | 配套调整 |
| `store.ts` | +20/-15（ensureAssistant 移除 + 流式优化） |
| `memory/promote.go` | +69 新文件 |
| `memory/queue.go` | 更新 |
| `memory/remember.go` | 更新 |
| `memory/search.go` | 更新 |
| `controller.go` | +7（sessionFacts + SetPromoter） |
| `controller_memory.go` | +30（PromoteSessionFacts + SessionFacts） |
| `control/input.go` | 更新 |
| `boot.go` | +3（注册 promote_session_facts） |
| `agent.go` | +6（SetSessionSaver + SetPromoter） |
| `agent/cache_shape.go` | 更新 |
| `agent/compact.go` | 更新 |
| `agent/execute_one.go` | 更新 |
| `tool/builtin/memory_search.go` | 更新 |
| `tool/builtin/websearch.go` | 更新 |

---

## 🔐 校验

```
SHA256: 01ac5f6a0397a62959a36b915606f94ebf71b37214a458c48cdd0332b0e0132c
```
