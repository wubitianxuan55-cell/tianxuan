## [10.169.0] — 2026-08-08

### ✨ bash 失败输出结构化头（对标 codex format_exec_output_for_model）

> plain 模式命令失败（非零退出码）时，输出改为 codex 风格结构化头，
> 模型无需解析裸错误串即可读取退出状态。

#### 变更
- plain 模式失败时输出：
  ```
  Exit code: 5
  Wall time: 0.6 seconds
  Total output lines: 3000   (仅截断时出现)
  Output:
  <命令自身输出>
  ```
- `Total output lines` 仅当输出被截断时出现（对齐 codex：模型看不到全部
  行时才报行数），小输出不噪声
- 成功路径保持裸输出不变（不破坏下游解析）；JSON 模式契约不变
  （失败编码在 payload，nil error）；超时路径不变（数据竞争安全考虑）

#### 验证（TDD）
- 新增 `bash_header_test.go` 6 例：失败头/输出保留/成功裸输出/JSON 不变/
  截断报行数/小输出不报行数
- 真实输出核对：`Exit code: 9 / Wall time: 0.6s / Total output lines: 3000`
- `go build`/`go vet` 干净；51 包全量测试全绿（仅预存 API 认证类失败）

---

## [10.168.0] — 2026-08-08

### 🐛 修复：技能工具（explore/research/review 等）不并发 + 使用异常

> 用户报告"子代理不并发 + 使用技能有问题"。根因链（5 级追溯）：

#### 根因
- **表象**：explore/research 等技能工具永远单独串行执行，不与任何工具并行；
  run_skill 调技能表现异常（被强制串行隔离）
- **直接原因**：`getConflictKey` 不认识技能工具名 → default 分支返回 `""`
- **本地根因**：`partitionToolCalls` 把 `key == ""` 当全局冲突
  （`hasGlobal := key == "" || key[0] == '!'`）→ 强制串行批
- **系统根因**：V10.124 从 `getConflictKey` 移除 explore/research/review
  （当时还原为纯技能），V10.147 把它们工具化回来（`subagentSkillTool`
  注释明确"可安全并行批处理"）但**没同步更新 `getConflictKey`**
- **过程根因**：`partitionToolCalls`/`getConflictKey` **零测试覆盖**，
  回归无人察觉

#### 修复
- `getConflictKey` 增加 registry 感知：default 分支查询 registry，
  ReadOnly 工具（含技能工具、grep/glob/ls/web_fetch 等）返回 `ro:<name>`
  共享键 → 互相并行 + 与 read_file 并行
- `partitionToolCalls` 增加 `ro:*` 与 `file:*` 互斥规则：只读工具（可能
  读全库的子代理）与写工具永不共享并行批，保住"写后读"顺序安全不变式
- `!spawn`/`!bash`/`!ledger` 等全局串行键语义不变（run_skill/task 仍串行）

#### 验证（TDD）
- 新增 `batch_parallel_test.go` 3 例：技能+read_file+research → 1 并行批；
  explore+read_file → 并行；writer 后技能串行（安全不变式）
- 新增 `batch_concurrency_test.go` 2 例：端到端 3 只读工具 0.30s（并行，
  修复前 0.90s 串行）；writer+skill 串行屏障 0.10s
- `go build`/`go vet` 干净；51 包全量测试全绿（仅预存 API 认证类失败）
- **缓存铁律**：改动仅限运行时调度（batch 分区），不触碰 L1/L2/tools
  schema/消息数组字节序——消息按 calls 索引稳定填充，前缀缓存不受影响

---

## [10.167.0] — 2026-08-08

### ✨ Codex 蒸馏 P2：get_context_remaining 工具 + ToolDispatchTrace 落盘

> 对标 Codex CLI 继续优化编程能力（蒸馏报告 P2-8 完成 + 新差距覆盖）。

#### get_context_remaining 工具（蒸馏 codex 同名工具）
- 只读工具，模型可主动查询当前上下文窗口剩余 token，长任务在窗口
  溢出前规划收敛（停止探索、收窄范围、wrap up）
- executeOne 经 context 注入 tokensLeft 闭包（compaction.Window 减
  chars*tokPerChar 估算），无窗口配置时友好提示，无注入大声失败

#### ToolDispatchTrace 落盘（报告 P2-8）
- `internal/tool/trace.go`: TraceStore JSONL 追加（ts/session/trace/call_id/
  tool/read_only/args/outcome/error/output_len/duration_ms），参数与错误
  截断（500/300），懒打开不持句柄（Windows 测试清理不卡）
- executeOne 改命名返回值 + defer 统一记录所有 dispatch 路径（success/
  error/blocked），trace_id 随每轮传播
- boot.go 注入 TraceStore + 注册新工具；CLI 新增 `tianxuan tools trace [-n N]`

#### 验证
- TDD: trace_store 4 例 + context_remaining 4 例 + execute_trace 端到端 2 例
  + cli trace 2 例全绿；go build/vet 干净；内部测试全绿（仅预存 API 认证
  失败与本次无关）

---

## [10.166.0] — 2026-08-08

### 🔄 架构回退：删除双模型（Hermes），重回单模型规划模式

> 经大量验证，双模型架构（Hermes 规划者 + Hephaestus 执行者，两个独立
> session / 两套 L1 前缀）效果不达预期。本版本整体删除，原"规划 → 确认 →
> 执行 → 自动修正"工作流由**单模型规划模式（PlannerHost）**承接：同一
> 模型、同一 session、同一工具列表，复杂任务先只读规划、确认后执行。

#### 核心设计（`internal/agent/planner_host.go`）
- `PlannerHost` 包装唯一 `AgentRunner`：`DecidePlannerRoute` 三值路由 →
  `RoutePlanAndExec` 且规划模式开启时进入两阶段
  - 规划轮：注入 `planmode.Marker` + `SetPlanMode(true)`（运行时只读门控，
    工具 schema 全程不变 —— 遵守前缀缓存铁律，避免 V8.0.2 filteredSchemas
    事故重演）+ 临时 `plannerMode=true` 跳过三闸门
  - 确认：headless / 简单计划（≤3 步、无新文件、Verify 可执行）自动确认；
    交互模式弹确认卡（提交执行 / 仅聊天 / 按用户意见修改计划 / 取消）
  - 执行轮：同一 session 继续（计划文本就在上下文中，无 handoff 交接）
  - 自动修正循环：失败步骤 → `buildFixPrompt`（round 2 定向 / round 3 反思）
    → 同一模型生成修正计划并立即执行（≤3 轮）
- 规划模式开关：`agent.auto_plan`（`off` 默认 = 直接执行；`ask` = 复杂任务
  先规划确认），桌面端 Composer 底部新增"规划"按钮热切换
- 默认关闭，行为与之前单模型 Adaptive Execution（Codex 方式）一致

#### 删除（双模型专属）
- `internal/agent/hermes.go`、`hermes_confirm.go`、`hermes_compact_test.go`、
  `workflow_test.go`（编排逻辑迁移/改写）
- `HermesPrompt` / `HephaestusSystemPrompt` / `hephaestusHandoffMarker` /
  `formatHandoff` / `HandoffTask` / `feedResultToPlanner` / `injectProjectMap`
- `agent.planner_model` / `planner_temperature` / `planner_effort` /
  `planner_max_steps` 配置及桌面设置 UI（模型卡、规划器步数）
- 前端 Hermes/Hephaestus 双标签（RunStatus 改为单阶段"规划/执行"）、
  planner 用量标签、ProcessCard 双模型流程文案
- `memory.PlannerBlock` 死代码（双模型规划者专用记忆过滤）

#### 保留复用
- `planmode.Policy` 只读安全门 + `Marker`、`DecidePlannerRoute` 分类器、
  `planparse.go` 计划解析、`shouldAutoConfirm` / `displayPlan` /
  `resolveConfirmChoice`、`buildVerifyTriad` / `checkCoherence` /
  `checkStepCoverage`（SDD 反馈）

#### 验证
- 新增 `planner_host_test.go` 9 个契约测试（直接执行 / 规划→执行 / 直接回答
  / 简单任务 / `!` 快速路径 / headless 自动确认 / 修订循环 / 取消 / 修正循环）
- `go build ./...`、`go vet ./...`、`go test ./internal/...` 全绿
  （`internal/cli` 的 `TestModelRefsSkipsUnconfigured` 为预存失败：zen
  provider 测试未清 `OPENCODE_API_KEY`，与本版本无关）
- 桌面端 `go build` / `go vet` 通过；前端 `tsc --noEmit` 通过、142 个
  vitest 测试全部通过

---

## [10.165.0] — 2026-08-08

### 📸 AI 自主截图工具 `capture_screen`

> 之前截图只能靠用户点相机按钮。现在模型自己就能截屏：你只需要说
> "截个屏看看 / 看看我屏幕上的报错 / 帮我评审当前界面"，AI 会主动调用
> `capture_screen` 截主屏，再通过 vision 技能识图回答。

#### 变更
- 新增内置工具 `internal/tool/builtin/screenshot.go`（+Windows GDI 实现
  `screenshot_windows.go`、跨平台回退 `screenshot_other.go`）：
  - 截取主屏幕 → PNG 保存到 `.tianxuan/attachments/screen-<时间戳>.png`
  - 返回 `@` 相对路径（与粘贴/截图附件管线同一约定，@引用与 vision 技能
    直接可用）
  - 返回文本明确提示模型："invoke the vision skill (run_skill name=vision)"
- 截图函数为包级变量（`captureScreenShot`），测试注入确定性假图
- 默认全量工具集自动包含，CLI/桌面端均可用；无参工具（Schema 无字段）

#### 测试（TDD RED→GREEN）
- `TestCaptureScreenSavesAttachment`：注入假 PNG → 验证附件落盘、
  `@.tianxuan/attachments/screen-` 路径、vision 技能指引
- `TestCaptureScreenPropagatesCaptureError`：截图失败大声传播
- `TestCaptureScreenRejectsBadArgs`：非法 JSON 参数大声失败
- 真实 GDI 截屏验证：本机截得 869KB PNG
- `go test ./internal/tool/... ./internal/control/...` 全绿；`go build ./...` 通过

#### 权限
- `capture_screen` 是写者工具（写附件文件），ask 权限下首次调用需用户确认
  （截屏含屏幕内容，首次确认是安全惯例；确认时可选"记住"，之后自动放行；
  或自行在 `[permissions] allow` 加入 `capture_screen`）

## [10.164.0] — 2026-08-08

### 📷 截图→识图闭环

> 桌面端 Composer 的"截屏发给 AI"（相机按钮）和粘贴/拖拽图片早已存在：
> 截图保存到 `.tianxuan/attachments/` 后以 `@路径` 引用发送。但附件管线给
> 模型的提示写的是 "use an image/OCR/vision MCP tool"——tianxuan 没有
> vision MCP，只有 vision 技能，模型不会主动关联，导致截图发了却"看不见"。

#### 变更
- `internal/control/refs.go`：`refImage` 提示文案改为
  "invoke the vision skill (run_skill name=vision) with this path"，
  让 DeepSeek 收到截图/图片附件时明确调用识图技能
- `vision` 技能 description 增加触发条件：收到 `[image attachment]`
  附件引用（`@.tianxuan/attachments/` 路径）时必须使用

#### 测试
- `go test ./internal/control/` 全绿（附件引用分类/渲染逻辑不受影响）

## [10.163.0] — 2026-08-08

### 🛠️ 修复更新安装时旧版本未关闭导致失败

> 应用内更新器启动 NSIS 安装器后才退出旧进程，或用户直接双击安装包升级时
> 旧版仍在运行——此时 `wails.files` 覆盖 `tianxuan-desktop.exe` 会被 Windows
> 拒绝，安装失败（Wails 官方模板不处理运行中的进程）。

#### 变更
- `build/windows/installer/project.nsi`：Section 开头终止旧进程并轮询等待其
  退出（`taskkill /F /IM tianxuan-desktop.exe /T`，最多 10 秒），确认句柄释放
  后再覆盖 exe
- 命令经 `cmd /c` 包裹执行重定向：`2>nul` 直接传给 `nsExec::ExecToStack`
  会导致 taskkill 不执行（实测复现）；包一层 cmd 后正常
- `project.nsi` 加 UTF-8 BOM：新增注释含中文后无 BOM 的 UTF-8 脚本会让
  makensis 报 `Bad text encoding`（NSIS 按 ANSI/GBK 解析无 BOM 文件）

#### 测试与验证（真实场景）
- makensis 单独编译通过；`LogicLib`/`nsExec` 用法用最小安装器复现验证
- 端到端：启动伪造 `tianxuan-desktop.exe`（PowerShell 副本）→ 运行带修复的
  安装器 `/S` → 伪造进程被终止、真实安装目录 exe 覆盖成功（SHA-256 一致）、
  安装器退出码 0
- 对比实验确认根因：`2>nul` 重定向是 taskkill 未执行的直接原因（去掉后进程
  可杀）

## [10.162.0] — 2026-08-08

### 👁️ 新增 vision 识图技能（OpenCode Zen 视觉模型）

> 主模型 DeepSeek 无视觉输入能力，无法直接"看"图片。新技能通过
> OpenCode Zen 的视觉模型（`mimo-v2.5-free`，已验证支持多模态）提供
> "外挂眼睛"：脚本把图片 base64 后 POST 到 Zen 的 chat/completions，
> 返回文字描述，主模型把描述当作看到的画面整合进回答。

#### 变更
- 新增内置技能 `.tianxuan/skills/vision/`（同步至 `internal/skill/bundled/vision/`）：
  - `SKILL.md`：触发条件（用户发图/问图）、调用方式（bash 工具跑
    `vision.ps1`）、模型/Key 配置、安全规则
  - `scripts/vision.ps1`：零依赖 PowerShell 脚本（Windows 安装版无
    node/python 也能跑），支持多图、`-Prompt`/`-Model`/`-Api` 参数
- Key 复用现有 `OPENCODE_API_KEY`（tianxuan 已从 `.env` 加载，子进程继承）
- 修两个 Windows PowerShell 5.1 编码坑：UTF-8 BOM 保证中文脚本可解析；
  HttpWebRequest + 手动 UTF-8 解码绕开 Invoke-RestMethod 按 Latin-1 解码
  无 charset JSON 响应的问题（否则中文输出双重编码乱码）

#### 测试与验证
- 真实 key + 真实截图端到端实测：mimo-v2.5-free 返回准确中文描述，
  stdout 纯 UTF-8 字节、退出码 0
- 错误路径验证：缺 key / 图片不存在 → 大声失败 + 非零退出
- 临时冒烟测试确认 `Store.Read("vision")` 解析正确、脚本随
  `EnsureBundled` 提取为真实文件（跑完已删）
- `go test ./internal/skill/` 全绿

## [10.161.0] — 2026-08-08

### 🖥️ 桌面端设置面板支持 OpenCode Zen

> 承接 [10.160.0]：CLI 已支持 zen，但桌面端"模型服务"面板看不到——desktop 主程序
> 未注册 opencode provider kind，预设模板也没有 Zen 入口。

#### 变更
- `desktop/main.go` blank import `internal/provider/opencode`：桌面端
  `provider.Kinds()` 现在包含 opencode，设置面板 kind 下拉可选
- `SettingsProviders.tsx` 预设模板新增 OpenCode Zen（kind=opencode、
  base_url=https://opencode.ai/zen/v1、deepseek-v4-flash-free、
  OPENCODE_API_KEY），"+ 添加服务"一步建好
- 用户级 `%APPDATA%/tianxuan/config.toml` 预置 zen provider（免费模型起步）

#### 测试（TDD RED→GREEN）
- `SettingsProviders.test.ts`：PROVIDER_PRESETS 含 opencode 预设且字段正确
- 前端 vitest 15 文件 142 例全绿 · desktop `go build ./...` 通过

## [10.160.0] — 2026-08-08

### 🌐 OpenCode Zen 模型接入

> 单一 API key 访问 DeepSeek / Kimi / GLM / MiniMax / Claude / Qwen / GPT / Grok，
> kind=opencode 按模型自动路由到 Zen 的对应协议端点。

#### 变更
- 新增 `opencode` provider kind（`internal/provider/opencode/`）：`claude-*/qwen*` →
  `/v1/messages`（复用 anthropic provider，剥掉重复 `/v1`）、`gpt-*/grok-*` →
  `/v1/responses`（新 Responses 流式客户端）、其余 → `/v1/chat/completions`
  （复用 openai provider）；`gemini-*` 明确报错暂不支持
- Responses 客户端：chat 历史 → input 转换（assistant tool_calls 展开为
  function_call / function_call_output）、工具参数流式累积、reasoning /
  usage 解析、工具配对修复、空闲超时守卫
- 免费模型（如 deepseek-v4-flash-free）**无需 API key 匿名可用**；
  config.Validate/Configured 放行无 key 的 opencode provider
- 示例配置：`tianxuan.example.toml` + `.env.example`（OPENCODE_API_KEY）

#### 测试（TDD RED→GREEN）
- `provider/opencode/`：协议路由 17 例、Responses 请求构建（含工具配对修复）、
  SSE 文本/工具调用/reasoning/错误/401 解析
- `config/configured_test.go`：opencode 无 key 放行
- 真实 API 三协议端到端：deepseek-v4-flash-free（匿名）、gpt-5.4-nano
  （responses）、claude-haiku-4-5（messages）全部打通，CLI `run -model zen/...`
  验证通过 · 全量 `go test ./...` 通过

## [10.159.0] — 2026-08-05

### 🔧 重度使用痛点修复（第三批）：read_file 按符号跳读

> 承接 [10.157.0]/[10.158.0]：大文件（oauth.go 等）不再需要盲读/分页——offload
> 预览加长之外，增加"按符号定位"的轻量跳读接口。

#### 变更
- **read_file 新增 `symbol` 参数**：直接定位函数/方法/类型的定义行并从此处读取
  （复用 code_index 的符号解析：Go AST + 多语言 regex）；未显式 limit 时默认读
  200 行（覆盖大多数函数体）；输出标注 `[symbol "X" at line N]`
- 未找到符号时大声失败并附附近符号名（5 个），而不是静默从文件头读取

#### 测试（TDD RED→GREEN）
- `builtin/tool_extra_test.go`：TestReadFileSymbolLocatesFunction（定位定义行 +
  标注行号 + 不包含符号前内容）、TestReadFileSymbolNotFound（大声失败）
- 全量 `go test ./...` 通过 · `go vet` 干净 · gofmt 干净

## [10.158.0] — 2026-08-05

### 🔧 重度使用痛点修复（第二批）

> 承接 [10.157.0]：todo blocked 状态（外部依赖空转降噪）+ edit_lines 多行锚点。

#### 变更
- **todo_write 支持 blocked 状态**：schema/compact/状态机全部接受 `blocked`
  （等待外部依赖：用户实测、服务重启、外部 API）；blocked 项从"未完成"中排除，
  taskGate 不再对纯 blocked 清单注入"请继续执行"空转提示；todo ack 报 blocked 计数
- **edit_lines 多行锚点**：`start_anchor`/`end_anchor` 可含 `\n` 匹配起始行起的
  连续多行（单行不唯一时不再需要"单行锚点 + 行号"双约束踩坑）；任一行动不符
  即拒绝且不写文件

#### 测试（TDD RED→GREEN）
- `evidence`：IncompleteTodos 排除 blocked、ValidateSerialTodos 接受 blocked
- `builtin/todo_test.go`：todo_write 接受 blocked 并计数
- `builtin/editlines_test.go`：多行锚点匹配成功 / 任一行动不符拒绝且文件不动
- `agent/stop_gate_test.go`：纯 blocked 清单不触发 taskGate
- 全量 `go test ./...` 通过 · `go vet` 干净 · 改动文件 gofmt 干净

## [10.157.0] — 2026-08-05

### 🔧 重度使用痛点修复（需求驱动，非蒸馏）

> 基于重度使用者会话反馈（bash 连续 5-6 次 chat 参数误用、edit_file stale 硬拦截
> 造成往返、offload 预览过短），逐项修复：

#### 变更
- **validation 错误附正确示例**：`tool.ExampleFromSchema` 从工具 schema 生成精简
  示例（如 `{"command": "<command>"}`），validation_error 信息追加
  "expected args like: ..."，模型一次修正，不再反复撞同一错误
- **bash chat 参数误用检测**：`tool.MisuseHint` 识别传入 bash 的 chat-API 风格参数
  （model/messages/max_tokens/temperature/top_p），错误信息直接提示
  "bash 只接受 command 字段"
- **stale 编辑守卫改软警告**：同一轮二次编辑同一文件不再硬拦截（原
  "blocked: [stale content]"），改为放行并在结果前注入警告——写工具自身的锚点
  匹配才是真正的 stale 守卫，连续编辑不同区域免去强制 read_file 往返
- **offload 预览 200 → 800 字符**：大文件判断成本下降，减少被迫 sed 分块读

#### 测试（TDD RED→GREEN）
- `internal/tool/example_test.go`：ExampleFromSchema（required 字段/类型/退化）
  + MisuseHint（bash chat 参数/正常调用/非 bash）
- `internal/agent/stale_guard_test.go`：软警告放行 + 重读后清除警告
- `internal/agent/validation_hint_test.go`：validation 错误附示例 + chat 误用提示
- `internal/agent/offload/offload_test.go`：预览截断于 PreviewChars
- 全量 `go test ./...` 通过 · `go vet` 干净 · `gofmt` 干净

## [10.156.0] — 2026-08-05

### 🍴 同类产品蒸馏阶段一：背景会话 fork（Qwen Code `/fork` 语义）

> 蒸馏计划（docs/design/2026-08-05-peers-distillation-plan.md）P0-1 落地：
> `task` 工具新增 `inherit_context`，子代理以父会话最近上下文快照启动
> （Qwen `/fork` 继承上下文语义），而非空白会话；配合已有 `run_in_background`
> 即"带上下文的后台 fork"。跨项目记忆（P0-2）经调研已被 GlobalDir/ScopeUser
> 覆盖，按极简原则不实施。

#### 变更
- `internal/agent/task.go` — `inherit_context` 参数：forkCtx 快照注入子代理首条
  user 消息（`<forked from parent conversation>` 标记），任务 prompt 保持末条；
  无 provider 时大声失败（防御性）
- `internal/boot/boot.go` — `taskTool.SetForkContext` 注入 + `forkContextText`
  提取（末条 user 输入 + 后续 turns，4000 字符预算、follow 保底 800，只读 Snapshot
  不触碰父会话前缀缓存）
- 工具描述同步：Schema / CompactSchema / CompactDescription 增加 inherit_context

#### 测试（TDD RED→GREEN）
- `task_test.go`：TestTaskForkInheritsContext（注入生效 + prompt 保持末条）、
  TestTaskForkWithoutProviderFails（无 provider 大声失败）
- `boot_test.go`：TestForkContextText（从末条 user 开始）、
  TestForkContextTextEmpty（空/纯 system 返回空）、TestForkContextTextTruncates
  （预算截断 + follow 保底不饿死后续 turns）
- 全量 `go test ./...` 通过 · `go vet` 干净

## [10.155.0] — 2026-08-05

### 📦 Windows 安装包与自动更新链路打通

> 首个可推送更新的桌面端发布：NSIS per-user 安装器 + 修复后的 manifest/签名/发布管道。

#### 变更
- 新增自定义 NSIS 模板 `desktop/build/windows/installer/project.nsi`：per-user 安装（免管理员）、完成页自动运行应用
- updater 端点修复：`esengine/tianxuan` → `wubitianxuan55-cell/tianxuan`，移除 Reasonix R2 bucket 引用
- minisign 公钥轮换（key ID `154E38FBADA79807`），签名/校验工具同步更新
- 新增 `scripts/publish-desktop.ps1` 本地一键发布脚本（构建 → 签名 → manifest → GitHub Release）

#### 验证
- desktop `go test ./...` 全绿 · `go vet` 干净
- `scripts/publish-desktop.ps1 -Version v10.155.0` 构建安装包 + 签名 + manifest 校验通过

## [10.152.0] — 2026-08-02

### 🔀 回合循环减负：删除过时 nudge，工具反馈改为仅硬模式

> 与提示词重构同一方向：减少宿主对模型节奏的打断。

#### 变更
- **删除 investigationNudge**（V10.139 子代理优先引导）：新提示词已明确“调查直接用 grep/read_file”，
  宿主在调查中途注入“改派子代理”只会打断节奏
- **删除 todoStepStuckNudge**（V10.136 Adaptive 引导）：与 Auto Failure Guard（V10.148）
  功能重叠，后者已覆盖“同一操作 3 次失败停止 + 回合 6 次无进展暂停”
- **tool_feedback 改为仅硬模式**：删除温和模式（2+ 错误即注入），
  只有“全部调用连续 3 轮失败”才注入一次 STOP 引导（模型自己能读到 tool_result 错误）

#### 测试（TDD RED→GREEN）
- 删除 investigation_nudge / todo_step_nudge 测试；重写 tool_feedback 测试
  （非全败不注入、连续 3 轮全败注入、per-turn 连击重置）
- 全量 go test ./... 通过；go vet 干净；go build EXIT 0

### 🧰 每轮开销优化：compact 工具集默认生效验证与防回退锁定

> 审计发现：Default() 中 tools.compact=true + 18 工具核心白名单早已生效
> （用户 tianxuan.toml 无 [tools] 段，保留默认）；本次做确认与固化。

#### 变更
- **实测验证**：当前配置下只注册 18 个核心内置工具，
  applyCompactToolset 再隐藏冗余（glob/delete_range/multi_edit/kill_shell/wait 等仍可调用）
- **防回退锁定测试**：TestToolsCompactDefaultsEnabled（默认 compact=true
  且白名单 ≤25）+ TestApplyCompactToolsetHidesRedundant（隐藏生效且仍可调用）
- **文档化**：example.toml 补充 compact 参数说明（boot 一次决定、
  session 内不变、缓存稳定）
- **缓存安全结论**：compact 只走静态 Hide（boot 阶段），
  不触及 SetActiveSchemas 动态路径；session 内工具列表字节不变，
  4外前缀缓存完整命中

#### 验证
- 新增 2 例锁定测试；全量 go test ./... 通过；go vet 干净；go build EXIT 0

### 📝 工具描述质量：统一英文 + 修复过时项

> 模型每轮看到的工具描述与提示词必须语言一致——中英混杂会降低指令遵从度。

#### 变更
- **完整描述英文化**：code_index/grep/read_skill/web_search/search_large_output 的
  Description 从中文改为英文（含这三个工具的 schema 参数描述）
- **双语冗余修复**：ui_styling/design_router 的完整描述原为中英双语重复，
  改为纯英文
- **compact 描述过时项更新**：edit_file 的“须全局唯一”改为
  “replace_all可全部替换”（与 schema 一致）；grep 补充 glob 过滤
- **语言契约锁定测试**：全部内置工具完整 Description + Schema 必须
  纯英文（TestBuiltinFullDescriptionsEnglish）；compact 中文描述保留为有意
  设计（省 token，TestCompactDescriptionsStayChinese）

#### 验证
- 新增 2 例语言契约测试；全量 go test ./... 通过；go vet 干净；go build EXIT 0

### 🧑‍💻 计划仪式降级：单模型解除 complete_step 强制

> 提示词已崩紧（“简单任务直接做”），但宿主还在强制 complete_step 签收——
> 内部矛盾。单模型没有接收方，完成仪式是纯开销。

#### 变更
- **finalReadinessCheck 单模型跳过**：strictVerify=false 时不再检查“todo 已
  completed 但无 complete_step”， todo_write 的 completed 状态即为充分的宿主可观察证据
- **taskGate 消息按模式分化**：单模型引导“将完成步骤在
  todo_write 中更新为 completed”；双模型保留 complete_step 引导
  （Hermes 依赖证据回报）
- 双模型路径不受影响（strictEvidence=true 保持原契约）

#### 测试（TDD RED→GREEN）
- 新增 TestFinalReadinessCheck_SingleModelSkipsCompleteStep + TestTaskGateDualModelMentionsCompleteStep；
  原有双模型测试显式 SetStrictVerification(true)
- 全量 go test ./... 通过；go vet 干净；go build EXIT 0

### 🎨 编辑工具收敛：恢复 multi_edit 可见（多 hunk 原子编辑）

> multi_edit 是 Codex apply_patch 的多 hunk 等价物（单次批量编辑、全或无），
> 但被 compact 隐藏且不在默认白名单——模型只能单次 edit_file，
> 多编辑任务被拖慢。

#### 变更
- **multi_edit 恢复可见**：从 applyCompactToolset 隐藏清单移除 + 加入
  Default 白名单（内置可见 18→19）
- **edit_file 描述引导**：多个编辑点用 multi_edit（原子化 all-or-nothing）
- 双模型/存在性不受影响；只是模型可见工具面多了一个高价值工具

#### 测试
- 更新 TestApplyCompactToolsetHidesRedundant：multi_edit 从 hidden 转为 keep
- 全量 go test ./... 通过；go vet 干净；go build EXIT 0

### ✂️ 剩余工具收敛：默认白名单 19→17

> 基于历史调用频率审计：move_file（1 次）与 git_worktree（12 次）
> 是超低频工具，模型完全可用 bash 替代（mv / git worktree）。

#### 变更
- 从默认白名单移除 move_file + git_worktree（仍可通过配置显式启用）
- 保留高频 git 工具：git_status/git_diff/git_commit/git_log（核心工作流）
- 内置可见：19 → 17，每轮 schema 开销进一步下降

#### 测试
- 白名单 ≤25 锁定测试保持通过；全量 go test ./... 通过；go build EXIT 0

### 🎯 测试先行规则确定化：场景分叉代替自由裁量

> 实测发现 DeepSeek 会系统性跳过 "TDD when it fits" 的自由裁量。
> 改为场景分叉的确定性规则：修改存量代码必须测试先行，
> 修 bug 必须复现测试先行，新建文件/配置变更豁免。

#### 变更
- **SoloSystemPrompt Verify 段**："TDD when it fits" → "修改存量代码时写/更新测试并确认其失败，
  然后实现；修 bug 复现测试先行；新建文件/配置变更不需要测试先行"
- **HephaestusSystemPrompt Verify 段**：同步加入同一规则（执行器也改存量代码）
- 保留简单任务豁免（新建/文档）：不恢复仪式化指挥

#### 测试（TDD RED→GREEN）
- 新增 TestSoloPromptTestFirstExistingCode + TestHephaestusPromptTestFirstExistingCode
- 全量 go test ./... 通过；go build EXIT 0

### 🛠️ 实测驱动优化：bash 环境注入 + cwd 引导

> 修 bug 实测暴露两个真实浪费：① 模型找不到 go
> （每次探测路径，浪费 1-2 轮）；② 模型不知道 cwd，用错误相对路径导致找文件（浪费 2-3 轮）。

#### 变更
- **bash 环境注入（对齐 Reasonix login-shell PATH）**：ConfineBash 改 options
  模式（WithBashTimeout/WithBashEnv）；boot 探测项目 tools/go/bin、tools/node 及
  常见 Windows 安装目录（Program Files Go/nodejs/Git）注入 PATH，commandEnv 用注入
  PATH 替换继承 PATH（唯一 PATH 生效）
- **cwd 引导（Solo + Hephaestus）**：明确“工作目录即项目根，路径相对于它，
  绝不要用目录名前缀路径”

#### 测试（TDD）
- 新增 TestBashCommandEnvInjectsPath/NilWhenUnset、TestBashPathEnvIncludesProjectTools/ExcludesMissing、
  TestSoloPromptWorkingDirectoryGuidance/TestHephaestusPromptWorkingDirectoryGuidance
- 全量 go test ./... 通过；go build EXIT 0

#### 实测验证（修 bug 任务）
- 修前：9 轮 / 203s（含 3 轮找文件 + 2 轮探测 go）
- 修后（干净环境）：9 轮 / 113s（cwd 迷惑与 go 探测消除，剩余浪费来自基准目录历史残留）
- 方法学训训：基准必须用全新目录 + 无 .tianxuan 残留，否则模型会调查历史浪费轮次

### 🚀 编程能力强化：系统提示词重构（单模型/双模型分开）

> 实测对比 Codex：相同模型下 tianxuan 速度与效果均差一大截。根因：系统提示词方法论堆砌 + 每轮固定 token 开销大 + 仪式化工具拉高轮数。按单模型/双模型设计差异分别重构三套提示词。

#### 重构
- **SoloSystemPrompt**（单模型）：159 行/2084 tokens → 48 行/629 tokens，
  去掉强制 TDD 仪式与 SDD 规划契约，聚焦自适应执行/最小变更/验证
- **HermesPrompt**（双模型·规划器）：417 行/5312 tokens → 90 行/974 tokens，
  保留计划契约（<!--plan-->/Delta/File(s)/Constraint/Verify/Fix plan），
  去掉 SDD 教条与自检清单
- **HephaestusSystemPrompt**（双模型·执行器）：257 行/3276 tokens → 49 行/636 tokens，
  保留执行契约（todo 同步、complete_step 证据、File(s) 锚点、失败升级）
- 三套统一英文，总固定开销 -79%（~10.7k → ~2.2k tokens）

#### 测试（TDD RED→GREEN）
- 更新提示词契约锁定测试：adaptive_workflow、hermes_sdd、subagent_priority、scoped_verify
- 全量 go test ./... 通过；go vet 干净；go build EXIT 0

## [10.151.0] — 2026-08-02

### 🖱 浏览器右侧分栏可拖拽调整宽度（修复宽度写死 CSS、无 resizer）

> 问题：浏览器打开时占右侧分栏，宽度写死 `clamp(360px, 38vw, 760px)`，
> 没有自己的拖拽把手（workspace 面板的 resizer 不服务浏览器分栏），用户
> 无法拖动变宽。

#### 修复
- **useBrowserPanel hook**：浏览器分栏独立宽度 state + 拖拽调整（与
  workspace 面板同模式）——pointer 拖拽、键盘（方向键/Home/End）、
  双击恢复默认，宽度持久化到 localStorage
- **clampBrowserPanelWidth**：360px 下限 / 1080px 上限 / 62% 视口比例 /
  对话区最小 200px 保护（原 38vw≈560px 默认不变）
- **接线**：浏览器打开时 `--workspace-width` 轨道跟随浏览器宽度，resizer
  复用 workspace-panel-resizer 样式（拖拽高亮/光标/触摸）
- **i18n**：en/zh/zh-TW 新增 `browser.resizePanel`

#### 测试（TDD RED→GREEN）
- `useLayoutSizes.test.ts` 7 例：默认不变、下限、上限、视口比例、对话区保护、
  窄窗下限、取整
- 前端全量 135/135 通过；`tsc --noEmit` 0 错误；`vite build` EXIT 0

#### 验证
- `pnpm test` 135/135 · `pnpm typecheck` 0 错误 · `pnpm run build` EXIT 0
- `wails build -ldflags "-X main.version=v10.151.0"` EXIT 0（23s）

## [10.150.0] — 2026-08-02

### 🧵 Model Failover 蒸馏（OpenClaw model-failover 移植 — turn-local 模型回退链）

> 需求：蒸馏 OpenClaw 的模型故障转移——primary 模型限流/过载/断网时不再
> 整轮失败，而是按序尝试备用模型（同 provider 同 key 不同 model），只当前
> 回合生效（turn-local），下回合回到 primary 保持前缀缓存稳定；纯过载时
> 整链指数退避重试；全部耗尽返回带逐候选细节的摘要错误。

#### 功能
- **失败分类**：ShouldFailover 只对 failover-worthy 错误切候选（429/408/5xx
  与网络瞬态）；参数错误（400/404/409）、鉴权失败（同 key 切了没用）、
  调用方取消、流中断都立即传播，不烧备用候选
- **候选链**：Chain 实现 provider.Provider 接口，agent 层零改动；主模型
  正常应答，失败时按序试 fallback，切成功时触发 OnSwitch 通知
  （前端 Notice：主模型 → 备用模型 + 失败原因）
- **整链退避重试**：所有候选同轮全为纯过载时，按 MaxChainRetries 轮指数
  退避重试（复用 provider.RateLimitBackoff），恢复后当前回合照常完成
- **摘要错误**：全部耗尽返回 FallbackSummaryError，逐候选列出失败原因，
  供 CLI 直接呈现
- **配置**：`[[providers]] fallbacks = ["model-b"]`；空/重复 fallback 跳过，
  无 fallback 时行为与原先完全一致（零开销）

#### 测试（TDD RED→GREEN）
- failover 包 11 例：primary 成功只用 primary、429 切 fallback + OnSwitch
  一次、400 不切、全败摘要（顺序正确）、整链重试轮数、重试后恢复、取消
  不切、auth 不切、ShouldFailover 分类 18 子例、通知携带原因、无 fallback 退化
- boot 接线 3 例：有 fallbacks 构建 Chain、空/重复跳过、无 fallback 返回普通 provider

#### 验证
- `go test ./...` 全绿；`go vet` 干净；我改动的文件 gofmt 干净

## [10.149.0] — 2026-08-02

### 🛡 Auto Failure Guard 蒸馏（Reasonix internal/recovery 移植 — 宿主侧失败升级决策）

> 需求：蒸馏 Reasonix Auto Guard 的失败升级决策，让 tianxuan agent 不再
> 限于"注入 nudge 让模型自己改"，而是在宿主侧做硬决策：同一精确操作失败 3
> 次后停止，回合累计 6 次失败且无真实进展后停回合变更/验证，只留只读诊断。
> 只读失败不累计（blocked/权限拒绝不算失败），成功的变更/验证即清零回合
> 预算——与 Reasonix rules.QualifyingFailure + budget 的语义对齐。

#### 功能
- **精确操作指纹**：OperationFingerprint（工具名 + key 无序 JSON 参数
  SHA-256），模型以不同顺序重发同参不会被误判为新操作
- **操作级升级**：同一指纹失败 3 次后宿主阻止该操作（blocked 结果
  反馈给模型），换参数/改方案即视为新操作重新计数
- **回合级升级**：回合内累计 6 次失败且无进展后，宿主停所有变更/验证
  调用，只读诊断（read_file/ls/glob）保持可用
- **回合预算重置**：成功的变更/验证 = 真实进展，清零回合失败数；
  已停止的操作不解除
- **缓存安全**：纯宿主决策，不碰 L1-L4 前缀；失败路径动态 user-tail
  引导与现有 nudge 同模式，和存量 stop_gate/budget_gate 互补

#### 测试（TDD RED→GREEN）
- 纯状态机 8 例：指纹稳定/有别、操作停止、参数变化重新计数、只读失败不累计、回合停止、成功清零、已停操作不解除、reset
- 接线集成 3 例（testutil.Mock 走真实 runDirect 主循环）：同操作 3 次失败后第 4 次被拒、回合停止后变更被拒只读放行、成功变更清除回合预算

#### 验证
- `go test ./...` 全绿；`go vet ./internal/agent/...` 干净；`go build ./...` EXIT 0

## [10.148.0] — 2026-08-02

### 🌐 集成应用内浏览器（仿 Codex 独立视图：Ctrl+Shift+B + 多标签 + 权限确认）

> 需求：给 tianxuan 集成浏览器，可直接查看阅览网页。先调研 Codex 内置
> 浏览器真实形态（官方文档：Ctrl+Shift+B 独立视图、多标签、首次访问域名
> 权限确认、与 AI 共享页面），按此对齐改造。渲染方案为"iframe + 文本模式
> 兜底"：WebView2 完整渲染网页，X-Frame-Options/CSP 拒绝嵌入的站点可切
> 文本模式（复用内核 web_fetch 的 SSRF 防护与去标签逻辑），保证任何站点
> 都能查看。

#### 功能
- **独立浏览器视图**：顶栏 Globe 按钮 + Ctrl+Shift+B 快捷键打开，替换
  聊天主区全高度展示（非侧边面板）；ESC 或工具栏 × 关闭返回聊天
- **多标签**：标签行支持新建 / 切换 / 关闭（关闭当前标签激活右邻，空则
  关视图），每标签独立历史栈与页面/文本模式状态
- **权限确认**：首次访问域名弹确认卡（仿 Codex allowed hosts），允许后
  写入 localStorage 白名单（tianxuan.browser.allowedHosts）免问，拒绝则
  不加载；确认前页面不渲染
- **外部浏览器一键打开**：导航栏 ExternalLink 按钮，当前页交给系统浏览器
  （桥接 `BrowserOpenURL`）
- **标签域名徽标**：标签行显示域名首字母胶囊（空标签回退 Globe 图标）
- **会话状态保留**：浏览器视图常驻挂载，切回聊天再打开不丢标签/历史栈
  （display 切换，非卸载）
- **浏览器快捷键**：Ctrl+T 新标签 / Ctrl+W 关闭当前标签 / Ctrl+L 聚焦地址栏
  （仅视图可见时生效）
- **地址栏智能解析**：完整 URL 直接打开；裸域名/localhost/IP/带路径域名补
  https://；其余输入（含空格、单单词）自动转 Bing 搜索（Codex 同款行为）
- **新标签页起始页**：最近访问列表（localStorage 跨会话持久化，按域名去重
  置顶、上限 8 条）+ 快捷键提示
- **选中文本发送 AI**：文本模式选中文字浮层"发送给 AI 分析"，自动附来源
  URL 填入 Composer（iframe 跨域无法做元素级注释，此为实现 Codex 注释
  模式的现实蒸馏）
- **AI 分析整页**：文本模式无选区时浮层按钮一键把全文（附来源 URL）发给
  AI，对齐 Codex"AI 与用户共享页面"理念
- **加载超时提示**：页面模式 8 秒未加载完成显示提示条（引导切文本模式或
  外部打开），避免白屏干等
- **右侧面板折叠**：面板 tab 行右侧折叠按钮；打开浏览器视图自动折叠面板
  让位、关闭时恢复原状态（浏览器内手动展开不被自动折叠打断）；面板折叠
  时顶栏显示展开按钮
- **Appshots 蒸馏（截屏发给 AI）**：Composer 新增截屏按钮，GDI 截取主屏幕
  生成 PNG dataURL，复用粘贴图片附件管线（@path 引用）发送给 agent——
  Codex Appshots 的 Windows 版，UI/布局评审场景一键分享当前屏幕
- **任务完成系统通知**：长任务结束 / 需要批准时弹 Windows 通知中心提醒
  （Codex 长任务通知蒸馏）；设置 > 通用 > 外观与布局可关闭（localStorage
  持久化）。Go 侧 `App.Notify` 基于 go-toast（AUMID 注册一次，非 Windows
  平台大声报错）
- **Code review 审查面板（diff 视图）**：变更 tab 打开文件默认显示 unified
  diff（新增绿 / 删除红 / hunk 深色分隔 + 双行号列），替代纯文件预览——
  Codex review pane 蒸馏。后端把会话 diff（executor PendingDiffs 的 unified
  diff）透传到前端，diffParser 纯函数解析（TDD 4 例）
- **diff 行内评论**：diff 行 hover 显示评论按钮，行内输入保存（Enter 保存 /
  Esc 取消），评论按 workspace+文件路径持久化（localStorage 跨会话）；
  有评论的行显示计数徽标，弹窗底部评论列表支持删除与"发送给 AI"（自动附
  路径:行号，逐条处理）——Codex review 注释工作流闭环
- **Visualizations 蒸馏（Mermaid 图表）**：Markdown 渲染支持 ```mermaid
  代码块 → SVG 图表（流程图/时序图/架构图），agent 讲解复杂逻辑直接配图；
  mermaid 按需懒加载（独立 chunk 149KB gzip，不拖首屏）、securityLevel
  strict 防注入、深色主题适配科幻风
- **独立 WebView2 浏览器窗口**：浏览器导航栏新增"独立窗口"按钮（AppWindow），
  用 go-webview2 打开专用 WebView2 窗口——独立 user data folder（cookies/
  登录与主应用隔离，Codex 独立 profile 蒸馏）、真实完整渲染（突破 iframe
  的 X-Frame-Options/CSP 限制）、可登录、窗口复用（已开则仅导航）。Wails v2
  同窗口内嵌第二面板需迁移 v3 WebviewPanel，此为当前架构下的落地形态
- **浏览器右侧分栏（可边对话边看）**：浏览器视图从覆盖聊天主区改为右侧
  独立分栏（clamp 360–760px，随窗口自适应），对话/Composer 保持可见可
  交互；关闭即收起，标签状态保留
- **技能面板可见子代理技能**：含 runAs=subagent 技能的分组默认展开（此前
  项目/全局分组折叠，子代理技能被折叠隐藏）；子代理徽标加"子代理"文字，
  与普通技能一眼区分
- **导航**：地址栏（URL 规范化自动补 https://）、前进/后退（自维护历史
  栈，前进分支裁剪语义与浏览器一致）、刷新、页面/文本双模式切换
- **页面模式**：iframe 渲染（sandbox 放开脚本/表单/弹窗），加载指示
- **文本模式**：内核抓取返回干净可读文本（等宽字体展示），受限网站兜底
- **Go 侧**：`App.BrowserFetchText` 绑定（desktop/browser.go），复用
  web_fetch 工具：空 URL / 非 http(s) 协议大声拒绝，SSRF 边界一致
- **i18n**：en/zh/zh-TW 浏览器文案（含权限确认/标签操作）

#### 测试（TDD RED→GREEN）
- Go：`TestBrowserFetchTextRequiresURL` / `RejectsNonHTTP` / `Network`
  （真实抓取 example.com）+ `TestPNGDataURL`（dataURL 前缀/PNG 可解码/
  尺寸一致）+ `TestOpenBrowserWindowRejectsInvalidURL`（空/非 http(s) 拒绝）
- 前端：`browserHistory.test.ts` 8 例（推入/去重/前进/后退/分支裁剪/
  URL 规范化）+ `browserTabs.test.ts` 10 例（新建/切换/关闭邻居激活/
  不可变更新/标题/徽标首字母）+ `browserPerm.test.ts` 5 例（域名解析/
  白名单读写/损坏数据兜底）+ `browserRecent.test.ts` 5 例（去重置顶/
  上限裁剪/损坏数据兜底/清理）+ `browserHistory` 地址解析 6 例（搜索编码/
  裸域/localhost/IP/兜底搜索）+ `reviewComments.test.ts` 5 例（增删/按
  workspace 隔离/空文本防御/AI 格式化）+ `diffParser.test.ts` 4 例

#### 验证
- `go test ./...` desktop 全绿；`tsc --noEmit` 0 错误；vitest 128/128；
  `vite build` EXIT 0

### 🖥️ 科幻质感细节：终端代码块 + Composer HUD 上缘光

- **代码块终端窗口质感**（Markdown.tsx）：头部加三色圆点（终端惯例，
  err/warn/ok 降饱和）+ 语言标签青色化 + 右对齐复制按钮
- **Composer HUD 上缘光**（Composer.tsx）：输入卡片顶部常驻青色渐变细线
  （pointer-events-none，不干扰拖拽把手），聚焦时与 accent 光晕叠加
- 运行状态行（Hermes 规划紫 / Hephaestus 执行青脉冲）沿用现有设计

#### 验证
- `tsc --noEmit` 0 错误；vitest 83/83；`vite build` EXIT 0

### 🚀 UI 升级：默认主题改为未来科幻风（深空黑 + 电光青）

> 使用 taste-skill + ui-ux-pro-max 双技能完成设计（Design Read：dark-tech
> developer tool for technical users, sci-fi HUD language）。市场调研同类
> AI 编程工具（Cursor/Devin/Windsurf 系）确认：深色控制台 + 单一冷色强调 +
> 克制光效是当前主流方向，避免 AI 默认的紫色渐变 slop。

#### 配色（styles.css default scheme 升级，明暗双态）
- 暗色：深空黑蓝基底（#05070E → #171D2E 层级），强调色绿 → 电光青 #22D3EE，
  边框/文本冷调化，语法高亮保持可读（紫色仅限代码关键字）
- 浅色：冷白基底 + 青色强调 #0891B2（cyan-700，WCAG AA 达标）
- 语义色保持：ok 绿 / warn 琥珀 / err 红 与强调色分离

#### 质感细节（克制、单一强调色）
- 顶栏：accent 细发光底线（HUD 分隔感，跟随各主题 accent）
- 状态脉冲默认色：暖金 → 电光青（--pulse-color 默认值）
- 滚动条 hover：冷灰青
- 设置页主题预览卡同步新色值

#### 验证
- `tsc --noEmit` 0 错误；vitest 83/83；`vite build` EXIT 0
- 无 backdrop-filter（遵守多平台性能约束）；无 web 字体引入；无 em-dash

### 🎨 蒸馏 taste-skill 为前端审美子代理技能

### 🎨 蒸馏 taste-skill 为前端审美子代理技能

> 用户要求将 Leonxlnx/taste-skill（MIT）蒸馏进 tianxuan。该技能是 Claude 生态
> 知名的"反模板（anti-slop）"前端审美技能：先读需求推断设计方向（Design Read）、
> 按三拨盘（DESIGN_VARIANCE / MOTION_INTENSITY / VISUAL_DENSITY）定视觉基调、
> 映射真实设计系统、用 AI Tells 与预检清单验收。

#### 蒸馏内容
- **`.tianxuan/skills/taste-skill/SKILL.md`**（运行时源，gitignore）：原 87KB
  SKILL.md 忠实保留核心章节（0-14 + Appendices），frontmatter 转为 tianxuan
  子代理格式：`name: taste-skill`、`runas: subagent`、`allowed-tools:
  read_file, ls, grep, bash`，新增 Subagent mode 自包含任务说明与 MIT 版权声明
- **`internal/skill/bundled/taste-skill/SKILL.md`**（构建产物，随二进制分发，
  新环境 EnsureBundled 自动提取）

#### 接线
- **自动注册**：boot 遍历 `runas: subagent` 技能，`taste_skill` 成为独立工具，
  原生 tool-calling 直接可见；技能索引自动排除（不增加前缀 token）
- **`boot.go` subagentSkillToolDescs**：注册 taste-skill 的模型可见
  desc/compactDesc（中文描述三拨盘工作流）
- **`skill/tools.go` design_router**：新增审美/反模板/landing/作品集/改版 →
  taste-skill 路由分支，描述同步更新

#### 测试
- `TestTasteSkillBundledSubagent`：EnsureBundled 提取后技能可读、
  RunAs=subagent、allowed-tools 完整、描述有效
- `TestDesignRouter` 新增 3 个 taste-skill 路由 case
- boot 工具注册测试新增 `taste_skill` 断言

#### 验证
- `go build ./...` EXIT 0；`go vet` 无警告；`go test ./internal/...` 全绿

### 🛡️ 修复 compact schema 系统性丢失 enum/minimum 约束（13 工具 18 处）

> V10.146 只修了 edit_lines 的 minimum:1，但同类根因并未根治：compact schema
> 是手写压缩，没有自动化防线，模型每轮看到的是压缩版，完整 Schema 的约束
> 一旦丢失，模型就会漏传或传非法值（V10.146 真实事故模式）。

#### 根因（5 级追溯）
- 表象：部分工具模型偶发传非法参数（如 code_index 传未知 action、
  complete_step 传 step_index=0、todo_write 传非法 status）。
- 直接原因：CompactSchema 缺失 enum/minimum，模型无法感知合法取值。
- 本地根因：compact.go 手写压缩时只保留 type，丢掉了全部 enum/minimum 约束。
- 系统根因：压缩逻辑无"约束保留"校验——V10.146 修 edit_lines 时只补了
  2 个字段，未建立覆盖全部工具的自动化防线，同类问题必然复发。
- 过程根因：修复停留在单点，未做全量清单审计。

#### 修复
- **`tool/builtin/compact.go`**：补全 13 个工具 18 处丢失约束——
  - minimum：read_file.offset(0)/limit(1)、code_index.limit(1)、
    complete_step.step_index(1)、wait.timeout_seconds(1)、web_fetch.retries(0)、
    web_search.topK(1)、verify_gate.checks[].timeout(1)
  - enum：bash.output_format、code_index.action、complete_step.evidence[].kind、
    git_worktree.action、grep.sort_by、memory_search.kind、notebook_edit.edit_mode/
    cell_type、todo_write.todos[].status/level
- **`tool/builtin/verify_gate.go`**：CompactDescription 改为引用 compactDesc map
  （与其他工具一致，消除硬编码重复，防文本漂移）

#### 测试
- **`tool/builtin/compact_wiring_test.go`**：新增 `TestCompactSchemaKeepsConstraints`
  （通用防线，RED→GREEN）——遍历所有 CompactDescriptor 工具，递归对比完整
  Schema 与 CompactSchema，断言 required/enum/minimum（含嵌套 items[].prop）
  全部保留；未来任何工具新增约束丢失都会在此拦截

#### 验证
- `go build ./...` EXIT 0；`go vet ./internal/...` 无警告
- `go test ./internal/...` 全绿（含 30 个 CompactDescriptor 工具的约束审计）

---

## [10.147.0] — 2026-08-02

### 🧬 技能系统重构 — 子代理化 + 工具化 + 压缩

> 大量实测证明 DeepSeek 不爱调用技能（run_skill 间接路径），本次把技能转为第一等
> 工具/子代理，并压缩模型可见工具集，降低注意力稀释与 schema token 成本。

- **探索类技能独立工具**：explore / research / review / security_review 注册为独立
  工具（名字即触发词，原生 tool-calling 直接可见），不再要求模型先想起技能名再走
  run_skill
- **技能蒸馏转子代理**：ui-ux-pro-max（设计检索）/ slides / banner-design / brand /
  design-system 5 个技能声明 `runas: subagent` + `allowed-tools` + Subagent mode 自包含
  任务说明；bundled 分发源同步，新环境自动获得 subagent 版
- **ui_styling 工具**：封装 tailwind_config_gen 脚本执行 + references 指南关键词
  内容检索（文件名优先，全文匹配返回上下文片段）
- **design_router 工具**：设计任务路由到对应子代理（banner/slides/brand/
  design-system/ui-ux-pro-max/ui_styling/explore），返回结构化派发建议
- **技能索引去重**：已注册为独立工具的子代理技能不再进入系统提示 # Skills 索引
  （工具 schema 已含名称+描述），索引 12+ 条 → 7 行，省 ~150 tokens/请求
- **工具压缩默认开启**：Compact 默认 true，父级隐藏 codegraph 深度工具（6 个）、
  lsp_rename/lsp_completion、install_skill；可见工具 55 → 46，schema tokens
  4,345 → 3,590（-17%）；隐藏工具在 explore 子代理内仍全部可用

### 🔧 修复 edit_lines 吞行

- 文件末尾存在空行（如 "a\nb\n\n"）时编辑中间行，末尾空行被吞——重建条件把
  "空行"误判为"已有尾随换行"。修复 + 2 个回归测试（LF/CRLF 变体）

### 💰 DeepSeek 峰谷计价

- V4 正式版高峰时段（北京时间 9:00-12:00、14:00-18:00）所有计费项价格翻倍；
  Pricing 新增 PeakMultiplier（TOML `peak_multiplier`），统计面板 costUsd、
  预算门控、CLI 状态行全链路按调用时刻生效；mimo 等未配置 provider 行为不变

### 🧠 记忆系统增强

- 待确认记忆超过 30 条自动提炼为 1 条（CondensePending，聚合后原候选移除）
- 记忆面板建议项（pending + suggestions）点击展开详情（body + evidence + scope）；
  技能候选项新增中文说明（descriptionZh）

### 验证

- `go test` 全绿（boot/skill/control/agent/tool/cache/config/desktop），vet 干净，
  `tsc --noEmit` 0 错误，vitest 66/66，主模块与 desktop 构建 EXIT 0

---

## [10.146.0] — 2026-08-01

### 🔧 修复 edit_lines 报 "start_line must be >= 1" — compact schema 丢失 minimum 约束

> 用户反馈：edit_lines 反复报 start_line >= 1 错误，疑似前端 bug。排查后确认
> **不是前端问题**——是模型看到的工具 Schema（压缩版）缺失行号约束，导致模型
> 漏传 start_line 或传 0。

#### 根因（5 级追溯）
- 表象：模型调用 edit_lines 报 `start_line must be >= 1`。
- 直接原因：模型参数里没有 start_line（或为 0）——反序列化后触发校验。
- 本地根因：模型看到的 **CompactSchema**（compact.go）里 `start_line`/`end_line`
  只有 `{"type":"integer"}`，**没有 `minimum:1`**；完整 Schema（editlines.go）
  有 minimum，但模型每轮看到的是压缩版（省 ~75% token）。
- 系统根因：compact schema 是手写压缩，压缩过程中丢掉了 minimum 约束——
  模型无法从 schema 感知"行号必须 ≥1"，偶尔漏传/传 0。
- 过程根因：compact.go 引入时按"省 token"优先精简字段，未对照完整 Schema
  逐字段保留约束（description 可删，minimum 不可删）。

#### 修复
- **`tool/builtin/compact.go`**：edit_lines 的 CompactSchema 中 start_line/end_line
  补回 `"minimum":1`——模型看到的 schema 与完整版约束一致

#### 测试
- **`tool/builtin/compact_wiring_test.go`**：`TestEditLinesCompactSchemaMinimum`
  （新增）——断言 compact schema 必须包含 `"minimum":1`（RED→GREEN），防止
  未来压缩 schema 再次丢失行号约束

#### 验证
- `go test ./internal/tool/builtin/ -count=1` 全 ok；`go vet` 无告警；
  `go build ./...` EXIT 0

---

## [10.145.0] — 2026-08-01

### 🧹 全面清理废弃残留 — web 前端目录 + 根目录杂物

> 用户反馈「先清理工作空间，清除淘汰的文件和版本，减轻空间负担」。主动盘点
> 发现 V10.101 移动端移除后遗留的废弃 `web/` 前端目录（零业务引用），以及
> 根目录的临时文件与备份包。

#### 清理内容
- **删除 `tianxuan/web/`**（11 文件 4761 行）：废弃前端残留——V10.101 移动端
  移除后未清理；`serve` 命令实际使用 `internal/serve/webui/`（另一套编译产物）；
  `web/src/bridge.ts` 含 `Compact` 键重复定义的代码异味（改一个漏一个会出 bug）
- **删除根目录 `query`**（360 卸载排查的一次性残留）
- **删除 `tianxuan-source.tar.gz`**（43MB 源码备份包——仓库本身是 git）
- **`tools/` 加入 .gitignore**：Go/Node 构建工具链（构建脚本依赖，绝不能删，
  但不应入库）

#### 验证
- `go build ./...` EXIT 0；desktop `go build ./...` EXIT 0
- `tsc --noEmit` 0 错误；vitest 61/61 全绿（web/ 不参与构建，删除无影响）

---

## [10.144.0] — 2026-08-01

### 🖱️ 修复对话输出时无法滚动查看前面 — rAF 与 React onScroll 竞争

> 用户反复反馈的痛点（Dream 主题第 1 条）：对话流式输出时无法滑动滚轮查看
> 前面内容，只能等输出完。根因是滚动跟随逻辑的时序竞争——不是滚动被禁止，
> 而是刚向上滚就被拽回底部。

#### 根因（5 级追溯）
- 表象：流式输出期间滚轮上滚无效，位置立即被拉回底部，只能等输出完成。
- 直接原因：`scrollToBottom` 的 rAF 回调无条件执行 `el.scrollTop = el.scrollHeight`
  （只要 `stick.current` 为 true）；而 `stick` 由 React 合成 `onScroll` 事件更新
  （委托到根节点、异步批处理）。流式输出时每个 chunk 更新 items → contentVersion
  变化 → 排新的 rAF。
- 本地根因：rAF 回调与 React scroll 事件存在窗口期——用户滚轮后浏览器原生滚动
  已发生（scrollTop 已变），但 React 的 onScroll 尚未执行，`stick` 仍为 true；
  下一帧 rAF 抢先执行把位置拽回底部。
- 系统根因：决策只依赖 `stick` 标志（异步更新），未校验真实 DOM 位置；rAF 是
  原生时机、onScroll 是 React 合成时机，两者不同步。
- 过程根因：V10.87 引入的滚动逻辑只测试了"用户滚到底部后 stick 恢复"的正常路径，
  未覆盖"输出中 rAF 抢跑"的高频竞争路径。

#### 修复
- **`lib/scrollFollow.ts`**（新增）：滚动决策提取为纯函数——`distanceToBottom` /
  `isNearBottom` / `shouldFollowAfterGrow(stick, scrollTop, scrollHeight,
  clientHeight)`。核心规则：**只有 stick 为 true 且真实位置仍在底部阈值内才跟随**；
  即使 stick 未及更新，只要用户真实位置已离开底部（rAF 抢跑场景），拒绝拽回。
- **`components/Transcript.tsx`**：`onScroll`/`scrollToBottom` 改用纯函数签名；
  rAF 回调执行前用真实 `scrollTop/scrollHeight/clientHeight` 做 `shouldFollowAfterGrow`
  决策，用户离开底部后不再被拉回。

#### 测试
- **`lib/scrollFollow.test.ts`**（新增，9 用例）：距离计算、贴底阈值（<80px）、
  自定义阈值、stick=false 绝不跟随、stick=true 贴底跟随、**rAF 抢跑回归**
  （stick 仍 true 但真实位置离开底部 → 不跟随）、内容增长贴底跟随、空容器、
  阈值常量导出稳定。

#### 验证
- `tsc --noEmit` 0 错误；vitest 61/61 全绿（4 文件，含新增 9 用例）；
  前端 build 通过（Wails 打包含前端编译）

---

## [10.143.0] — 2026-08-01

### 🧠 记忆提取质量修复 — 控制块泄漏 + 重复候选堆积

> 用户反馈驱动：pending 待确认区堆积 10 个候选，大量是控制块泄漏、重复规则、
> 半截噪声。定位到两个根因：(1) `isTransientBlock` 只做前缀匹配（HasPrefix），
> 宿主在用户消息尾部追加的 `<memory-update>` 块（如 "默认是不是中文 <memory-update>..."）
> 逃过过滤，连同 "默认" 标记一起被提取成记忆候选；(2) 去重基准只查 active memory，
> 不查 pending 内已有候选，且只做包含式匹配——同一规则换措辞重复出现（go build
> 三兄弟）不断堆积新 pending 文件。

#### 修复
- **`memory/extract.go`**：`isTransientBlock` 尖括号控制块（`<memory-update>`/
  `<session-facts>`/`<background-jobs>`）改为任意位置匹配；`[auto-recall]`/`[system]`
  保留前缀语义（用户行文中可出现方括号字样）
- **去重基准合入 pending**：`ExtractCandidates` 的 existing 集合追加 `PendingCandidates`
  描述——新 session 重述同一规则不再重复 staging
- **`sharesCorePhrase`**：`existingCovers` 增加 ≥10 runes 公共子串检测，覆盖换措辞
  重复（"go build + 受影响包测试" 类核心短语）
- **`desktop/memory_suggestions.go`**：同源修复——`suggestMemories` 补 transient 过滤 +
  公共子串去重（此前桌面建议入口同样泄漏 `<memory-update>` 块）

#### 测试
- **`memory/extract_test.go`**：`TestExtractSkipsEmbeddedTransientBlocks`（嵌入控制块
  不得泄漏，RED→GREEN）、`TestExtractDedupAgainstPending`（pending 内候选跨 session
  去重）、`TestExtractCursorAdvance` 更新（新 session 提取新消息、重复消息不堆积）
- **`desktop/memory_suggestions_test.go`**：`TestIsTransientBlockEmbedded`、
  `TestSharesCorePhrase`

#### 清理
- pending 待确认区 10 → 2：删除控制块泄漏（memory-update 候选）、半截噪声
  （bash-shell / candidate-*）、重复规则（go-go-build 变体）、已被记忆覆盖项
  （codex 清理瞬时状态 / git PATH）；保留 Dream 机制产物 + go-build 规则候选

#### 验证
- `go build ./...` EXIT 0；`go vet ./internal/memory/` 无告警；
  `go test ./internal/memory/ ./internal/control/` 全 ok；desktop `go test ./...` 全 ok

---

## [10.141.0] — 2026-08-01

### 🔧 验证分级 — 简单修改/前端修改不再强制后端全量测试套件

> 用户反馈：每轮完成时系统强制要求运行完整测试套件（go test ./...）太过机械
> ——简单修改、前端修改也要跑后端全量测试，成本和反馈延迟不成比例。本次把
> 验证改为按变更范围分级。

#### 分级规则
| 变更范围 | 验证方式 |
|----------|----------|
| Go 代码 | `go build` + 受影响包测试（`go test ./<pkg>`） |
| 前端文件（.ts/.tsx/.css 等） | `tsc --noEmit` / 前端测试 / build，**不需要后端测试** |
| 文档/配置（.md/.toml/.json） | 内容核对，不强制测试 |
| 跨模块/重构 | 全量套件（`go test ./...` 或等价） |

#### 改动
- **`agent/stop_gate.go`**：verify_gate 强制提示文案改为分级验证——模型完成前
  按本次变更选择匹配的验证方式，只有对应检查通过才允许结束
- **`agent/hermes_prompt.go`**：SoloSystemPrompt 的 Pre-completion checklist 与
  Complete 段落、HephaestusSystemPrompt 的完成前检查同步为分级规则

#### 测试
- **`agent/scoped_verify_test.go`**（新增）：verify_gate nudge 与 Solo/Hephaestus
  提示词必须包含分级验证关键词（matches the change / affected package tests /
  frontend / no backend tests needed / docs/config / full suite）

#### 验证
- `go build ./...` EXIT 0；`go vet ./...` 无告警；`go test ./...` 全 ok

---

## [10.140.0] — 2026-08-01

### 🐛 修复单模型下技能几乎不触发 — 自动触发关键词扩展 + inline 技能主动调用

> 用户反馈：最新桌面端单模型下完全不用技能。系统性排查技能全链路（bundled
> 技能同步 ✓、技能索引注入 ✓、withAutoSkill 机制 ✓、统计事件 ✓），定位到
> 根因：自动触发关键词覆盖最普遍的编程措辞——"修复缓存问题"、"这个功能不
> 工作"、"补个测试用例" 全部不命中（规则只有"修 bug/调试/排查/写测试用例"
> 等窄词），模型也不会主动 run_skill，技能自然零使用。

#### 根因（5 级追溯）
- 表象：单模型运行中技能使用为 0。
- 直接原因：`autoTriggerRules` 关键词覆盖窄——"修复 xxx"（最常见的编程任务）
  不含"修 bug/调试/排查"，不触发 systematic-debugging 自动注入。
- 本地根因：规则词表以"调试场景黑话"为主（崩溃/报错/排查），未覆盖任务型
  措辞（修复/故障/不工作/失败）。
- 系统根因：技能使用依赖"自动注入命中 + 模型主动 run_skill"双通道；自动注入
  命中面窄，而提示词没有要求模型在未命中时主动 run_skill inline 技能。
- 过程根因：V10.122 引入自动触发时词表按当时测试用例设计，未按真实任务分布
  扩展。

#### 修复
- **`skill/autotrigger.go`**：systematic-debugging 关键词扩展——加 "修复"、
  "bug"、"故障"、"出错"、"不工作"、"失败"、"修一下"；tdd 加 "补测试"、
  "测试用例"、"写个测试"。误触发代价低（注入的是引导性 playbook，正文截断
  2000 字符）
- **`agent/hermes_prompt.go`** SoloSystemPrompt 新增 inline 技能主动调用规则：
  tdd / systematic-debugging / requesting-code-review 等任务匹配时**主动
  run_skill** 获取完整 playbook——自动注入只覆盖关键词命中的高频场景，未命中
  但任务匹配仍要调用，不凭记忆省略技能步骤

#### 测试
- **`skill/autotrigger_coverage_test.go`**（新增）：8 组常见措辞断言——
  "修复缓存问题"/"修复这个 bug"/"修一下登录报错"→ systematic-debugging；
  "用 TDD 实现"/"先写测试再实现"/"补个测试用例"→ tdd；"审查一下这次改动"
  → requesting-code-review（修复前 RED：4 组不命中）
- **`agent/adaptive_workflow_test.go`**：Solo 提示词 inline 技能主动调用契约

#### 验证
- `go build ./...` EXIT 0；`go vet ./...` 无告警；`go test ./...` 全 ok

---

## [10.139.0] — 2026-08-01

### 🧵 子代理并行优先 — 调查默认走子代理，主上下文只留决策/实现/验证

> 问题：tianxuan 几乎不使用子代理，主 agent 自己用 read_file/grep 做大批量
> 调查，中间信息（多文件内容、大文件、搜索细节）永久堆积在主上下文——而这些
> 信息本不需要传递到上下文，只需结论。本次把"调查默认子代理"写成工作流规则，
> 并加宿主信号兜底。

#### 信息分类原则（核心）
- **需要进主上下文**：决策、实现、验证所需的信息
- **不需要进主上下文**：批量调查的中间信息（多文件阅读、大文件内容、深度
  搜索、外部研究）——由子代理在隔离上下文消化，只回传精炼结论（file:line 锚点）

#### 提示词强化（三个提示词统一）
- **`hermes_prompt.go`** SoloSystemPrompt 的 Sub-agents 重写为
  "default for investigation"：3+ 文件/跨模块/深度搜索/外部研究 → run_skill
  派发 explore/research；多个独立调查 → parallel_skills 并行；主上下文只用于
  单文件定位（≤2）/决策/实现/验证；子代理结论是事实锚点
- **HephaestusSystemPrompt**：执行阶段的编辑锚点/调用链调查同样子代理优先，
  禁止在主上下文批量 read_file（上下文留给实现与验证）
- **HermesPrompt**：规划阶段 2+ 独立调查必须并行派发，禁止顺序铺开大调查

#### 宿主信号（兜底）
- **`agent/investigation_nudge.go`**（新增）：`maybeNudgeInvestigation`——单轮
  调查类工具调用 ≥8（read_file/grep/glob/ls/code_index/web/codegraph）且无写
  工具（纯调查轮）→ 注入"改用 explore 子代理 + parallel_skills"引导；每 turn
  上限 3 次（`InvestigationNudgeThreshold` / `InvestigationNudgeCap`）
- **`agent_run.go`**：工具批次后接线 + turn 开始重置计数

#### 测试
- **`agent/investigation_nudge_test.go`**（新增）：阈值触发、低于阈值静默、
  混入写工具不触发（执行非调查）、per-turn 上限
- **`agent/subagent_priority_test.go`**（新增）：Solo/Hephaestus/Hermes 提示词
  包含子代理优先关键词
- **`adaptive_workflow_test.go`**：Solo 子代理默认调查契约

#### 验证
- `go build ./...` EXIT 0；`go vet ./...` 无告警；`go test ./...` 全 ok

---

## [10.138.0] — 2026-08-01

### 🔧 构建脚本自动补齐 PATH — build-desktop.bat 不再依赖手动环境设置

> V10.137 发布时首次遇到：bat 直接运行时前端编译报 `'node' is not recognized`，
> 必须手动把 Go/Wails/Node 路径加入 PATH 才能构建。本次把环境探测写进脚本。

#### 修复
- **`build-desktop.bat`**：脚本开头自动补齐构建 PATH——存在 `tools\go\bin`、
  `tools\node`（node.exe / pnpm.cmd）则加入；`where wails` 找不到时回退探测
  `%USERPROFILE%\go\bin\wails.exe`。双击/直接运行即可构建，无需手动环境
- 注意：bat 为 ANSI 编码环境，注释必须纯 ASCII（中文注释会导致 cmd 解析乱码，
  首次尝试已踩坑回退）

#### 验证
- 无手动 PATH 环境下 `build-desktop.bat` 直接运行：`[sync] OK` +
  Wails 全流程 Done，`Built ... tianxuan-desktop.exe`，EXIT 0

---

## [10.137.0] — 2026-08-01

### 🎯 单模型中途纠偏 — 运行中输入即时注入当前任务（steer）

> 需求：单模型执行过程中，用户可中途输入信息纠偏（"跳过测试"、"改成方案 B"），
> 无需等任务结束，也无需取消重来。Agent 层 `Steer` 机制早已存在（V1.12 蒸馏），
> 但 Controller 无接口、前端无入口——运行中输入被排队等任务结束，纠偏能力从未
> 暴露。本次全链路接通。

#### 实现
- **`control/controller.go`**：新增 `Steer(input)`——运行中注入 executor 的
  steer 队列（下一模型步骤生效，不取消不重启），空闲时回退为 `Send`（正常开
  新轮）；注入时 emit Notice 让用户看到"纠偏已注入"
- **`desktop/app_submit.go`**：Wails 绑定 `Steer`（前端可调用）
- **`desktop/frontend/src/lib/bridge.ts`** / **`mock.ts`**：AppBindings 加
  `Steer(input)`（类型断言保证与 Go 方法同步）
- **`desktop/frontend/src/components/Composer.tsx`**：运行中发送（Enter/按钮）
  从"排队等任务结束"改为**立即注入纠偏**（`app.Steer`）；移除排队列表 UI 与
  队列状态；Shift+Enter 取消+重发保留
- **`desktop/frontend/src/lib/store.ts`** / **`types.ts`**：`steer` 事件渲染为
  用户消息（用户在 transcript 中能看到自己发的纠偏内容）+ Notice
- **locales**：zh / zh-TW / en 的占位符与按钮文案更新为纠偏语义

#### 交互语义
- 运行中：Enter 发送 = 纠偏指令，当前步骤完成后生效（前缀提示"不要当作新任务，
  仅作为当前任务的附加指导"）
- Shift+Enter：取消当前轮 + 立即重发（激进纠偏，保留原能力）
- 空闲时：正常新任务

#### 测试
- **`control/steer_test.go`**（新增）：运行中 Steer 注入 executor 队列且不启动
  新 turn；空闲 Steer 回退 Send
- 前端：`tsc --noEmit` EXIT 0；vitest 52 用例全过；`go build ./...` /
  `go vet ./...` / `go test ./...` 全绿

---

## [10.136.0] — 2026-08-01

### 🔧 Adaptive Execution 细节打磨 — 同步骤失败宿主信号 + Solo 进度保护

> 延续 V10.135 的单模型工作流：把 Failure adaptation 从"纯提示词自觉"升级为
> "宿主检测 + 提示词兜底"，并补齐单模型缺失的进度保护。

#### 细节 1 — 宿主级"同步骤持续失败"信号（V10.136）
> 缺口：现有防护只覆盖"相同动作重复"（detectRepeatedSteps）与"同工具同错误"
> （风暴断路）。单模型在同一步骤内用**不同动作**反复失败（read→edit→test
> 各失败一次）时没有任何宿主信号，只能等模型自觉换方案。
- **`agent/todo_step_nudge.go`**（新增）：`maybeNudgeStuckTodoStep`——跟踪当前
  in_progress todo 步骤的失败轮次：每轮存在（非权限阻断的）工具失败则累计，
  同一步骤连续 4 轮失败注入 Adaptive 引导（诊断根因→todo 调整→换方案→ask）；
  todo 步骤变化或成功轮重置计数
- **`agent/agent.go`**：`todoFailStep`/`todoFailCount` 状态字段
- **`agent/agent_run.go`**：工具批次处理后接线（与 maybeInjectToolFeedback
  互补：后者单轮多失败，本检测跨轮累计同一步骤）
- **`agent/loop_limits.go`**：`TodoStepFailNudgeThreshold = 4`

#### 细节 2 — Solo 进度保护（Progress guard）
> 缺口：双模型执行者有提示词级 progress guard（8 轮无进展重新评估、16 轮交还
> Hermes）；单模型（Solo）完全没有——模型可能陷入"既不成功也不失败"的循环
> （反复读文件不行动、重复无意义操作）。
- **`hermes_prompt.go`** SoloSystemPrompt 新增 Progress guard：连续 8 轮无新
  完成/读取/命令/变更 → 重新评估 todo（签收/换方案/缩小范围/ask）；连续 16 轮
  无进展 → 暂停并 ask 或报告阻塞。收敛对象是自己（Adaptive），不是"交还 Hermes"

#### 测试
- **`agent/todo_step_nudge_test.go`**（新增）：阈值触发、步骤切换重置、成功轮
  中断、无 todo 不触发
- **`agent/adaptive_workflow_test.go`**：新增 Progress guard 契约（8/16 轮关键词
  + 不含"交还 Hermes"）

#### 验证
- `go build ./...` EXIT 0；`go vet ./...` 无告警；`go test ./...` 全 ok

---

## [10.135.0] — 2026-08-01

### 🔄 单模型工作流重定义 — Adaptive Execution（回退 V10.134 AutoPlan）

> 方向修正：单模型不应复制双模型的"完整计划→确认→执行"工作流——两者分开的
> 意义在于**不同的编程工作流**。双模型 = 规划质量优先（分工、计划即合同）；
> 单模型 = 迭代速度优先（零交接损耗、计划随执行滚动更新）。V10.134 的 AutoPlan
> 计划确认接线是双模型流程的复制，本次回退，并落地单模型专属的 Adaptive
> Execution 工作流。

#### 回退 V10.134（AutoPlan 计划确认）
- **`control/controller.go`**：撤销 Options.AutoPlan、计划模式进入/退出、
  Ask 批准钩子、autoPlanEnabled/maybeExitPlanMode
- **`agent/planner_route.go`**：移除 `ShouldAutoPlan`
- **`boot/boot.go`**：移除 AutoPlan 传参；`agent/agent.go`：移除 PlanMode getter
- **`config/config.go`** / **`tianxuan.example.toml`**：auto_plan 标注为
  reserved（未接线）——单模型用 Adaptive Execution，双模型用 Hermes 自带确认
- **`planmode/policy.go`**：Marker 移除 ask 批准指引（恢复纯计划指令）；
  保留 V10.134 的修正——read_only_task/read_only_skill 是实际不存在的工具，
  改为只读工具（read_file/grep/glob/web_search）调查指引，并明确计划模式禁止
  派发子代理

#### 单模型新工作流：Adaptive Execution（提示词层）
> 核心差异一句话：双模型"先想清楚再动手（plan→execute）"；单模型"小步前进、
> 边做边学、计划随执行滚动更新（learn→do→adapt）"。
- **`hermes_prompt.go`** SoloSystemPrompt 的 Workflow 重写：
  1. **Skeleton plan** — todo_write 3–10 步骨架，每步可独立验证；**无计划确认
     往返**，直接开始执行；ask 仅用于真正决策岔路
  2. **Adaptive loop** — 每步 TDD（失败测试→最小实现→验证→complete_step）后
     **Adapt**：执行中发现更优方案/新约束/依赖 → 更新 todo（拆分/合并/重排/
     补充）再继续；"计划是活文档不是合同"
  3. **Failure adaptation** — 分级升级：1 次失败读错误重试 → 同方案 2 次失败
     停下诊断根因（reproduce→isolate）→ 3 次失败换方案（更新 todo 改路径/
     工具/范围，注明原因）→ 换方案仍停滞则收敛（ask 或缩小到可交付子集）
  4. **Complete** — 全部签收后全量测试 + vet + 回归
- 明确"没有规划者需要汇报，调整计划不需要额外流程"——与双模型"执行者禁止改
  计划（NEVER write a new plan）"形成对照

#### 宿主机制
- 无需新增：todo 推进、verify_gate、重复检测（3 次相同步骤 nudge 换策略）、
  风暴断路、finalReadinessCheck 已支撑 Adaptive loop 与 Failure adaptation

#### 测试
- **`agent/adaptive_workflow_test.go`**（新增）：SoloSystemPrompt 必须包含
  Adaptive Execution 关键词（living document / 无计划确认 / Adapt / 换方案 /
  根因诊断 / 收敛），且不得携带双模型"计划即合同"措辞
- **`planmode/policy_test.go`**：改为断言 Marker 只指向真实只读工具、不含
  不存在的 read_only_task/read_only_skill
- 移除 V10.134 的 auto_plan/policy/controller 测试

#### 验证
- `go build ./...` EXIT 0；`go vet ./...` 无告警；`go test ./...` 全 ok

---

## [10.134.0] — 2026-08-01

### 🧠 单模型架构优化 — AutoPlan 接线：复杂任务自动计划→确认→执行

> 单模型（Solo）架构的系统性缺口：planmode 计划模式基础设施（只读门 + Marker）
> 与 auto_plan 配置（off|ask|on）全部是死代码——SetPlanMode 无调用者、Marker
> 无注入点、AutoPlan 无读取方，用户配置 auto_plan 完全无效。本次接线完整激活。

#### 根因（5 级追溯）
- 表象：单模型模式下复杂任务直接执行，没有计划确认环节；配置 auto_plan 无效。
- 直接原因：`planmode.Marker` / `SetPlanMode` / `AutoPlan` 三者各自实现完整，
  但没有任何调用链把它们连起来。
- 本地根因：V10.22 蒸馏 Plan Mode（Claude Code）时只移植了门与策略，控制器
  层没有"何时进入计划模式"的决策。
- 系统根因：双模型模式有 Hermes 自带计划确认，单模型模式缺少同等的宿主级
  规划阶段；AutoPlan 配置因此沦为占位。
- 过程根因：计划模式随双模型开发引入，Solo 路径从未被同一套机制覆盖。

#### 实现
- **`agent/planner_route.go`**：新增 `ShouldAutoPlan(input, mode)`——复用
  DecidePlannerRoute，仅复杂/多步骤任务（RoutePlanAndExec：high_risk /
  cross_surface / structured / complex）进入计划模式；原子编辑/只读/指令/
  聊天保持直接执行
- **`control/controller.go`**：`Options.AutoPlan` 接线；交互模式下 Solo 模式
  （runner 非 Hermes）复杂任务自动 `SetPlanMode(true)` 并注入 `planmode.Marker`；
  计划模式严格限定本 turn（defer 退出，无跨轮泄漏）；`Ask` 增加批准钩子——
  用户选择"提交执行"即退出只读门，同轮继续执行；"按意见修改"保持计划模式
  重新规划；"取消"结束
- **`planmode/policy.go`**：Marker 补充 ask 批准工作流（输出计划后必须调 ask
  请求批准，选项固定为 提交执行/按意见修改/取消）；顺带修正 Marker 指引的
  `read_only_task`/`read_only_skill` 为实际不存在的工具——改为只读工具
  （read_file/grep/glob/web_search）自行调查，并明确计划模式下禁止派发子代理
- **`boot/boot.go`**：`cfg.Agent.AutoPlan` 传入 controller

#### 启用方式
- `config.toml` 的 `[agent]` 加 `auto_plan = "ask"`（或 "on"）。交互桌面/Web
  模式下复杂任务会先进入计划模式：模型只读调查 → 输出分层计划 → ask 确认卡 →
  批准后同轮执行

#### 测试
- **`agent/auto_plan_test.go`**（新增）：off/空/非法模式禁用；ask/on + 复杂任务
  启用（high_risk/complex/cross_surface/multi-file）；简单任务（聊天/原子编辑/
  指令/只读）不启用
- **`planmode/policy_test.go`**（新增）：Marker 必须包含 ask 批准工作流关键词
- **`control/auto_plan_test.go`**（新增）：批准退出只读门 + autoPlanActive 复位；
  修改/取消保持计划门；非交互/off/nil executor 不启用
- `agent.go`：新增 `PlanMode()` getter 供断言与诊断

#### 验证
- `go build ./...` EXIT 0；`go vet ./...` 无告警；`go test ./...` 全 ok

---

## [10.133.0] — 2026-08-01

### 🐛 修复技能/子代理使用统计恒为 0 — 自动注入可观测 + Solo 子代理指引

> 用户反馈：发布版测试中技能使用为 0、子代理使用为 0。系统性调试：
> 梳理 工具注册（addBuiltins + boot 无条件注册）→ 技能索引注入 → 自动触发
> （withAutoSkill）→ run_skill → 子代理（NestedSink）→ 前端统计（useToolStats /
> StatsPanel）全链路，定位为两个可观测性盲区，而非工具缺失。

#### 根因（5 级追溯）
- 表象：技能侧栏与统计面板的技能/子代理使用恒为 0。
- 直接原因 1：V10.122 技能自动触发层把高频内联技能（tdd / systematic-debugging /
  requesting-code-review 等）的 playbook 直接注入输入，模型不再调用 run_skill；
  而前端统计只统计 run_skill 工具事件 → 技能实际被使用但统计恒 0。
- 直接原因 2：SoloSystemPrompt 的 Sub-agents 章节只写 "explore sub-agent /
  review sub-agent"，未说明子代理技能要通过 run_skill 显式派发（自动注入仅覆盖
  内联技能）→ 模型主动调用子代理技能率低。
- 本地根因：自动注入路径（withAutoSkill）不发出任何事件，注入对前端完全不可见；
  子代理技能缺少调用指引。
- 系统根因：技能统计以"run_skill 调用"为唯一信号，没有覆盖"自动注入"这条
  使用路径。
- 过程根因：V10.122 引入自动注入时只考虑了缓存与去重，未同步统计口径。

#### 修复
- **`agent_run.go`**：新增 `withAutoSkillEvent`（返回注入的技能名）与
  `emitAutoSkillEvent`——自动注入时发出合成的 run_skill 工具事件（ToolDispatch +
  ToolResult，args 含技能名）。事件只进前端统计，不进 session、不产生 API 流量、
  不影响缓存前缀；前端 toolCounts["run_skill"] 与 skillCounts["tdd"] 立即可见
- **`hermes_prompt.go`**：SoloSystemPrompt 的 Sub-agents 章节补充说明——子代理
  技能在技能索引中标记 [🧬 subagent]，通过 `run_skill(name, arguments)` 显式派发，
  自动注入只覆盖内联技能

#### 测试
- **`skill_stats_test.go`**（新增）：`TestAutoSkillInjectionEmitsStatEvent`——
  "用 TDD 实现这个功能" 触发自动注入后必须发出含技能名的 run_skill 统计事件
  （修复前 RED：无事件）；`TestSoloSystemPrompt_GuidesSubagentRunSkill`——
  提示词必须包含 run_skill / [🧬 subagent] / explore / review 指引
- 既有 `TestWithAutoSkillDedup` / `TestCacheHitPrefixStableWithAutoSkill` 保持通过
  （去重与缓存前缀不变性未破坏）

#### 验证
- 修复前：自动注入无任何事件（RED 复现）；修复后：合成 run_skill 事件可统计。
- `go build ./...` EXIT 0；`go vet ./...` 无告警；`go test ./...` 全 ok

---

## [10.132.0] — 2026-08-01

### 🐛 修复执行时代办进度落后于实际完成 — complete_step 步骤匹配容错

> 用户反馈：大部分计划执行完了，代办弹窗进度才走到一半。系统性调试：
> 先梳理 todo 推进链路（todo_write → complete_step → advanceCanonicalTodo /
> rebuildTodoState / verifyTodoStep），写复现测试确认根因，再修复。

#### 根因（5 级追溯）
- 表象：计划执行完成但代办面板只推进一部分。
- 直接原因：`matchTodoStep` 对 complete_step.step 的解析只支持**纯数字索引**
  （"2"、"2."），标题匹配是**完全相等**（TrimSpace + EqualFold）。
- 本地根因：执行者按提示词习惯写 "步骤 2：标题" / "Step 3" / "步骤2:标题"，
  或一侧带编号前缀一侧用纯标题（todo Content="实现功能"，step="步骤 2：实现功能"），
  两种写法都无法命中 todo 项。
- 系统根因：todo 匹配逻辑假定模型两侧用词完全一致，没有考虑编号前缀是计划
  格式（"步骤 N："）的自然产物。
- 过程根因：V10.99 引入 step_index 建议时只加了新字段，未增强既有 step 解析。

#### 修复
- **`evidence.go`** `parseStepIndex`：支持中英文编号前缀——"步骤 2"、"步骤2:标题"、
  "Step 3: verify"、"step 3" 均解析为索引；纯数字（"2"、"2."、"2:"）保持。
  标题中间的编号（"更新 file2.txt"）不会误当作索引
- **`evidence.go`** `sameStepText`：比较前剥离任一侧的 "步骤 N："/"Step N:" 前缀
  ——todo 带前缀而 step 用纯标题（或反向）也能精确匹配；仍是精确比较，
  不引入包含匹配（防止短标题误推进错误任务）
- **`completestep.go`** `verifyTodoStep`：严格模式下匹配失败时，报错附带当前
  任务列表（`1="写失败测试", 2="实现功能"`）与 step_index 提示——模型重试时
  不再瞎猜，直接引用列表中的标题或编号

#### 测试
- **`evidence/todo_match_test.go`**（新增）：中文/英文编号前缀解析、前缀标题 vs
  纯标题双向匹配、纯数字兼容、防过度匹配（子串不匹配、标题中间数字不当索引）
- **`tool/builtin/completestep_test.go`**：新增不匹配报错列出任务列表的用例

#### 验证
- 修复前：`MatchStep("步骤 2：实现功能", todos)` 返回 found=false（复现 RED）
- 修复后：全部匹配；`go build ./...` EXIT 0；`go vet ./...` 无告警；
  `go test ./...` 全 ok

---

## [10.131.0] — 2026-08-01

### 🔄 双模型工作流闭环 II — 步骤覆盖度对照 / 修正计划补全漏签收步骤

> 工作流深挖：规划者此前只能从 StepResults 数量与文件维度判断执行质量，
> 无法感知"计划有 3 步但执行者只签收了 2 步且全部成功"（合并步骤是允许的，
> 所以不能硬校验数量）。新增步骤标题的软对照：完全无匹配的步骤进入 verify
> 反馈，并强制进入修正计划。TDD，`go build` / `go vet` / `go test ./...` 全绿。

#### W4 — 步骤覆盖度对照（coverage 维度）
> 根因：执行者可能跳过计划中的某一步或全部改写步骤标题，StepResults 仍全
> success → allStepsPassed=true，漏掉的部分永远不会被修复（V10.128 为防
> 合并误伤放弃了数量硬校验，留下这个盲区）。
- **`planparse.go`**：新增 `extractPlanStepTitles` / `normalizeStepTitle` /
  `similarStepTitle` / `checkStepCoverage`——归一化（小写、去"步骤 N："编号
  与图标、压缩空白）后做包含匹配（容忍"实现+重构"这类合并与措辞微调），
  返回计划中存在但零签收匹配的步骤标题
- **`hermes_sdd.go`**：`buildVerifyTriad` 从三维扩为四维——`coverage=ok|warn(N)`，
  仅在有 StepResults 且计划含步骤时输出；warn 是信号不是失败判定（合并/改写
  合法，由规划者判断）
- **`hermes.go`**：`buildFixPrompt` 在失败步骤之外附加"计划中存在但执行未签收
  的步骤"清单——修正计划必须覆盖，否则漏掉的步骤永远不会被修复
- **`hermes_prompt.go`**：规划者提示词的 verify 说明同步为四元组

#### 测试
- **`plan_coverage_test.go`**（新增）：全匹配（含无编号/半角冒号变体）、合并
  步骤覆盖两个标题、跳过步骤被检出、nil/空计划/无签收边界、triad 输出
  coverage=warn、修正计划包含未签收步骤清单

#### 验证
- `go build ./...` EXIT 0；`go vet ./...` 无告警；`go test ./...` 全 ok

---

## [10.130.0] — 2026-08-01

### 🔄 双模型工作流闭环 — 直接执行回灌规划者 / 修正轮次标识 / 修正计划可见

> 本次聚焦规划→确认→执行→修复→反馈的工作流闭环，修复三处断裂：
> 直接执行结果对规划者不可见（"继续"类请求丢上下文）、修复轮次反馈无法区分、
> 自动修正对用户不透明。全部 TDD，`go build` / `go vet` / `go test ./...` 全绿。

#### W1 — 直接执行结果回灌规划者（多轮连续性）
> 根因（5 级追溯）：表象 = 用户执行简单任务后说"继续"，规划者答非所问。直接原因 =
> auto-skip（atomic/read_only/directive）与 `!` 快速路径直接调 executor，结果只
> 发前端事件，从不注入 `hermesSess`。本地根因 = 这两条路径被设计为"绕过规划者"
> 时只考虑了省成本，没考虑后续对话需要上下文。系统根因 = 回灌（feedResultToPlanner）
> 只挂在完整执行路径上，快速路径没有等价闭环。过程根因 = V10.31/V10.102 引入快速
> 路径时未把"执行结果必须回到规划者 session"作为不变式。
- **`hermes.go`**：`runFastPath` 与 auto-skip 分支执行成功后调用
  `feedResultToPlanner(execResult, 1)`——规划者 session 记录 [上一轮执行结果]，
  下一轮"继续"、"接着改刚才的文件"等请求有上下文锚点

#### W2 — 修正轮次反馈标识
> 根因：自动修复轮次（round 2/3）的执行反馈与原始执行格式相同，规划者看到多条
> [上一轮执行结果] 无法区分时间线。
- **`hermes.go`**：`feedResultToPlanner(r, round)` 新增轮次参数；`executePlan` 透传
  round（原始执行=1，修复执行=2/3）；round>1 时反馈以 `[第 N 轮修正执行结果]`
  开头，规划者能明确区分原始执行与每次修复

#### W3 — 自动修正计划对用户可见
> 根因：自动修正完全在后台进行，前端只有一行 Phase "修正执行 (轮 2/3)"，用户
> 不知道规划者本轮打算修什么。
- **`hermes.go`**：修复计划生成后 emit Text 事件，展示 `<!--plan-->` 后的计划
  摘要（截断 800 runes）——用户看到"🔧 自动修正计划（轮 N）"再配合最终结果卡

#### 测试
- **`workflow_test.go`**（新增）：`TestHermesDirectExecutionFeedsPlanner`（auto-skip
  后 planner session 含执行反馈且规划者零调用）、`TestHermesFastPathFeedsPlanner`、
  `TestHermesFeedResultToPlanner_RoundMarker`（round=2 带轮次标识、round=1 保持
  旧格式）

#### 验证
- `go build ./...` EXIT 0；`go vet ./...` 无告警；`go test ./...` 全 ok

---

## [10.129.0] — 2026-08-01

### 🧠 双模型架构优化 II — 漏标记补偿 / 路由三值化 / 规划者步数默认上限

> 承接 V10.128.0 的评审清单：`<!--plan-->` 漏标记静默吞任务（P1）、
> RouteExecOnly 语义重载（P2）、planner_max_steps 无默认上限（P2）、
> 只读注册表注释债务（P3）。全部 TDD，`go build` / `go vet` / `go test ./...` 全绿。

#### O4 — `<!--plan-->` 漏标记补偿检测（P1）
> 根因：`isAnswerNotAction` 只认 `<!--plan-->` 标记；规划者漏写标记时，计划被
> 当作聊天回复，任务**静默不执行**且无任何报错。
- **`hermes.go` / `planparse.go`**：新增 `looksLikePlan`——无标记但呈计划结构
  （≥2 个步骤行，或 1 个步骤行带 Verify/File(s)/Files/Delta 字段）→ 仍视为计划，
  走确认流程（交互模式用户可点"仅聊天"纠错；headless 自动确认直接执行）。
  普通聊天/回答不含"步骤 N："标题格式，不误伤

#### O5 — 路由三值化消除语义重载（P2）
> 根因：`RouteExecOnly` 同时承载"直接执行"与"规划者回答"两种语义，Run 只能靠
> reason 白名单（atomic_edit/read_only/directive）区分——枚举含义与执行行为
> 不一致，9 种 reason 中 6 种落入规划者但代码不表达这层意图。
- **`planner_route.go`**：新增 `RoutePlannerChat`（规划者直接回答，无计划无执行）；
  empty/slash_command/short_reply/conversation/low_risk_question/no_work 归入。
  `RouteExecOnly` 现在只表示"跳过规划者直接执行"
- **`hermes.go`**：Run 的 auto-skip 条件从 reason 白名单简化为 `Route == RouteExecOnly`
- 运行时行为不变（chat 类输入原来就由规划者处理），但语义自文档化
- **`planner_gate.go`**：词表一致性修复——`rename` 在 `planAtomicTerms` 却不在
  `planWorkTerms`/`planMutationTerms`，导致英文 rename 指令永远无法命中 atomic
  分支（work 前置条件不满足），实际走了 no_work

#### O6 — planner_max_steps 默认上限（P2）
> 根因：`planner_max_steps` 未配置时 0 = 无限轮只读调查，规划者没有自然停止
> 条件，成本无界。
- **`config.go`**：新增 `DefaultPlannerMaxSteps = 12` + `PlannerMaxStepsVal()`——
  未配置（0）时应用保守默认，显式大值仍可获得近似无限；`boot.go` 接线
- **`tianxuan.example.toml`**：注释同步

#### O7 — 注释债务（P3）
- **`boot.go`**：`newReadOnlyRegistry` 注释声称"子代理工具一律排除（保护缓存
  前缀）"，与实际实现（Boot 显式加回只读版 task/run_skill/parallel_skills）不符，
  更新为真实的安全理由：默认子代理工具集是全量注册表，会经 headlessGate 写文件

#### 测试
- **`plan_marker_test.go`**（新增）：漏标计划结构 → 非回答；无结构 → 回答；有标记 → 计划
- **`planner_route_chat_test.go`**（新增）：聊天/问题/空输入/斜杠 → planner_chat；
  原子编辑/只读/指令 → executor_only
- **`planner_max_steps_test.go`**（新增，config 包）：未配置 → 12；显式 5 → 5
- `hermes_test.go`：`DefaultIsExecutor` 改名 `NoWorkIsPlannerChat`（hello → planner_chat）；
  Directives 用例剔除不在 work 词表的"保存会话/提交代码"

#### 验证
- `go build ./...` EXIT 0；`go vet ./...` 无告警；`go test ./...` 全 ok

---

## [10.128.0] — 2026-08-01

### 🧠 双模型架构优化 — 规划者压缩保留 / 完成判定以计划为准 / 计划解析统一

> 三项目标明确：修复规划者压缩被快照恢复抵消的长期缺陷（P0，成本+上下文失控）、
> 修复完成判定掩盖未完成轮次（P0，maxSteps 中途停止被误判成功）、收敛三处重复的
> 计划步骤解析（P1，格式演进不同步风险）。全部 TDD：先写失败测试→确认 RED→
> 最小实现→GREEN，`go build ./...` / `go vet ./...` / `go test ./...` 全绿。

#### O1 — 规划者压缩 digest 不再被丢弃
> 根因（5 级追溯）：表象 = 长期会话中规划者上下文持续膨胀、超阈值后每轮重复出现
> compaction。直接原因 = `planWithConfirmation`/`planFix` 每次退出都
> `Replace(prePlanMsgs)`，把规划期间 `maybeCompact` 生成的 `<compaction-summary>`
> digest 连同中间规划消息一起还原掉。本地根因 = 快照/恢复模式把"压缩产物"当成
> 瞬态规划消息一并丢弃。系统根因 = 规划者会话没有获得与执行者一致的压缩语义
> （压缩触发于 AgentRunner.Run 内部，而会话恢复发生在 Hermes 编排层，两者无契约）。
> 过程根因 = V10.34 引入快照恢复时只考虑"中间计划不污染持久上下文"，未考虑压缩。
- **`hermes.go`**：新增 `restorePlannerBaseline`——退出时若 `RewriteVersion` 变化
  或消息数减少（发生过 compaction/trim），调用 `trimPlannerTurn` 只移除本轮规划
  追加的尾部消息、保留压缩 digest；否则维持原快照恢复。`planWithConfirmation`
  全部 8 个退出点与 `planFix` 统一走新逻辑
- 规划消息仍为瞬态（用户输入/中间计划不残留），但 digest 作为历史抽象存活，
  持久 session 可真正缩减，超阈值后不再每轮重复支付 summarizer 全量调用

#### O2 — allStepsPassed 以计划步数为 ground truth
> 根因：旧分支"无 StepResults + 文件变更 → pass"掩盖了 maxSteps 中途停止的轮次
> （文件改了一半、一个 complete_step 都没签收，却判成功，3 轮修复循环不触发）。
- **`hermes.go`**：`allStepsPassed(r, plan)` 新增计划参数；无 StepResults 时若计划
  含步骤 → 判未完成并进入自动修复循环；仅计划无步骤（read-only 任务）退回
  Success/Errors 判定。StepResults 全 success 仍直接通过（执行器允许合并步骤，
  不做步数完整性硬校验，避免误伤）
- **`planparse.go`**：新增 `countPlanSteps`（复用统一步骤行判定）

#### O3 — 统一计划解析器
- **`planparse.go`**（新增）：`isStepLine`/`extractStepTitle` 单一实现 + 契约测试
- **`hermes_confirm.go`** / **`hermes_sdd.go`**：删除重复实现（`planLineRE`、
  旧 `isStepLine`、旧 `extractStepTitle`），调用点统一引用
- 顺带修复：`extractStepTitle` 对多位数步骤号（"步骤 12："）只跳过一个数字的缺陷

#### 测试
- **`hermes_compact_test.go`**（新增）：SSE mock 强制触发 planner 压缩，断言
  `planWithConfirmation`/`planFix` 退出后 digest 存活且本轮规划输入不残留
- **`allsteps_plan_test.go`**（新增）：计划有步骤但零 complete_step 证据 → 失败；
  全部签收 → 通过；合并步骤 → 通过；无计划步骤退回旧判定
- **`planparse_test.go`**（新增）：中英文步骤行/多位数/边界契约
- `hermes_test.go`：既有 allStepsPassed 用例迁移到新签名，maxSteps+文件变更+错误
  场景期望从"通过"改为"不通过"（O2 新契约）

#### 验证
- `go build ./...` EXIT 0；`go vet ./...` 无告警；`go test ./...` 全 ok

---

## [10.127.0] — 2026-08-01

### 🐛 修复 bash 启动服务类命令堵死进程 — 自动后台化 + Wait 永不阻塞

> 根因（5 级追溯）：表象 = 用 bash 启动服务（npm run dev / node server.js / go run server）后 tianxuan 整个进程卡死，无法强制关闭，只能重启。直接原因 = 前台 bash 的 `cmd.Wait()` 要等 stdout/stderr 管道 EOF 才返回，而服务进程持有管道写端永不关闭；120s 超时 Kill shell 后服务进程仍存活，Wait 仍阻塞（Go exec 的 copy goroutine 等 EOF）。本地根因 = 前台路径假定命令会退出，没有服务类命令的防护；卡住的 bash 没有 job id，`kill_shell` 无从下手。系统根因 = 服务类命令允许前台执行。过程根因 = bash 描述虽有 run_in_background 提示但靠模型自觉，无防呆兜底。

#### 修复
- **`bash.go` 服务类命令自动转后台**：新增 `isServiceCommand`（关键词表：http.server/uvicorn/gunicorn/ngrok/nodemon/compose up/npm run dev/npm start/pnpm/yarn dev/cargo run/go run/tail -f/ssh -R/ssh -L/ serve/ server/ listen/ daemon/ watch 等，带空格前缀避免误伤 serverless/watchman 等）。未显式 `run_in_background` 的服务命令自动转后台：立即返回 job id + 中文提示，`kill_shell` 可随时强制关闭——从源头杜绝"前台等 120s"
- **`bash.go` 前台 Wait 兜底**：`cmd.Wait()` 改为 goroutine + `select { waitCh | ctx.Done() }`——任何前台命令超时/取消时**立即返回**（进程树由既有 kill goroutine 清理），永不永久阻塞主进程；超时分支不读取输出 buffer（copy goroutine 可能仍在写入，避免数据竞争）

#### 测试
- **`bash_service_test.go`**（新增）：`TestIsServiceCommand`——12 个服务命令命中（npm run dev/python http.server/docker compose up/node server.js/go run server/tail -f/ngrok/cargo run 等）+ 8 个普通命令不命中（echo/go test/git status/ls/grep/rm/npm install/python 脚本）
- `TestBashServiceCommandAutoBackground`——真实 node http 服务命令：Execute **0.01s 立即返回** job 信息（不阻塞 turn），并验证 `kill_shell` 可停止；无 node 环境自动跳过

#### 验证
- 修复前：服务命令前台执行 120s 超时后仍卡死（测试超时 + 残留 node 进程）
- 修复后：自动后台 0.01s 返回；`go build ./...` EXIT 0；`go vet ./...` 无告警；`go test ./...` 全部 ok

#### 文件变更
- `internal/tool/builtin/bash.go` — +40 行（服务检测 + 自动后台 + Wait 兜底）
- `internal/tool/builtin/bash_service_test.go` — 新增（+110 行）

---

## [10.126.0] — 2026-08-01

### 🧹 全量工具梳理与完善 — compact 接线 / Kind 一致化 / 工具集补全

> 盘点全部 27 个内置工具（名称/只读分类/Kind/描述/schema/compact），修复 5 类问题：两个工具的 compact 条目存在但接口未接线（每轮全量描述进上下文）、verify_gate 分类自相矛盾、coding 工具集漏配新工具、git_commit 描述携带内部版本标记、read_skill 描述缺少使用引导。

#### 变更
- **`codeindex.go` / `movefile.go`**：实现 `CompactDescriptor`——`compactDesc`/`compactSchema` 里早已有 code_index/move_file 条目但类型从未实现接口（每轮全量 278+643 / 165+263 字节进上下文）；接线后走一行中文描述 + 精简 schema，每轮省 ~75% token
- **`verify_gate.go`**：`Kind()` 从 `KindExecute` 改为 `KindRead`——此前 `ReadOnly=true`（可并行、无权限拦截）却标 `KindExecute`（`IsMutator=true`），分类自相矛盾；统一为只读验证门控语义
- **`tool.go`**：`DefaultToolsets["coding"]` 补全 5 个漏配工具——`edit_lines`、`verify_gate`、`notebook_edit`、`git_worktree`、`search_large_output`（此前配置 `toolsets=[coding]` 的用户会静默丢失这些能力）
- **`git.go`**：git_commit 描述移除内部版本标记 "V10.6: "（描述是给模型的，只保留行为警告）
- **`readskill.go`**：read_skill 描述从 64 字节扩为引导式——说明正文按需加载、run_skill 前可用它预览技能玩法

#### 测试
- **`tool/builtin/compact_wiring_test.go`**（新增）：code_index/move_file 必须实现 CompactDescriptor（单行描述 + 非空 schema）；verify_gate Kind=read 与 ReadOnly 一致；git_commit 描述不含版本标记但保留 main 警告；read_skill 描述含 run_skill 引导
- **`tool/toolset_test.go`**（新增）：`TestCodingToolsetComplete`——coding 工具集必须包含 5 个新工具

#### 验证
- TDD 红灯（4 项失败：compact 未接线 / Kind 矛盾 / 描述标记 / 工具集缺失）→ 绿灯
- `go build ./...` — EXIT 0；`go vet ./...` — 无告警
- `go test ./...` — 全部 ok，无 FAIL

#### 文件变更
- `internal/tool/builtin/codeindex.go` — +2 行；`movefile.go` — +2 行；`verify_gate.go` — 1 行；`git.go` — 描述 1 行；`readskill.go` — 描述 +4 行
- `internal/tool/tool.go` — +3/−2 行（工具集补全）
- `internal/tool/builtin/compact_wiring_test.go` — 新增；`internal/tool/toolset_test.go` — 新增

---

## [10.125.0] — 2026-08-01

### 🧬 蒸馏 superpowers receiving-code-review — 审查反馈接收技能

> 审查闭环补全：requesting-code-review（主动请求审查）已有，receiving-code-review（收到反馈后如何正确回应）缺失——AGENTS.md 只有"拒绝谄媚"原则，没有操作流程。蒸馏 superpowers v5.1.0 的 receiving-code-review，把"先验证再实现、技术正确性优先于社交舒适"落成 6 步可执行流程。

#### 变更
- **`skill/bundled/receiving-code-review/SKILL.md`**：新增内置技能——6 步响应（读→理解→验证→评估→回应→逐项实现）、禁止表演性同意（"你说得完全对"等）、模糊反馈先澄清不部分实现、外部审查 5 条检查清单、YAGNI 检查（先 grep 确认是否被使用）、实现顺序（阻塞→简单→复杂，每项单独测试）、反驳条件
- **`skill/autotrigger.go`**：自动触发规则新增 receiving-code-review（关键词：审查意见/审查反馈/review 反馈/review 意见/按反馈修改）

#### 测试
- **`skill/autotrigger_test.go`**：`TestMatchSkill` 新增 2 个命中用例（"按审查意见修改"、"根据 review 反馈调整"）
- **`skill/skill_test.go`**：`TestBundledCoreWorkflowSkills` 断言表加入 receiving-code-review（描述带"必须"触发规则）

#### 验证
- TDD 红灯（技能缺失 + 规则未命中）→ 绿灯
- `go build ./...` — EXIT 0；`go vet ./...` — 无告警
- `go test ./...` — 全部 ok，无 FAIL

#### 文件变更
- `internal/skill/bundled/receiving-code-review/SKILL.md` — 新增
- `internal/skill/autotrigger.go` — +1 行（触发规则）
- `internal/skill/autotrigger_test.go` — +2 行；`internal/skill/skill_test.go` — +1 行

---

## [10.124.0] — 2026-08-01

### 🧹 工具与技能重新分离 — subagent 工作流回归技能单一入口

> 技能系统可用后（自动触发 V10.122 + 可观测性），不再需要"勉强把技能做成工具"来保证使用。本轮把 explore / research / review / security_review 从顶层工具外壳还原为**纯技能**：模型通过 `run_skill` 调用（技能索引 `[🧬 subagent]` 条目 + Hermes 提示词指引），工具列表回归"原子操作 + 技能系统入口"的干净边界。调用统计也统一进入技能侧栏（可观测性一致）。

#### 变更
- **`skill/tools.go`**：删除 `subagentSkillTool` 与 `BuiltinSubagentTools`（工具外壳 ~150 行）——subagent 技能统一走 `run_skill`（其 Execute 已支持 RunAs=subagent 派发 runner）
- **`skill/builtins.go`**：删除无调用方的 `BuiltinNames`（死代码）
- **`boot/boot.go`**：移除 executor 与 planner 两处 `BuiltinSubagentTools` 注册
- **`agent/batch_executor.go`**：conflict key 移除 4 个工具名（`run_skill` 已是 `!spawn`，串行隔离语义不变）
- **`planmode/policy.go`**：安全工具表移除 4+1 个别名（`run_skill` 已允许，plan mode 下隔离子代理调查路径不变）
- **`agent/hermes_prompt.go`**：规划者提示词"Sub-agents"段改为"Sub-agent skills（run_skill 调用，索引标记 [🧬 subagent]）"
- **`desktop/frontend/.../RuntimePanel.tsx`**：工具面板移除 4 个子代理条目（与工具注册表一致）

#### 保留不动（本就是原子操作/系统入口）
- `verify_gate`（shell 验证门控）、`bash`、文件编辑系、`git_*`、`lsp_*`、`code_index`、`task`、`run_skill`/`install_skill`/`parallel_skills` 等

#### 测试
- **`skill/tools_test.go`**：`TestBuiltinSubagentToolsRunner` 替换为 `TestRunSkillInvokesBuiltinSubagent`——锁定分离后的正确路径：`run_skill({name:"explore"})` 派发 builtin subagent 正文到 runner，arguments 即子代理任务

#### 验证
- `go build ./...` — EXIT 0；`go vet ./...` — 无告警
- `go test ./...` — 全部 ok，无 FAIL（skill/agent/boot/planmode 全绿）

#### 文件变更
- `internal/skill/tools.go` — −155 行（工具外壳删除）
- `internal/skill/builtins.go` — −12 行（死代码）
- `internal/boot/boot.go` — −4 行
- `internal/agent/batch_executor.go` — −2/+1 行
- `internal/planmode/policy.go` — −5 行
- `internal/agent/hermes_prompt.go` — −1/+1 行（提示词更新）
- `internal/skill/tools_test.go` — −27/+22 行（路径测试替换）
- `desktop/frontend/src/components/RuntimePanel.tsx` — −5 行

---

## [10.123.0] — 2026-08-01

### 🛡️ 技能自动注入缓存加固 — 全部消息前缀不变性

> DeepSeek 前缀缓存匹配的是**整个消息数组的连续前缀**（`L1 system | L2 runtime | tools | user | assistant | tool_result | ...`）——任何一条已入库消息的字节变化都会从该位置起全部断裂，不只是 system/tools。V10.122 的技能注入据此重新审计：注入只发生在消息首次进入会话时（turn 入口一次），注入后的字节一经 `session.Add` 便固定，历史消息从不重写。本轮再加三重加固，把"每轮重复注入正文"的成本和风险消除。

#### 加固
- **会话内去重（`agent_run.go` / `agent.go`）**：`AgentRunner.autoInjected` 记录已注入技能——同一技能后续轮次不再重复注入，后续 user 消息保持接近原始输入（重复正文每轮都会按缓存 miss token 重新计费，去重后只付一次）
- **compaction 后重置（`compact.go`）**：`compact()` 重写历史后清空 `autoInjected`——摘要可能丢弃技能正文，允许后续匹配重新注入（新 user 消息携带正文，正常按新增消息计费）
- **注入长度上限（`skill/autotrigger.go`）**：`maxAutoSkillBodyChars = 2000` runes，超长 playbook 确定性截断并提示（每注入一个字符 = 该轮一个 miss token，上限控制单轮成本）

#### 缓存安全结论（与约束对齐）
- 命中率不受损：注入字节落在**本轮新增消息**区域（与用户输入一样按新 token 计费），不改变已入库历史 → 每轮命中的前缀量与非注入版本一致
- 真正会断缓存的行为已被排除：历史消息重写（不重写）、注入非确定性（纯函数 + 测试锁定字节）、同一消息跨请求字节变化（session.Add 持久化）
- 全部消息前缀不变性由 e2e 测试直接锁定

#### 测试
- **`agent/auto_skill_test.go`**（新增）：`TestWithAutoSkillDedup`（同技能只注入一次 / 不同技能仍注入 / 重置后重新注入）、`TestCacheHitPrefixStableWithAutoSkill`（e2e：注入技能后多轮请求 `hitChars[i] == reqChars[i-1]`，即每轮缓存前缀 = 上一轮完整请求，字节稳定）
- **`skill/autotrigger_test.go`**：`TestInjectAutoSkillTruncatesLongBody`（截断确定性 + 上限 + 提示）

#### 验证
- TDD 红灯（`maxAutoSkillBodyChars`/`autoInjected` 未定义）→ 绿灯
- `go build ./...` — EXIT 0；`go vet ./...` — 无告警
- `go test ./...` — 全部 ok，无 FAIL

#### 文件变更
- `internal/agent/agent_run.go` — +15 行（去重逻辑）
- `internal/agent/agent.go` — +6 行（autoInjected 字段）
- `internal/agent/compact.go` — +4 行（compaction 重置）
- `internal/skill/autotrigger.go` — +8 行（长度上限 + 截断）
- `internal/agent/auto_skill_test.go` — 新增（+85 行，含 e2e 前缀稳定性）
- `internal/skill/autotrigger_test.go` — +24 行（截断测试）

---

## [10.122.0] — 2026-08-01

### ⚡ 技能自动触发层 — 把技能调用从"模型自觉"变成"系统决策"（缓存安全）

> 技能不调用的根治方案：不再依赖模型主动 run_skill，而是系统在用户输入进入 agent 循环时，用**确定性规则**匹配技能并自动注入正文。与"把技能改成工具"相比，不膨胀 tools 列表、保留技能按需加载与用户可扩展性。

#### 缓存安全设计（DeepSeek 前缀缓存适配）
- **只修改 user 消息字节**：注入发生在 `runDirect` 中 `session.Add(user message)` 之前，包装为 `<auto-skill>...</auto-skill>` transient 块——与既有 `withTurnPreferences`（语言偏好块）同一模式，前端用 `StripTransientBlocks` 剥离，显示干净输入
- **不触碰 L1/L2/tools**：不动态增删工具（规避 V8.0.2 filteredSchemas 事故），不注入 system prompt，`verifyPrefixAndShape` 守卫的 SystemHash/ToolsHash 均不变
- **字节确定性**：`MatchSkill`/`InjectAutoSkill` 是纯函数——同输入 + 同技能 → 完全相同的注入字节；用户输入变化导致的缓存断开是任务本身的自然成本，无额外损失
- **白名单触发**：只对核心 inline 技能注册规则（systematic-debugging / tdd / requesting-code-review / finish-development-branch）；subagent 技能（explore/review 等）已工具化、不注入正文；设计类技能不自动触发（避免误命中与正文过大）

#### 变更
- **`skill/autotrigger.go`**（新增）：`AutoTriggerRule` + 内置规则表 + `MatchSkill`（大小写不敏感子串匹配，确定性）+ `InjectAutoSkill`（inline 技能正文注入 `<auto-skill>` 块；subagent/缺失技能/不匹配一律原样返回）
- **`agent/agent.go`**：AgentRunner 新增 `autoSkill *skill.Store` 字段 + `SetAutoSkillStore` setter（nil 禁用，向后兼容）
- **`agent/agent_run.go`**：`runDirect` 中 `withTurnPreferences` 后调用 `withAutoSkill`（仅 `!plannerMode`——executor/solo 注入，Hermes 规划者不注入）
- **`boot/boot.go`**：executor 构造后 `executor.SetAutoSkillStore(skillStore)` 接线
- **`agent/session/transient.go`**：`StripTransientBlocks` 新增 `<auto-skill>` 分支（前端剥离）

#### 测试
- **`skill/autotrigger_test.go`**（新增）：匹配命中/不命中/优先级、注入字节确定性（同输入两次字节一致——缓存安全锁）、不匹配原样、subagent 技能不注入、缺失技能原样
- **`agent/session/transient_test.go`**（新增）：`<auto-skill>` 块与混合 transient 块剥离

#### 验证
- TDD 红灯（InjectAutoSkill 未定义 + auto-skill 块未剥离）→ 绿灯
- `go build ./...` — EXIT 0；`go vet ./...` — 无告警
- `go test ./...` — 全部 ok，无 FAIL（agent/skill/boot/session 全绿）

#### 文件变更
- `internal/skill/autotrigger.go` — 新增（+85 行）
- `internal/skill/autotrigger_test.go` — 新增（+115 行）
- `internal/agent/agent.go` — +7 行（字段 + setter）
- `internal/agent/agent_run.go` — +11 行（接线 + withAutoSkill）
- `internal/agent/session/transient.go` — +6 行（剥离分支）
- `internal/agent/session/transient_test.go` — 新增（+15 行）
- `internal/boot/boot.go` — +4 行（接线）

---

## [10.121.0] — 2026-08-01

### 🎯 修复技能系统几乎不调用 — 触发指引 + 核心编程工作流技能化

> 诊断（5 级追溯）：表象 = 模型从不调用 run_skill。直接原因 = (1) run_skill 工具描述自我贬低——"Prefer dedicated top-level tools (explore/review/etc) when available" 暗示技能是次选；(2) 技能索引里 10 个技能 8 个是 UI/设计向（banner-design/brand/design/slides 等），编程任务找不到匹配项。本地根因 = 内置技能库与 tianxuan 主场景（编程）错配。系统根因 = 编程方法论全部写死在提示词（AGENTS.md 铁律），技能库无对应 playbook，技能系统对编程场景零增量价值 → 整体被忽略。过程根因 = 技能库沿设计向蒸馏演进，核心编程工作流从未技能化注入。

#### 修复
- **`skill/tools.go`**：run_skill 描述移除 "Prefer dedicated top-level tools" 自我贬低，改为触发指引——"任务匹配技能描述时必须调用"，明确技能承载提示词之外的详细工作流（tdd / systematic-debugging / requesting-code-review / finish-development-branch），禁止用通用工具自行拼凑
- **`skill/bundled/tdd/SKILL.md`**：新增——红绿重构循环（写失败测试→确认失败→最小实现→确认通过→重构），铁律"无失败测试禁止产品代码"，反模式清单；描述带触发规则"功能开发或修复 bug 前必须使用"
- **`skill/bundled/systematic-debugging/SKILL.md`**：新增——4 阶段根因定位（调查→假设→最小修复→验证），含 5 级追溯与复现测试要求；描述带触发规则"提出修复前必须使用"
- **`skill/bundled/requesting-code-review/SKILL.md`**：新增——任务完成/主要功能/合并前强制审查，review 子代理上下文构造，按严重性分级处理反馈；描述带触发规则"完成任务或合并前必须使用"

#### 测试
- **`skill/skill_test.go`**：`TestRunSkillToolDescriptionGuidesTrigger` 锁定描述不再自我贬低且含触发指引；`TestBundledCoreWorkflowSkills` 锁定 3 个核心技能可提取、描述带"必须"触发规则、正文非空

#### 验证
- TDD 红灯（描述含 "Prefer dedicated top-level tools" + 3 技能缺失）→ 绿灯
- `go build ./...` — EXIT 0
- `go vet ./...` — 无告警
- `go test ./...` — 全部 ok，无 FAIL

#### 文件变更
- `internal/skill/tools.go` — 描述 −1/+1（触发指引）
- `internal/skill/bundled/{tdd,systematic-debugging,requesting-code-review}/SKILL.md` — 新增 3 个技能
- `internal/skill/skill_test.go` — +54 行（2 个新测试）

---

## [10.120.0] — 2026-08-01

### 🧬 重蒸馏 superpowers v5.1.0 — finish-development-branch 内置技能

> V10.50 蒸馏的是当时版本的 superpowers 方法论（设计优先/TDD/子代理）。v5.1.0（2026-04，204K⭐）演进出完整的收尾工作流技能 **finishing-a-development-branch**：任务完成后 → 验证测试 → 检测环境 → 4 选项决策（本地合并/推送建 PR/保留/丢弃）→ 归属式 worktree 清理。tianxuan 已有 `git_worktree` 工具（add/remove/list）但缺少这条收尾流程——分支做完后没有结构化的合并/PR/清理决策，worktree 容易遗留成垃圾。

#### 变更
- **`skill/bundled/finish-development-branch/SKILL.md`**：新增内置技能（embed 打包 + 启动提取 + 技能索引注入），按 tianxuan 工具集适配：验证门禁用 `verify_gate`/`bash`、环境检测用 `git_status`/`git_worktree list`、清理遵循归属检查（只删自己创建的 worktree、先回主仓库根再 remove、合并成功后才删分支、`git worktree prune` 自愈）；保留 superpowers 红线（测试未全绿禁止合并、丢弃需用户明确确认、不清理外部托管工作区、禁止未经询问 force-push）
- **`skill/skill_test.go`**：新增 `TestBundledFinishDevelopmentBranch`——确保新技能可提取、可解析、描述为中文、正文含核心流程关键词、并出现在 Skills 索引

#### 验证
- TDD 红灯（技能未发现）→ 绿灯
- `go build ./...` — EXIT 0
- `go vet ./...` — 无告警
- `go test ./...` — 全部 ok，无 FAIL

#### 文件变更
- `internal/skill/bundled/finish-development-branch/SKILL.md` — 新增（收尾工作流）
- `internal/skill/skill_test.go` — +38 行（新测试）

---

## [10.119.0] — 2026-08-01

### 🧬 蒸馏 Aider repo map — CoreTypes 按引用频率排名

> Aider 的 repo map 用 tree-sitter 提取符号并构建引用图，地图展示**被引用最多的标识符**（PageRank 排名），而非扫描顺序的前 N 个。tianxuan 的 `discoverCoreTypes` 此前按文件遍历顺序取前 15 个类型——扫到谁就是谁，与重要性无关。本轮蒸馏：项目地图的 Core Types 改为按全库引用频率降序排名（同频次按名称字母序，保证确定性）。

#### 变更
- **`codegraph/projectmap.go`**：
  - 新增 `rankCoreTypes`——统计每个类型名在扫描树（Go: `internal/`，Rust: `src/`）全部源码文件中的**完整标识符出现次数**，按次数降序排序后截断至 15
  - 新增 `countIdentifier`/`isIdentByte`——按标识符边界（字母/数字/下划线）计数，避免 `Handler` 被 `HTTPHandler` 的子串污染
  - `discoverCoreTypes`（Go）与 `discoverRustCoreTypes`（Rust）去重后统一接入排名
- **`codegraph/projectmap_test.go`**：新增 `TestCoreTypesRankedByReference`——Alpha 定义在 internal/e（扫描靠后）但被 b/c/d 频繁引用，断言其排名超过先扫描到的 Zeta

#### 验证
- TDD 红灯（旧实现 `[Zeta (a) Alpha (e)]` 扫描序）→ 绿灯（`[Alpha (e) Zeta (a)]` 引用序）
- `go build ./...` — EXIT 0
- `go vet ./...` — 无告警
- `go test ./...` — 全部 ok，无 FAIL

#### 文件变更
- `internal/codegraph/projectmap.go` — +72 行（排名 + 计数 helper）
- `internal/codegraph/projectmap_test.go` — +49 行（新测试）

---

## [10.118.0] — 2026-08-01

### 🆕 Rust 项目地图补模块与核心类型 — 规划者可见 Rust 项目结构

> Rust 项目此前的地图只有 Language/Entry/Deps/FileCount，没有 Go 项目那样的 Packages 与 CoreTypes——规划者看不到 Rust 项目的模块划分和公共类型。本轮为 Rust 补上结构发现，与 Go 支持对齐。

#### 变更
- **`codegraph/projectmap.go`**：Analyze Rust 分支新增 `discoverRustPackages`（`src/` 下含 .rs 的直接子目录 + 顶层模块文件，排除 crate 根 `main.rs`/`lib.rs`）与 `discoverRustCoreTypes`（`src/` 下 .rs 文件前 40 行内的 `pub struct/enum/trait/type`，忽略 `pub fn/mod/use` 等非类型声明，输出 `Name (目录)` 格式并去重截断至 15）
- **`codegraph/projectmap_test.go`**：新增 `TestAnalyze_RustStructure`——构造 lib.rs（Engine/Runner）、lexer.rs（TokenKind）、parser/mod.rs（Ast），锁定 Packages 排序与 CoreTypes 格式，并断言 `pub fn` 不误收为类型

#### 验证
- TDD 红灯（旧实现 Packages 为空）→ 绿灯
- `go build ./...` — EXIT 0
- `go vet ./...` — 无告警
- `go test ./...` — 全部 ok，无 FAIL

#### 文件变更
- `internal/codegraph/projectmap.go` — +95 行（Rust 结构发现 + 3 个 helper）
- `internal/codegraph/projectmap_test.go` — +52 行（新测试）

---

## [10.117.0] — 2026-08-01

### 🛡️ git 工具长输出截断保护 + verify_gate 截断统一

> git_diff / git_log 此前把完整输出直接抛给模型——大 diff（全文件改动、批量重构）可达数十万字节，直接撑爆上下文窗口；同一份上下文保护逻辑在 bash（`truncateStream`）与 verify_gate（上一轮的 `truncateOutput`）里重复实现且细节不一致。本轮为 git 工具补上截断保护，并把 verify_gate 统一到唯一的 `truncateStream` 实现。

#### 变更
- **`git.go`**：新增 `gitOutputMaxBytes = 48KB`；`git_diff` 与 `git_log` 输出经 `truncateStream` 头尾保留（头部 diff 头 + 尾部末 hunk / 长 commit message 尾），截断时在输出顶部注入引导提示（diff 用 `path=<file>` 收窄、log 用 `count=/path=/author=` 收窄）
- **`verify_gate.go`**：删除 V10.116 引入的重复实现 `truncateOutput`（+20 行 → −25 行），改调 `truncateStream(output, 2000)`——获得对称头尾 + elided 字节数提示 + 不过截保护；新增 `verifyGateMaxBytes` 常量
- **`git_test.go`**：新增 2 个集成测试——在真实 git 仓库构造 6000 行全量修改 diff（约 200KB）与 6000 行 commit message（约 126KB），断言截断提示存在、头尾保留、中间省略；测试用 `t.Chdir` 模拟 agent 运行在仓库根（git 工具无 workDir 字段、继承进程 cwd）
- **`tool_extra_test.go`**：删除 `truncateOutput` 的 3 个单测（由 `truncateStream` 既有测试覆盖）；保留 verify_gate 集成测试（尾部 FAIL 详情在对称截断下仍可见）

#### 验证
- TDD 红灯（旧实现 git_diff 204KB / git_log 126KB 全量返回）→ 绿灯
- `go build ./...` — EXIT 0
- `go vet ./...` — 无告警
- `go test ./...` — 全部 ok，无 FAIL

#### 文件变更
- `internal/tool/builtin/git.go` — +16 行（截断保护 + 提示）
- `internal/tool/builtin/verify_gate.go` — +3/−20 行（统一到 truncateStream）
- `internal/tool/builtin/git_test.go` — 新增 +110 行（2 个集成测试）
- `internal/tool/builtin/tool_extra_test.go` — −45 行（移除已覆盖的单测）

---

## [10.116.0] — 2026-08-01

### 🔧 verify_gate 长输出截断保留头尾 — 失败详情不再丢失

> verify_gate 输出超过 2000 字符时此前只保留头部（`output[:2000]`）。而 `go test` 类失败详情——`--- FAIL: TestX`、断言行（`foo_test.go:12: expected 2, got 3`）——位于输出**尾部**，被截掉后模型只能看到 "GATE FAILED" 却不知道哪个测试失败、为什么失败，修复时只能盲目重跑或猜测。

#### 根因
- **`verify_gate.go`**：截断用 `output[:2000] + "...[truncated]"`，头部优先；长输出 + 尾部失败详情 = 关键信息丢失

#### 修复
- **`verify_gate.go`**：新增 `truncateOutput`——保留头 600 + 尾 1400 字符，中间省略标记；按 rune 切片避免截断切坏 UTF-8 多字节序列；成功/失败输出统一走该逻辑（成功场景尾部也含 `ok` 摘要，更利于确认）

#### 测试
- **`tool_extra_test.go`**：集成测试构造 250 行长输出 + 尾部 FAIL 详情（Windows cmd / POSIX bash 双命令），断言测试名与断言行在截断后仍可见；纯函数测试锁定短输出不动、头尾保留 + 中间丢弃、UTF-8 输出截断后仍合法

#### 验证
- TDD 红灯（旧实现截断在 `PASS lin`，尾部 FAIL 详情丢失）→ 绿灯
- `go build ./...` — EXIT 0
- `go vet ./...` — 无告警
- `go test ./...` — 全部 ok，无 FAIL

#### 文件变更
- `internal/tool/builtin/verify_gate.go` — +20 行（truncateOutput + 调用点）
- `internal/tool/builtin/tool_extra_test.go` — +80 行（4 个新测试）

---

## [10.115.0] — 2026-08-01

### 🆕 Refresh 增量检测扩展到 Rust 项目

> codegraph.Refresh 此前只对 Go（go.mod + internal/）与 Node/TS（package.json + src/）做增量判断，Rust 项目（Cargo.toml）每轮都全量 Analyze。新增 Cargo.toml + src/ 的结构代理检测：manifest 与 src/ modtime 均未变化时直接复用缓存 ProjectInfo；src/ 内变化才重扫。同时 Rust 分支补上 Source Files 统计（.rs 计数，跳过 target/ 构建产物），使项目地图信息与 Go/TS 对齐、重扫结果可观测。

#### 变更
- **`codegraph/projectmap.go`**：Analyze Rust 分支补 `FileCount`（新增 `countRSFiles`，跳过 target/.git/node_modules）与 `LastModified`（Cargo.toml）基准；Refresh 新增 Rust 分支（Cargo.toml + src/ 未变化 → 复用缓存）；`ProjectInfo.FileCount` 字段注释同步覆盖 .rs
- **`codegraph/projectmap_test.go`**：新增 `TestRefresh_RustIncremental`，锁定 Rust 增量语义（未变化复用 / src/ 变化重扫 / src/ 外变化不重扫）
- **`agent/hermes.go`**：`hasStructuralChange` 注释同步补充 Cargo.toml（代码早已覆盖，仅文档对齐）

#### 验证
- TDD 红灯（旧实现 Rust FileCount=0）→ 绿灯
- `go build ./...` — EXIT 0
- `go vet ./...` — 无告警
- `go test ./...` — 全部 ok，无 FAIL

#### 文件变更
- `internal/codegraph/projectmap.go` — +22 行（增量分支 + 计数 + 注释）
- `internal/codegraph/projectmap_test.go` — +46 行（新测试）
- `internal/agent/hermes.go` — 注释 −1/+1 行（文档同步）

---

## [10.114.0] — 2026-08-01

### 🆕 Refresh 增量检测扩展到 Node/TS 项目

> codegraph.Refresh 此前只对 Go 项目（go.mod + internal/）做增量判断，Node/TS 项目每轮都全量 Analyze。新增 package.json + src/ 的结构代理检测：manifest 与 src/ modtime 均未变化时直接复用缓存 ProjectInfo；src/ 内变化才重扫。Rust 等无识别 manifest 的项目保持全量分析（不退化）。

#### 测试
- **`codegraph/projectmap_test.go`**：新增 `TestRefresh_NodeIncremental`——未变化复用 / src/ 变化重扫 / src/ 外变化不重扫

#### 验证
- TDD 红灯（Node root 级文件旧实现重扫）→ 绿灯；`go build ./...` EXIT 0；`go test ./internal/...` 全部 ok

#### 文件变更
- `internal/codegraph/projectmap.go` — +27 行；`internal/codegraph/projectmap_test.go` — +107 行

---

## [10.113.0] — 2026-08-01

### 📌 计划 File(s) 锚点交接 — 执行阶段免重复定位调查

> Hermes 规划时的调查轨迹只留在 planner session，Hephaestus 收到 handoff 后需要重新定位文件。主计划格式加入 File(s) 字段（与修正计划格式对齐），要求 Hermes 输出规划中已验证的文件路径锚点；Hephaestus 提示词改为优先使用锚点直接进入实现，仅锚点缺失/失效时才重新调查并报告偏差。

#### 收益
- 执行阶段省去 1~3 轮 LLM 定位往返；File(s) 锚点辅助 parallel_tasks 的 disjoint file lists 判断

#### 验证
- TDD 红灯（旧措辞断言失败）→ 绿灯；`go build ./...` EXIT 0；`go test ./internal/...` 全部 ok

#### 文件变更
- `internal/agent/hermes_prompt.go` — +16/−6 行；`internal/agent/hermes_test.go` — +22 行

---

## [10.112.0] — 2026-08-01

### ⚡ Hermes 项目地图增量缓存 — 消除每轮规划前全量扫描

> injectProjectMap 此前每次 Run/planFix 都调用 codegraph.Analyze 全量遍历工作区（tianxuan 自身约 1.8s/次，539 个 .go 文件）。改用已有 codegraph.Refresh 做增量检测：仅首次 Analyze，后续仅当 go.mod/internal/ 变化时才重扫；结构变化（feedResultToPlanner）强制失效重扫，不依赖 modtime。同时修复空 wsRoot 会注入空地图消息的缺陷。

#### 验证
- 性能实测 Analyze 486ms-1.78s → Refresh 0ms；`go build ./...` EXIT 0；`go test ./internal/...` 全部 ok

#### 文件变更
- `internal/agent/hermes.go` — +38/−7 行；`internal/agent/hermes_test.go` — +80 行

---

## [10.111.0] — 2026-07-31

### 🆕 接线激活上下文卸载（Context Offloading）+ 静默吞错隐患修复

> 两项迭代：(1) offload 功能（设计自 manishiitg/mcpagent，代码早已存在但从未接线——`agent.Options.OffloadDir` 零消费、`WireSearchLargeOutputStore` 零调用）现已完整激活：大型工具输出自动落盘并替换为紧凑引用，模型用 `search_large_output` 按需读取，防止上下文窗口饱和。(2) 批量修复静默吞错——错误不再无声丢弃，改为记录日志。

#### 功能：上下文卸载接线
- **`config.go`**：`AgentConfig` 新增 `offload_dir` / `offload_threshold_chars` 字段；默认 `offload_dir = ".tianxuan/offload"`（启用），阈值 0 = 默认 10000 字符
- **`agent.go`**：`agent.New` 消费 `Options.OffloadDir`/`OffloadThresholdChars`（此前死字段）；`SetOffload` 改为延迟初始化——per-session 子目录依赖 `sessionID`，在 `SetArchive` 时真正创建 store；新增 `OffloadStore()` getter
- **`boot.go`**：相对 offload 路径基于 cwd 解析；`SetArchive` 兜底分支（archive 打开失败也设置 sessionID，确保 offload 仍初始化）；`WireSearchLargeOutputStore` 注入 `search_large_output` 工具；cleanup 包装追加 `CloseOffload()` 会话结束清理
- **`checkpoint_test.go`**：新增 3 个接线测试（延迟初始化 / 空目录禁用 / sessionID 后即时创建）

#### 修复：静默吞错 → 日志记录
- **`context/flow.go`**：`Add`/`ReplaceMessages` 的 store 失败静默（会无声丢上下文）→ `slog.Error`
- **`schedule/scheduler.go`**：调度状态持久化 `_ = Save(...)` 吞错（重启丢调度）→ `slog.Error`
- **`tool/builtin/todo.go`**：进度 markdown 保存 `_ = WriteFile(...)` 吞错 → `slog.Error`
- **`agent/session/save.go`**：会话缓存写入 + 归档/恢复时 meta sidecar 移动共 3 处吞错 → `slog.Warn`/`Error`

#### 验证
- `go build ./...` — EXIT 0
- `go test ./...` — 全部 ok，无 FAIL

#### 文件变更
- `internal/config/config.go` — +13 行（字段 + 默认值）
- `internal/agent/agent.go` — +37/−6 行（消费 + 延迟初始化 + getter）
- `internal/boot/boot.go` — +30/−5 行（接线 + 兜底 + 清理）
- `internal/context/flow.go` — +15/−4 行（吞错修复）
- `internal/schedule/scheduler.go` — +3/−1 行
- `internal/tool/builtin/todo.go` — +4/−1 行
- `internal/agent/session/save.go` — +12/−3 行
- `internal/agent/checkpoint_test.go` — +52 行（新测试）

---

## [10.110.0] — 2026-07-31

### 🔧 修复桌面端构建失败 — 移除 app.go 孤儿 import

> V10.108.0 把 `app.go` 中 Resume 的 provider 逻辑移到别处后未清理 import，导致 desktop 独立 go module 编译失败（`imported and not used`）。根目录 `go build ./...` 不覆盖该 module 故未暴露。

#### 根因
- **`desktop/app.go`**：残留的孤儿 import（V10.108.0 遗留）——`desktop/` 是独立 go module，仅运行 `cd desktop && go build ./...` 时才暴露编译错误

#### 修复
- **`desktop/app.go`**：移除该孤儿 import 行（−1 行），恢复桌面端独立 module 构建

#### 验证
- `cd desktop && go build ./...` — 通过（EXIT 0）

#### 文件变更
- `desktop/app.go` — −1 行（移除孤儿 import）

---

## [10.109.0] — 2026-07-31

### 🔧 修复 edit_lines 换行污染 — CRLF 归一化 + 行号对齐 read_file

> 定位到 3 处根因导致 `edit_lines` 在 CRLF/混合换行文件上编辑错行、吞行甚至数据丢失，且 AI 发送 CRLF 内容时 `\r` 泄漏成 `\r\r\n` 污染文件。

#### 根因（3 处）
1. **行号错位**：content 按 `detectLineEnding` 单一风格 Split → 混合换行文件行数与 `read_file`（按 `\n` 计行）不一致，导致编辑错行/吞行/数据丢失
2. **`\r` 泄漏**：`new_content` 按 `\n` 分割但不清 `\r` → AI 发 CRLF 内容时 `\r` 泄漏成 `\r\r\n`，文件被污染后 AI 只能整文件重写
3. **尾空元素**：`new_content` 尾随 `\n` 的尾空元素未 trim → 每次编辑注入一个空行

#### 修复
- **`editlines.go`**：content 与 new_content 统一归一化 `\r\n→\n` 后处理，输出按原文件主导换行风格（fileLE）拼接，行号与 `read_file` 完全对齐

#### 测试
- 新增 `editlines_test.go` 8 个边界用例：CRLF 泄漏 / 混合换行 / 连续编辑 / 删除 / 追加 / 无尾换行，全部通过

#### 文件变更
- `internal/tool/builtin/editlines.go` — +16/−6 行（归一化 + 拼接）
- `internal/tool/builtin/editlines_test.go` — 新增 +252 行

---

## [10.108.0] — 2026-07-29

### 🔧 修复重启后旧会话上下文被清空

> 定位到三个根因导致重启后自动恢复的会话丢失关键上下文：(1) 系统提示词被 `app.go` 手动替换为不完整的 L1 Identity，导致 executor 丢失全部行为指令；(2) 双模型架构中 Hermes 规划器会话未恢复，重启后规划历史归零；(3) TCCA FlowLayer 未同步恢复后的会话历史。

#### 根因 1（致命）：系统提示词被截断为 L1 Identity
- **`desktop/app.go:228-231`**：自动恢复时用 `ctrl.SystemPrompt()`（仅 L1）替换了 executor 会话中完整的 `L1+Instructions` 提示词，导致 Hephaestus/Solo 丢失所有编码铁律和行为规则

#### 根因 2（严重）：Hermes 规划器上下文泄漏
- **`internal/control/controller.go:Resume()`**：只恢复了 executor 会话，未处理 Hermes 的 `hermesSess`，重启后规划器携带陈旧或不完整的上下文

#### 根因 3（相关）：TCCA FlowLayer 未同步
- `ctrl.Resume()` 没有像 `NewSession()` 那样更新 FlowLayer，影响 compaction 状态和缓存指标

#### 修复
- **`Resume()` 重写**：内部自动从当前 executor session 提取完整 L1+Instructions 系统提示词替换加载会话中的旧版本；检测 `*agent.Hermes` runner 时调用 `ResetSession()` 清空规划器旧上下文；同步 `ctxMgr.Flow().ReplaceMessages()` 更新 TCCA 状态
- **`app.go` 简化**：移除自动恢复路径中错误的系统提示词手动替换逻辑（−12 行），改为直接调用 `ctrl.Resume()`（现已内聚处理）

#### 影响范围
- 覆盖全部 11 处 `ctrl.Resume()` 调用点：桌面端自动/手动恢复、CLI `--resume`/`/resume`、模型切换重建、HTTP API、ACP 协议 — 所有路径统一受益，无回归

#### 文件变更
- `internal/control/controller.go` — `Resume()` +30/−4 行
- `desktop/app.go` — 恢复逻辑 −12 行（移除错误的手动替换）

---

## [10.107.0] — 2026-07-27

### 🔧 修复"无故崩溃"：并发 map 写入 fatal error + goroutine recover 补全

> 定位到两条独立的 `fatal error: concurrent map writes` 路径，均通过 `runParallel` 8 并发执行 `executeOne` 时触发——Go runtime 级致命错误不可被 recover 捕获，直接杀进程且不留 crash 日志，完美解释了"无故崩溃"现象。同时补全 3 个遗漏 recover 的 goroutine。

#### 根因修复
- **`staleWrittenFiles`/`staleReadFiles` 并发写入**：`executeOne` 在 `runParallel` 中并行执行时，对这两个 map 的并发读写未受保护 → 新增 `staleMu sync.Mutex`
- **`repeatSuccessCounts` 并发写入**：`repeatedSuccessBlock`/`recordRepeatSuccess` 中 map 的 nil-reset + 读写未受保护 → 新增 `repeatMu sync.Mutex`

#### 防御性修复
- **`anthropic.go`**：body-close goroutine 缺少 recover，`resp.Body.Close()` panic 可直接杀进程 → 添加 `crash.Recover("anthropic-body-close")`
- **`probe.go`**：`runProbesUncached` 的并行 probe goroutine 缺少 recover → 添加 `crash.Recover("env-probe")`
- **`batch_executor.go`**：`runParallel` 内部 goroutine 缺少 recover → 添加 `crash.Recover("batch-parallel")`

#### 文件变更
- `internal/agent/agent.go` — +2 行（staleMu + repeatMu）
- `internal/agent/execute_one.go` — +17/-6 行（两处 map 访问加锁 + repeatedSuccessBlock/recordRepeatSuccess 加锁）
- `internal/agent/agent_run.go` — +6/-2 行（重置改用对应 mutex）
- `internal/agent/batch_executor.go` — +2 行（crash.Recover + import）
- `internal/environment/probe.go` — +2 行（crash.Recover + import）
- `internal/provider/anthropic/anthropic.go` — +2 行（crash.Recover + import）

---

## [10.106.0] — 2026-07-26

### 🔧 formatSummary 优化 — 纯只读任务不再显示「未记录步骤详情」

> auto-skip 只读任务（如 "运行测试"）成功时，summary 原先显示 "✅ 任务完成\n（未记录步骤详情）"，提示冗余且有误导性。修复后纯无文件变更的任务直接输出 "✅ 任务完成"。

#### 修复
- **`formatSummary`** 条件收紧：`r.Success && (len(FilesCreated)>0 || len(FilesModified)>0)` — 仅在有文件产出但未记录步骤时才提示
- **新增 `TestFormatSummary_SuccessWithFilesNoDetails`**：验证有文件但未记录步骤时仍显示提示
- **更新 `TestFormatSummary_SuccessNoDetails`**：验证纯只读任务不显示提示

#### 文件变更
- `internal/agent/hermes.go` — ±3 行
- `internal/agent/hermes_test.go` — +22 行

---

## [10.105.0] — 2026-07-26

### 🔧 消除 TurnResultEvent 重复代码 + directive 边界测试

> 提取 `emitExecutorResult` 消除 auto-skip/fast-path/retry 三条路径的 75 行重复代码；新增 directive 边界测试覆盖 40/41 runes 阈值。

#### 重构
- **`hermes.go`**：新增 `emitExecutorResult(r, execErr, retriesExhausted, clearPlan)` 方法，统一 summary + Text + TurnResultEvent emit
- auto-skip 路径：25 行 → 1 行 `h.emitExecutorResult(execResult, execErr, false, true)`
- fast-path 路径：25 行 → 1 行同上
- retry 路径：24 行 → 1 行 `h.emitExecutorResult(result, err, !h.allStepsPassed(result), false)`
- 净减少 ~50 行

#### 测试
- **`hermes_test.go`**：新增 `TestDecidePlannerRoute_DirectiveBoundary` — 8 个子测试覆盖：
  - ≤40 runes 短命令 → directive
  - 41 runes 边界 → planner
  - 高危/复杂/跨面短命令 → planner（验证优先级不被 directive 劫持）

#### 文件变更
- `internal/agent/hermes.go` — -48/+37（净 -11）
- `internal/agent/hermes_test.go` — +38 行（新增测试）

---

## [10.104.0] — 2026-07-26

### 🎯 Auto-skip 路径完善 + 多项收紧优化

> 双模型 auto-skip 路径现在有完整的结果处理（sink event / summary / TurnResultEvent），allStepsPassed 支持只读任务，directive 阈值收紧，UI 卡片宽度优化。

#### Auto-skip 路径增强
- **`hermes.go`**：auto-skip 分支（atomic_edit/read_only/directive）增加 `wrapExecutorSink()`、`formatSummary()`、`TurnResultEvent` emit，与正常路径保持一致的结果卡片输出
- **`allStepsPassed`** 第 4 条规则放宽：Success=true + 无错误 → 通过，支持只读任务（无文件变更、无 complete_step）正常结束

#### 阈值收紧
- **`planner_route.go`**：directive 字符阈值从 100 runes 收紧到 40 runes，更长输入走完整规划流程

#### UI 优化
- **AskCard / PlanCard**：卡片最大宽度减半 `max-w-[calc(var(--maxw)/2)]`，避免过宽

#### 清理
- 移除 `code-review-guard/SKILL.md` — 不再需要内联捆绑技能

#### 文件变更
- `internal/agent/hermes.go` — +46/-2
- `internal/agent/hermes_test.go` — +12/-12
- `internal/agent/planner_route.go` — +3/-1
- `desktop/frontend/src/components/AskCard.tsx` — +1/-1
- `desktop/frontend/src/components/PlanCard.tsx` — +1/-1

---

## [10.103.0] — 2026-07-26

### 🔧 Precheck 模糊匹配 — 修复 edit_file 误拦导致整文件重写

> Precheck 的 `strings.Contains` 精确匹配与 Execute 的模糊匹配不一致，导致有尾部空白/Tab空格/行号前缀差异的 edit_file 被错误阻止，模型放弃 edit_file 改用 write_file 重写整个文件。

#### 修复
- **Precheck 新增 `fuzzyPrecheckMatch`**：和 Execute 使用相同的 4 种模糊模式（trimTrailing/expandTabs/stripOldReadPrefixes）
- 编辑 `old_string` 有细微差异时不再被 Precheck 误拦，Execute 的模糊匹配可以正常处理
- 适用 `edit_file`、`multi_edit`、`delete_range` 的预检查

#### 文件变更
- `internal/agent/tool_precheck.go` — +116 行：`fuzzyPrecheckMatch` + `normLine` + `stripReadPrefix` + `hasReadFileNumberPrefix`

---


### 🎯 双模型智能跳过规划者 — 特征分析引擎

> 从 reasonix-src 蒸馏确定性特征分析引擎，双模型模式下自动跳过不必要的规划者调用。纯聊天/简单操作/复杂任务三路分发，system prompt 不变，缓存不裂。

#### 特征分析引擎
- **`DecidePlannerRoute`**：15 级优先级决策链，纯文本分析，无模型调用
- **12 维特征提取**：work / highRisk / multiFile / crossSurface / structured / complex / atomic / readOnly / guidance / anchored / ambiguous / directive
- **中英双语词表**：覆盖 10 类特征词（work/mutation/readOnly/highRisk/crossSurface/complex/atomic/namedTargets/ambiguous）
- **`directive` 直行指令**：短操作（"构建""更新文件""add pagination"≤100字）跳过规划者

#### 三路分发
- **纯聊天**（hello/ok/谢谢）→ Hermes 规划者直接回答，不产生 plan，不过 executor
- **简单工作**（typo 修正/运行测试/构建/更新文件）→ ⚡跳过规划者，Hephaestus 执行者直接用 HephaestusSystemPrompt
- **复杂任务**（重构/前后端联调/高风险管理）→ 完整规划者→确认→执行流程

#### 文件变更
- `internal/agent/planner_route.go` — 新增：`PlannerDecision` + `DecidePlannerRoute` 决策链 (+105行)
- `internal/agent/planner_gate.go` — 新增：12 维特征提取 + 中英词表 (+225行)
- `internal/agent/hermes.go` — `Run` 中插入 6 行 auto-skip 判断
- `internal/agent/hermes_test.go` — +6 个测试：AtomicEdit/HighRisk/ReadOnly/Directives/ShortComplex/Default

#### 缓存安全
- 执行者 system prompt 始终是 `HephaestusSystemPrompt`，一次编译，字节不变
- 跳过规划者只减少 API 调用轮次，不影响 L1/L2 前缀缓存

---


### 🔧 修正循环根因修复 + 停止门全面加固 + 移动端移除

> 三管齐下消除无意义修正循环：核心判断逻辑修正 + 停止门计数器跨 turn 重置 + 证据检查真正的拦截。

#### 修正循环修复（3 项）
- **`allStepsPassed` 重写决策顺序**：`StepResults` 全成功时忽略 `Success`/`Errors` 字段——非致命错误（loop guard 阻止、maxSteps 耗尽）不再触发不必要的修正循环
- **`complete_step` 失败也记录进 `StepResults`**：移除 `!isErrorResult` 守卫，失败步骤记录 `Status:"error"`，`allStepsPassed` 能正确检测失败步骤
- **`executePlan` 不再强制 `Success=false`**：非致命 execErr（如 maxSteps 耗尽）仅追加到 Errors，不覆盖正确的 Success 状态

#### 停止门加固（4 项）
- **gate 计数器每 turn 重置**：`taskGateReentry`/`goalGateReentry`/`verifyGateFired` 不再跨 turn 累加——之前第二个用户 turn 后停止门永久失效
- **`finalReadinessCheck` 传入当前 todo 列表**：旧代码传 `nil` 导致 `UnverifiedCompletedTodos` 永不拦截——修复后真正检查 `complete_step` 缺失
- **`finalReadinessBlocks` 对齐重置**：随工具调用重置（与 `emptyFinalBlocks` 对称）
- **`goalGate` 添加 `disableVerify` 检查**：与 `taskGate`/`verifyGate` 保持一致

#### 其他修复（4 项）
- **`formatExecutionFeedback` 三分支结论**：`Success + Errors` 非空时不矛盾输出
- **grace round nudge 泄漏修复**：3 个 ctx 取消路径添加 `RemoveLast()`
- **ASK 卡宽度对齐**：`max-w-2xl`(672px) → `max-w-[--maxw]`(1100px)
- **移动端代码完全移除**：删 62 文件，清理 7 处引用（−6233 行）

#### 测试
- `hermes_test.go` +3 测试：`StepSuccessOverridesNonFatalErrors`、`StepSuccessButMixedWithMaxSteps`、`PartialSuccessStillFails`
- `stop_gate_test.go` +1 测试：`UnverifiedTodosBlocks`，适配新签名
- 全量 50 包 Go 测试通过，TS 编译通过

---


### 🪜 Ponytail 7 级梯子 + 🛡️ guard-skills AI 失败模式：双重编码质量提升

> 从 ponytail (89k⭐) 蒸馏 7 级决策梯子（benchmark: −54% LOC），从 guard-skills 蒸馏 15 种 AI 生成代码失败模式。

#### ponytail 蒸馏
- **7 级实现梯子注入 L2 hint**：在写代码前从第 1 级爬梯——(1)需要存在？→(2)已有？→(3)标准库？→(4)原生API？→(5)已装依赖？→(6)一行搞定？→(7)最少代码。`原生优于依赖,删除优于添加,无聊优于聪明`
- 梯子在理解问题后运行，不是替代理解

#### guard-skills 蒸馏
- **code-review-guard skill**：内置 bundle skill，三种模式：
  - `guard-pass`：交付前 diff 自检 → 修复 → 输出 `N fixed, M flagged`
  - `review`：结构化 PR 审查报告
  - `live`：边写边遵循规则
- **15 种 AI 失败模式**：catch-all 异常吞咽、幻觉API、过早抽象、硬编码成功返回、死代码、为琐事加依赖等
- **11 项自检清单**：交付前逐项走查 diff
- **安全护栏**：永不简化的底线（输入验证、错误处理、安全、无障碍）

#### 文件变更
- `internal/boot/sysprompt.go` — L2 hint 升级（代码阶梯+实现梯子）
- `internal/skill/bundled/code-review-guard/SKILL.md` — 新 skill (+105行)

---

## [10.98.0] — 2026-07-25

### 🔗 Closure Evidence：AI-Atomic-Framework 蒸馏

> 从 ATM 确定性治理框架蒸馏闭包证据机制 — complete_step 自动 Git 锚定形成不可抵赖的证据链。

#### Closure Evidence 蒸馏
- **complete_step Git 锚定**：每个步骤完成时自动 `git rev-parse --short HEAD`，输出中附带 `Git anchor: <sha>`，形成不可抵赖的证据链
- **evidence-freshness**：已由 `Ledger.Reset()` 天然保证（每轮清空收据），无需额外代码
- 蒸馏原理：参考 ATM 的 `closure-packet.v1`（`commandRuns SHA256 + targetCommit + governedTreeSha`），简化版只取 `git HEAD` 锚定

#### 文件变更
- `internal/tool/builtin/completestep.go` — Git 锚定注入 (+11行)

---

## [10.97.0] — 2026-07-25

### 🚦 Phase Gate + 🧠 Auto-Dream：headsign + Bamboo 联合蒸馏

> 两个互补改进：确定性 shell 验证门控 替代 LLM 判断，被动记忆提取 减少 Agent 认知负担。

#### headsign Phase Gate 蒸馏
- **`verify_gate` 工具**：shell 命令验证门控。退出码裁决 pass/fail，第一个失败即停。比 complete_step 的 LLM 判断更确定性。支持超时（默认 120s，最大 600s）
- **`compact` 注册**：紧凑描述 + Schema 已注册到工具集

#### Bamboo Auto-Dream 蒸馏
- **`archive.Dream` — 规则驱动的被动记忆提取**：从会话归档 JSONL 中自动提取用户偏好（"I prefer"/"我喜欢"）和项目约定（"we use"/"我们采用"）
- **`archive.DreamBatch` — 批量梦境运行**：遍历所有归档会话，带冷却期控制，避免重复提取
- **零额外 LLM 调用**：纯正则规则匹配，不增加 API 成本
- 输出格式：frontmatter（type/source/date）+ Markdown body，写入 `.tianxuan/memory/auto-dream/`

#### 文件变更
- `internal/tool/builtin/verify_gate.go` — 新工具
- `internal/tool/builtin/compact.go` — verify_gate 紧凑描述 + Schema
- `internal/tool/builtin/tool_extra_test.go` — 4 个 verify_gate 测试
- `internal/archive/dream.go` — Auto-Dream 提取引擎
- `internal/archive/archive_test.go` — 4 个 dream 测试

---

## [10.96.0] — 2026-07-25

### 🧠 四项目联合蒸馏：SDL-MCP + jcode + Bernstein

> 从四个新兴开源 agent 项目蒸馏关键设计精华到 tianxuan 内核。
> 聚焦编程工作流与编程能力，不动 L1 前缀缓存。

#### SDL-MCP 蒸馏
- **渐进式上下文阶梯**：L2 Runtime 注入四层代码阅读模型（lsp_definition → grep → read_file(offset) → 全文件），引导 Agent 从最便宜的方式开始探索代码
- **策略门控大文件保护**：read_file 超 200 行且无 offset/limit 时在返回结果中注入阶梯式降级建议，减少盲目全文件读取

#### jcode 蒸馏
- **语义记忆自动回想**：Agent 每轮启动时自动用 FTS5 BM25 搜索记忆，无需显式调用 `memory_search`。零外部依赖，沿用现有 BM25 索引
- **Split Prompt 优化**：确认 tianxuan TCCA L1/L2 分离 + cache_guard 已覆盖 jcode 设计，无需额外改动

#### Bernstein 蒸馏
- **Checkpoint 完整性校验**：FileSnap 新增 SHA-256 ContentHash 字段，`/undo` 恢复后验证字节级一致性，确保可重现性

#### 文件变更
- `internal/tool/builtin/readfile.go` — 阶梯式阅读门控提示
- `internal/tool/builtin/tool_extra_test.go` — TestReadFileRungLadderHint
- `internal/boot/sysprompt.go` — L2 渐进式上下文阶梯注入
- `internal/boot/boot.go` — MemorySearchFunc 注入
- `internal/agent/recall_reminder.go` — maybeAutoRecall + MemorySearchFunc
- `internal/agent/agent_run.go` — 调用 maybeAutoRecall
- `internal/checkpoint/checkpoint.go` — SHA-256 完整性校验

---

## [10.95.0] — 2026-07-24

### 🔧 双模型架构 15 项逻辑修复 + 契约简化

> Hermes/Hephaestus 协作质量提升：会话膨胀修复、反馈格式统一、计划粒度简化、子代理安全加固。

#### 逻辑修复
- **会话膨胀**：`planWithConfirmation` 退出/ask残留路径增加 `Truncate` 清理中间计划；`executePlan` 删除冗余预注入
- **修正图谱过期**：`planFix` 调用 `injectProjectMap()` 刷新项目结构
- **反馈降级**：`formatExecutionFeedback` → `formatExecutionFeedbackEnhanced`，统一 SDD 格式；超大反馈截断（>4KB）
- **userNote 丢失**：修正计划路径保留原始 `userNote`
- **子代理逃逸**：`subagentReg` 移除 bash，子代理只读
- **temperature 独立**：规划者用 `PlannerTemp()`，子代理用 `SubagentTemp()`
- **allStepsPassed**：空 `StepResults` 返回 false
- **wrapExecutorSink 泄漏**：`ResetSession` 重置 `executorSinkWrapped`
- **上下文窗口**：规划者窗口小于模型默认时 emit Warning
- **complete_step 去重**：同名步骤保留最后一次

#### Hephaestus 上下文补齐
- **handoff 注入项目根目录**：`formatHandoff` 新增 `项目根目录` 行
- **AGENTS.md 阈值上调**：`compactMemoryThreshold` 4096→16384，编码铁律完整进入 L1

#### 契约简化
- **计划格式**：Delta/File(s)/Change/Depends on → Goal + Constraint + Verify
- **Hephaestus 自主权**：从 "NEVER re-explore" 放宽为 "信任目标，自己找实现路径"
- **Hermes 计划前自检**：缓存前缀不变性 + 根因溯源 + 兄弟组件扫描
- **Proposal 强制**：每次 `<!--plan-->` 前必须写分析段
- **HephaestusSystemPrompt 去重**：删除与 L1 重复的编码铁律（TDD/Surgical/Simplicity）
- **设计技能表**：7 个设计技能完整注入 HermesPrompt

#### 前端对齐
- **StatusBar/AskCard** 宽度约束 `max-w-[--maxw]` 与 Composer 对齐

#### 桌面端
- `tianxuan-desktop.exe`：Wails build 固定位置

---

## [10.94.0] — 2026-07-24

### 🧠 XAI 规划者独立架构 + 轻量子代理路径

> XAI(Grok) 规划者完全独立于 DeepSeek AgentRunner，不沾 TCCA 四域缓存。
> 借鉴 Headroom 内容感知压缩策略。子代理对 XAI 走轻量路径跳过 TCCA 全套。

#### 新增
- **Planner 接口** (`planner.go`)：统一 DeepSeek AgentRunner 和 XAIPlanner 的契约
- **XAIPlanner** (`xai_planner.go`)：独立规划者，自然对话循环（模型自主探索→规划）
- **分层上下文** (`xai_context.go`)：L0-L4 五层管理 + Live-zone 压缩（grep/代码/日志 各有专用压缩器）
- **XAI Provider** (`internal/provider/xai/`)：OAuth 集成
- **轻量子代理** (`task.go`)：XAI 子代理走 runLightSubAgent，跳过 AgentRunner/TCCA

#### 改动
- `hermes.go`：plannerAgent → planner(Planner 接口)，newPlanner 工厂自动分发；XAI 用精简反馈
- `config.go`：planner_model 支持 XAI provider
- DeepSeek 代码（agent.go / cache_guard.go / compact.go）零改动

#### 设计借鉴
- Headroom live-zone 压缩 + 内容感知路由（compressGrepOutput/compressFileContent 等）
- Claude Code Plan Mode（工具始终可用 + prompt 引导替代阶段状态机）

#### 桌面端
- `tianxuan-desktop.exe`：20.1 MB（+0.8 MB，XAI provider + planner 代码）

---

## [10.93.0] — 2026-07-24

### 🔬 Gemini CLI 接口设计蒸馏：Kind 分类 + tailToolCall + SubagentDefinition

> 从 google-gemini/gemini-cli 蒸馏三项接口设计精华到 tianxuan 内核。
> 全部遵循 Go 可选接口模式，零破坏性、纯增量。

- **ToolKind 分类系统**：`tool.ToolKind` 枚举 10 种分类 (Read/Edit/Write/Delete/Move/Search/Execute/Fetch/Agent/Other)，`KindedTool` 可选接口 + `ToolKindOf()`/`IsMutator()`/`String()` 辅助函数
- **27 内置工具 Kind() 实现**：全部 builtin 工具声明精确分类，read_file→KindRead, bash→KindExecute, edit_file→KindEdit 等
- **tailToolCallRequest 链式调用**：`toolOutcome.tailCall` + `executeBatch` for 循环，工具可链式调用另一个工具替换自身结果
- **SubagentDefinition**：新文件 `agent/subagent_def.go`，7 字段类型安全定义 + `SubagentBuilder` + `BuildTaskArgs()`
- **清理 release 目录**：移除 11 个历史 release 文件

### 📁 文件变更
- `internal/tool/tool.go` (+77): ToolKind 枚举 + KindedTool 接口
- `internal/tool/builtin/*.go` (27 files): 全部添加 Kind() 方法
- `internal/agent/batch_executor.go` (+12): tailCall 链式处理
- `internal/agent/subagent_def.go` (新文件): SubagentDefinition + Builder

## [10.92.0] — 2026-07-23

### ⚡ 单模型编程能力深度优化

> SoloSystemPrompt 结构化重写，全面对齐双模型 Hephaestus 的编程纪律。
> 新增 Think Before Coding 阶段、TDD 5 步显式循环、Pre-completion 回归测试清单。

- **Think Before Coding**：新增预编码阶段——先读相关代码→了解模式/签名/错误风格→检查规范→用读验证
- **TDD 5 步循环**：从隐式 `TDD per step` 展开为显式 a)写失败测试→b)确认失败→c)最小代码→d)确认通过→e)complete_step
- **Pre-completion 清单**：新增 5 项交付前检查（测试套件、go vet、文件一致性、最终 verify）
- **Per-step 报告格式**：明确 `complete_step` result 格式 + 具体示例
- **ask 工具强化**：对齐 Hephaestus 强度——"text question = 轮次终止，zero excuse"
- **Go 错误处理指引**：Defensive 规则新增 `fmt.Errorf("...: %w", err)` 包装 + 禁止 `_` 丢弃
- **Simplicity First 去重**：合并到 Core Principles 的 Minimal 规则
- **并行指引精简**：合并 parallel_tasks/parallel_skills/bash background 为一行

### 📁 文件变更
- 1 file: hermes_prompt.go (+72/-46)

## [10.91.0] — 2026-07-23

### 🧬 ui-ux-pro-max 技能完整内置 + go:embed 嵌入编译

> 从官方 nextlevelbuilder/ui-ux-pro-max-skill v2.11.0 完整下载 7 个设计技能，
> 通过 go:embed 嵌入二进制，首次运行自动解压到 `~/.tianxuan/skills/`。

- **技能内置**：ui-ux-pro-max 从扁平 `.md` 转为子目录格式（45 文件），含 search.py / design_system.py / 14 CSV 数据 / 22 技术栈 CSV / 2 references
- **6 个子技能同步**：banner-design / brand / design / design-system / slides / ui-styling SKILL.md 全部更新为官方 v2.11.0
- **go:embed 嵌入编译**：`internal/skill/embed.go` 嵌入全部 151 技能文件，`EnsureBundled()` 解压到全局目录
- **初始化集成**：`sysprompt.go` 在 Store 创建前调用 `skill.EnsureBundled("")`，幂等安全
- **构建脚本**：build-desktop.bat / build-wails.bat / Makefile 添加编译前 robocopy 同步步骤
- **测试**：+2 新测试（TestBundledSkillsEmbedded + TestEnsureBundledExtracts），70 测试全部通过

### 📁 文件变更
- 159 files: bundled/ (151) + embed.go + sysprompt.go + skill_test.go + 2 build scripts + Makefile + CHANGELOG

## [10.85.0] — 2026-07-15

### 🎨 设置面板六轮深度 UI 打磨（ui-ux-pro-max 驱动）

> 基于前三轮结构重构后的精细化打磨：对比层次、高亮选择、动画过渡、按钮补齐。

- **R1 卡片化**：SettingsSection 从纯文本分组改为 `bg-bg-soft` 卡片容器 + `shadow` 微阴影 + `rounded-xl`；SettingsField 字段间添加 `border-b` 微分隔线
- **R2 嵌套对比链**：`bg-elev`（内容区）→ `bg-bg-soft`（Section 卡片）→ `bg-bg`（内部控件/输入框/子卡片）三层深度对比
- **R3 深度打磨**：卡片添加 `shadow-[0_1px_3px_rgba(0,0,0,0.2)]` 浮起感；页面标题加 `border-b` 底部分隔；CollapsibleSection 展开区加 `border-l-2` 左侧标记+`bg-bg/40` 微背景；子卡片 border 从 `border-soft` 升级到 `border`
- **R4 高亮选择**：SegmentedButton 激活项加 `ring-1 ring-accent/40 scale-[1.02]`；EffortSelect 升级 `ring-2 shadow-sm`；FontChip 加 `ring-1 shadow-sm`；配色方案色块 `ring-2 shadow-md scale-[1.03]`；NavButton 激活态 `bg-accent/20 font-bold w-[4px]`
- **R5 按钮补齐**：`btn--small`/`btn--ghost`/`btn--tiny` 三个 `@utility` 修复实际样式缺失 bug（5 页面引用但从不生效）；全部 SettingsSection 标题图标统一 `text-accent` 色
- **R6 动画打磨**：标签页切换添加 `animate-[fadeIn_150ms_ease-out]` 淡入动画；空态/加载态包裹卡片容器；Modal header 加 `bg-bg-soft/50` 微背景；SettingsGeneral 包裹 SettingsPageShell 获得统一标题栏
- **Sidebar**：移除设置按钮 `running` 禁用限制，允许运行中打开设置面板

### 📁 文件变更
- 16 files: 14 Settings*.tsx + SettingsPageShell.tsx + tailwind.css + Sidebar.tsx

## [10.84.0] — 2026-07-14

### 🎨 设置面板全面 UI 重构（ui-ux-pro-max 驱动）

> 14 个设置标签页全部统一为 SettingsPageShell 包装 + Lucide 图标 + 设计系统优化。
> 使用 ui-ux-pro-max search.py --design-system 获取 Swiss Modernism 2.0 / Aurora UI / Exaggerated Minimalism 等 4 套设计系统数据。

- **设置面板精简**：20→14 个标签页。删除 plugins（与 MCP 重复）、diagnostics；models+subagents 合并入 agent 智能体标签
- **通用面板**：10 个散落 Section 合并为 5 组（语言/外观与布局/工具/智能体/记忆与上下文）；声音和状态栏改用 chevron 折叠面板；Section 标题加图标
- **智能体面板**：Swiss Modernism 2.0 网格布局；3 个 ModelCard（执行/规划/子代理）+ ModelPicker + EffortSelect；步数/温度/推理用 SettingsField 统一
- **外观面板**：Aurora UI 渐变配色 swatch；亮暗模式 emoji→Sun/Moon/Monitor 图标；字体选择 <select>→FontChipGroup 胶囊按钮
- **权限面板**：SettingsPageShell + Shield 图标；🚫❓✅ emoji→ShieldBan/Question/Check 彩色图标
- **沙箱面板**：SettingsPageShell + Box 图标；裸字段→SettingsSection 重组
- **MCP 面板**：Wrench 图标 + 状态灯 pulse 动画；空状态 Server 图标居中引导
- **技能面板**：Zap 图标 + scope 彩色标签（蓝/绿/琥珀/紫）+ 搜索过滤
- **记忆面板**：Bookmark/Archive/FileText 子标签图标；Marketplace 统计概览条；快速添加 chevron 折叠
- **其余面板**：Providers/Network/Updates/Shortcuts/Hooks + 对应图标；Search/LSP/Codegraph 图标+中文标题
- **共享组件**：SettingsPageShell/SettingsSection title 类型 string→ReactNode

### 🧠 记忆功能补齐

- **Go 后端**：MemoryView 新增 StoreGlobalDir + Archives 字段；Memory/MemoryForTab 填充归档记忆数据
- **TS 类型**：MemoryView 新增 storeGlobalDir，archives 从可选改为必选
- **输入统一**：左侧记忆按钮→直接打开设置面板 memory 标签页；删除独立 MemoryPanel 抽屉（-625行）

### 🗑️ 代码清理

- 删除 CapabilitiesPanel（MCP+技能独立抽屉，-625行），能力配置统一到设置面板
- 移除 App.tsx 中 8 个 memory 回调 + capsOpen 状态，净减 ~120 行
- 修复 settings_advanced.go 缺少 package main + config 导入 + API 字段名

### 📦 构建

- 桌面端构建通过：tianxuan-desktop.exe 16.7 MB
- TypeScript 编译零错误

### 🔒 安全加固 + 后端核心蒸馏

> 从 DeepSeek-Reasonix-latest 蒸馏 4 个核心子系统：planmode.Policy 安全策略、secrets 密钥脱敏、goal FSM 目标状态机、environment 环境探测。

- **planmode.Policy 工具安全策略**：新建 `internal/planmode/` (policy.go, ~600行) — 11 类工具自动分类（knownBlocked/alwaysAllowed/planSafeAudited/PlanSafeSelfReported）+ bash 参数级写操作检查（`find -exec`/`git --output`/`go -mod=mod` 被拦截）；移植 `shellparse` + `shellsafe` 依赖包（~540 行）；`tool` 包新增 `PlanModeClassifier` / `PlanModeUntrustedReadOnly` 接口
- **AgentRunner 集成**：`planModeGate` (atomic.Bool) + `SetPlanMode` + `executeOne` 提前检查（在 dispatcher/gate 之前）；`Hermes.SetPlanMode` 双传播方法对齐 DeepSeek Coordinator
- **secrets 密钥脱敏**：新建 `internal/secrets/` (redact.go, ~200行) — 10 种 token 格式识别脱敏（API_KEY/SECRET/TOKEN/PASSWORD/JWT/OpenAI/GitHub/Slack/AWS/Bearer）；`executeOne` 在工具结果进入模型上下文前自动脱敏
- **goal FSM 目标状态机**：新建 `internal/goal/` (goal.go, ~330行) — 完整 4 状态机（running/complete/blocked/stopped）+ turn/idle/intercept/strict 管理 + 持久化
- **environment 环境探测**：新建 `internal/environment/` (probe.go + snapshot.go, ~500行) — 11 工具运行时版本探测 + 5分钟内存缓存 + 24小时快照持久化 + 跨重启稳定合并逻辑
- **依赖**：新增 `mvdan.cc/sh/v3 v3.13.1`

### 📦 变更统计

- **10 新文件**：shellparse/shellsafe/planmode/secrets/goal/environment（~2,500 行）
- **5 修改文件**：tool.go/agent.go/agent_config.go/execute_one.go/hermes.go（~180 行）
- **go vet** 零警告，缓存前缀稳定性验证通过

## [10.64.0] — 2026-07-13

### 🚀 5 阶段追赶 Reasonix 设置面板差距

> 从 7 Tab → 17 Tab，补齐 Provider 高级字段、DesktopConfig、ToolsConfig、MemoryCompiler、预设系统、诊断面板。

- **阶段1: Provider 高级字段（9字段）**：`ProviderEntry` 新增 ChatURL/ModelsURL/Headers/ExtraBody/AuthHeader/VisionModels/ReasoningProtocol/SupportedEfforts/DefaultEffort；`render.go` 完整 TOML 渲染；前后端完整读写闭环
- **阶段2: DesktopConfig + Tools扩展 + MemoryCompiler**：新增 `[desktop]` 配置段（8字段）控制布局/显示/关闭/状态栏/遥测；`[tools]` 扩展 bash超时/MCP超时/shell/搜索引擎；`AgentConfig.MemoryCompilerEnabled`；edit.go 10新 mutation + settings_app 10新 setter + DesktopView
- **阶段3: 纯前端追赶**：默认模型选择器从原生 `<select>` 升级为 ModelSwitcher（搜索+分组+Check标记）；Subagents 面板从占位升级为真实数据面板（全局设置+按技能覆盖）；字体/缩放/Zoom；Hooks 9事件类型说明
- **阶段4: Provider 预设**：7 模板一键填表（DeepSeek/OpenAI/Anthropic/Kimi/Qwen/GLM/Ollama）+ 快速添加下拉 + 自动填充 ProviderEditor 表单
- **阶段5: Diagnostics 诊断面板**：MCP服务器/Skills/Memory/Version/Context 5项实时检查，带绿/黄/红状态指示

### 🔧 设置面板全面重构 (v10.63.0)

> P1-P4 四阶段对齐 DeepSeek-Reasonix，16 Tab 全部就位，零编译错误。

- **后端**：AgentConfig 新增 MaxSubagentDepth/ColdResumePrune/ReasoningLanguage；edit.go 7 新 mutation；settings_app 9 新 setter；ProviderView 扩展 Thinking/Effort
- **前端**：11 新组件（SettingsGeneral/SettingsNetwork/SettingsMcp/SettingsSkills/SettingsMemory/SettingsSubagents/SettingsPlugins/SettingsHooks/SettingsPageShell/SettingsShortcuts）；分组导航+搜索+Record映射表渲染；16 Tab 全部就位零占位
- **Appearance 增强**：字体大小/显示缩放/布局风格（classic/workbench/creation）/关闭行为/状态栏
- **i18n**：en/zh/zh-TW 各新增 18 个 settings.* 键 + 全量 settingsTabMeta 使用 t() 调用
- **快捷键**：12 个全局快捷键列表 + 作用域说明
- **Provider Thinking/Effort**：前后端完整读写闭环

## [10.62.0] — 2026-07-13

### 🎨 思考卡/工具卡/过程卡 样式全面优化

- **思考卡 (Reasoning)**：字体 11.5→12px；头部增加 transition + active:scale 点击反馈；内容区增加微妙背景渐变 + 圆角边框；BrainIcon 增强 stroke 可见性；shimmer 动画节奏优化（5s→4s，220%→240%）
- **工具卡 (ToolCard)**：头部增加 transition + hover/active 三态反馈；Wrench 图标区分工具类型；内容区半透明边框 + 圆角；工具名称独立颜色层次；错误展示左边框强调 + 半透明底色；嵌套工具 70% 透明边框层级；diff 标签加粗对齐
- **过程卡 (TurnCollapse)**：独立 `turn-collapse__head` 样式（hover/active/transition）；内容区微妙渐变 + 圆角 + 间距优化；子思考卡 hover 高亮 + 圆角 + padding；inline-reasoning 75% 透明边框 + 圆角；compaction hover 边框过渡
- **React #310 热修复**：TurnCollapse 内 `useMemo(body)` 移到提前 `return null` 之前，确保 hooks 数量在所有渲染路径中一致（9→10）
- **布局**：对话区 padding px-12→px-24；输入框同步对齐 px-20（含 footer px-4 合计 96px）

### 🗑️ 移除移动端访问功能

- 删除 `desktop/mobile_access.go`（329行）— HTTP/SSE/ngrok 移动端远程访问
- 删除 `SettingsMobile.tsx` — 设置面板移动端标签页
- 清理 `app.go` 中 `serve.Broadcaster`、SSE 转发、FIXME 注释
- 清理 `bridge.ts`/`types.ts`/`mock.ts` 中 6 个移动端 API
- 清理 locales 中 6 个移动端 i18n key
- **二进制体积：23.6MB → 16.3MB（-31%）** — 移除 `internal/serve`（webui + mobileui 嵌入资源）及 ngrok/qrcode 依赖

## [10.61.0] — 2026-07-13

### 🎨 消息卡片 UI 全面对齐 DeepSeek-Reasonix

> 参考 DeepSeek-Reasonix main-v2 蒸馏优化，全线消息卡片视觉和交互升级。

- **ToolCard 重写**：Tailwind inline → 语义化 CSS 类体系（`.tool` / `.tool__head` / `.tool__body` / `.tool__label-group`）；状态文本图标（✓✗—）；Shell 输出前 10 行预览 + "显示全部"；子代理嵌套计数（Compass 图标）；错误摘要 + 可展开详情；客户端耗时追踪
- **ReadOnlyBatch 新增**：连续只读工具自动合并为折叠行
- **TurnCollapse 始终渲染**：过程卡始终存在——运行时自动展开 + shimmer，完成后自动折叠；思考块内每个推理可独立折叠（InlineReasoning）；阶段自动折叠（完成后冻结耗时不再读秒）
- **过程分段**：每个 assistant 文本作为分界点，形成"过程卡→文本→过程卡→文本"交替结构；思考统一放入过程卡，文本区纯净无重复
- **ReasoningProcess 升级**：ProcessBrainIcon SVG + `reasoning__head` CSS + `data-running` shimmer
- **PhaseCard 图标化**：ProcessPhaseIcon；phase 项移出过程卡作为章节标题
- **NoticeCard 重写**：图标 + title/body 解析 + 长文本折叠
- **CompactionCard CSS**：语义化样式
- **CSS 全面同步**：shimmer 三合一；process-sweep + card-body-in；reasoning 精确对齐；notice-line + diag-line
- **布局优化**：内容最大宽度 960→1100px，两侧留白 px-8→px-12
- **全量中文化**：所有英文标签→简体中文

### 🔧 热修复

- 移除 useNow 每秒重渲染，改用 Date.now() + ref 冻结耗时
- TurnCollapse key 从 segIdx→首项 ID 防 React 错配
- 过滤空 segment 防无内容 TurnCollapse 实例
- 过程卡左边框线恢复（border-left + padding）

- **ToolCard 重写**：Tailwind inline → 语义化 CSS 类体系（`.tool` / `.tool__head` / `.tool__body` / `.tool__label-group`）；状态文本图标（✓✗—）；Shell 输出前 10 行预览 + "显示全部"；子代理嵌套计数（Compass 图标）；错误摘要 + 可展开详情；客户端耗时追踪（useRef 计时 + useNow tick）
- **ReadOnlyBatch 新增**：连续只读工具（read_file/ls/grep/glob）自动合并为折叠行，减少视觉噪音
- **TurnCollapse 始终渲染**：不再区分运行时/完成时两套渲染路径，过程卡始终存在——运行时自动展开 + shimmer 扫光，完成后自动折叠；思考块内每个推理可独立折叠（InlineReasoning）
- **过程分段修复**：每个 assistant 文本作为分界点，形成"过程卡→文本→过程卡→文本"交替结构（对齐 Reasonix partitionTurnItems）
- **ReasoningProcess 升级**：Lucide Brain → ProcessBrainIcon SVG；Tailwind 按钮 → `reasoning__head` CSS 类 + `data-running` shimmer
- **PhaseCard 图标化**：新增 ProcessPhaseIcon SVG，阶段分隔带图标
- **NoticeCard 重写**：TriangleAlert/Info 图标按 level 区分；首行→title + 余文→body 解析；长文本（>200 chars）折叠展开
- **CompactionCard CSS**：语义化 `.compaction`/`.compaction__head`/`.compaction__body`
- **CSS 全面同步**：shimmer 三合一（tool__head / reasoning__head / turn-collapse__reasoning-head）；process-sweep + card-body-in 关键帧；reasoning 字体/间距/色值精确对齐；notice-line + diag-line 通知卡片样式体系
- **全量中文化**：TurnCollapse/InlineReasoning/CompactionCard/ReadOnlyBatch/Shell 预览/Warm 层所有英文标签→简体中文

## [10.60.0] — 2026-07-13

### 🧠🔨 双模型架构硬化

- **L1/L2 解耦**：AGENTS.md 剥离双模型/单模型专属规则，只保留通用编码铁律；模式专属行为由各自 L2 系统提示词独立定义
- **Hephaestus 提示词重构**：去掉「从 Hermes 或系统直传」等实现细节，统一为「Hermes 发送计划→执行」；补充 dual-model architecture 身份声明
- **快速路径消息统一**：`!` 前缀的快速执行路径改用 `formatHandoff` 包装消息，Hephaestus 始终收到一致的手交格式
- **Ask 工具全系统强制**：AGENTS.md + HephaestusSystemPrompt + SoloSystemPrompt 三层覆盖 🔴 级 ask 工具规则，杜绝纯文本提问导致的轮次中断
- **SoloSystemPrompt 清理**：移除误加的 Hermes 引用行，ask 规则从弱提示升级为 Core Principles 🔴 条目

### 🖥️ 桌面端改进

- **崩溃恢复通知**：崩溃堆栈提取摘要并通过 sink 发送 UI 通知（`[crash]` → 用户可见），补充 slog 日志记录
- **窗口状态容错**：`WindowGetSize`/`WindowGetPosition`/`WindowIsMaximised` 各自包裹 recover 保护，WebView2 nil 崩溃不再阻止状态保存
- **推理深度标签**：「推理→思考」重命名；关闭/标准/深度三档增加 hint tooltip 说明
- **配置渲染**：`config render` 支持 `effort`/`planner_effort`/`subagent_effort` 字段输出

## [10.59.0] — 2026-07-12

### 🎯 MCP 工具精简

- **移除 GitNexus MCP**：13 个工具从执行者 reg + 规划者只读注册表中移除，代码图能力已被 `mcp__codegraph__*` 完全覆盖
- **规划者排除 GitHub MCP**：9 个只读 GitHub 工具（search_code/list_issues 等）对本地代码调查无价值，从规划者 schema 中排除
- **HermesPrompt 更新**：工具列表移除 `gitnexus` 引用，与实际工具集一致
- 合计节省 ~6,500 schema tokens/请求（规划者 ~4,000 + 执行者 ~2,500）

### 🖥️ 桌面端 UI 优化

- **上下文卡片移至侧边栏**：规划者（紫）+ 执行者（青）用量条从顶栏移到左侧边栏独立卡片，折叠时自动隐藏
- **计划确认隐私修复**：`displayPlan()` 提取 `<!--plan-->` 之后的结构化计划，防止分析前言中的记忆内容泄漏到确认弹窗
- **会话模块优化**（5 项）：
  - `resumeSession` 闪白修复：新增 `resume` action 单次 dispatch
  - 搜索/分组 `useMemo`：Sidebar 搜索过滤 + HistoryPanel 日期分组 memo 化
  - 时间格式统一：`sessionTime` / `dayLabel` 跨年自动显示年份
  - 编辑状态互斥：`startRename`/`startDelete`/`cancelEdit` 包装函数防止同时激活

### ⚡ Go 后端优化

- **Session preview 缓存**：`.sessions.cache.json` 按 mtime 缓存 preview+turns，命中时跳过 jsonl 读取，大幅减少 ListSessions I/O

---

## [10.58.0] — 2026-07-12

### 📱 移动端远程操控

- Token 认证 + ngrok 外网访问
- 桌面端设置面板（移动访问开关 + Token 管理）
- web-mobile 复用架构
- 全库 goroutine panic 保护全覆盖

---

## [10.57.0] — 2026-07-11

### 🔴 双模型架构深度优化（4 轮，19 项改进）

#### 证据链严格验证

- **StrictEvidence 启用**：双模型模式下 complete_step 的 verification/diff/files 证据与 turn ledger 交叉验证，todo_write 的新 completed 项必须有对应 complete_step receipt
- **StrictEvidence 配置链路**：`agent_config.go` → `agent.go` New() → `boot.go` 双模型自动启用

#### 代码质量与重构

- **hermes.go 拆分**：736 行 → hermes_prompt.go（134行）+ hermes_confirm.go（62行）+ hermes.go（502行）
- **Hermes.Run() 重构**：168 行 → 24 行高层编排，提取 `runFastPath`/`injectProjectMap`/`planWithConfirmation`/`executePlan`/`feedResultToPlanner` 子函数
- **TurnResult.Plan 字段**：统一 TurnResult/PlanResult 结构，PlanResult 构造从 TurnResult 直接读取
- **配置连通**：`planner_max_steps` 全链路（config → boot → NewHermes），替代硬编码 0

#### 缺陷修复（6 项）

- **快路径双重 TurnStarted**：`!` 前缀现在也抑制 executor 的 TurnStarted，防止前端成本统计归零
- **planMaxSteps 边界**：移除 `>= 0` 条件，负值不再回退到零工具 planStream
- **重规划循环会话污染**：prePlanLen 不推进，失败回滚始终到循环入口基线
- **revise feedback 累积丢失**：`input = input + feedback` 替代 `input = origInput + feedback`
- **Controller panic 双 TurnDone**：recover 路径设置 panicked 标志，防止双发射
- **formatExecutionFeedback 冗余**：execErr 路径复用 `formatExecutionFeedback()` 替代内联拼接

#### 措辞与注释

- `"(no summary)"` → `"(execution produced no summary — check Errors for details)"`
- `V10.??` → `V10.58`，孤行注释缩进对齐，plannerAgent 显式 `StrictEvidence: false`
- direct answer 路径移除硬编码 `Summary: "direct answer"`

#### 测试覆盖

- **14 个新测试**：Solo/Hephaestus/Hermes 3 个 prompt 常量验证、互异检查、formatExecutionFeedback 3 场景、hasStructuralChange 5 场景

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
## [10.142.0] — 2026-08-01

### 🧠 记忆系统重构 — 项目归一存储 + 自动提取 + 跨会话记忆 + 自主进化

> 需求：以项目为基准强化会话持久性、跨会话记忆、自主进化；调研 GitHub 同类
> 产品（Qwen Code Managed Auto-Memory / Claude Code / agentmemory / Letta）并蒸馏
> 实现。全部按四域缓存前缀不变性约束落地：记忆变更一律 turn-tail 注入、下一
> session 才折入前缀，不破坏 DeepSeek 前缀缓存。

#### P1 项目基准归一
- **`memory/store.go`**：`StoreFor` 按 git-root slug 分目录——同一仓库任何
  子目录共享同一份记忆；无 git 回退 cwd slug
- **跨项目记忆**：新增 `GlobalDir=<userDir>/memories`，user/feedback 类型事实
  自动路由过去，`Save`/`ChangeType`/`Archive` 跨目录定位、同名项目级优先
- **`migrateLegacyStore`**：`Load` 时一次性迁移旧 cwd-slug 存储（目标存在则不
  合并）；`Index()` 合并 global+project，`ListIn(dir)` 按目录列出

#### P2 自动 Extract（用户确认后落盘）
- **`memory/extract.go`**：每轮 turn 后扫描 user+assistant 消息，extract-cursor
  游标（sessionId+offset）防重复、压缩后 clamp；候选暂存 `pending/`，过滤
  `<memory-update>`/`[auto-recall]` 等 transient 块，与已有记忆去重
- **`control/controller_memory.go`**：`autoExtract` 回合尾部触发；桌面面板新增
  「待确认」入口，接受/拒绝后落盘

#### P3 召回增强 + 缓存对齐
- **`memory/search.go`**：`SearchMatch` 携带 Body+Mtime；auto-recall 注入正文
  截断（1200 字符）+ 新鲜度提示（>1 天警告"时点观察需验证"）
- **`agent/recall_reminder.go`**：`maybeAutoRecall` 改为**会话级一次性**（首轮
  注入），避免每轮动态块落在缓存未命中区；命中记忆强化访问计数

#### P4 Dream 自动化
- **`memory/dream.go`**：24h/5 会话/same_session 门控 + `NewSessionFiles` mtime
  扫描 + `RunDream` 机械整合跨会话主题为 1 条候选；门控通过时顺带执行
  `EvictStale`（90 天 TTL 弱记忆归档）+ `BuildProfile` 项目画像

#### P5 @session 跨会话引用
- **`control/refs.go`**：`@session:<id>` 注入确定性只读摘要——保留 user/
  assistant 文本、工具结果压成单行长度、8k 预算保最新并标记省略，无 LLM 调用

#### P6 项目画像 + 强化衰减
- **`memory/strength.go`**：`strength.json` 强化计数 + `EvictStale` TTL 归档
  （可恢复）+ `profile.json` 画像（top concepts/类型分布/common errors）

#### 桌面端
- **记忆面板重排**：新增「待确认」subTab（自动提取+Dream 候选）、saved 区分
  项目/跨项目来源徽章、项目画像卡、召回次数 badge
- **文件预览弹窗化**：点击文件树/变更列表文件 → Modal 弹窗预览，不再内嵌占用
  面板宽度
- **发送排队恢复**：运行中 Enter 恢复排队（列表可视化、逐条取消、发送按钮
  badge），纠偏改为显式 ⚡ 按钮，Shift+Enter 取消+重发保留

#### 测试
- 新增 store_roots / extract / dream / strength / search / recall / refs /
  controller extract 测试；全仓 `go test ./...` 无 FAIL、go vet 通过、
  `tsc --noEmit` 0 错误、vitest 52 用例全过、desktop `wails build` 通过
## [10.153.0] — 2026-08-03

### 🛡️ 编辑安全：edit_lines 锚点校验 + 自动语法检查回滚

> P0 最痛点：纯行号替换多次编辑后偏移，导致误删声明/字段覆盖/括号缺失。

#### 变更
- **内容锚点定位**：`start_anchor`/`end_anchor` 校验起止行实际内容，不匹配即拒绝且不写文件
- **编辑后自动校验**：`validate`（默认 true）——`.go` 走 `gofmt -e`；`.ts/.tsx` 走项目本地 `tsc --noEmit --skipLibCheck`（限时 60s），失败/超时自动回滚
- 换行符感知保持（CRLF/LF 归一化，锚点比对前归一化）

#### 测试（TDD RED→GREEN）
- 新增 13 例：锚点不匹配拒绝×2、匹配通过、CRLF 锚点、Go/TS 校验回滚与通过、validate=false 跳过
- 全量 go test ./... 通过；go vet 干净

### 🪟 Windows/PowerShell 工具适配

> P1：heredoc 不支持、npx.ps1 被执行策略禁、git 不在 PATH，多次重试。

#### 变更
- heredoc 转 here-string：`cat > / >> file` → WriteAllText/AppendAllText；`python/node/tsx -` → stdin 管道
- 裸 npm/npx/pnpm/pnpx 自动 `.cmd` 后缀，绕开 PowerShell `.ps1` 优先解析的执行策略拦截
- git 不在 PATH 时自动注入常见安装目录（ProgramFiles/PortableGit）
- 服务命令自动后台改为基于原始命令判定

#### 测试（TDD RED→GREEN）
- 新增 11 例（含真实 PowerShell e2e 写文件）

### 🚀 发布流水线：内置 release 技能

> P3：打包手写临时脚本、版本号手同步、changelog 手写。

#### 变更
- release 技能（项目级 + bundled 内置）：版本号 4 处同步、全量验证先行、打包、模板、提交核对清单
- `scripts/package.ps1`：定位 exe → `release/vX.Y.Z/` + SHA256SUMS（内置排除清单）
- autotrigger 注册"发布/打版/打包发布"触发词

### 🌿 Git 工作流约定（L1 静态提示）

> P4：master 禁提交导致 19+ 项变更堆积。

- 多任务/长会话开始前提示 git_status 检查 + git_worktree 建功能分支
- 完成按逻辑单元拆分提交；被保护分支拒绝时先迁功能分支

### ⚠️ 高危环境变量主动预警

> P5：DATABASE_URL 用户级变量复发连错库。

- 数据库类命令（prisma/psql/mysql/migrate）执行前比对用户级 DATABASE_URL 与项目 .env
- 不一致时 plain 输出前缀注入 env-warning、JSON 模式进 warning 字段；一致/无关命令零噪音
## [10.154.0] — 2026-08-04

### 🧬 Codex CLI 工具蒸馏：执行前校验 + 参数语义 + 错误闭环

> 克隆 openai/codex 开源版（main @ 6d4d944）蒸馏其工具设计，直击"工具出错率高"
> 根因：模型看不到参数语义（compact schema 剥光描述）+ 参数错误被静默容错
> （模型永远学不会）+ 无错误度量。详见 docs/design/codex-tools-distillation.md。

#### 变更
- **执行前 schema 校验（对齐 codex json_schema.rs）**：新增 `internal/tool/validate.go`，
  内建工具在 precheck 之前校验必填/类型/枚举，错误以 `validation_error` 大声失败并
  附带完整 schema 提示（模型下一轮按 schema 修正）；别名（file→path、job_id→job_ids、
  timeout_ms→timeout_seconds）继续兼容并满足必填检查；无效 schema 安全降级放行
- **compact schema 保留参数描述（V10.154 根治参数盲猜）**：30 个内置工具 compactSchema
  补齐精简英文描述（默认值/范围/语义）；新增 `CanonicalizeSchemaVerbose` 防止规范化
  管线二次剥离 description（`FilteredSchemas` compact 分支启用）
- **工具错误统计（codex ToolDispatchTrace 蒸馏）**：新增 `internal/tool/stats.go`
  （tool × error_kind × count/last_seen，JSON 原子落盘 .tianxuan/tool-stats.json）；
  executeBatch 统一记录（排除权限门控）；`tianxuan tools stats` 新命令查看
- **修复 learning 学习链路 bug**：`Extract` 返回的 pattern 从未 merge（计数永不增长、
  系统提示注入形同虚设）→ 新增 `Observe`（提取+合并+持久化）并接线 executeBatch
- **validation_error 通配分类**：learning 识别校验器新错误类型，恢复提示引导模型
  按错误内嵌 schema 重发参数
- **编辑错误反馈补齐**：delete_range/delete_symbol 的 not found/not unique 错误补恢复
  hint（re-read / code_index outline 验证），对齐 edit_file 反馈质量

#### 测试（TDD RED→GREEN）
- 新增 ValidateArgs 10 用例（必填/类型/枚举/别名/嵌套/null/畸形 JSON/无效 schema 降级）
- 新增 Stats 7 用例（聚合/持久化/排序/空快照/ClassifyError 分类）
- 新增接线 3 用例（validation_error 阻断、file 别名放行、stats 记录）
- 新增 compact 描述保留 2 用例（FilteredSchemas 管线 + 防回归）
- 新增 learning Observe/阈值/validation_error 3 用例（锁定 merge 修复）
- 全量 go test ./... 通过，go vet 干净，go build EXIT 0
