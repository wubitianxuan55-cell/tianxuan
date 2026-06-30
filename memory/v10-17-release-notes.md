---
name: v10-17-release-notes
title: V10.17.0 发布记录
description: V10.17.0 发布记录 — 6个发布分支/22文件，编码修复+前端重设计全覆盖
metadata:
  type: project
---

V10.17.0 发布记录 — 6 个发布分支，22 文件变更。

**构建**: `release/v10.17.0/tianxuan-desktop.exe`
**SHA256**: `134144907b8d3fb672bc00fe587db3588c4db26a7d8c2e7d0619d352686bee87`

### v10.17.0 — 编码损坏修复 + CSS 冲突消除
- 🔴 63 处 U+FFFD 替换符修复（含 3 处 LLM 可见错误消息）
- 🎨 CSS 主题冲突消除：删除重复 light/warm 块，统一 @media 值
- 🧹 Go 死代码 + 重复注释 + 缩进修复
- 📝 11 文件变更

### v10.17.1 — 记忆面板全面优化
- 🎨 骨架屏加载状态 + 建议自动加载
- ⚡ FactCard React.memo（避免全列表重渲染）
- 🔧 acceptedSuggestions 自动重置
- ⌨️ `/` 快捷键扩展到所有标签
- 📝 2 文件变更

### v10.17.2 — 顶栏变更按钮 + 变更面板优化
- 🎨 StatusBar 新增 GitBranch 变更按钮
- 📊 变更视图：加载动画 + 空状态说明 + 变更摘要统计行 + 自动加载
- 🔧 WorkspacePanel initialViewMode prop
- 📝 3 文件变更

### v10.17.3 — 设置面板功能补充
- 🔧 字体选择器修复（从死控件→功能完整，localStorage 持久化）
- 🎨 紧凑模式开关（checkbox toggle）
- 📝 2 文件变更

### v10.17.4 — 工具卡重新设计
- 🎨 卡片式布局：彩色左边框(3px) + 状态 pill 徽章
- 🧰 工具栏：复制输出/参数按钮 + 操作目标链接
- 🔴 错误区差异化 UI（可恢复 vs 不可恢复）
- 📝 4 文件变更（含 11 个新 i18n 键）

### v10.17.5 — 文本输出重新设计（参考 Copilot/Cursor）
- 🎨 聊天式布局：用户右对齐气泡 + 助手左对齐 AI 头像
- 📝 Markdown 增强：代码块语言标签+复制按钮、表格/引用块样式
- ⚡ 流式光标动画 + AI 头像脉冲
- 📝 6 文件变更

**质量**: Go 36 包全绿 · TypeScript 零错误 · Go 测试全部通过
