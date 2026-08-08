---
name: version-history
title: 版本历史
description: 版本历史汇总 — V7.6 到 V10.88.0 全部主要版本摘要
metadata:
  type: reference
---

| 版本 | 日期 | 主题 |
| V10.161.0 | 2026-08-08 | 桌面端设置面板支持 OpenCode Zen — desktop 注册 opencode kind、预设模板一键添加、用户级 config.toml 预置 zen |
| V10.160.0 | 2026-08-08 | OpenCode Zen 模型接入 — opencode provider kind 按模型路由三协议（chat/completions/messages/responses），免费模型匿名可用，真实 API 三协议端到端验证通过 |
| V10.159.0 | 2026-08-05 | read_file 按符号跳读 — symbol 参数定位定义行 + 未找到大声失败附附近符号名（SHA256 见 release） |
| V10.158.0 | 2026-08-05 | todo_write blocked 状态 + edit_lines 多行锚点（外部依赖空转降噪、锚点可含换行） |
| V10.157.0 | 2026-08-05 | validation 错误附正确示例、bash chat 参数误用检测、stale 编辑守卫软警告、offload 预览 200→800 字符 |
| V10.156.0 | 2026-08-05 | 背景会话 fork — task 工具 inherit_context 参数（Qwen Code /fork 语义），forkCtx 注入子代理 |
| V10.155.0 | 2026-08-05 | Windows 安装包 + 自动更新链路打通 — NSIS per-user 安装器、发布目标修复（真实仓库）、minisign 密钥轮换、publish-desktop.ps1 一键发布 |
| V10.154.0 | 2026-08-04 | Codex CLI 工具蒸馏 — 执行前 schema 校验（validation_error+别名兼容）、compact 参数描述保留（CanonicalizeSchemaVerbose）、工具错误统计（tools stats）、learning 链路修复（Observe），SHA256 90a0b4b9… |
| V10.153.0 | 2026-08-03 | P0-P5 工程改进 — edit_lines 锚点校验+自动语法回滚、PowerShell heredoc/npx.cmd 适配、release 发布技能、L1 Git 工作流提示、DATABASE_URL 预警（SHA256 957ecce4…） |
| V10.152.0 | 2026-08-02 | 编程能力强化 — 提示词重构（总开销 -79%）+回合减负+工具收敛+测试先行规则；实测对比 Codex：从零实现 71s/¥0.02，修 bug 测试先行 100% 遵守；桌面端打包发布（SHA256 8a3310ec…） |

|------|------|------|
| V10.151.0 | 2026-08-02 | 修复浏览器右侧分栏无法拖动变宽 — 独立宽度 state + resizer（clamp 360-1080/62% 比例/对话区保护），前端 135 测试全绿，桌面端发布（SHA256 1ac93074…） |
| V10.150.0 | 2026-08-02 | 蒸馏 OpenClaw Model Failover — turn-local 模型回退链（429/5xx/断网切备用模型，整链退避重试，FallbackSummaryError），failover 包 11 例 + boot 接线 3 例全绿；桌面端打包发布（SHA256 c9e787b0…） |
| V10.149.0 | 2026-08-02 | 后端蒸馏 Reasonix Auto Failure Guard — 宿主侧失败升级决策（精确操作指纹/3 次停操作/6 次停回合/成功清零预算），TDD 8+3 例全绿 |
| V10.147.0 | 2026-08-02 | 技能系统重构：子代理化 + 工具化 + 工具压缩（55→46）；edit_lines 吞行修复；DeepSeek 峰谷计价；待确认记忆自动提炼 + 记忆面板详情 |
| V10.146.0 | 2026-08-01 | 修复 edit_lines compact schema 丢失 minimum 约束 — 模型不再漏传/传 0 行号 |
| V10.145.0 | 2026-08-01 | 全面清理废弃残留 — 删除 web/ 前端目录 + 根目录杂物 + tools/ 加 gitignore |
| V10.144.0 | 2026-08-01 | 修复对话输出时无法滚动查看前面 — rAF 与 React onScroll 竞争（scrollFollow 纯函数） |
| V10.143.0 | 2026-08-01 | 记忆提取质量修复 — 控制块泄漏 + 重复候选堆积（transient 任意位置匹配 + pending 去重 + 公共子串） |
| V10.142.0 | 2026-08-01 | 记忆系统重构 — 项目归一存储/自动提取/跨会话记忆/自主进化 + 桌面端面板重排与交互修复 |
| V10.141.0 | 2026-08-01 | 验证分级 — 简单/前端修改不再强制后端全量测试 |
| V10.140.0 | 2026-08-01 | 修复单模型技能几乎不触发 — 自动触发关键词扩展 |
| V10.139.0 | 2026-08-01 | 子代理并行优先 — 调查默认走子代理，主上下文只留结论 |
| V10.138.0 | 2026-08-01 | 构建脚本自动补齐 PATH |
| V10.137.0 | 2026-08-01 | 单模型中途纠偏（steer）+ Adaptive Execution 定稿 — 双模型/单模型工作流分离完成 |
| V10.136.0 | 2026-08-01 | Adaptive Execution 细节 — 同步骤失败宿主信号 + Solo 进度保护 |
| V10.135.0 | 2026-08-01 | 单模型工作流重定义 — Adaptive Execution（回退 AutoPlan 计划确认） |
| V10.134.0 | 2026-08-01 | 单模型 AutoPlan 接线（已回退） |
| V10.133.0 | 2026-08-01 | 修复技能/子代理统计恒 0 |
| V10.132.0 | 2026-08-01 | 修复代办进度落后 — complete_step 匹配容错 |
| V10.131.0 | 2026-08-01 | 双模型工作流 II — 步骤覆盖度对照 |
| V10.130.0 | 2026-08-01 | 双模型工作流 — 直接执行回灌规划者 |
| V10.129.0 | 2026-08-01 | 双模型优化 II — 漏标记补偿 / 路由三值化 |
| V10.128.0 | 2026-08-01 | 双模型优化 — 规划者压缩保留 / 计划步数判定 |
| V10.88.1 | 2026-07-15 | isErrorResult 死代码热修复 — isErrorResult 移出 switch case 覆盖全部工具调用 + gofmt 格式化 |
| V10.88.0 | 2026-07-15 | 双模型 Loop 工作流深度优化 — 三重门恢复+planFix反思+TurnResult去重+formatSummary改名，13 项改进 |
| V10.87.0 | 2026-07-15 | 双模型验证闭环 — Plan→Execute→Verify→Fix→Complete，18 项改进 |
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
