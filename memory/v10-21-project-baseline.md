---
name: v10-21-project-baseline
title: V10.21.0 项目基准
description: V10.21.0 项目基准 — 当前版本、构建命令、核心变更
metadata:
  type: project
---

## 当前版本
- **版本号**: V10.21.0
- **发布日期**: 2026-07-04
- **构建产物**: `release/v10.21.0/tianxuan-desktop.exe`
- **SHA256**: `6f4ae1615c0c560b4076c1774c7c08cde572f721b44db65c788fb57633e84d91`
- **构建命令**: `cd tianxuan/desktop && wails build`

## 核心变更 — 计划模式彻底删除 + 系统提示词更新
44 文件变更，+105/-1943 行，Go build ✅，tsc ✅。

### 重大变更
- **计划模式彻底删除**: 11 个文件删除，33 个文件修改，从 Go 后端到 React 前端全部清理
- **`[Clarify]` 骚扰彻底消失**: `maybeClarifyVagueInput` 连同 `auto_plan.go` 已删除
- **权限切换不再强制开启计划模式**: `useModeManager.ts` 中 `setPlan(true)` 副作用已移除

### 系统提示词
- 删除 Plan mode 规则，替换为规划执行规则：先探索 → todo_write 列步骤 → 用户确认 → 逐步骤 complete_step

### 附带修复
- 权限默认值 `ask` 保持不变，工具调用按需弹出确认框
- taskGate / verifyGate / goalGate 三重门禁保留，确保任务不中途停止
