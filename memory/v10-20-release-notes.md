---
name: v10-20-release-notes
title: V10.20.0 发布记录
description: V10.20.0 发布记录 — 记忆升降级 + 2阻塞修复 + 2Bug修复 + 清理
metadata:
  type: project
---

## V10.20.0 发布记录

- **版本号**: V10.20.0
- **发布日期**: 2026-07-03
- **构建产物**: `release/v10.20.0/tianxuan-desktop.exe`
- **SHA256**: `fde38adb2259d1eee69c41841916b2c8fe4f49866ae462a0a55f879c3cb2fc3b`
- **构建命令**: `cd tianxuan/desktop && wails build`
- **变更统计**: 74 文件，+1991/-1786 行，Go build ✅，tsc ✅

### 🔴 阻塞修复（2 项）

1. **死锁修复** — `controller_approval.go`
   - `AgentMode→PermLevel` 重构时误删 `g.c.mu.Unlock()`，导致 auto 分支锁永不释放、非 auto 分支 `requestApproval` 二次加锁死锁
   - 补回 `g.c.mu.Unlock()` 在 auto 赋值后、if 判断前

2. **出厂 YOLO 安全漏洞** — `controller.go`
   - `New()` 结构体字面量未初始化 `permLevel`，零值 `""` ≠ `"ask"` → `auto=true` 永久成立
   - 所有工具调用自动批准，权限机制完全绕过
   - 添加 `permLevel: "ask"` 到结构体字面量

### 🆕 记忆 Type 升降级（11 文件）

- **Go 后端**: `Store.ChangeType()` → `Controller.ChangeFactType()` → `App.ChangeFactType()` Wails 绑定
- **前端 FactCard**: 展开视图新增升降级按钮组——当前类型置灰，点击切换
  - ⬆ `用户` — 提升为用户级（跨项目共享）
  - 📋 `项目` — 设为项目级（当前项目专属）
  - ⬇ `反馈` — 设为反馈级（工作方式指导）
- **i18n**: 三语言新增 `changeType`、`typeCurrent`、`promoteToUser/Project/Feedback` 共 5 key

### 🐛 Bug 修复（2 项）

3. **新建会话统计不重置** — `StatsPanel.tsx`
   - 根因：新建会话后同一渲染周期内，清除 effect 和 turnSteps 写入 effect 同时执行，旧 usage 数据覆盖新会话空白统计
   - 修复：`skipWriteRef` 布尔守卫——resetKey 变化时置 true，turnSteps effect 跳过首次写入

4. **消息面板点击不跳转** — `Transcript.tsx`
   - 根因 1：`turnEls` Map 元素卸载时从未删除条目，残留脱离文档的旧 DOM 引用，`scrollIntoView` 静默无效
   - 根因 2：items 重置（新会话）时 `turnEls` 不清空
   - 修复：ref 回调处理 `null` → `delete(tn)` + items 为空时整体 `clear()`

### 🧹 清理（4 项）

5. **websearch.go DuckDuckGo 死代码** — 删除 `searchDuckDuckGo` + `parseDuckDuckGoLite` + `stripTags` + 4 regexp 变量约 150 行
6. **builtin_test.go DDG 测试** — 删除 2 个依赖已删除函数的测试
7. **StatusBar 死代码** — 移除不再传入的 `agentMode`/`yolo` prop 及关联 badge 渲染，新增 `permLevel` badge
8. **serve_handlers 语义修正** — JSON key `"bypass"` → `"autoApprove"`

### 验证

- Go build: ✅ 零错误
- TypeScript: ✅ 零错误
- 测试: memory 57/57 ✅ · builtin 90+/90+ ✅ · serve 20/20 ✅
