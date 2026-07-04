---
name: v10-22-release-notes
title: V10.22.0 发布记录
description: V10.22.0 发布记录 — 自动路由删除 + 子代理模型自由选择 + 统计面板重设计 + 权限修复
metadata:
  type: project
---

## V10.22.0 发布记录

- **版本号**: V10.22.0
- **发布日期**: 2026-07-04
- **构建产物**: `release/v10.22.0/tianxuan-desktop.exe`
- **SHA256**: `a93bf8bb50ee4cb59e4ba6492bcd3a72a04327636143f1d6367cfc55208099f8`
- **构建命令**: `cd tianxuan/desktop && wails build`
- **变更统计**: 26 核心文件，+384/-682 行，Go build ✅，TSC ✅

---

### 🔴 重大变更：删除自动路由 pro/flash

动态切换 pro/flash 模型会导致 DeepSeek 前缀缓存断裂。`auto_router.go`（96行）、`auto_router_history.go`（155行）、`session_route_features.go`（150行）及其测试文件全部删除。子代理不再继承主模型的 auto-routing 逻辑，改为独立配置。

### ✅ 子代理模型自由选择

- 设置面板 Agent 页新增「子代理模型」下拉框，可从所有已配置 provider 中选择
- 选空 = 继承主模型；选 flash = 子代理用便宜模型，不影响主代理前缀缓存
- 配置持久化到 `tianxuan.toml`（`agent.subagent_model`）
- `TaskTool` 新增 `SetSubagentProvider()`，`boot.go` 从配置解析并注入

### ✅ 统计面板重设计：主模型/子代理分开统计

- 删除 Tab 切换，改为平铺布局
- 会话级表格（Prompt/Compl/缓存命中率(高亮)/成本 × 主模型/子代理/汇总）
- 本轮级表格（同上，实时数据）
- 命中率趋势图拆为两个：主模型（accent 蓝）+ 子代理（warn 橙）
- 当前步增加 source 标签（主模型/子代理）
- `StepRecord` 持久化增加 `source` 字段

### ✅ 底栏显示主模型和子代理模型

- 连接灯右侧显示主模型 chip（Cpu 图标 + accent 蓝边框）
- 子代理 chip（GitBranch 图标 + warn 橙边框，仅与主模型不同时显示）
- 模型名自动简化（去掉 `deepseek-v4-` / `mimo-v2.5-` 前缀）

### ✅ 修复权限 ask/auto/YOLO 失效

- **Critical Bug**: `rebuild()` 和 `SetModel()` 重建 controller 时 YOLO/auto 状态静默丢失。修复：销毁前保存 `permLevel`，新 controller 构建后恢复。
- **死代码清理**: 删除 `agent.go` 中只写不读的 `permLevel` 字段和 `SetPermLevel` 方法，删除 `controller.go` 中对 `executor.SetPermLevel()` 的死调用。

### 技术细节

- `event.Event` 新增 `UsageSource` 字段 + `UsageSourceMain`/`UsageSourceSubagent` 常量
- 主模型 Usage 事件打标 `"main"`，子代理 `subSinkFor` 覆写为 `"subagent"`
- `wireUsage` 新增 `source` JSON 字段
- 前端 `store.ts` 新增 `perTurnMainUsage`/`perTurnSubUsage` 分离累加器
