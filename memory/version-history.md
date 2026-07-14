---
name: version-history
title: 版本历史
description: 版本历史汇总 — V7.6 到 V10.51.1 全部主要版本摘要
metadata:
  type: reference
---

| 版本 | 日期 | 主题 |
|------|------|------|
| V10.86.0 | 2026-07-14 | 8 项代码审查修复 — Bug修复+内存泄漏+CSS变量+架构守卫 |
| V10.85.0 | 2026-07-14 | 6 轮设置面板 UI 打磨详情 |
| V10.68.0 | 2026-07-14 | Prompt 约束强化 — 禁止执行者重新探索/推翻计划 + 桌面端构建 |
| V10.67.0 | 2026-07-14 | 从 Reasonix 蒸馏补齐设置面板 — ModelPicker/StepLimitControl/General增强/Shortcuts录制/Hooks管理/Sandbox Shell |
| V10.66.0 | 2026-07-14 | 5 项后端 BUG 清理 — goroutine泄漏/可取消ctx/错误日志 |
| V10.52.2 | 2026-07-09 | 双模型 Prompt 全面重写 + 执行契约 L2 化 + 整体优化 |
| V10.52.1 | 2026-07-09 | parallel_tasks 工具 + 品牌图标统一 + 系统提示并行指引 |
| V10.31.0 | 2026-07-04 | 双模型弹性降级 + 统计面板规划/执行拆分 + 子代理冷启动优化 |
| V10.30.0 | 2026-07-04 | web_fetch 代理(HTTP CONNECT+SOCKS5) + grep .gitignore 精确行走 + 启动动画重设计 |
| V10.26.0 | 2026-07-04 | Reasonix V1.15 蒸馏完成 + 双模型协调器(planner+executor) + 桌面端适配 |
| V10.25.0 | 2026-07-04 | 统计面板标题栏修复 + 构建脚本 |
| V10.24.0 | 2026-07-04 | agent 包拆分 + 子代理模型选择 + 统计面板优化 + 流式渲染修复 |
| V10.23.0 | 2026-07-04 | 测试修复 + boot 拆分 + 前端测试 + 缓存安全工具 |
| V10.22.0 | 2026-07-04 | 自动路由删除 + 子代理模型自由选择 + 统计面板重设计 + 权限修复 |
| V10.21.0 | 2026-07-04 | 计划模式彻底删除 + 系统提示词更新 — 44文件/-1943行 |
| V10.20.0 | 2026-07-03 | 记忆升降级 + 2阻塞修复 + 2Bug修复 + 清理 — 74文件 |
| V10.19.0 | 2026-07-03 | 系统模式重构(AgentMode→PermLevel) + 代码清理 + 前端优化 — 71文件 |
| V10.17.0 | 2026-06-30 | 编码修复+前端全面重设计 — 6分支/22文件 |
| V10.16.0 | 2026-06-30 | Bug修复+设计加固+性能优化+测试恢复 — 10 commits/30+文件 |
| V10.15.0 | 2026-06-29 | 启动黑屏热修复 + 会话记忆升级 + 前端优化 |
| V10.14.0 | 2026-06-29 | 自我进化迭代 — Reasonix 吸收 + 速度优化 |
| V10.13.0 | 2026-06-29 | 体验打磨 — 清除计划模式概念 + 流式闪烁修复 + 工具卡紧缩 |
| V10.12.0 | 2026-06-29 | 对话流式输出完整重设计 — 虚拟列表 + BEM 语义层 + 配色系统 |
| V10.11.0 | — | 体验优化迭代 |
| V10.10.0 | — | 16 项改进：Bug修复+代码清理+opencode吸收+跳转修复 |
| V10.9.0 | — | 记忆建议引擎 + 多标签页骨架 + UI 增强 |
| V10.8.1 | — | 会话体验优化 |
| V10.8.0 | — | 3 项智能化优化 |
| V7.5.0 | — | 初始提交 |
## V10.20.0 详情

- **产物**: `release/v10.20.0/tianxuan-desktop.exe`
- **SHA256**: `fde38adb2259d1eee69c41841916b2c8fe4f49866ae462a0a55f879c3cb2fc3b`
- **构建命令**: `cd tianxuan/desktop && wails build`
- **变更**: 74 文件，+1991/-1786 行

1. 🔴 阻塞修复：controller deadlock（Unlock 补回）+ permLevel 出厂 YOLO
2. 🆕 记忆 Type 升降级：Store→Controller→Wails + FactCard 按钮组
3. 🐛 StatsPanel 新会话统计不重置（skipWriteRef 守卫）
4. 🐛 消息面板点击不跳转（turnEls 清理 + items 重置清空）
5. 🧹 websearch DDG 死代码 ~150 行 + StatusBar agentMode/yolo 清理

## V10.19.0 详情

- **产物**: `release/v10.19.0/tianxuan-desktop.exe`
- **SHA256**: `6d4bb02d779b6e75d44a49fc77b2803008dc5a4019fa7738ff36b0d96fae2164`
- **变更**: 71 文件，+1891/-1704 行

1. 系统模式重构：AgentMode(explore/develop/orchestrate) → PermLevel(ask/auto/yolo)
2. 删除 mode_classifier.go + /perm 命令
3. DefaultSystemPrompt 精简 ~43→~33 行
4. 删除 L2Dir 死字段 + stopGate() ~72 行
5. 前端：usePaletteItems + useGlobalShortcuts + KaTeX 延迟 + 流式预览

## V10.30.0 详情

- **关键特性**: web_fetch 代理支持 + grep .gitignore 精确行走
- web_fetch: HTTP CONNECT + SOCKS5 代理隧道，支持 Proxy-Authorization 认证
- grep: gitignoreWalker ~260行，多层级 .gitignore 解析 + 匹配引擎
- 启动动画: Zap 图标 + 双层旋转环 + 卡片独立品牌色
- StatsPanel: planner 步骤正确归入主模型统计
- SetPlannerModel: ResolveModel 校验支持 provider/model 格式

## V10.26.0 详情

- **关键特性**: Reasonix V1.15 蒸馏完成 + 双模型协调器
- 编码管线: delete_range/delete_symbol/editlines 编码感知读写
- 子代理 transcript 持久化 (SubagentStore/SubagentRun)
- 双模型协调器 (Coordinator, ~260行): planner 流式规划 → executor 执行
- 桌面端: 双视图 Planner 模型选择器 + SetPlannerModel 绑定

## V10.53.0 详情

- **构建命令**: `cd tianxuan/desktop && wails build`
- **产物**: `build/bin/tianxuan-desktop.exe`
- **SHA256**: `0fb758fcb4637f022582d005f8d08c492400cd7699eb3b2cac2758ccc5d73d76`
- **变更**: 15 文件，+497/-605 行（含 11 个旧记忆文件清理）

### 规划者进化（HermesPrompt）
1. 角色定义：可行性/必要性/信息充分性前置三检查
2. 4 信条加固：证据自检 + API 过时警告 + 可逆性优先
3. 分类决策树 → 通用 5 步推理循环（理解意图→收集证据→评估可行性→决策→处理结果）
4. 新增 Intent check：检测隐藏意图、过早请求、非代码方案
5. 新增 Engineering judgment：blast radius / trade-off / scope discipline / priority
6. 新增"Your errors are executed blindly"警示

### 计划确认弹窗重构
7. planParser 工具 + 10 测试用例覆盖全部边界
8. PlanCard 重写：摘要栏 + 可折叠步骤卡片 + 降级方案
9. `<!--plan-->` 前缀剥离，仅传计划正文给 UI 和执行者

## V10.52.2 详情

- **构建命令**: `cd tianxuan/desktop && wails build`
- **产物**: `C:\Users\吴比\AppData\Roaming\tianxuan\bin\tianxuan.exe`
- **变更**: 6 文件，+235/-292 行

### 规划者（HermesPrompt）全面优化
1. 7原则→4信条（Evidence/Push back/Clarify/KISS+design quality）
2. 5分支决策树：纯操作/只读/需澄清/需规划/执行反馈
3. 5个研究终止条件（文件/签名/影响/测试/探索超越用户提及）
4. Zero flattery 防献媚声明 + 独立验证规则
5. 3-8步粒度约束 + Success 字段强制精确命令
6. HermesPrompt 从 155 行压缩到 90 行

### 执行者（HephaestusSystemPrompt 新增 L2 层）
1. 执行契约从 formatHandoff 移到 L2 system prompt
2. Pre-execution ritual / Step execution loop / Tool failure recovery / Parallel execution
3. 与 AGENTS.md 去重，编码铁律零重复

### 架构精简
1. formatHandoff 精简 83→16 行
2. formatExecutionFeedback 结构化 Markdown 格式
3. boot.go 注入 `compiler.WithInstructions(agent.HephaestusSystemPrompt)`
4. persistAnswer 空实现删除（-25行）
5. confirmPlan default 注释明确语义

## V10.51.1 详情

- **产物**: `build/bin/tianxuan-desktop.exe`
- **SHA256**: `c4ae09800a97ad9e40e14c58534e80d86f0fb1e9fb9b6b1e05014627ceb2fc4c`
- **构建命令**: `cd tianxuan/desktop && wails build`
- **变更**: 4 个核心文件 + 9 个周边文件

### 重启后历史会话中文输入显示修复

1. 根因：双模型模式下 handoff prompt 覆盖原始中文输入
2. `Hermes.Run` 先注入 origInput，再调用 formatHandoff，两条消息都进 session
3. `History()` 通过前缀检测识别 handoff，提取原始任务文本并显示
4. 文件：`agent.go`, `agent_run.go`, `hermes.go`, `app_session.go`

### 历史显示补齐
### 历史显示补齐

- `History()` 增加 `StripTransientBlocks` 调用
- extractOriginalTask 函数提取 handoff 中的原始任务
- Compaction summary 跳过逻辑保留并正确整合

## V10.86.0 详情

- **产物**: `release/tianxuan-v10.86.0-desktop.exe`
- **SHA256**: `3575dea003c4a80974f2158aff2b78b3e0adf08d3a823eb01e422983303837e8`
- **构建命令**: `cd tianxuan/desktop && wails build -ldflags "-s -w -H windowsgui" -o tianxuan-desktop.exe`
- **变更**: 20 文件，+146/-79 行
- **提交**: `3d6f637`

### 🔴 严重（3 项）

1. ProcessCard 图标不可见 — 10 处 `--ds-` 前缀移除
2. CSS 变量未定义 — ErrorBoundary/CapabilitiesPanel/Composer/Sidebar 修复
3. finalReadinessCheck 缺少 plannerMode 守卫

### 🟡 中等（3 项）

4. Modal setTimeout 泄漏
5. LSP client pipe fd 泄漏 + close() 错误日志
6. Transcript setTimeout 泄漏

### 🔵 低（2 项）

7. ApprovalModal inline DOM 操作 → CSS hover
8. LSP close() 静默吞错误 → slog.Warn
