# V10.10.0 发布记录

**发布日期**: 2026-06-29
**构建产物**: `release/v10.10.0/tianxuan-desktop.exe`
**上一版本**: V10.9.0

---

## 概述

V10.10.0 是一次综合性优化版本，包含 **16 项改进**，分为四个主题：

| 主题 | 改进数 | 来源 |
|------|--------|------|
| 🔴 Bug 修复 + 稳定性 | 6 项 | 代码审查 |
| 🧹 代码清理与精简 | 4 项 | 代码审查 |
| 📚 opencode 吸收 | 5 项 | opencode 深度学习 |
| 🖱️ 消息跳转修复 | 1 项 | 使用反馈 |

---

## 🔴 Bug 修复 + 稳定性（6 项）

### 1. FactCard 编辑保存按钮修复
- **问题**: 记忆面板中点击「编辑」后点「保存」，编辑内容静默丢失
- **修复**: 新增 `UpdateFact` 后端链路（Controller → App → Bridge），通过 `Store.Save` 覆盖写入
- **涉及**: 8 个文件 — `controller_memory.go`, `app_meta.go`, `bridge.ts`, `mock.ts`, `store.ts`, `FactCard.tsx`, `MemoryPanel.tsx`, `App.tsx`

### 2. randomTabID panic 消除
- **问题**: `crypto/rand.Read` 失败直接 `panic` 崩溃整个桌面进程
- **修复**: 改为时间戳回退 `hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))[:16]`
- **涉及**: `tabs.go`

### 3. memory_suggestions 并发安全澄清
- **问题**: 代码审查标记 `existingMemoryText` 可能存在 data race
- **修复**: 添加注释说明 `*memory.Set` 是不可变快照，`refreshMemoryLocked` 创建新实例
- **涉及**: `memory_suggestions.go`

### 4. saveTabs/loadTabs 错误日志
- **问题**: 5 处错误静默吞掉，标签页持久化数据损坏零日志
- **修复**: `MkdirAll`/`MarshalIndent`/`WriteFile`/`ReadFile`/`Unmarshal` 全部改为 `log.Printf`
- **涉及**: `tabs.go`

### 5. MemoryPanel render 副作用修复
- **问题**: `setScope()` 写在 render 函数体中，每次渲染可能触发 setState，违反 React 规则
- **修复**: 移到 `useEffect` 中
- **涉及**: `MemoryPanel.tsx`

### 6. 新增 17 个测试用例
- **tabs_test.go**: 5 个用例 — `randomTabID`, `newWorkspaceTab`, `tabMeta`, `nil`, `roundTrip`
- **memory_suggestions_test.go**: 12 个用例 — `extractMemoryStatement`, `normalizeSuggestionKey`, `existingCovers`, `oneLine`, `suggestionName/Title`

---

## 🧹 代码清理与精简（4 项）

### 7. 移除 4 个未使用的 autosave 字段
- 删除 `saveMu`/`saving`/`saveAgain`/`saveCond` — 全项目零 `Wait()`/`Signal()` 调用
- 保留 `closing`（仍在 `saveTabs` 和 `app.go:299` 中使用）
- **涉及**: `tabs.go`, `tabs_test.go`

### 8. 删除 `resolveSessionDisplay` 死函数
- 零非测试调用方
- 连带删除 `TestSessionDisplayRoundTrip` 测试 + `TestDeleteSessionFile` 中的引用
- **涉及**: `sessions.go`, `sessions_test.go`

### 9. 删除 `MemoryView.suggestions` 死字段
- Go 后端从未填充，前端从未读取
- **涉及**: `types.ts`

### 10. 提取 `saveAtomically` 共享函数
- `saveSessionTitles` 和 `saveSessionDisplays` 的原子写模式完全重复（各 ~18 行）
- 提取为 `saveAtomically(dir, pattern, path, v)` 单一函数
- **涉及**: `sessions.go`

---

## 📚 opencode 吸收（5 项）

通过对 [opencode](https://github.com/anomalyco/opencode) (6019 文件 TypeScript 项目) 的深度学习，吸收以下设计模式：

### 11. KeepProtected — 压缩时保护关键工具输出
- 借鉴 opencode `PRUNE_PROTECTED_TOOLS = ["skill"]`
- tianxuan 保护列表: `read_skill`, `memory_search`, `remember`
- 在 `compact.go`（LLM 折叠）和 `prune.go`（工具结果修剪）两处生效
- 默认启用（`KeepProtected` 标志位为 0 时自动开启）
- 新增测试 `TestKeepProtectedToolResult` 验证 tool-call-group 完整性

### 12. 压缩预算感知 — 最近 turn 保留保证
- 借鉴 opencode `preserveRecentBudget`（25% 窗口 + 2000-8000 钳制）
- `planCompaction` 中保证至少 25% 上下文窗口留给最近 turn
- 确保受保护工具输出和最近上下文在压缩中存活

### 13. ToolContext 抽象
- 借鉴 opencode `Tool.Context` 接口
- 新增 `ToolContext` 结构体和 `ContextualTool` 可选接口
- 在 `executeOne` 中注入：sessionID, agentName, toolCallID, full Messages
- 向后兼容——未实现 `ContextualTool` 的工具继续使用 `Execute(ctx, args)`

### 14. 子代理 XML 结构化输出
- 借鉴 opencode `<task>` 标签模式
- 成功结果包裹在 `<task-result>...</task-result>` 中
- `output_schema` 路径不受影响（JSON 结果不包装）

### 15. 消息面板跳转修复
- **根因**: `scrollToTurnRef` 中 `stick.current = true` 与 ResizeObserver 竞态
- **修复**: 移除 `stick.current = true`；fallback 路径增加位置估算
- **涉及**: `Transcript.tsx`, `JumpBar.tsx`, `MessageNavigator.tsx`

---

## 变更统计

| 指标 | 数值 |
|------|------|
| 修改文件 | 22 个 |
| 新增文件 | 2 个（测试） |
| 新增代码 | ~300 行 |
| 删除代码 | ~65 行 |
| 新增测试 | 18 个用例 |

## 缓存前缀不变性

✅ 全部 16 项变更经过缓存前缀路径验证——不修改 `memory.Compose`, `memory.Block`, `boot.Build` 系统提示词组装。

## SHA256

```
ebdd496cb4d87d9e7ab73847bae46b5e87abdf6fe4889053d59eb78bc7198c47  tianxuan-desktop.exe (16MB)
```
