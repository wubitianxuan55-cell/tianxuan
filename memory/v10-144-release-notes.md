---
name: v10-144-release-notes
title: V10.144.0 发布记录
description: V10.144.0 发布记录 — 修复对话输出时无法滚动查看前面（rAF 与 React onScroll 竞争）
---

## V10.144.0 发布记录

| 项目 | 值 |
|------|-----|
| 版本号 | V10.144.0 |
| 发布日期 | 2026-08-01 |
| 代码变更 | 3 文件（新增 lib/scrollFollow.ts + test、修改 components/Transcript.tsx）|
| 主题 | 对话输出滚动修复 |
| 构建产物 | `tianxuan-desktop.exe` |
| 文件大小 | 21,249,024 bytes (~20.3 MB) |
| SHA256 | `7aceb613cec2528285171c79d1ed976a7f230c8882cbfef92983da30df4b0385` |

### 本发布包含的提交

| 提交 | 主题 |
|------|------|
| (本轮) | 修复对话输出时无法滚动查看前面 — rAF 与 React onScroll 竞争 |

### 根因（5 级追溯）

- 表象：流式输出期间滚轮上滚无效，位置立即被拉回底部，只能等输出完成。
- 直接原因：`scrollToBottom` 的 rAF 回调无条件执行 `el.scrollTop = el.scrollHeight`
  （只要 stick 为 true）；stick 由 React 合成 onScroll 事件更新（异步批处理）。
- 本地根因：rAF 回调与 React scroll 事件存在窗口期——用户滚轮后浏览器原生滚动
  已发生但 React onScroll 未执行，stick 仍 true；下一帧 rAF 抢先拽回。
- 系统根因：决策只依赖 stick 标志（异步更新），未校验真实 DOM 位置；rAF 是原生
  时机、onScroll 是 React 合成时机，不同步。
- 过程根因：V10.87 滚动逻辑只测了正常路径，未覆盖"输出中 rAF 抢跑"竞争路径。

### 修复

- `lib/scrollFollow.ts`：滚动决策纯函数化——`shouldFollowAfterGrow` 要求 stick 为
  true **且** 真实位置在底部阈值内才跟随；rAF 抢跑（用户已滚离但 stick 未更新）
  时拒绝拽回
- `components/Transcript.tsx`：rAF 回调前用真实 scrollTop/scrollHeight/clientHeight
  决策

### 测试

- `lib/scrollFollow.test.ts` 9 用例：距离/阈值/自定义阈值/stick 语义/rAF 抢跑回归/
  内容增长/空容器/常量

### 验证

- `tsc --noEmit` 0 错误；vitest 61/61 全绿；Wails build（含前端编译）通过
