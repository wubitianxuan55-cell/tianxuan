---
name: v10-21-release-notes
title: V10.21.0 发布记录
description: V10.21.0 发布记录 — 计划模式彻底删除 + 系统提示词更新
metadata:
  type: project
---

## V10.21.0 发布记录

- **版本号**: V10.21.0
- **发布日期**: 2026-07-04
- **构建产物**: `release/v10.21.0/tianxuan-desktop.exe`
- **SHA256**: `6f4ae1615c0c560b4076c1774c7f08cde572f721b44db65c788fb57633e84d91`
- **构建命令**: `cd tianxuan/desktop && wails build`
- **变更统计**: 44 文件，+105/-1943 行，Go build ✅，tsc ✅

### 🔴 重大变更：计划模式彻底删除

用户反馈计划模式"太恶心"——动不动 `[Clarify]` 骚扰、权限切换时强制开启、软件彻底无法运行。本次版本做了外科手术式切除：

**删除的核心文件（11个）:**
- `internal/control/auto_plan.go` — 自动计划模式（maybeClarifyVagueInput, maybeAutoPlan, autoPlanScore）
- `internal/control/auto_plan_classifier.go` — 自动计划分类器
- `internal/control/auto_plan_classifier_test.go`
- `internal/control/auto_plan_e2e_test.go`
- `internal/control/plan_seed_test.go`
- `internal/agent/plan_bash.go` — 计划模式 bash 安全白名单
- `internal/agent/plan_bash_test.go`
- `internal/agent/planmode_test.go`
- `desktop/frontend/src/components/PlanPanel.tsx` — 前端计划面板
- `desktop/frontend/src/hooks/usePlanExtractor.ts` — 计划提取器
- `desktop/frontend/wailsjs/go/main/App.js` + `.d.ts` — Wails SetPlanMode 绑定

**修改的关键文件（33个）:**
- `controller.go` — 删除 planMode 字段、SetPlanMode()、PlanMode()、isPlanMode()、planApprovalTool
- `controller_approval.go` — 删除 seedPlanTodos()、PlanTodosJSON()、parsePlanTodos()、listItem()
- `controller_submit.go` — 删除 /goal 命令
- `input.go` — 删除 PlanModeMarker 常量、Compose() plan 分支
- `agent.go` — 删除 planMode atomic.Bool、SetPlanMode()、SetDispatcherPlanMode()
- `execute_one.go` — 删除 planMode.Load() 写工具门禁、SetStrictVerification(false)
- `stop_gate.go` — 删除 planMode.Load() 特殊处理
- `tool_dispatch.go` — 删除 planMode 字段、planBashCheck 引用
- `boot.go` — 删除 autoPlan classifier 构建逻辑
- `config.go` — 删除 AutoPlan 默认值；系统提示词更新（见下方）
- `serve.go` — 删除 /plan HTTP 路由
- `chat_tui.go`/`chat_tui_view.go` — 删除 Tab 切换、plan 状态、renderUserBubble plan 参数
- 前端 App/Bridge/Mock/Store/Tools/ApprovalModal/Transcript/useModeManager/Sidebar — 全面清理
- `i18n zh/en/zh-TW` — 删除 12+ plan 相关翻译 key

### 🆕 系统提示词更新

**新规则（替代旧 plan mode 规则）:**
> 复杂任务先探索代码库、制定方案，用 todo_write 列出步骤。等用户发送"批准"或"继续"确认后再逐步骤执行。每完成一步用 complete_step 附验证证据。权限系统自动管控写入。

**旧规则（已删除）:**
> Plan mode 下写工具被阻拦：只做只读研究，给出简洁计划后停止。

### 🐛 附带修复

- `useModeManager.ts` — 删除切换权限时强制 `setPlan(true)` 的副作用
- `web/src/bridge.ts` — 删除 SetPlanMode 前端绑定
- `input_test.go` / `outcome_test.go` / `steer_test.go` — 更新测试适配新架构
- `render_test.go` — 删除 auto_plan 断言
