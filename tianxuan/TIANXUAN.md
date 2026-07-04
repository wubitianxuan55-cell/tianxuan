# tianxuan project memory

> V10.23.0 — 测试修复 + boot 拆分 + 前端测试 + 缓存安全工具 · 2026-07-04

## V10.23.0 (2026-07-04)

### P0: 修复 6 个已知失败测试
- verify gate 在无 todo 的 session 中误触发 → `DisableVerify` 选项
- `stop_gate.go`: taskGate 检查 disableVerify 跳过 verify nudge
- `task.go`: RunSubAgent 自动设 DisableVerify=true
- `toolcache_test.go`: Windows mtime 精度修复

### P1: 拆分 boot.go
- `sysprompt.go` (85行): 系统提示词/记忆/技能/Profile 组装
- `plugins.go` (93行): CodeGraph/Context7/MCP/LSP 启动

### P3: 前端测试
- `lib/stats.ts`: 纯函数模块 (13 functions)
- 32 Vitest tests (27 stats + 5 store)
- StatusBar 消除重复代码

### P4: 构建产物管理
- `Makefile`: webui / clean-webui 目标

### P5: 缓存安全工具
- `cmd/cacheguard`: 三规则静态分析器 (零外部依赖)
- `make lint-cache`

### ⏳ 未完成: agent 包拆分 (P2, 跳过)

## V10.22.0 (2026-07-04)

### 自动路由删除
- 删除 `auto_router.go` / `auto_router_history.go` / `session_route_features.go` (共 401 行)
- 删除 AgentRunner 中 `flashProv`/`routeHistory`/`activeProv` 等字段和 `AutoRoute`/`RouterKeywords` Options

### 子代理模型自由选择
- `internal/config/edit.go`: 新增 `SetSubagentModel()` 持久化方法
- `internal/agent/task.go`: TaskTool 新增 `subagentProv` + `SetSubagentProvider()`
- `internal/boot/boot.go`: 从 `cfg.Agent.SubagentModel` 解析并注入
- 桌面设置面板 Agent 页增加子代理模型下拉框

### 统计面板主模型/子代理分开统计
- `internal/event/event.go`: Event 新增 `UsageSource` 字段
- `internal/agent/agent_run.go`: 主模型 usage 标记 `"main"`
- `internal/agent/task.go`: subSinkFor 覆写为 `"subagent"`
- `desktop/wire.go`: wireUsage 新增 `source` JSON 字段
- `StatsPanel.tsx`: 三栏表格（主模型/子代理/汇总）+ 分源命中率趋势图

### 底栏模型显示
- `desktop/app.go`: App 增加 `subagentLabel` 字段
- `desktop/app_meta.go`: Meta 新增 `SubagentLabel`
- `StatusBar.tsx`: 连接灯右侧显示主模型 + 子代理 chip

### 权限修复
- `desktop/settings_app.go`: rebuild() 跨重建携带 permLevel
- `desktop/app_meta.go`: SetModel() 跨重建携带 permLevel
- `internal/agent/agent.go`: 删除死字段 permLevel + SetPermLevel
- `internal/control/controller.go`: 删除对 executor.SetPermLevel 的死调用

## V10.11.0 (2026-06-28)

### DSpark 吸收
🧠 tool_precheck.go: 确定性预检查（edit_file old_string验证、delete_range anchor验证）
🧠 tool_coherence.go: 批次一致性后验证（读失败→写失败因果链检测）
🧠 session_route_features.go: 会话状态特征路由（TurnCount/RecentErrors/PendingTodos→强制pro）

### 流式输出全栈重构
📡 SSE快速路径: 90%行字符串扫描替代json.Unmarshal
📡 Go流批量化: streamBatcher 16ms/64B合并，800→40事件/响应
📡 Go渲染零分配: writeDim三步Write替代dimText字符串拼接
📡 TS状态O(1): items.map()→直接索引，~1000x迭代减少
📡 TS渲染平滑: 动态窗口+Markdown粗糙缓存，消除纯文本→格式化跳变
📡 CSS GPU合成: translateZ(0)隔离流式文本布局重算
📡 StatusBar memo: 流式更新时跳过状态栏重渲染

### 工具增强
🔧 compact.go: memory_search/read_skill中心映射，grep/bash/complete_step描述优化
🔧 completestep.go: 拒绝纯manual证据（编码铁律）
🔧 task.go: 新增CompactDescriptor，Description突出output_schema

### 代码清理
🧹 checkpoint.go: joinStr→strings.Join
🧹 flow.go: toLower→strings.ToLower
🧹 provider_adapter.go: 自实现→标准库

### 验证
✅ go build ✅ go vet ✅ go test ✅ npx tsc --noEmit ✅ wails build 11.7s

## V8.23.0 (2026-06-22)

### 🔙 V9.1/V9.2 精确摘除

| 删除 | 说明 |
|------|------|
| compress_json.go | JSON结构掩码压缩 |
| ccr/ccr.go | CCR可逆压缩 |
| retrieve.go | retrieve工具 |
| anysearch.go | AnySearch API |
| compress/prune/boot/webfetch/websearch | 调用点回退 |
| compact_summary.go | messageImportance/keepThreshold死代码 |

### ✅ 保留 (V8.22 + V8.23)

- V8.23 agentMode三模式切换 (explore/develop/orchestrate + YOLO)
- V8.22 技能强制触发 / L1 Token精简 / 面板全面优化
- V8.22.1 热修复 (fakeTool / RLock guard / 429重试 / isServerError)

### 构建产物
- release/v8.23.0/tianxuan.exe (16MB)
- release/v8.23.0/tianxuan-desktop.exe (16MB)

## V9.1.0 (2026-06-22)

### 压缩优化（借鉴 Headroom）
📦 JSON结构掩码压缩: tokenize→mask→span, 键名保留/长值压缩（借鉴 json_handler.py）
📦 重要性评分折叠: 错误+0.5/硬约束+0.4/编辑+0.2, keepThreshold=0.35
📦 结构化摘要引导: BuildCompactSummary 注入 LLM 摘要 prompt
📦 CCR可逆压缩: .tianxuan/ccr/ + retrieve 工具, hash取回原文
📦 缓存零破坏: L1/L2 hash 所有轮次一致

### V9.0: 模式统一
🎛️ Go: app.go SetAgentMode/AgentMode · TS: bridge/types/mock/store
🎛️ UI: Composer三模式按钮 · StatusBar agentMode badge · YOLO toggle
🎛️ i18n: zh/en/zh-TW 新增模式标签键

### 验证
✅ go build ✅ go vet ✅ go test ✅ 缓存e2e

## V8.23.0 (2026-06-22)

### 桌面端 agentMode 自由切换
🎛️ Go 桥接: app.go 新增 SetAgentMode/AgentMode + Meta.AgentMode — 12 文件, +100/-9
🎛️ TS 接口: bridge.ts + types.ts + mock.ts 完整同步
🎛️ 状态管理: useModeManager + store 新增 agentMode 状态 + setAgentMode
🎛️ UI: Composer 新增探索/开发/编排按钮 · StatusBar 底栏 agentMode badge
🎛️ i18n: zh/en/zh-TW 新增 4 个键

### 验证
✅ go build ✅ go vet ✅ npx tsc --noEmit ✅ wails build (10.8s)

## V8.22.1 (2026-06-21)

### Bug 修复 (热修复)
🔴 agent 测试套件: guards_test.go 删除时带走 fakeTool → 10个测试文件编译失败 → 补回 helpers_test.go
🔴 WorkspaceChanges + SubmitDisplay: 缺 RLock/nil guard → controller 未就绪时 nil panic
🟡 sendWithRetry: rateLimitCount 死代码 → 429 只重试3次(应为5次) → 外循环上限修正
🟡 isServerError: 字符串降级遗漏 501/505+ → 补全
🔵 mode_classifier: "run" 不在 actionVerbs + 阈值/权重偏保守 → 扩充+调整

### 验证
✅ go build ✅ go vet ✅ go test ./internal/agent/... (46s) ✅ go test ./internal/provider/... ✅ 缓存 e2e

## V8.22.0 (2026-06-21)

### 技能触发策略强化 (Reasonix push-based 五层机制)
🧠 indexHeader: "you can invoke" → "you MUST consult before acting" + 中文引导句
🧠 8 个项目技能 description: 功能清单 → 触发条件+简短提示（3,820→741 chars, -80%）
🧠 7 个子代理工具 description 精简（tools schema 2,052→1,380 chars, 每次API调用-168 tokens）
🧠 run_skill 加入默认 Allow 列表——消除技能调用审批摩擦
🧠 AGENTS.md: "记得使用skill" → 6 条具体技能优先规则 · 技能架构章节精简 -30%

### L1 Token 优化
📐 AGENTS.md 技能架构去重: 完整35技能清单 → 4行摘要（-1,974 chars）
📐 TIANXUAN.md: 删除 V8.16-V8.18 版本历史 + 缓存规则去重
📐 典型会话（10轮）Token 节省: ~2,758（技能索引770 + 工具schema 1,680 + 记忆文件308）

### 面板全面优化
🎨 技能面板: 范围分组+折叠+搜索+描述+🧬子代理标签
🎨 工具面板: 9组分类+折叠+搜索+紧凑描述（文件/命令/版本/网络/任务/子代理/技能/记忆/系统）
🎨 计划面板: Phase卡片+子步骤层级+状态图标(○/⟳/✓)+渐变进度条
🎨 记忆面板: 恢复居中弹窗+全中文显示+displayTitle修复
📊 Token趋势图: 单Y轴→双Y轴（输入K单位/输出原始tokens）
📊 缓存详情: 下拉popover→底栏平铺

### Bug 修复
🐛 文件变更面板: app.go 新增 WorkspaceChanges() 方法（之前缺失导致面板空白）
🐛 CommandPalette: compact grid 键盘导航高亮修复
🐛 useGSAPCollapse: opts ref化消除过期闭包
🐛 isRateLimit/isServerError: 字符串匹配→结构化错误类型 httpStatusError

### 发布产物
📦 tianxuan.exe (16.6MB, CGO_ENABLED=0)
📦 tianxuan-desktop.exe (16.2MB, Wails v2.12.0)

## V8.21.0 (2026-06-21)

### 设计系统落地
🎨 CSS 配方类: btn-primary/btn-secondary/card/badge + 5 语义变体
🎨 32 组件迁移硬编码 → --ds-* 令牌（0 硬编码色值/阴影残留）
🎨 系统配色统一: 渐变 token 化 + 抽屉背景对齐 + 边框/遮罩统一

### UI 优化
🔧 DrawerHeader 共享组件: 统一 5 面板 header
🔧 动画类化: anim-drawer-in/out + anim-menu-in + 30+ duration → var(--dur-*)
🔧 流式文本可见性修复: -webkit-text-fill-color 覆盖
🔧 输入框圆角 28px → 16px · 思考卡/工具卡默认折叠
📁 新增: DrawerHeader.tsx · CommandPalette.tsx · Tooltip.tsx · gsapAnimations.ts
📁 51文件 +1962/-732 · Go 核心零变更


## Project

tianxuan 是一个面向 DeepSeek V4 的极简 Coding Agent。单 Go 二进制，零外部依赖。
核心目标：极低成本、极快响应。

## Architecture

```
用户输入 → Controller → ContextManager(L1+L2+L4) → AgentRunner.runDirect()
                                                          │
                                              DeepSeek V4 API (1次调用)
                                                          │
                                              工具执行 (流式预执行 + 文件缓存)
                                                          │
                                              LLM摘要digest累积 (compact)
```

### 四域前缀 (TCCA)
- **L1 Identity** (~300 tok): 身份 + 规则，SHA-256 不可变校验
- **L2 Runtime** (~100 tok): 项目/语言/入口，首轮锁定
- **L3 Skill** (~1,200 tok): 工具紧凑描述，prefix cache 完全命中
- **L4 Flow**: 对话历史，LLM 摘要 digest 累积

## 命令

```
# 构建 CLI（发布版）
set GOOS=windows&& set GOARCH=amd64&& go build -ldflags="-s -w" -o release/vX.Y.Z/tianxuan.exe ./cmd/tianxuan/

# 构建桌面端
cd desktop && wails build
cp build/bin/tianxuan-desktop.exe ../release/vX.Y.Z/

# Go 测试 / vet
go test ./internal/... -short
go vet ./internal/...

# 前端
cd desktop/frontend && pnpm dev
cd desktop/frontend && npx tsc --noEmit
```

## 关键模块

- `internal/agent/agent.go` — AgentRunner 主循环 + 6层防御
- `internal/agent/compact.go` — LLM 摘要 digest 累积压缩
- `internal/agent/prune.go` — compact 前免费清理
- `internal/agent/cache_shape.go` — PrefixShape + CompareShape 诊断
- `internal/agent/cache_guard.go` — verifyPrefixAndShape Notice
- `internal/boot/boot.go` — Build() 装配工厂
- `internal/cache/` — 四域管理 (Identity/Runtime/Skill/Spawn)
- `internal/context/` — TCCA 内核 (ContextManager)
- `internal/control/` — Controller 会话驱动
- `internal/tool/` — Tool 接口 + CompactDescriptor
- `internal/plugin/` — MCP 客户端 (stdio + Streamable HTTP)
- `internal/provider/openai/` — DeepSeek provider (thinking 注入)
- `internal/lsp/` — LSP 集成
- `desktop/frontend/src/hooks/` — 桌面端 hooks

## 工具一览

| 工具 | 用途 |
|------|------|
| `bash` / `bash_output` / `kill_shell` / `wait` | Shell 执行 + 后台任务管理 |
| `read_file` / `write_file` / `edit_file` / `multi_edit` | 文件读写与编辑 |
| `delete_range` / `delete_symbol` | 删除操作（行锚点 / AST） |
| `glob` / `grep` / `ls` | 文件搜索 |
| `web_fetch` / `web_search` | 网络工具 |
| `git_status` / `git_diff` / `git_commit` / `git_log` | 原生 Git |
| `lsp_definition` / `lsp_references` / `lsp_hover` / `lsp_diagnostics` | LSP 查询 |
| `lsp_completion` / `lsp_rename` | LSP 扩展 |
| `doctor` / `time` | 系统工具 |
| `todo_write` / `complete_step` | 任务跟踪 |
| `notebook_edit` / `memory_search` | Jupyter / 记忆搜索 |

## 约定

- Go kernel under internal/; each package owns one concern
- Transport-agnostic Controller behind every frontend
- Config: tianxuan.toml, secrets in .env
- 桌面端: Wails v2, React 18, Vite 6, Zustand 5
