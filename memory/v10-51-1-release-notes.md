---
name: v10-51-1-release-notes
title: V10.51.1 发布记录
description: V10.51.1 发布记录 — 重启后历史会话中文输入显示修复
metadata:
  type: reference
---

## V10.51.1 发布记录

### 重启后历史会话中文输入显示为英文（Bug 修复）

- **根因**: 双模型（Hermes + Hephaestus）模式下，用户原始中文输入在 `Hermes.Run` 中被 `formatHandoff()` 输出（大段英文 handoff prompt）替换，该英文 prompt 被 Hephaestus 的 `runDirect` 作为"用户消息"存入 session 并持久化到 JSONL。重启后 `History()` 从 JSONL 加载消息，展示的是英文 handoff prompt。
- **修复**: 
  - `Hermes.Run` 先注入原始中文输入（`origInput`），再调用 `formatHandoff`，两条消息都进入 session 供模型使用
  - `History()` 通过前缀检测（`"# tianxuan hephaestus handoff"`）识别 handoff 消息，提取其中的原始任务文本并显示
  - 缓存命中不受影响：system prompt 不变，formatHandoff 缓存行为与修复前一致
- **文件**: `agent.go`, `agent_run.go`, `hermes.go`, `app_session.go`

### 相关优化

- `History()` 增加了 `StripTransientBlocks` 调用（此前遗漏），正确剥离 `<reasoning-language>`、`<session-facts>`、`<procedural-rules>`、`<episodic-memory>` 等 transient blocks
- Compaction summary 跳过逻辑保留并正确整合

### 构建

- SHA256: `c4ae09800a97ad9e40e14c58534e80d86f0fb1e9fb9b6b1e05014627ceb2fc4c`
- 构建命令: `cd tianxuan/desktop && wails build`
- 产物: `build/bin/tianxuan-desktop.exe`
