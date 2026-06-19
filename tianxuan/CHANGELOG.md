## [8.2.1] — 2026-06-19

### 🎨 桌面端 UI — Tailwind CSS 全量迁移

24 个前端组件从手写 BEM CSS 迁移至 Tailwind 工具类，styles.css 缩减 28.3%。

| 模块 | 文件 | 说明 |
|------|------|------|
| 🏗️ 地基 | `tailwind.css` | `@theme` 令牌全部通过 `var()` 引用主题变量，自动跟随 dark/light/warm/ice 四套主题；新增 30+ 令牌 + 5 组 @keyframes |
| 🧩 小组件 | Toast / ErrorCard / JumpBar / Skeleton | `animate-[toast-in]` / `border-err text-err` / `hover:scale-[1.6]` |
| 🏠 布局壳 | Topbar / Sidebar / Drawer / ResizableDrawer | 伪元素→真实 DOM，`group-hover:bg-accent`，`animate-[drawer-in]` |
| ⌨️ 输入系统 | Composer(全) / SlashMenu / FileMenu / ArgMenu | 输入/按钮/模式/附件/斜杠菜单/workspace-switcher |
| 💬 消息系统 | Message / ToolCard / ToolGroup / StatusBar | 状态变体 border 色微分，思考折叠 `rotate-90` |
| 📁 文件面板 | WorkspacePanel (全) | tabs/crumbs/tree/search/empty 全部 Tailwind |
| 🗄️ 抽屉面板 | MemoryPanel / HistoryPanel (全) / SettingsPanel (表单90%) / CapabilitiesPanel (核心+ServerRow+SkillRow) | 搜索/过滤/事实卡片/文档编辑/服务器列表/技能卡片 |
| 🔧 其他 | ApprovalModal / TodoPanel | icon 修复 |

**数据**：`styles.css` 5263→3773 行 (-1490, -28.3%)，CSS bundle 151.41→126.71 KB (-24.70 KB, -16.3%)，gzip 30.58→26.84 KB (-3.74 KB, -12.2%)。

### 🔧 Go 核心 — 6 修复

| 修复 | 文件 | 说明 |
|------|------|------|
| `wait` 工具 schema 修正 | `compact.go` | `job_id`(string) → `job_ids`(array) + `timeout_seconds`，修复批量等待 |
| Windows Job Object | `bash.go` | 后台任务进程树可靠清理，fallback `taskkill` |
| MCP HTTP 超时 | `ssrf.go` | 全请求 60s 超时，防止 MCP 服务器无响应永久阻塞 |
| stdio readLoop 优雅退出 | `transport_stdio.go` | session context 检查，防止挂起服务器阻塞 goroutine |
| glob/grep context 取消 | `glob.go` / `grep.go` | 大目录遍历周期性检查 `ctx.Done()` |
| doctor go version 超时 | `doctor.go` | `exec.CommandContext` 10s 超时 |

### 发布

- CLI: `tianxuan.exe`
- 桌面端: `tianxuan-desktop.exe` (Wails v2.12.0)

---

## [8.2.0] — 2026-06-18

### Go 核心优化

| 优化 | 说明 |
|------|------|
| 大文件拆分 | `agent.go` 2085→1260行 (-40%), `controller.go` 1472→911行 (-38%) |
| 空方法修复 | 删除 `SetTaskKind`，文档化 `CompactNow`/`SummarizeFrom`/`SummarizeUpTo` |
| 测试补充 | `learning` 包 13 用例 + `archive` 包 9 用例 |
| 重复逻辑消除 | `executeOne` 内联 fallback 精简，dispatcher 路径统一 |
| 并发安全文档化 | `gate`/`hooks`/`asker`/`onPreEdit` 加 happens-before 注释 |

### 🔴 缓存保护红线

| 规则 | 说明 |
|------|------|
| L1 Identity 字节不变 | `verifyPrefix` SHA-256 守卫 |
| tools 列表 session 不变 | V8.0.2 filteredSchemas 教训 |
| L2 Runtime 首轮锁定 | 缓存 key 含 L2 |
| 禁止动态系统提示词注入 | 破坏 L1→user 前缀结构 |
| 工具描述不可热更新 | CompactDescriptor 是缓存前缀一部分 |

### HTTP/SSE 前端重写

- `index.html` 拆为 3 文件 (HTML/CSS/JS)，SSE 指数退避重连
- thinking 折叠块 + tool 耗时显示 + `/health` 端点

### TUI 增强

- 状态栏：实时 ¥ 成本 + cache 命中率颜色编码 (绿/黄/红)
- md.go: ANSI 转义 → 统一 `style.go` 常量体系

### 桌面端 UI

| 特性 | 说明 |
|------|------|
| Logo 重设计 | 4 变体，暖铜色渐变统一品牌色 |
| Tailwind CSS | 主题 tokens + 3 组件迁移 + styles.css -10% |
| JumpBar | 右侧圆点轮次导航 + hover 预览 + 点击跳转 |
| 状态栏增强 | 连接状态灯 + 上下文用量四段色条 |
| Composer 模式按钮 | 3 按钮并排 (auto/plan/yolo) |
| ErrorCard + Toast | turn_done 错误可视化 + 全局通知系统 |

### 品牌清理

`reasonix → tianxuan`: 26 文件全局替换

### 发布

- CLI: `tianxuan.exe`
- 桌面端: `tianxuan-desktop.exe` (Wails v2.12.0)

---

## [8.0.6] — 2026-06-18

### V8.0 系列稳定版 — 8 新特性 + 6 补丁 + 全量审计

| 特性 | 说明 |
|------|------|
| 确定性结果剪枝 | 相同工具+参数+结果不重复发 token |
| Mid-turn Steer | 检测错误螺旋，注入纠偏提示（blocked≠failed） |
| Plan 智能澄清 | 模糊输入主动追问 |
| read_skill 工具 | Agent 按需读取技能 body |
| Plan bash 安全白名单 | 20+ 安全命令 + 元字符/重定向/危险参数检测 |
| Context7 MCP | `CONTEXT7_API_KEY` 自动启用 |
| /goal 命令 | 大目标分解为子任务 |
| PermissionRequest Hook | 自定义审批策略 + 参数修改 |

### 补丁链 (V8.0.1–V8.0.6)

- V8.0.1: 死代码 + 状态泄漏 + 接口遗漏 (4 Bug)
- V8.0.2: 🔴 致命 — filteredSchemas 破坏缓存前缀不变性
- V8.0.3: Plan bash 安全白名单
- V8.0.4: `2>&1` 误拦 + `go test` + find/go 危险参数
- V8.0.5: steer blocked≠failed 误判修复
- V8.0.6: `2>>` 误拦 + steer 全面测试

### 发布

- CLI: `tianxuan.exe` (13.4 MB)
- 桌面端: `tianxuan-desktop.exe` (16.7 MB, Wails v2.12.0)

---

## [7.7.1] — 2026-06-18

### 架构优化

- **双重 JSON Repair 消除** (`agent.go`): 提取 `repairArguments` 辅助函数，executeBatch 开头统一 repair，executeOne 不再重复 — 每轮节省 N×2 次 JSON marshal/unmarshal
- **toolcache O(1) 路径失效** (`toolcache.go`): `invalidatePath` 从线性扫描改为 `pathKeys map[string]map[string]struct{}` 路径索引，消除写入密集型工作流 O(n²) 退化

### Bug 修复

| Bug | 级别 | 文件 | 修复 |
|-----|:----:|------|------|
| serve_test nil deref | 🔴 | `serve_test.go` | `http.Post` 失败时 resp2 为 nil，加 err2 检查 |
| serve_test 端口耗尽 | 🔴 | `csrf_test.go` | `http.DefaultClient` → `srv.Client()` |
| debug skill 未注册 | 🟡 | `builtins.go` | body 已定义但未注册到 `builtinSkills()` |

### 测试

- `skill_test.go`: 新增 `TestBuiltinDebugIsInlineSkill`

### 发布

- CLI: `tianxuan.exe` (13.3 MB)
- 桌面端: `tianxuan-desktop.exe` (16.7 MB, Wails v2.12.0)

---

## [7.7.0] — 2026-06-17

### 内置技能全面升级

从零散的 Agent 技能集 → 8 个深度整合 tianxuan 独特能力的技能。

| 技能 | 变化 | 关键提升 |
|------|:--:|----------|
| **`explore`** | 升级 | 融合 CodeGraph 工具选择表 + 7 个 codegraph 工具 + 3 个 LSP 查询工具，Agent 不再盲目 grep |
| **`research`** | 升级 | 同 explore + web_search/web_fetch |
| **`review`** | 升级 | 用 `git_status/git_diff/git_log` 替换 `bash git`，加入 `codegraph_impact` 影响分析 + `lsp_diagnostics` 编译检查 |
| **`security-review`** | 升级 | 同 review + `codegraph_trace` 追踪输入路径 |
| **`tdd`** | 重写 | 吸收 debug 的隔离阶段：RED（隔离+写失败测试）→ GREEN（最小修复+影响分析）→ REFACTOR（回归测试+清理） |
| **`lsp`** | 新增 | 诊断→理解→修复→验证完整工作流 |
| **`debug`** | 新增 | 4 阶段系统化调试：Reproduce → Isolate (lsp_diagnostics/git_diff/codegraph_trace) → Fix (含影响分析) → Prevent (单元测试+回归) |
| ~~`karpathy-guidelines`~~ | 移除 | 内容已在系统 prompt，冗余 |
| ~~`test`~~ | 移除 | 升级为 `tdd` |

### 发布

- CLI: `tianxuan.exe` (13 MB)
- 桌面端: `tianxuan-desktop.exe` (16 MB, Wails v2.12.0)

---

## [7.6.0] — 2026-06-17

### 代码清理与空间瘦身

- **删除 ~452MB 构建产物**: `bin/`, `dist/`, `build/`, `release/` 历史版本
- **删除死代码 `internal/inspect/`**: 零引用，任何非测试代码均未导入
- **删除无关脚本**: `dashboard_healthcheck.py`（属于外部项目 hermes）、`cache-bench.ps1`（引用不存在的标签）
- **benchmark 脚本修复**: `cache-bench-tools/*.go` 添加 `//go:build ignore`，消除 `go build ./...` 报错
- **.gitignore 增强**: 新增 `/build/` 和 `/release/` 忽略规则

### 发布

- CLI: `tianxuan.exe` (13 MB)
- 桌面端: `tianxuan-desktop.exe` (16 MB, Wails v2.12.0)

---

## [7.5.0] — 2026-06-14

### 缓存架构收敛（前缀稳定性优化）

- **L2 合并到 L1**: `MergeRuntimePrompt` 将运行时上下文拼入系统提示词末尾，移除每次 stream() 时的 L2 注入 → 前缀永不改变
- **删除 WarmupCache**: 预热方案（`[L1,L2,"ping"]`）预热了错误的前缀，且 V7.5 前缀天然稳定后不再需要
- **删除 preTurnCompact**: 轮间压缩逻辑已整合回 `maybeCompact`
- **AutoRoute 粘性路由**: 会话首次路由后锁定模型选择，避免 flash↔pro 切换导致缓存清空
- **子代理回温删除**: 子代理 API 调用不再需要额外预热，前缀稳定后自然增长

### 清理

- `stream()` 流程精简：msgs 直接从 session 读取直通 API，中间零修改
- 移除 `verifyPrefix`、`cacheBreakDetector`、`DetectToolCatalogDrift` 等监控代码（纯诊断，不改变行为）

### 发布

- 桌面端: `desktop/build/bin/tianxuan-desktop.exe` (16 MB, Wails v2.12.0)

---

## [7.3.0] — 2026-06-14

### 桌面端修复

- **统计面板布局恢复**: 按 V7.2 规格重排 StatsPanel，顺序：上下文→会话→本轮→当前步→命中率趋势→Token累计图→工具统计
- **Token 累计图修复**: 数据源从按步改为按轮累计，正确反映轮次间 token 增长
- **App.tsx TCCA 残留清除**: 移除已删除 TCCA 面板的 `tcca={state.tcca}` prop 传递，修复 tsc 编译错误导致 wails build 使用旧 dist 的问题

### 核心清理

- **删除「前缀挥发性检测」模块** (`prefix_volatility.go`): 只检测不修复，纯诊断噪音，零生产价值（V5.11 引入）
- **删除「压力冲刷」模块** (`pressure_flush.go`): 70% 压力注入存储提示，与 V7.1 轮内压缩策略矛盾（V6.0 引入）
- **删除「检查点重建」模块** (`rebuild.go`): `buildCheckpointRebuild` 整函数已成死代码（V7.0 引入）

### DSR 收敛

- **交换压缩 fallback 优先级**: Legacy Truncate 优先（保留 `[L1+prefix+summary+tail]` 结构，缓存只断一次），Budgeted Rebuild 降为后备
- **compactStuck 降级路径**: 不再静默返回，改为 force 模式纯截断，不注入 checkpoint/memory/tasks
- **删除 compaction digest marker**: 在摘要中写 SHA256 标记的唯一作用是证明"确实破坏了缓存"——移除

### 发布

- CLI: `bin/tianxuan.exe` (12.5 MB)
- 桌面端: `desktop/build/bin/tianxuan-desktop.exe` (15.7 MB, Wails v2.12.0)

---

## [7.2.0] — 2026-06-13

### Bug 修复

| Bug | 级别 | 文件 | 修复 |
|-----|:----:|------|------|
| compactStuck 计数器逻辑反转 | 🔴 | `compact.go` | `consecutiveCompacts++` 从成功路径移到失败回退路径 |
| allocations 排序索引错位 | 🔴 | `compact.go` | allocations 计算移到 sort 之后 |
| msgs[1] 缺角色检查 | 🟡 | `compact.go` | 追加 msgs[1] 前检查 `Role == provider.RoleSystem` |
| preTurnCompact 首次 turn 空调用 | 🟢 | `agent.go` | TruncateCount==0 && LastPrompt==0 时跳过 |
| checkpoint rebuild 未重置 stuck | 🟢 | `rebuild.go` | 成功后添加 consecutiveCompacts=0; compactStuck=false |
| legacyTruncate force mode 遗漏 | 🟢 | `compact.go` | switch 添加 `case "force"` 设置 prefixCount=2 |

### 发布

- CLI: `bin/tianxuan.exe` (13.1 MB)
- 桌面端: `desktop/build/bin/tianxuan-desktop.exe` (16.5 MB, Wails v2.12.0)

---

## [7.1.0] — 2026-06-12

### 缓存命中波动修复（核心）

修复多步单轮对话中，中间步的缓存命中率出现上下波动（锯齿波）的问题。

#### 根因分析

`maybeCompact` 在 `runDirect` 步循环内每步都可能触发。压缩替换消息列表后，
DeepSeek 前缀缓存与新消息结构不匹配 → 缓存断裂 → 命中率骤降。
形成"增长→压缩→骤降→再增长"的锯齿波图案。

Budgeted Rebuild（V6.0 P5 主策略）几乎不保留原始消息结构，导致恢复后的缓存
命中率从 95%+ 降至接近 0%。

#### 修复方案

- **移除中轮压缩**: 从 `runDirect` 步循环末尾删除 `maybeCompact(ctx, usage)` 调用
- **新增 `preTurnCompact`**: 在每轮首次 `stream()` 前压缩上轮历史，不破坏当前轮内缓存单调增长
- **压缩后自动预热**: `preTurnCompact` 后调用 `WarmupCache` 让 DeepSeek 预缓存新前缀

**原理**: 轮内缓存从 [L1, L2, U1] → [L1, L2, U1, A1, T1] → ... 单调增长，
每步命中前一步的全部缓存。仅在轮次之间（用户新消息前）压缩并预热。

#### 涉及文件

```
修改: internal/agent/agent.go  +40 -3 行 (preTurnCompact + runDirect重构)
```

#### 构建

- CLI: `bin/tianxuan.exe`
- 桌面端: `desktop/build/bin/tianxuan-desktop.exe` (16.5 MB, Wails v2.12.0)

## [6.1.0] — 2026-06-12

### 运行时增强

- **并行工具默认化**: 工具调用按冲突键分组成并行批次。读工具连续并行，写不同文件的编辑工具也可并行。batch 提示注入系统消息。
- **子代理轻量化**: 新增 `TemplatePrefix` + `ActiveSchemas` 支持，子代理继承父代理前缀缓存。via `cache/spawn.go` + `agent/task.go`
- **3 Agent 模式**: `/mode explore/develop/orchestrate` 三模式。explore 跳过计划审批 + stopGate；orchestrate 专属审批消息 + verify 闸门。
- **缓存友好压缩**: `output_continue.go`（输出截断重试）、`repeat_detect.go`（重复步骤检测）、`pressure_flush.go`（70% 压力冲刷）、`stop_gate.go`（停止闸门 + goal 闸门 + Judge 模型）

### 智能进化

- **Budgeted 上下文注入**: `maybeCompact` 重写为预算制重组。4 组件按重要性权重分配：checkpoint(0.9) 30% / memory(0.7) 15% / tasks(0.5) 10% / recent(1.0) 45%。回退 legacy 截断。
- **Dream & Distill**: `/dream [scan|extract]` 跨会话知识提取。`/distill [scan|create]` 模式检测 + 自动生成 `.tianxuan/skills/` 模板。
- **Judge 模型独立验证**: 独立 flash API 调用评估 goal 条件。返回结构化 verdict `{ok, impossible, reason}`。消除乐观停止。
- **工具合并**: Registry 新增 `Hide/HideUnlessOnly`，`[tools] compact = true` 隐藏 11 个冗余工具。隐藏工具仍可调用，向后兼容。
- **Checkpoint-Writer**: 预算重建后自动 `QueueMemory` checkpoint 摘要。
- **树形任务**: nudge 注入 T1/T1.1 格式提示（不修改工具 schema，保护 L3 缓存）。
- **会话归档**: JSONL archive（`.tianxuan/archive/`）自动记录 assistant 消息，支持跨会话搜索和工具统计。

### 死代码清理

移除 ~800 行遗留代码：GoalRouter、FirstTurnHandler、Planner（已被 V3.0 ContextManager 替代）。

### 新增命令

- `/mode explore|develop|orchestrate` (或 e/d/o)
- `/goal <description>` — Judge 模型验证
- `/dream [scan|extract]`
- `/distill [scan|create]`
- `/memories [query]`

### 配置新增

- `[tools] compact = true` — 启用精简工具集

### 缓存安全

全部改动严格遵守 L1/L2/L3 前缀缓存不变原则：
- Judge → 独立 API 调用
- Archive → 独立目录
- Nudge/提示 → 仅用户消息层面
- Schema → 完全不变

## [5.30.0] — 2026-06-11

### 缓存架构优化（核心）

- **SpawnPolicy L4 模板化** (cache/spawn.go, agent/task.go, boot/boot.go): 新增子代理模板注册表。同类子代理（explore/review/research/security）共享固定 L4 模板前缀作为独立 System 消息，DeepSeek 服务端缓存覆盖模板段。实测同类子代理第三调用缓存命中率 2%→99.2%。

- **L2 紧凑格式** (cache/runtime.go, boot/boot.go): 新增 @p/@w/@g 前缀 KV 行格式替代 Markdown 列表，L2 从 ~100 tok 降至 ~40 tok（-60%）。

- **异步预热** (control/controller.go): WarmupCache 改为后台协程，首轮不阻塞 1-5s。

### 网络优化

- **HTTP 连接池** (provider/openai/openai.go): 按 baseURL 共享连接池（MaxIdleConns=100）。
- **智能重试** (provider/openai/openai.go): 429 指数退避至 60s，5xx 快速重试。

### 内存优化

- **记忆体紧凑模式** (memory/memory.go): >4KiB 时前缀只放索引，正文在 turn-tail 注入。
- **Schema 无损压缩** (provider/schema_canonicalize.go): 移除属性级 "type":"string" 和空 "required":[]，L3 减少 ~15%。

### 实测验证

10轮多轮对话（41 API 调用）
总输入: 554K tok · 缓存命中: 91.4%（含 3 次断裂）
排除断裂: 97.5% · 总成本: ¥0.2150

子代理缓存测试（5轮独立进程）
explore: 2% → 14.2% → 99.2% 🟢
review: 2% → 99.2% 🟢

## [5.28.0] — 2026-06-10

### 缓存优化（核心）

- **SpawnPolicy L4 领域管理** (cache/spawn.go): 完整实现 SpawnPolicy（原为空壳）。
  子代理任务描述按 task kind 产生相同的 L4 前缀，同类子代理共享 DeepSeek 前缀缓存。
  支持 ForkDefault/ForkLight/ForkWarm 三种分叉模式，带指标追踪。

- **压缩缓存保护** (gent/compact.go): 修改 maybeCompact 的内存布局，
  将 L1 + 首条用户消息 + 最近 tail 放在摘要之前，确保压缩后前缀[L1, first_user, tail]
  与压缩前匹配。压缩后的缓存 miss 从 ~66K tokens 降至 ~200 bytes（99.5% 减少）。

- **WarmupCache 注入完整历史** (gent/agent.go): 预热请求现在注入 L2 + 全部会话历史，
  与真实 stream() 调用完全一致。预热创建的缓存条目可被真实请求 100% 复用。

- **PromptHint 注入 L2** (cache/runtime.go, context/manager.go):
  PromptHint（原死字段）现通过 SetPromptHint() 在 Lock() 前注入 L2，
  每个 task type 提供任务特有优化提示。缓存安全：仅在首轮写入，Lock 后冻结。

- **提示词优化** (oot/boot.go): L1 中加入批量工具调用引导。
  PromptHint 更新到 16 个 Profile，强调批处理、减少步骤。

### 工具与截断

- **会话截断收紧** (gent/agent.go): 新增 	runcateToolOutputForSession() 使用
  sessionToolResultLines=120/sessionToolOutputBytes=12KB 的紧限制。
  selectHygieneLines 头尾保留 + 信号行提取（最大48行含error/fail/panic），不丢关键信息。
  显示截断保持原有 32KB/320 行不变。

- **WarmupCache 诊断** (gent/agent.go): emitPrefixHashDiagnostic() 每步追踪
  L1/L2/Tools SHA-256 哈希，发生任何前缀变化时在浏览器输出 [cache-diag] 警告。

- **工具注册修复** (oot/boot.go): 使用 uiltin.Workspace{Tools()} 注册全部 20+ 内置工具，
  修复子代理无 bash/read 工具的问题。

### 构建

- CLI: in/tianxuan.exe（18MB）
- 桌面端: desktop/build/bin/tianxuan-desktop.exe（16MB, Wails v2.12.0）

# Changelog

All notable changes to the Go line (tianxuan) are recorded here.

## [5.23.0] — 2026-06-08

### 桌面端全面优化

#### 代码架构 (Phase 1)
- **App.tsx 拆分**: 976→785 行 (-20%)，提取 6 个自定义 hooks
  - `useLayoutSizes` — 布局常量 + clamp 函数 + localStorage 持久化
  - `useTodoExtractor` — 待办项提取 (todo_write 解析)
  - `usePlanExtractor` — 计划 Markdown 内容提取
  - `useToolStats` — 工具/技能使用统计
  - `useModeManager` — 模式/温度/主题/模型切换
  - `useSessionManager` — 会话 CRUD + 侧边栏刷新

#### 渲染性能 (Phase 3)
- **React.memo**: ToolCard + MemoMarkdown 避免流式更新时的不必要重渲染
- **React.lazy**: 5 个抽屉组件按需加载 (Settings/Capabilities/Memory/History/Plan)，首屏 JS -82%
- **Vite manualChunks**: vendor-markdown (435KB) + vendor-ui (187KB) + highlight (66KB) 独立分包

#### 清理
- 删除 `tianxuan-desktop/` (Tauri 独立项目)

#### 构建验证
- Go 测试: 35/35 通过
- Go vet: 零警告
- Wails build: 18.03s → 16.3 MB EXE

## [5.22.0] — 2026-06-07

### 缓存守卫 + 成本优化 + GUI 升级 + Bug 修复 (12 个版本合并)

基于 DeepSeek-GUI (Kun) 深度研究和真实 API 验证。

#### 缓存守卫 (V5.10-V5.14)

- **ImmutablePrefix 指纹校验**: stream() 入口 SHA256 校验 L1+L2+tools，漂移→panic (Kun immutable-prefix.ts 移植)
- **History Hygiene 升级**: 三维压缩 (行数+字节数+token估算) 替换 32KB 硬截断 (Kun request-history-hygiene.ts 移植)
- **Token Economy 按工具策略**: bash 180行/24KB, read_file 320行/32KB, glob 160行/24KB, ls 120行/24KB (Kun token-economy.ts 移植)
- **前缀挥发性扫描**: 检测 L1/L2 中的 UUID/ISO8601/hex hash/JWT，防止缓存前缀被破坏 (Kun prefix-volatility.ts 移植)
- **工具目录指纹**: 检测工具集漂移 (additive/breaking)，breaking 时 emit Warning (Kun tool-catalog-fingerprint.ts 移植)
- **模型历史修复升级**: SanitizeToolPairing 处理桥接消息，不阻断配对扫描 (Kun model-history-repair.ts 移植)
- **compaction digest marker**: 压缩摘要附加 SHA256 hash，确保缓存稳定性可验证 (Kun compaction-marker.ts 移植)

#### 成本优化 (V5.13-V5.15)

- **ParamStormBreaker**: 参数级重复调用检测，窗口8阈值3，写入清零只读历史 (Kun tool-storm-breaker.ts 移植)
- **三级压缩**: normal/aggressive/force，动态调整保留消息数 (Kun context-compactor.ts 移植)
- **BudgetGate**: 会话成本预算门控，80%警告/100%阻断 (Kun checkBudgetGate 移植)
- **ModelContextProfile**: 按模型配置 compaction 阈值 (flash 128K / pro 1M)
- **Tool-Call-Repair per-call**: 展平包装器+捞取JSON+截断超大参数 (Kun tool-call-repair.ts 移植)
- **启发式自动路由**: 关键词+长度匹配路由 flash/pro，零额外API成本 (Kun auto-model-router.ts 移植)

#### GUI 升级 (V5.16, V5.20, V5.22)

- **快捷任务卡片**: 3个预设卡片 (了解项目/定位问题/实现方案)，DeepSeek-GUI 风格
- **PlanPanel**: 右侧计划面板，Markdown 渲染 + 自动提取 create_plan 内容
- **会话归档 API**: ArchiveSession/UnarchiveSession

#### Bug 修复 (共 8 个)

- **Critical**: executeBatch param storm 全量抑制→仅抑制触发调用
- **High**: compact normal 模式静默覆盖 RecentKeep→使用配置值
- **High**: storm 检测与 executeOne 参数不一致→统一 repair
- **High**: BudgetGate 80%警告一次后静默→重复警告
- **Medium**: ParamStormBreaker 缺 mutex→加 sync.Mutex
- **Medium**: requestApproval session flag 错误→修正返回值
- **Low**: executeOne 多余 nil 检查→简化
- **Low**: runGuarded 静默返回→加 Notice 事件

#### 死代码清理

- 删除 fork_pool.go (105行)，memory/graph*.go (571行)，retriever.go (192行)，extract.go (76行)
- 删除 review_changes + goal 工具 (4个，~345行)
- 删除 V1.3-V3.0 旧版本文档

#### 30 轮缓存实测 (mock + 真实 API)

| 测试场景 | 命中率 |
|---------|--------|
| Mock 无压缩 14 轮 | 93% |
| Mock 小窗口 30 轮 | 91% (压缩后 10 轮恢复) |
| 真实 API 大前缀 | 94% |
| **CLI 10 轮 (与 V5.7 同方法)** | **98.9%** |
| V5.7 基线 | 99.0% |

## [5.9.0] — 2026-06-08

### 紧凑升级 + 缓存断裂检测 + MarkItDown 集成

基于 Claude Code 源码（promptCacheBreakDetection.ts）和 claw-code（compact.rs）研究。

#### ① 确定性规则摘要 compact（claw-code 风格）

- **重写 `buildCompactSummary`**：从 V5.8 的简单计数升级为 claw-code 风格的结构化摘要
- **提取维度**：用户请求（最后 3 条）、编辑文件、工具统计、待办项（含 todo/next/pending 关键词）、关键文件路径（`.go`/`.ts`/`.rs` 等）
- **格式**：`[Earlier conversation summary:\n- Scope: N messages, M turns\n- Recent requests:\n  - ...\n- Files modified: ...\n- Tools used: ...\n- Pending work:\n  - ...\n- Key files: ...]`
- **完全确定性**：相同输入 → 相同输出，不影响缓存稳定性
- **辅助函数**：`truncateText`（rune 安全截断）、`extractKeyFiles`（从消息中提取含扩展名的路径）

#### ② 缓存断裂检测（CC 风格）

- **`cacheBreakDetector`** 类型：两阶段检测——调用前 FNV-1a 哈希 L1/L2/tools，调用后对比 cache_read
- **触发条件**：cache_read 下降 >5% 且 >2000 tokens
- **输出**：`[cache break #N: 16000→4352 tok (server-side)]` 通过 event.Notice 发出
- **纯读操作**：不修改任何缓存前缀，不影响 L1/L2/tools 稳定性
- **静默**：正常波动不告警，首次调用不告警（无基线）

#### ③ compact 边界保护（claw-code 风格）

- **机制**：`maybeCompact` 中回退 `keepFrom` 边界，确保不切断 tool_use/tool_result 配对
- **检测**：保留段第一条是 tool_result 但前一条无 tool_calls → 回退一步
- **效果**：防止 OpenAI API 400 错误（孤儿 tool 消息）

#### ④ MarkItDown 二进制文件自动转换

- **接入点**：`read_file` 工具检测到二进制文件（NUL 字节）→ `tryMarkItDown(path)`
- **查找链**：`markitdown` CLI → `python3 -m markitdown` → `python -m markitdown`
- **支持格式**：`.pdf` `.docx` `.xlsx` `.xls` `.pptx` `.epub` `.html` `.htm` `.csv` `.ipynb`
- **超时**：60 秒，转换失败静默回退到错误提示

### 涉及文件

```
修改: internal/agent/compact.go         +160 -60 行 (摘要重写+边界保护)
修改: internal/agent/agent.go           +85 行 (cacheBreakDetector)
修改: internal/tool/builtin/readfile.go +40 行 (markitdown 回退)
修改: CHANGELOG.md                      +75 行
```

### 回退

```
git reset --hard v5.8.0
git clean -fd
go build -ldflags "-s -w" -o bin/tianxuan.exe ./cmd/tianxuan
```

---

## [5.8.0] — 2026-06-08

### 成本与性能优化（Headroom 启发的四件套）

基于 [Headroom](https://github.com/chopratejas/headroom) 源码研究（CacheAligner 检测器模式、SearchCompressor 确定性压缩、CCR 可逆压缩）设计，四项确定性优化——不引入 ML 依赖、不改 L1/L2 前缀缓存。

#### ① SmartCompress — 工具结果智能压缩 (`compress.go`)

- **grep/search_content 压缩**: 解析 `path:line:text` → 按文件分组 → 每文件保留首条+末条+错误行 → 全局 30 条上限 / 15 文件上限 → 省略项显示摘要 `[… and N more matches in file.go]`
- **错误行加权**: FATAL/ERROR/panic/exception/fail 自动保留（得分 +0.5）
- **directory_tree 压缩**: 自动折叠 `node_modules`/`.git`/`dist`/`target`/`__pycache__` 等 14 种噪声目录，显示 `[N hidden — 依赖/构建目录]`
- **确定性保证**: 相同输入 → 相同输出，不影响 DeepSeek 前缀缓存
- **Windows 路径**: 正确处理 `C:\Users\...` 盘符，文件名含横线（`pre-commit-config.yaml`）不误解析
- **接入点**: `executeOne()` 中 `SmartCompress(call.Name, result)` 在 `truncateToolOutput` 之前
- **测试**: 10 个单元测试（分组/错误保留/全局上限/passthrough/空输入/Windows路径/横线文件名/空行/tree折叠/tree直通）

#### ② 跨轮 toolCache (`toolcache.go`)

- **TTL 改为无过期**: 从 `5 * time.Second` 改为 `-1`（永不过期，仅依赖 mtime 校验）
- **移除每轮 clear()**: `runDirect()` 中不再清空缓存，跨轮复用文件读取结果
- **mtime 自动失效**: 文件被外部修改时自动检测并失效——读操作重读磁盘，写操作主动 `invalidatePath()`
- **零配置**: 无需用户介入，框架自动生效

#### ③ CompactSummary — 紧凑确定性摘要 (`compact.go`)

- **触发时机**: 紧凑截断历史消息时
- **摘要内容**: 从被截断消息中提取：完成轮次数、编辑文件列表（最多 10 个，去长路径前缀）、工具使用统计（按调用次数降序）
- **插入位置**: L1 系统消息与保留的最近消息之间，以 `[Context from earlier turns: ...]` 格式
- **完全确定性**: 相同消息历史 → 相同摘要字节 → 不破坏缓存稳定性
- **模型感知**: 帮助模型"记住"紧凑前做了什么，防止失忆

#### ④ CacheWarmup — 新会话缓存预热 (`agent.go` + `controller.go`)

- **机制**: 新会话首轮前发送微型 ping 请求 → DeepSeek 服务端建立 [L1+L2+tools] 前缀缓存 → 首轮真实请求仅 miss 最后一条用户消息
- **ping 请求**: `[L1 system, L2 system, user:"ping"]` + 全量 tools + `max_tokens=1`
- **代价**: ~500 tokens input + 1 token output ≈ ¥0.0005
- **收益**: 首轮 cache miss 从 ~15,660 tok 降至 ~99 tok（-99.4%）
- **静默失败**: ping 失败不影响正常流程（预热是优化，不是必需品）
- **接入点**: `controller.go` 两处（`runTurnWithRaw` + `Run`），均在一轮 L2 注入后调用

### 实验数据

**10 轮缓存测试（两次独立运行，完全一致）**:

| 指标 | V5.7 | V5.8 | 改善 |
|------|------|------|------|
| R1 缓存命中率 | 2.4% | **99.3%** | +96.9pp |
| R1 Cache Miss | 15,660 tok | **99 tok** | -99.4% |
| R2-R10 命中率 | 99.7%（波动） | **99.3%（零波动）** | 稳定 |
| R2-R10 缓存锁定 | 16,000 tok | **13,952 tok** | 完全稳定 |
| V5.7 R7 缓存异常 | 27.1% | **未复现** | 修复 |

**数据文件**:
- `docs/superpowers/plans/2026-06-08-v58-cost-optimization.md` — 实现计划
- `benchmarks/v58-run1.txt` — 第一次 10 轮原始输出
- `benchmarks/v58-run2.txt` — 第二次 10 轮原始输出

### 涉及文件

```
新增: internal/agent/compress.go       +260 行 (grep/tree/SmartCompress)
新增: internal/agent/compress_test.go   +180 行 (10 测试)
修改: internal/agent/agent.go           +37 -2 行 (SmartCompress接入/toolCache/WarmupCache)
修改: internal/agent/compact.go         +82 -1 行 (buildCompactSummary/imports)
修改: internal/control/controller.go    +4 行 (WarmupCache调用×2)
```

### 回退

```
git reset --hard v5.7.0
git clean -fd
go build -ldflags "-s -w" -o bin/tianxuan.exe ./cmd/tianxuan
```

---

## [5.7.0] — 2026-06-08

### L2 缓存破坏修复 (V3.0 回归)

- **SystemPrompt 移除可变字段**: L2 系统消息中移除 `RecentEdits`、`ActiveFiles`、`Phase`、`Hypothesis`。这些字段每轮变化导致 DeepSeek 前缀缓存在 L2 处完全失效——V3.0 已修复但 V5.0 极简重构时意外回归
- **RecentEdits 公开 getter**: 新增 `RuntimeLayer.RecentEdits()` 方法，供 Controller 通过 turn-tail 注入到用户消息末尾（而非 L2 前缀）
- **Controller 双路径 L2 注入修复**: `runTurnWithRaw()` (交互) 和 `Run()` (头戴) 路径统一在首轮注入 L2，后续轮次通过 `IsLocked()` 守卫跳过——L2 字节完全稳定
- **测试固化**: 新增 3 个测试 (`TestSystemPromptExcludesRecentEdits`、`TestSystemPromptExcludesActiveFiles`、`TestRecentEditsGetter`) 防止回归

**实测效果 (10轮)**：缓存命中率从 V5.0 的 97-99% 波动 → 稳定在 **99.7%**（零波动），每轮 Cache Miss 从 ~182 tok 降至 **~49 tok**（-73%）

### 统计面板

- **命中率精度**: 所有命中率显示从 `.toFixed(1)` 改为 `.toFixed(2)`（99.7% 而非 100%）
- **趋势图纵轴**: 窄区间（≤3%）自动切换 1% 粒度，允许纵轴缩放至 97%-100%；宽区间保持原有 5% 粒度

### 后端修复（审查驱动）

- **maybeCompact nil usage 截断失效**: usage==nil 时 fallback 到 LastPrompt，防止静默超出窗口
- **toolcache TOCTOU 竞态**: 写锁内双重检查条目指针，防止并发 set 被误删
- **readStream goroutine 泄漏**: out channel 加 16 缓冲，ctx 取消时安全退出
- **ChunkError 后 preWG 泄漏**: stream() 返回错误后调用 preWG.Wait() 清理
- **runParallel 死代码**: 删除外层空 recover 块
- **SSE idle timeout**: 120s 无数据自动关闭连接，防止长时间思考时连接断开

### 安全加固

- **MCP HTTP SSRF 防护**: transport_http 使用 ssrfGuardedHTTPClient（私有 IP/DNS 重绑定阻断）
- **Hook 沙箱集成**: DefaultSpawner 在 enforce 模式下通过 sandbox.Command() 包装

### UI 优化

- **统计面板科技感卡片**: 渐变背景、顶部光线、命中绿色发光、等宽数字
- **本轮统计 4 卡片**: Prompt/Completion/缓存命中/未命中卡片组
- **输入框读秒**: composer 左上角实时显示回复耗时
- **工具卡片状态边框**: 运行(accent)/错误(red)/完成(transparent)/停止(warn) 左边框
- **卡片全面左对齐**: 修复 toolgroup/notice/compaction 居中问题
- **删除死代码**: Transcript.module.css + tokens.css
- **文本溢出修复**: msg__body/reasoning__body/tool__body 添加 word-break
- **复制按钮精简**: 删除所有单条复制按钮，流式光标改为静态
- **顶部栏精简**: 删除重复的记忆/技能/设置按钮，保留导出/清空/主题文字按钮
- **更新检查静默**: 网络错误不显示红色 banner
- **虚拟滚动折叠**: 动画完成后自动触发 measure() 修正高度
- **CSS 清理**: 合并重复 tool__body/notice/compaction 定义

### 基础设施

- **GitNexus MCP**: 注册 gitnexus MCP 服务器（16+ 代码智能工具）
- **构建**: go build + go vet + tsc + vite + wails 全部零错误

## [5.6.0] — 2026-06-06

### 卡片布局统一

- **全宽左对齐**: 移除 `.transcript > *` max-width 限制，所有卡片 `max-width: 100%`
- **AI 文本卡**: 去阴影、圆角统一 `6px`、padding 收窄
- **用户气泡**: 右对齐 (`align-self: flex-end`)、圆角统一 `6px`
- **工具卡**: `margin: 4px 0` 左对齐，read-only 工具 `margin: 1px 0`
- **间距收紧**: `.msg` margin `18px→6px`，padding `4px→2px`

### 默认折叠

- **思考卡**: 始终默认折叠（含流式），点击 `💭` 按钮随时展开/折叠
- **工具卡**: 始终默认折叠，点击展开；移除运行时自动展开逻辑
- **移除读秒**: 思考卡和工具卡的 elapsed 读秒计时器已移除
- **空文本不渲染**: `item.text` 为空时不渲染 MemoMarkdown，消除思考卡下的空白气泡

### 思考卡紧凑化

- `margin-bottom: 0`，字号 `11px`，`line-height: 1.2`，消除下方空行

## [5.5.0] — 2026-06-06

### 对话交互优化

- **虚拟滚动**: Transcript 接入 `@tanstack/react-virtual`，动态高度测量 + 5 条预渲染，长对话流畅不卡
- **流式缓存**: 新增 `MemoMarkdown` 组件，AST 主体缓存 + 尾部纯文本追加，流式输出不再全量 re-parse
- **思考卡重构**: 流式时自动展开、完成后默认折叠，显示段落计数
- **工具卡优化**: 运行中自动展开，已完成默认折叠；quiet 只读工具更紧凑
- **气泡布局**: 虚拟滚动 wrapper 改用 flex 容器 + `align-self` 左右对齐，用户消息右蓝色气泡、AI 消息左暗色气泡
- **输入历史**: 空输入框按 ↑↓ 回溯最近 50 条已发送消息 (sessionStorage)
- **折叠动画**: 思考卡/工具卡 `max-height` 过渡动画 + 流式光标闪烁

### 布局加宽

- **对话宽度**: `--maxw` 820→960px，气泡 `max-width` 72%→88%，transcript 左右 padding 收窄
- **窗口尺寸**: 初始 1400×820，最小 900×520 (main.go)

### 右侧面板修复

- Grid 三列均加 `minmax(0, ...)` 防溢出
- `.right-panel`、`.workspace-panel`、`.right-panel__tabs` 加 `overflow: hidden`
- `CHAT_MIN_WIDTH` 420→200，`WORKSPACE_PANEL_MIN_WIDTH` 640→320
- 窗口 < 784px 时自动隐藏面板 (resize + 初始化检查)

## [1.4.0] — 2026-06-02

### 401 故障修复

- **API key 全局化**: key 存入 `~/.env`，`loadDotEnv()` 自动从 cwd → home 加载，桌面端不再依赖项目目录下的 `.env` 副本
- **wails.ps1**: 移除 `.env` 复制逻辑，key 一次配置全局生效

### 会话按工作空间隔离

- **WorkspaceSessionDir(cwd)**: 新增 `config.WorkspaceSessionDir()`，会话存入 `cwd/.tianxuan/sessions/`，切换工作空间只显示当前空间的会话
- **boot.Options.SessionDir**: 新增字段，桌面端传入 workspace-scoped 路径，CLI 保持全局会话

### 前端重构

- **删除死代码**: `useController.ts`（零引用确认后删除）
- **骨架屏**: 替换加载转圈，模拟对话结构（用户消息 → 助手回复 → 工具调用）的脉冲占位块
- **右边栏重构**:
  - 改为 Grid 第 3 列常驻布局（不再需要折叠按钮）
  - 标签切换：「文件」「工具」
  - 移除顶栏和右边栏的折叠按钮
  - 宽度从 760px 缩至 280px
- **工具标签页**:
  - 卡片式布局，一列排列
  - 三类固定显示：工具 / 技能 / 子代理
  - 右侧数字为整个会话累计调用次数
  - 活跃卡片高亮（彩色边框 + 背景），未调用灰色

### ToolCard 升级

- **默认折叠**: 所有工具卡片默认折叠，点击展开
- **操作按钮**: 悬停显示复制输出按钮
- **运行计时器**: running 状态显示 `Xs` 计时
- **脉冲动画**: running 卡片呼吸边框效果
- **折叠组**: ≥3 个连续同名 read-only 工具自动折叠为一行 `📁 grep × 5`

### 推理合并

- **mergeConsecutiveReasoning**: 连续纯推理卡片自动合并为一张，不再碎片化
- **Transcript 性能**: `scanGroups` + `mergeConsecutiveReasoning` 使用 `useMemo` 缓存

### HistoryPanel

- **搜索框**: 顶部搜索栏，实时过滤标题和预览
- **无匹配提示**: 过滤为空时显示 "没有匹配的会话"

### Logo 替换

- Reasonix → tianxuan: 欢迎界面、侧边栏、HTML 标题、Wails 配置全部替换
- 图标资源: logo.png 替换为 tianxuan logo

### Karpathy 编码原则

- **内置技能**: 新增 `/karpathy-guidelines` 内置技能
- **TIANXUAN.md**: 四条原则持久化入项目记忆
  1. Think Before Coding — 先想清楚，暴露假设
  2. Simplicity First — 最小代码量，不写推测性代码
  3. Surgical Changes — 只改必须改的
  4. Goal-Driven Execution — 定义验证标准，循环直到通过

### 待处理（下版）

- [ ] 会话自动恢复（上次实现导致死机，已回滚）
- [ ] Drawer 离场动画
- [ ] 组件测试覆盖

## [1.3.0] — 2026-06-02

### Frontend — Zustand Migration & UX Polish

- **Zustand store** (store.ts): reactive state management replacing useReducer; `sessionTotal`
  auto-accumulated on each `turn_done`
- **Timeline interleaving**: `tool_dispatch` clears `currentAssistant` so reasoning / tool /
  text render in chronological order instead of being collapsed into one bubble
- **Reasoning blocks**: pure-reasoning (no text) assistant items expand by default with a
  subtle background; mixed blocks keep reasoning collapsed
- **Global keyboard shortcuts**: Ctrl+N (new session), Ctrl+K (focus composer), Ctrl+,
  (settings), Ctrl+Shift+M (memory), Ctrl+Shift+H (history), Ctrl+B (sidebar),
  Ctrl+J (workspace panel), Esc (close overlays)
- **Menu / dropdown entrance animation**: SlashMenu, FileMenu, ModelSwitcher fade+slide in
  120ms
- **Status bar**: context window shows raw `used/window` token count instead of percentage;
  cumulative session tokens displayed
- **Top bar**: workspace switcher chip, thinking-intensity dropdown (fast/normal/deep),
  theme toggle (light/dark), export Markdown button, clear-session button
- **Sidebar**: session search input (>3 items), hover-to-delete (×) per session,
  auto-refresh on workspace switch
- **History panel**: search/filter by title or preview
- **New-session toast**: brief green confirmation after creating a session
- **Workspace panel**: open by default on launch
- **Enhanced browser-dev mock**: input-aware reasoning + tool + text flow (greeting / poetry
  / code-request detection)
- **ToolCard**: read-only quiet mode now has a visible left border
- **Right drawer exit animation**: 120ms slide-out + backdrop fade before unmount

### Desktop

- **Config auto-discovery**: Wails desktop copies `.env` and `tianxuan.toml` into
  `build/bin/` on launch so the binary finds them regardless of working-directory changes
  (`ensureWorkspace`)
- **`wails.ps1` / `dev.ps1`**: one-click launchers for desktop and browser-dev modes

## [1.2.0] — 2026-06-02

### Brand & Desktop

- **Rebranded from Reasonix to tianxuan**: desktop app brand name, window title, sidebar, composer placeholder, and translations updated
- **New logo**: custom PNG logo replacing legacy SVG
- **Design tokens**: CSS variable system (`tokens.css`) with light/dark theme support

### Backend — Concurrency & Logic Fixes (13 bugs)

- **Compiler**: `AddContextHint` / `SystemPrompt` data race fixed with `sync.RWMutex`
- **GoalRouter**: removed overly-broad `"null"` keyword causing false `fix_bug` classification; added word-boundary matching for Gather mode
- **Agent**: `runParallel` goroutine panic recovery; `applyStormBreaker` now covers all results (not just `results[0]`); `isToolMisuse` consecutive duplicate detection
- **Controller**: `toolFilterApplied` race fixed with `atomic.Bool`; `c.turn` race in headless `Run()` path; `/compact` and `/new` goroutines now check `running` gate
- **Learner**: `sync.RWMutex` protecting `l.rules` map and `save()`; atomic file writes (temp + rename); `FailureRate()` method
- **Compact**: `l2Rings` concurrent read/write protected with `sync.RWMutex`; session pointer consistency check before `Replace`
- **ActiveSchemas**: `sync.RWMutex` protecting `SetActiveSchemas` / `stream` read

### Backend — Features (Phase 2-4)

- **Phase 2 — Gather mode**: GoalRouter matches user input against project structure (module name, dirs, entry points) to inject Focus Areas into Context domain
- **Phase 3 — Multi-resolution compaction**: L2 ring buffer (max 5) with FailureDetector-triggered backtrack injection; L3 disk persistence (`.tianxuan/l2/index.json`); `LoadL2Rings()` for cross-session recall
- **Phase 4 — Experience loop**: FailureDetector → Learner → GoalRouter feedback chain; adaptive tool-set expansion when `FailureRate > 50%`; `.tianxuan/learner.json` persistence
- **Fork cache sharing**: `task` and skill sub-agents inherit parent Identity+Context domains via `Compiler.Fork()`
- **TaskTool L2/Learner propagation**: sub-agents inherit L2 directory and task learner

### Frontend

- **Zustand store** (store.ts) prepared for V1.1 — reactive state management replacing useReducer
- **Vitest** test infrastructure with 8 passing tests
- **Dead code removed**: old `useController.ts` (zero callers confirmed by GitNexus)
- **Profile.scanTree**: `maxDepth=8` limit and additional skip dirs (`third_party`, `testdata`)

### Dev tooling

- **Wails CLI** installed for desktop development
- **GitNexus** indexed (11,511 nodes / 31,168 edges) — impact analysis validated all changes

## [1.0.0] — 2026-06-02

First stable release — a **ground-up rewrite in Go**. Not an upgrade of the `0.x`
TypeScript line; a new codebase that becomes the default (`main-v2`).

### Highlights

- **Go kernel**: a single static binary (CGO-free), cross-compiled for
  darwin/linux/windows on amd64 + arm64. Distributed via npm (the package wraps
  the native binary) and release archives; no Node runtime needed to run it.
- **Agent core**: the loop, built-in tools (read/write/edit/multi_edit/glob/grep/
  ls/bash/web_fetch/todo_write), permission gate, sandboxed bash, and the
  DeepSeek prefix-cache–oriented design.
- **Subagents**: `task` plus explore/research/review/security_review skill agents
  inheriting parent Identity+Context domains via Compiler.Fork() for near-zero
  token-cost prefix cache hits.

### Four-domain prefix architecture

- **Identity**: core persona, version-stable
- **Context**: project profile + memory index + skills index + Focus Areas (Gather mode via ProjectProfiler word-boundary matching)
- **Skill**: GoalRouter intent classification (fix_bug/write_feature/review/explain) with Learner adaptive tool-set expansion when FailureRate > 50%
- **Flow**: multi-resolution compaction — L1 online summary at 80% window, L2 ring buffer (max 5) for backtrack injection, L3 disk persistence (.tianxuan/l2/index.json)

### Key features

- **Gather mode (Phase 2)**: GoalRouter matches user input against project structure to inject ContextFocus into the Context domain
- **Multi-resolution compaction (Phase 3)**: L2 ring buffer with FailureDetector-triggered backtrack injection; L3 disk archive with index.json for cross-session recall
- **Experience loop (Phase 4)**: FailureDetector → Learner → GoalRouter feedback chain with .tianxuan/learner.json persistence
- **Fork cache sharing**: task and skill sub-agents inherit parent Identity+Context domains
- **13 concurrency and logic bugs fixed** during pre-release audit
- **Skills & hooks**: Claude-Code-style skills (`internal/skill`) and hooks
  (`internal/hook`), symlink-aware and slash-integrated.
- **MCP client**: connect external servers over stdio / Streamable HTTP; reads
  `[[plugins]]` and a Claude-Code `.mcp.json`.
- **Code intelligence via CodeGraph**: a tree-sitter symbol/call graph
  (`codegraph_*` tools) replaces embedding semantic search — no embedding service
  or API cost. Fetched into a local cache on first use (or `reasonix codegraph
  install`) and indexed in the background, so installs and startup stay fast.
- **Plan mode** with evidence-backed step sign-off (`complete_step`).
- **Memory**: `REASONIX.md` hierarchy + auto-memory, folded into the cache-stable
  prefix.
- **ACP** (`reasonix acp`) and an HTTP/SSE server frontend; desktop app (Wails).

### Notes

- Versions: the legacy TypeScript line stays in `0.x`; the Go line starts at
  `1.0.0`. See [docs/MIGRATING.md](docs/MIGRATING.md).
- Release archives ship a bare binary; CodeGraph is fetched on first use. Windows
  support for the fetched runtime is unverified — install `codegraph` on PATH if
  the auto-fetch doesn't resolve there.

[1.0.0]: https://github.com/esengine/DeepSeek-Reasonix/releases/tag/v1.0.0
