---
name: v10-86-0-release-notes
title: V10.86.0 发布记录
description: V10.86.0 发布记录 — 8 项代码审查修复：Bug修复+内存泄漏+CSS变量+架构守卫
metadata:
  type: project
---

# V10.86.0 发布记录

**产物**: `release/tianxuan-v10.86.0-desktop.exe`
**SHA256**: `3575dea003c4a80974f2158aff2b78b3e0adf08d3a823eb01e422983303837e8`
**构建命令**: `cd tianxuan/desktop && wails build -ldflags "-s -w -H windowsgui" -o tianxuan-desktop.exe`
**变更**: 20 文件，+146/-79 行
**提交**: `3d6f637`

## 🐛 8 项代码审查修复

### 🔴 严重（3 项）

1. **ProcessCard 图标不可见** — TONE_COLORS/STATE_COLORS 中 `--ds-` 前缀与 styles.css 定义的 `--fg-faint`/`--ok`/`--warn`/`--err`/`--accent` 不匹配，10 处 CSS 变量前缀已移除
2. **CSS 变量未定义** — ErrorBoundary.tsx 中 5 处 `--ds-fg`/`--ds-bg`/`--ds-border-soft` 前缀错误；CapabilitiesPanel.tsx 引用未定义的 `--ds-danger-soft`；Composer/Sidebar 引用未定义的 `--ds-shadow-accent-btn`
3. **finalReadinessCheck 缺少 plannerMode 守卫** — 注释说 plannerMode 跳过 readiness gates 但实际未跳过，导致规划者因 todo_write 标记循环触发证据验证

### 🟡 中等（3 项）

4. **Modal setTimeout 泄漏** — 组件卸载后 120ms 定时器仍调用 `onClose()`，引入 `useRef` 跟踪 + `useEffect` cleanup
5. **LSP client pipe fd 泄漏** — `StdinPipe`/`StdoutPipe` 成功后若 `Start()` 失败，已打开的 pipe 未关闭；`close()` 中 shutdown/exit/kill/wait 错误被 `_` 静默丢弃
6. **Transcript setTimeout 泄漏** — `scheduleMeasure` 中 250ms 定时器未清理，引入 `measureTimer` ref + cleanup

### 🔵 低（2 项）

7. **ApprovalModal inline DOM 操作** — `onMouseEnter`/`onMouseLeave` 直接操作 `style.boxShadow`，改用 CSS `hover:shadow-[var(--ds-shadow-card-hover)]`
8. **LSP close() 静默吞错误** — 改用 `slog.Warn` 记录 shutdown/exit/kill/wait 错误
