## [10.56.0] — 2026-07-11

### 🧠 双模型提示词全面重写

- **Hermes（规划者）**：从 178 行碎片化 checklist 重写为 42 行 Reasonix 风格
  - HARD-GATE 前置、5 步思考流程、Anti-patterns 拒绝标准全部删除
  - 只定义输入输出边界：只读工具、输出类型（直接回答/Ask/计划）、步骤格式
  - 搭档约束（Hephaestus 零判断执行）+ 执行回执协议（`[上一轮执行结果]`）
  - UI 设计时调用 `read_skill(name="ui-ux-pro-max")`，skill 自身引导
- **Hephaestus（执行者）**：从 107 行重写为 62 行，Karpathy 4 原则为骨架
  - Think Before Coding / Simplicity First / Surgical Changes / Goal-Driven Execution
  - 并行优先（Parallel first），Ask 工具允许真正的用户决策
  - 步骤格式 5→3 字段（砍 Success、Risk recovery），TDD 自动

### 🎨 8 套配色全面重新设计

- **基于 ui-ux-pro-max skill**：`--design-system` + `--domain color` 生成，零手搓 hex
- 默认/暖色/冰蓝/森林/霓虹/午夜/玫红/石墨 —— 每套独立个性
- fg vs bg 对比度 ≥ 10:1，拒绝灰色模糊字体
- SettingsAppearance 预览色同步更新

### 🔧 计划确认弹窗同步优化

- PlanCard 详情区删除 Success/Risk recovery 渲染（与步骤格式同步）
- `RotateCcw` 导入移除
- `planParser` 步骤标题正则增强：支持 `###`/`##` Markdown 前缀、数字编号列表

## [10.55.0] — 2026-07-10

### 🎨 计划确认弹窗重构

- **InlineMarkdown**：步骤标题/变更描述/风险恢复正确渲染 `**粗体**` / `` `代码` ``
- **依赖可视化**：紫色 badge 显示步骤依赖关系
- **折叠修改意见**：`+ 修改意见…` 按钮，点击展开输入框
- **键盘快捷键**：`1` 提交 · `2` 修改 · `3` 仅聊天 · `Esc` 取消
- **拖拽 hook 提取**：`useDraggableCard` 可复用

### 💬 对话输出 TurnCollapse

- **处理过程折叠**：工具调用 + 推理思考收入折叠条，最终回答独立显示在下方
- **自动展开/折叠**：流式时展开，完成后用户可手动切换

### 🛠 优化

- **planParser 鲁棒性**：`<!--plan-->` 剥离 + 宽松 fallback regex
- **i18n 精简**：计划弹窗按钮去数字编号

## [10.54.0] — 2026-07-10

### 🧬 V1.17.10 蒸馏（内核）

- **任务/聊天分类器** (`internal/agent/task_classifier.go` 新建)：LLM + 启发式双模分类，SHA256 LRU 缓存，区分 "fix the bug" vs "hello"
- **审批安全强化** (`internal/control/controller_approval.go`)：`remember`/`forget` 在 yolo/auto 模式下仍需人工审批
- **模式切换排空** (`internal/control/controller.go`)：切换到 auto/yolo 时自动排空可批准项
- **瞬态块剥离** (`internal/agent/session/transient.go`)：新增 `<hook-context>` / `<active-goal>` / `<capability-route>` 三种块类型

### 🖥️ V1.17.10 蒸馏（桌面端）

- **OS 错误友好化** (`desktop/session_errors*.go` 新建)：文件共享冲突/权限拒绝/磁盘满 → 用户可读消息
- **Prompt 交换诊断** (`desktop/settings_app.go`)：配置变更恢复会话时若 system prompt 变化，记录 warn 日志
- **TurnCollapse 推理折条** (`desktop/frontend/src/components/Message.tsx`)：推理过程独立折叠在正文上方，流式自动展开/完成后自动折叠

### 🔧 改进

- **Token 统计对齐分析**：对比 DeepSeek 官方 tokenizer，确认动态校准机制覆盖主要风险

## [10.53.0] — 2026-07-09

- Hermes prompt 全面升级 + 计划确认弹窗重设计 + 记忆文件发布记录 + 项目基准 + 版本历史

## [10.52.2] — 2026-07-09

- 双模型 Prompt 全面重写 + 执行契约 L2 化

## [10.52.1] — 2026-07-09

- parallel_tasks 工具 + 品牌图标统一 + 系统提示并行指引

## [10.52.0] — 2026-07-08

- UI 全面优化 — ui-ux-pro-max 设计规则系统应用

## [10.51.1] — 2026-07-08

- 修复重启后历史会话中文输入显示为英文 + 启动命令跨平台自动后台化

## [10.51.0] — 2026-07-08

- 配色重设计 + 记忆面板重构 + 模型面板升级 + 双模型协作强化

## [10.50.0] — 2026-07-08

- Hermes 设计质量原则 + Superpowers v6.1.1 蒸馏 + 双模型 AGENTS.md 角色区分

## [10.49.0] — 2026-07-08

### 🕐 定时任务系统（全新功能）

- **核心调度器** (`internal/schedule/`)：进程内 goroutine 调度器，1 秒 ticker 检测到期任务
- **数据模型**：Schedule（hourly/daily/weekly + 时间点）+ ScheduleResult（执行记录，最多保留 20 条）
- **双层存储**：全局（`~/.config/tianxuan/schedules.json`）+ 工作区（`.tianxuan/schedules.json`），JSON 原子写入
- **执行桥接**：跳过 Hermes 规划者，直接用 Hephaestus 执行者，PlannerMode=true
- **桌面端集成**：7 个 Wails bindings（GetSchedules/CreateSchedule/UpdateSchedule/DeleteSchedule/ToggleSchedule/RunScheduleNow/GetResults）
- **前端面板**：SchedulePanel 组件（列表/新建/编辑/删除/启停/立即执行/执行历史折叠），侧边栏入口
- **系统托盘**：定时任务子菜单（暂停全部/恢复全部），5 秒更新状态标题

### 🐛 修复

- **规划者 ASK 工具**：显式注入 readOnlyReg + planWithTools 运行时重传 Asker，修复 asker=nil 导致 [Never-Ask]
- **complete_step todo 同步**：同轮内 todo_write 立即同步 a.todoState + advanceCanonicalTodo 防御重建，修复代办窗口卡在第一步
- **bash PowerShell 启动命令**：自动检测 start/npm start/wails dev/go run 等启动类命令，包裹为 `cmd /c start` 弹出独立窗口，避免阻塞

## [10.48.0] — 2026-07-07

### 🐛 修复

- **complete_step strictVerify**：verifyStepEvidence/verifyTodoStep 在非严格模式（生产默认）下跳过 host receipt 匹配；execute_one 不再硬覆盖 strictVerify=false

## [10.47.0] — 2026-07-07

### ⚡ 优化

- **Grace Round 跳过 maybeCompact**：轮末不再触发无效压缩
- **技能工具 CompactDescriptor**：6 个工具（run_skill/install_skill/parallel_skills/explore/research/review/security_review）省约 1079 tokens/调用

### 🧹 清理

- 删除 7 个无用 ClaudeKit 技能目录（~6MB）+ 6 个未注册死代码 body 常量（~400 行）+ 重复 review/skills 文件

### 🎨 UI

- 顶栏上下文左右排列、删除 Composer 重复计时条、RunStatus 固定角色名

## [10.46.0] — 2026-07-07

### ⚡ 优化

- **MCP 工具 schema 压缩**：compressSchema 递归 strip description，节省约 600-1000 token/API 调用
- **PlannerMode**：规划者跳过 6 项执行器专属逻辑（turnPrefs/todo/recall/steer/bgCycle/repeat/graceRound），省约 60 token/轮

### 🎨 UI

- 统计面板可折叠详情、思考卡 Brain 图标+读秒、上下文条移到顶栏双行、RunStatus 双模型状态行

---

## [10.45.0] — 2026-07-06

### 🧠 流程优化

- Hermes prompt 新增操作类任务分类：构建/启动/测试/git 等纯操作任务跳过代码研究，输出极简计划
- Hephaestus 强制 ask 弹窗：执行中需用户决策时必须用 ask 工具，纯文本提问会导致重新规划

### 🎨 前端优化（11项）

- SettingsPanel 拆分：1056行→9文件（Shared/Models/Providers/Permissions/Sandbox/Agent/Appearance/Updates）
- useController 全量订阅修复：`store(s=>s)` → `useShallow`，流式输出减少全局重渲染
- StatsPanel 条件渲染：移除 display:none 始终挂载，提取 useStatsPersistence hook
- PlanCard useEffect 补充依赖数组、Transcript as any 类型守卫、store.ts 重复注释清理
- App.tsx 删除空 useEffect 死代码、删除重复注释行
- scrollVersion/currentSessionKey 细粒度优化、splashHold 统一来源、TokenTrendChart 提取独立组件
- 命中率趋势图增加各模型 API 调用次数显示（`12次调用 · 均值 85.3%`）
- **HitRateTrend 完全恢复**：自适应 Y 轴粒度（99.5%→0.1%）、面积填充、X 轴步号标签、SVG H=80
- **StatsTable 合计行补全**：显示 Prompt/Compl/成本/缓存命中率四项
- **StatsPanel 修复**：executorSteps 正向匹配过滤、StepRecord 去重加 source 检查、resetKey 竞态修复
- **TrendChart 组件提取**：通用 SVG 趋势图组件，支持 Y 轴标签/面积填充/X 轴标签

### 🔧 后端优化（7项）

- hermes.go 新增 20 个单元测试（shouldSkipPlanner/isAnswerNotAction/formatHandoff/HandoffTask/persistAnswer）
- LastCacheShape 死代码清理：删除 AgentRunner/Controller 存根 + serve_handlers 不可达分支
- agent.go 死代码删除 + clearSteerQueue 内联 + 重复注释清理
- agent_run.go/boot.go 版本标记残留注释清理（~40 处）
- **isAnswerNotAction 修复**：移除 100 字符阈值短路，改为仅依赖 `<!--plan-->` 标记判断
- **serve/wire.go 同步**：wireUsage 新增 Source/Turn 字段，对齐 desktop/wire.go
- **PlanCard 三路决策**：checkbox 兜底（仅聊天）+ 修改意见重规划 + 正常执行

## [10.41.0] — 2026-07-05

### 📊 统计面板成本重构

- 前端完全改用后端 `costUsd` 汇总成本，删除硬编码 `MODEL_PRICES`
- `store.ts` usage 累加器新增 `costUsd` 累加
- `StepRecord` 加 `cost` 字段，`aggSteps`/`colFromUsage` 改用 `costUsd`
- 修复不同模型单价不同时 TurnRecord 成本计算错误
- 命中率趋势图标题改为 `Hermes` / `Hephaestus` 角色名

### 🐛 Bug 修复

- 修复设置面板思考深度选择后无高亮（`||` → `??`）
- 修复计划确认弹窗无计划内容（`desktop/wire.go` 缺少 `Plan` 字段）
- 修复 TodoPanel 无法正确追踪进度（`step_index` 字段断裂）
- `complete_step` 精简 Schema 补上 `step` 必填要求
- `complete_step` 返回消息不再指示手动调用 `todo_write`

### 🧠 Hermes 执行反馈增强

- HermesPrompt 新增 `[上一轮执行结果]` 消息的识别和信任指令
- `formatExecutionFeedback` 改用明确标记、不截断摘要、区分 Created/Modified
- `TurnResult` 新增 `FilesCreated` 字段区分新建和修改

### 📐 前端

- 顶栏新增上下文用量双色迷你条（紫色 Hermes + 青色 Hephaestus）
- 状态栏上下文条支持分角色显示

## [10.40.0] — 2026-07-05

### 🧠 推理深度分角色控制

- 删除顶栏"快速/标准/深度" temperature 按钮，改为设置面板内按角色控制推理深度
- `agent.effort`（执行者）/ `agent.planner_effort` / `agent.subagent_effort` 分别控制
- 空值继承上级：`planner_effort=""` 则使用 `effort`，`effort=""` 则用 provider 默认
- 设置面板 EffortSelect：关闭(`""`) / 标准(`high`) / 深度(`max`)
- boot.go 在 NewProvider 前为各角色注入对应 effort 值

### 📐 前端改动

- `MemoMarkdown` 改为渐进式 Markdown：稳定段落实时渲染，未完成尾部简单样式
- 顶栏双色上下文横道图：紫色=规划者 青色=执行者，显示各自 Token 占比
- 设置面板 Models 标签三张模型卡片各加推理深度选择器

### 🔧 配置整理

- 保留 `agent.temperature` / `planner_temperature` / `subagent_temperature` 温度控制（独立于 effort）
- `config/render.go` 同步渲染新字段

---

## [10.39.0] — 2026-07-05

### 🐛 双模型 Hermes 修复

- **计划弹窗消失修复**: `planWithTools` 不再提前剥离 `<!--plan-->` 标记，`isAnswerNotAction` 能正确检测可执行计划并弹出确认框
- **子代理工具泄漏修复**: `newReadOnlyRegistry` 恢复对 `explore`/`research`/`review`/`security_review` 的硬编码排除——它们虽 ReadOnly=true 但会启动拥有完整写权限的子代理
- **删除 stripPlanMarker**: `<!--plan-->` 是 HTML 注释，Markdown 中不可见，无需剥离，直接原样传给确认弹窗和执行者
- **phase 标签优化**: 规划阶段显示 `hermes`，执行阶段显示 `Hephaestus`

### 🎨 前端优化

- **渐进式 Markdown 渲染**: `MemoMarkdown` 流式期间稳定段落（`\n\n`）用完整 Markdown 渲染，未完成尾部简单样式，解决长文本输出"全是 markdown 格式"的阅读问题
- **顶栏双色上下文横道图**: 紫色=规划者(Hermes) 青色=执行者(Hephaestus)，显示各自的 Token 占比和数值，悬停查看详情

### 🏷️ 配置修复

- 用户级 `config.toml` 中「规划者」和「子代理」的 `api_key_env` 误填为原始 Key，修正为环境变量名 `DEEPSEEK_API_KEY`

---

## [10.37.0] — 2026-07-04

### 🔧 Runner 返回结构化 TurnResult

- 新增 TurnResult 结构体（FilesModified / Summary / Success / Errors）
- Runner.Run 从 error 改为 (*TurnResult, error)，执行者主动报告结果
- 删除 lastExecutorResult() 硬截断 400 字符逻辑
- formatExecutionFeedback() 替代，含 success/errors 标记

### 🏷️ UsageSource 修复

- 执行者 UsageSource 从 main 改为 executor（死常量复活）
- plannerSink 拦截条件同步更新

### 🧹 CHANGELOG 归档

- 旧条目归档到 _archive/CHANGELOG-2026H1.md，保持仓库整洁

---


## [10.30.0] — 2026-07-04

### 🔍 grep .gitignore 精确行走
- 纯 Go 回退路径新增 `gitignoreWalker` (~260行)：多层 .gitignore 解析 + `**` 递归匹配
- 支持 `.git/info/exclude` + `!` 否定规则（last-match-wins）
- WalkDir 集成：规则栈 enter/leave，对齐 ripgrep 忽略行为

### 🌐 web_fetch HTTP CONNECT + SOCKS5 代理
- `ssrfGuardedTransport`：自动选择 HTTP CONNECT 或 SOCKS5 隧道
- SSRF 保护保持生效：IP 字面量本地检查，域名由代理远程解析（GFW 场景）
- Workspace.ProxySpec 注入：支持 auto/env/custom/off 四种模式

### 📦 桌面端构建
- 前端 TypeScript + Vite 构建通过（1975 modules）
- wails build 生成 `tianxuan-desktop.exe`（17MB, SHA256: `f61c4382...`）

### 🎯 蒸馏计划收尾
- **24/24 特性全部完成**，~3,400 行新增代码
- Reasonix V1.15 全部核心特性已移植到 tianxuan

---

## [10.26.0] — 2026-07-04

### 🧬 Reasonix V1.15 蒸馏完成（22 特性，~3000 行新增）

> 跨四个模块系统性移植 Reasonix V1.15 全部核心特性到 tianxuan

#### 编码管线遗留
- `delete_range` / `delete_symbol` / `editlines`: 编码感知读写，`writeFileEncoded` 保留原编码（GB18030/UTF-16 等）
- `writefile`: 覆盖已有文件时保留原编码，新文件默认 UTF-8
- 已有基础设施：8种编码检测 (`fileutil/encoding/`)、模糊编辑匹配、大括号完整性校验

#### 子代理 transcript 持久化
- 新建 `internal/agent/subagent_store.go` (240行)：`SubagentStore`/`SubagentRun`/`SubagentMeta`
- `task.go` 新增 `continue_from` 参数：子代理跨轮次续跑，输出 `Subagent reference: sa_xxx`

#### 双模型协调器（planner + executor）
- 新建 `internal/agent/coordinator.go` (260行)：`Coordinator` 实现 `Runner` 接口
- `boot.go` 集成：`planner_model` 配置时自动启用，planner 独立会话保证缓存稳定
- `event` 包新增 `UsageSourcePlanner`/`UsageSourceExecutor`
- `agent.go` 新增 `ProvName()` 方法

#### 桌面端双模型适配
- `settings_app.go`: `SetPlannerModel` Go 后端
- `bridge.ts` / `mock.ts`: TypeScript 绑定
- `SettingsPanel.tsx`: ModelsSection + AgentSection 双视图 Planner 模型选择器（ModelSwitcher）

### 🔧 其他改进
- `bg_startkill_test.go`: 后台启停循环检测单元测试
- `docs/specs/2026-07-04-reasonix-distillation-plan.md`: 完整蒸馏实施记录

---

## [10.24.0] — 2026-07-04

### 🏗️ Agent 包架构拆分

> agent 包从单层 44 文件拆分为 1 核心 + 6 子包

- `session/` — Session 结构体 + Save/Load/Branch（16 测试迁移）
- `budget/` — BudgetGate + ModelProfile（6 测试迁移）
- `textutils/` — 工具输出截断/规范化/终端宽度
- `render/` — TextSink ANSI 渲染 + StreamBatcher（6 测试迁移）
- `toolguard/` — 工具参数修复 `RepairDispatchToolArguments`
- `cache/` — 工具目录指纹 + 只读文件缓存（7 测试迁移）
- agent 核心 170/171 测试通过，全项目编译通过

### ⚙️ 设置面板：子代理模型选择增强

- 全局子代理模型：原生 `<select>` → 搜索式 `ModelSwitcher` 下拉
- Per-skill 独立配置：可折叠分组，为 explore/research/review/security-review 分别选择模型
- ModelSwitcher 支持 `allowInherit`/"继承主模型"选项
- 后端新增 `SetSubagentModelForSkill` 配置 API

### 📊 统计面板优化

- 标题栏列宽对齐（标签 `w-[34%]` + 数据 `w-[22%]`×3）
- 所有命中缓存率统一 `.toFixed(2)` (0.01% 精度)
- 会话级/本轮级命中率加大加亮显示（`text-xl font-bold`，模仿"当前步"样式）
- 趋势图标题动态显示实际模型名（替代硬编码"主模型/子代理"）

### 🎛️ 布局调整

- 变更按钮从底栏移至顶栏右侧（GitBranch 图标）
- 底栏上下文进度条升级为弹性宽度横道图（`flex-1` × `8px`，带 `used/window` 数字）

### ⚡ 流式渲染性能修复

- MemoMarkdown 流式预览从 O(n²) 全量重处理改为增量渲染（仅处理新增行）
- 新增 `requestAnimationFrame` 节流，限制每帧一次 DOM 更新
- 修复中文长文本流式输出时"等全部输出完才渲染"的问题

## [10.23.0] — 2026-07-04

### 🎨 体验优化迭代

> 基于 V10.10.0 · 流式输出流畅度 + 终端降噪 + 记忆面板重设计 + CMD 窗口修复

#### 流式输出流畅度
- stream_batcher: maxBytes 64→8, maxDelay 16ms→4ms（消除文字爆发感）
- Transcript: 流式时 scrollTop 直接跟随（替代 GSAP tween 重启抖动）
- shiny-text: background-clip:text 渐变→border-left 脉冲（降低 GPU 开销）

#### 终端输出降噪
- textsink: 推理 500ms 节流 + `\r` 进度指示器（替代 2000+ 字刷屏）
- textsink: ≥3 工具合并 `▸ N tools running...` 一行摘要
- textsink: ≥2 错误合并 `⊘ N tools failed: ...` 聚合显示

#### 记忆面板重设计
- MemoryPanel: 卡片式布局 + 全中文 i18n（14 新翻译键）
- SuggestionCard: 提取独立组件, badge 胶囊样式, evidence 引用线
- 搜索框仅在有事实时显示, 空结果 + 清空筛选按钮

#### CMD 窗口闪现修复
- hideBashWindow: +CREATE_NO_WINDOW 标志（比 HideWindow 更彻底）
- git.go/readfile.go/hook.go/notify.go/plugin: 补全 HideWindow 调用
- hide_window_windows.go: 统一 proc.HideWindow 导出

#### 其他
- ToolGroup: CSS Grid→GSAP 动画（修复 Chrome 闪烁）
- StreamingIndicator: return null→invisible 固定占位（防布局跳动）
- ThemeSwitcher: 5→9 主题 + forest/midnight/neon/mono
- 回到底部按钮: absolute→fixed + backdrop-blur 毛玻璃
- 推理→正文: msg-fade-in 0.25s 过渡动画

### 🚀 DSpark 吸收 + 流式输出全栈重构（V10.11.0 上轮）

> 基于 V10.10.0 · 25文件 +550/-140 · 核心: 推测解码思想吸收 + 输出管线性能优化

#### DSpark 吸收（借鉴 DeepSeek DeepSpec 推测解码架构）
| 新增 | 功能 | 映射 |
|------|------|------|
| tool_precheck.go | 确定性预检查 | Confidence Head |
| tool_coherence.go | 批次一致性后验证 | Block Verify |
| session_route_features.go | 会话特征路由 | extract_context_feature |

#### 流式输出全栈优化
| 层 | 优化 | 效果 |
|----|------|------|
| SSE | 字符串扫描快速路径 | 90% 跳过 json.Unmarshal |
| Go 流 | streamBatcher 批量合并 | 800→40 事件/响应 |
| Go 渲染 | writeDim 零分配 + Write | 消除 ANSI 字符串分配 |
| TS 状态 | items.map()→直接索引 | O(n)→O(1) |
| TS 渲染 | 动态窗口 + Markdown 粗糙缓存 | 平滑过渡 |
| CSS | GPU 合成层隔离 | 避免布局重算 |

#### 工具增强
- compact.go: memory_search/read_skill 统一映射，grep/bash/complete_step 描述优化
- completestep.go: 拒绝纯 manual 证据
- task.go: 新增 CompactDescriptor，突出 output_schema

#### 代码清理
- checkpoint.go: joinStr→strings.Join
- flow.go: toLower→strings.ToLower
- provider_adapter.go: 自实现→标准库

#### 构建产物
- release/v10.11.0/tianxuan.exe (16MB CLI)
- release/v10.11.0/tianxuan-desktop.exe (16MB Wails)

---

