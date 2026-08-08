## V10.171.0 (2026-08-08) — Codex 蒸馏 P2-7 收尾：apply_patch 补丁编辑 + tools trace-report

### 变更
- **apply_patch 工具**（蒸馏 codex freeform 编辑，P2-7）：patch 文本（*** Begin Patch
  ... *** End Patch），支持 Add File / Delete File / Update File（@@ 锚点 + 上下文行 +
  行级 -/+ 增删 + *** End of File 钉尾）
- **行级模糊匹配**：忽略行首尾空白（对标 codex seek_sequence），告别 old_string
  大小写/空白/CRLF 精确重现；无 @@ 锚点时删除块必须唯一，多处匹配大声报错
- **跨文件原子**：全部 hunk 内存校验通过才写盘；CRLF 保持、权限保留、路径 confine
- **tools trace-report**：从 JSONL trace 聚合每工具错误率表（calls/success/errors/
  error_rate/avg_ms + top 3 错误），真实数据驱动下一轮优化决策
- **apply_patch 错误细分**：ClassifyError 支持 patch_parse_error / block_not_unique /
  block_not_found 分类，tools stats 与 trace-report 有细粒度视图

### 发布产物
- `release/v10.171.0/tianxuan-desktop.exe` · 25050624 bytes (~23.9 MB)
- SHA256: `ab3fb76f0a20c914f8a768c2869ccc58719531319c4a8818afa9ad8c91840a57`
- 验证：`go test ./...` 全绿（仅 5 个预存 API 认证类失败）· `go vet` 干净 · 打包 EXIT 0

---

## V10.170.0 (2026-08-08) — Codex 蒸馏 P2 + 技能并发修复 + bash 结构化头

### 变更
- **get_context_remaining 工具**（蒸馏 codex）：模型主动查询上下文剩余 token，
  长任务在窗口溢出前规划收敛；executeOne 注入 tokensLeft 闭包（Window − chars×tokPerChar）
- **ToolDispatchTrace 落盘**：每次工具调用结构化 JSONL（ts/session/trace/call_id/
  tool/args/outcome/error/duration），参数截断 500/错误 300，懒打开不持句柄；
  CLI 新增 `tianxuan tools trace [-n N]`
- **修复技能工具不并发**：getConflictKey 增加 registry 感知，ReadOnly 技能工具
  （explore/research/review 等）返回 ro: 共享键互相并行，与写工具 file:* 互斥
  （保住写后读顺序）；V10.124→V10.147 技能工具化回归导致永久串行
- **bash 失败输出结构化头**（对标 codex format_exec_output_for_model）：
  plain 模式失败输出 `Exit code / Wall time / [Total output lines] / Output`，
  成功路径与 JSON 模式契约不变

### 发布产物
- `release/v10.170.0/tianxuan-windows-amd64-installer.exe`
- SHA256: 待打包后填写
- 验证：`go test ./...` 48 包全绿（仅预存 API 认证类失败）· `go vet` 干净 · `wails build` EXIT 0

---

## V10.166.0 (2026-08-08) — 删除双模型架构，重回单模型规划模式

### 变更
- 删除双模型架构（Hermes 规划者 + Hephaestus 执行者，两个独立 session / 两套 L1 前缀）
- 新增单模型规划模式 `PlannerHost`：同一模型、同一 session、同一工具列表，
  复杂任务先只读规划（planmode.Marker + 运行时只读门）→ 确认 → 执行 →
  自动修正（≤3 轮）；规划/执行阶段工具 schema 全程不变，遵守前缀缓存铁律
- 规划模式开关 `agent.auto_plan`（默认 off = 直接执行；ask = 复杂任务先规划确认），
  桌面端 Composer 底部新增"规划"按钮热切换
- 删除 `planner_model` / `planner_temperature` / `planner_effort` /
  `planner_max_steps` 配置与设置 UI；前端运行状态行改为单阶段"规划/执行"

---

## V10.165.0 (2026-08-08) — AI 自主截图工具 capture_screen

### 变更
- 新增内置工具 `capture_screen`：模型可主动截取主屏幕，保存到 `.tianxuan/attachments/screen-*.png` 并返回 @路径，配合 vision 技能完成"截图→识图"自主闭环（如"看看我屏幕上的报错"）
- Windows GDI 实现（复用桌面端 CaptureScreen 同款逻辑），非 Windows 大声报错
- 工具返回明确指引模型调用 vision 技能；截图涉及隐私，ask 权限下首次调用需用户确认

---

## V10.164.0 (2026-08-08) — 截图→识图闭环：附件提示指向 vision 技能

### 变更
- refs.go 图片附件提示改为明确指引 `run_skill vision`（原文案指向不存在的 "vision MCP tool"），截图/粘贴图片发送后模型会调用识图技能读图
- vision 技能 description 增加触发条件：收到 `[image attachment]` 附件引用（`@.tianxuan/attachments/` 路径）时必须使用

---

## V10.163.0 (2026-08-08) — 修复更新安装时旧版本未关闭导致失败

### 变更
- NSIS 安装器安装前终止并等待旧版 tianxuan-desktop.exe 退出（轮询最多 10 秒），解决覆盖 exe 被占用导致的更新失败
- 实测验证：伪造旧版进程运行中执行静默安装 → 旧进程被终止、exe 成功覆盖（哈希一致）、退出码 0

---

## V10.162.0 (2026-08-08) — 新增 vision 识图技能（OpenCode Zen 视觉模型）

### 变更
- 新增 `vision` 内置技能：无视觉模型（DeepSeek）通过 OpenCode Zen 视觉模型（mimo-v2.5-free）识图
- `scripts/vision.ps1` 零依赖：图片 base64 → POST Zen chat/completions → 返回文字描述
- 真实 API 实测通过：中文描述准确、UTF-8 输出正确、错误路径大声失败

---

## V10.161.0 (2026-08-08) — 桌面端设置面板支持 OpenCode Zen

### 变更
- desktop 主程序注册 opencode provider kind（设置面板 kind 下拉可选）
- 模型服务预设模板新增 OpenCode Zen（一键添加，OPENCODE_API_KEY）
- 用户级 config.toml 预置 zen provider（deepseek-v4-flash-free / claude-haiku-4-5 / gpt-5.4-nano）

---

## V10.160.0 (2026-08-08) — OpenCode Zen 模型接入

### 变更
- 新增 `opencode` provider kind：按模型自动路由 Zen 三协议（chat/completions / messages / responses），免费模型匿名可用
- Responses 流式客户端：工具调用、reasoning、usage 解析；config 放行无 key opencode
- 真实 API 三协议端到端验证通过（deepseek free / gpt-5.4-nano / claude-haiku-4-5）

---

## V10.159.0 (2026-08-05) — 重度使用痛点修复（第三批）：read_file 按符号跳读

### 变更
- read_file 新增 `symbol` 参数：定位函数/方法/类型定义行并从此处读取；未找到时大声失败并附附近符号名

---

## V10.158.0 (2026-08-05) — 重度使用痛点修复（第二批）

### 变更
- todo_write 支持 blocked 状态（外部依赖空转降噪）；edit_lines 多行锚点（start/end_anchor 可含换行）

---

## V10.157.0 (2026-08-05) — 重度使用痛点修复（需求驱动，非蒸馏）

### 变更
- validation 错误附正确示例；bash chat 参数误用检测；stale 编辑守卫改软警告；offload 预览 200 → 800 字符

---

## V10.156.0 (2026-08-05) — 背景会话 fork（Qwen Code `/fork` 语义）

### 变更
- `task` 工具新增 `inherit_context` 参数：forkCtx 快照注入子代理首条；boot 注入 forkContextText

---

## V10.155.0 (2026-08-05) — Windows 安装包 + 自动更新链路打通

### 变更
- Windows NSIS 安装包：`wails build -nsis` 产出 per-user 免管理员安装器，完成页自动运行应用（自定义 `desktop/build/windows/installer/project.nsi`）
- 自动更新发布目标修复：updater / cmd/sign / release-desktop.yml 从 Reasonix 残留（R2 bucket、`esengine/tianxuan`）切换到真实仓库 `wubitianxuan55-cell/tianxuan`；R2 镜像改为仓库变量配置，未配置时自动跳过
- minisign 签名密钥轮换为新密钥对（key ID `154E38FBADA79807`，私钥加密存本机 `~/.tianxuan-release/`，不入 git）
- 新增 `scripts/publish-desktop.ps1` 一键发布：构建 → 签名 → `latest.json`（含更新说明）→ GitHub Release

### 发布产物
- `release/v10.155.0/tianxuan-windows-amd64-installer.exe` · 10,731,509 bytes (~10.2 MB) · per-user NSIS 安装器
- SHA256: `afb864475273053721d9bd48370f1f17b93d648ad6a244655081a76fccd55d19`
- 验证：desktop `go test ./...` + `go vet` 全绿 · minisign 签名验证通过 · manifest sha256 一致

---

## V10.154.0 (2026-08-04) — Codex CLI 工具蒸馏：降低工具出错率

### 变更
- 执行前 schema 校验（对齐 codex json_schema.rs）：内建工具必填/类型/枚举错误以
  `validation_error` 大声失败并附带完整 schema 提示；别名（file→path、job_id→job_ids、
  timeout_ms→timeout_seconds）兼容并满足必填
- compact schema 保留参数描述：30 个工具补精简英文描述（默认值/范围/语义）；
  `CanonicalizeSchemaVerbose` 修复描述被规范化管线二次剥离
- 工具错误统计（codex ToolDispatchTrace 蒸馏）：`tianxuan tools stats` 查询
  tool × error_kind × count，落盘 `.tianxuan/tool-stats.json`
- 修复 learning 链路 bug：`Observe`（提取+合并+持久化）替代从未 merge 的
  Extract+SaveStore，模式计数真正增长并注入系统提示
- validation_error 通配分类 + delete_range/delete_symbol 错误反馈补齐

### 发布产物
- `release/v10.154.0/tianxuan-desktop.exe` · 25,054,720 bytes (~23.9 MB)
- SHA256: `90a0b4b9ceb6fcbbb8115f3a445a640d68b99c0b4909f39ce71f10ad909ea218`
- 验证：`go test ./...` 全绿 · `go vet` 干净 · `wails build` EXIT 0

---

## V10.153.0 (2026-08-03) — 编辑安全 + Windows 适配 + 发布流水线

### 变更
- edit_lines 内容锚点校验（start_anchor/end_anchor 不匹配即拒绝且不写文件）+ 编辑后自动语法检查回滚（.go→gofmt -e，.ts/.tsx→项目本地 tsc，失败/超时自动回滚）
- bash PowerShell 适配层：heredoc 转 here-string、npm/npx/pnpm .cmd shim 绕开执行策略、git 不在 PATH 时自动注入常见安装目录
- 内置 release 发布技能：版本号同步/打包/CHANGELOG/记忆一键化 + autotrigger 触发词
- L1 注入 Git 工作流约定：多任务开始前提示 git_worktree 建功能分支，完成按逻辑单元拆分提交
- bash 高危环境变量主动预警：数据库类命令执行前比对用户级 DATABASE_URL 与项目 .env

### 发布产物
- `release/v10.153.0/tianxuan-desktop.exe` · 25,020,928 bytes (~23.9 MB)
- SHA256: `957ecce4a2357e63bd68a0e596a2fcf81e38e71476769c83265ba8b947c0924e`
- 验证：`go test ./...` 全绿 · `go vet` 干净 · `wails build` EXIT 0

---

## V10.152.0 (2026-08-02) — 编程能力强化：向 Codex 工程效率看齐

### 重构
- 三套系统提示词按单/双模型分别精简：Solo 159→48 行、Hermes 417→90 行、Hephaestus 257→49 行，总固定开销 -79%
- 回合循环减负：删 investigationNudge + todoStepStuckNudge；tool_feedback 仅硬模式（连续 3 轮全败才注入）
- 工具面收敛：compact 默认生效（18 核心工具）；multi_edit 恢复可见；move_file/git_worktree 移出白名单
- 计划仪式降级：单模型解除 complete_step 强制（finalReadinessCheck/taskGate 按模式分叉）
- 测试先行规则确定化：改存量代码必须测试先行，修 bug 复现测试先行，新建豁免
- bash 环境注入（项目 tools + 常见安装目录 PATH）+ cwd 引导；修 bug 实测 203s → 113s

### 发布产物
- `release/v10.152.0/tianxuan-desktop.exe` · 24,991,744 bytes (~23.8 MB)
- SHA256: `8a3310ec2aa6e46d59b9fe5572dbef42613e6219e8261ff1baa6890c56ac830b`
- 验证：`go test ./...` 全绿 · `go vet` 干净 · `wails build` EXIT 0

---

# Tianxuan 版本变更日志

## V10.151.0 (2026-08-02) — 浏览器右侧分栏可拖拽调整宽度

### 修复
- 浏览器分栏宽度从写死 CSS 改为独立 state（useBrowserPanel），拖拽 resizer 调整
- clamp：360 下限 / 1080 上限 / 62% 视口比例 / 对话区最小 200px 保护；宽度持久化
- 键盘方向键/Home/End 调整、双击恢复默认；i18n 三语言补齐

### 发布产物
- `release/v10.151.0/tianxuan-desktop.exe` · 24,999,936 bytes (~23.8 MB)
- SHA256: `1ac93074fb67e48d05326b59f0775573bc0b7c3039c77ed513167ad1e144ad53`
- 验证：前端 135/135 · tsc 0 错误 · `wails build` EXIT 0

---

## V10.150.0 (2026-08-02) — 后端蒸馏（Auto Failure Guard + Model Failover）+ 桌面端发布

### 🛡 宿主侧失败升级（Reasonix recovery 蒸馏）
- 精确操作指纹（工具 + key 无序 JSON 参数 SHA-256）：同操作失败 3 次宿主停该操作，换参/换方案即新操作重新计数
- 回合累计 6 次失败且无真实进展 → 宿主停变更/验证，只读诊断保持可用；成功变更清零回合预算
- 只读失败不累计、blocked/权限拒绝不算失败（QualifyingFailure 语义）；纯宿主决策，不碰 L1-L4 前缀

### 🧵 turn-local 模型回退链（OpenClaw model-failover 蒸馏）
- 429/408/5xx/断网时按序切备用模型（同 key 不同 model），回退只当前回合生效，下回合回到 primary 保缓存
- 整链纯过载时指数退避重试（MaxChainRetries）；全部耗尽返回 FallbackSummaryError 逐候选细节
- 参数错误/auth（同 key）/取消/流中断不切候选；Chain 实现 provider.Provider 接口，agent 层零改动
- 配置：`[[providers]] fallbacks = [...]`；无 fallback 零开销

### 发布产物
- `release/v10.150.0/tianxuan-desktop.exe` · 24,997,888 bytes (~23.8 MB)
- SHA256: `c9e787b049a044a52c96b3b42a4d216a662ce29c1939267edfa50028e58b00e4`
- 验证：`go test ./...` 全绿 · 前端 vitest 128/128 · `wails build` EXIT 0

---

## V10.50.0 (2026-07-08) — Superpowers 蒸馏 + 双模型角色区分 + 完整准则体系

### 🧠 Superpowers v6.1.1 → AGENTS.md 内联
- 修复 design_session 虚假引用（V10.4.0 已移除）→ ask/explore 真实工具
- systematic-debugging：5级根因追溯 + 2条禁止项
- TDD 强化：修 bug 必须先写复现测试

### 🔀 双模型角色区分
- memory.PlannerBlock()：Hermes 过滤编码铁律 + Superpowers 方法论，减 ~35% token
- 保留"拒绝谄媚"（规划者与用户交流最多）
- 角色指代修正：（执行者，你）→（执行者）

### 🧠 Hermes 规划者准则（7 原则）
- 专业判断 5 原则：证据优先/敢反驳/澄清不猜测/简单即美/拒绝坏计划
- 设计质量 4 原则：SRP / OCP / YAGNI / KISS
- 计划验证标准：每步含可验证成功条件

### 🔨 Hephaestus 执行者铁律（7 → 10 条）
- 🆕 手术级变更（Karpathy）：每行改动可追溯到需求 / 禁顺手优化 / 清 orphan
- 🆕 极简实现（Karpathy + YAGNI/KISS）：不写未要求功能 / 3 行能解决不写 30 行
- 🆕 防御性编程：错误大声失败 / 所有外部输入校验边界

### 🖥️ 桌面端
- 🐛 一键到底按钮不随视口固定（从 .transcript 内部移到 wrapper）
- 定时任务 LLM 辅助 + Welcome 简化

---

## V10.15.0 (2026-06-29) — 启动黑屏热修复

### 🔥 关键修复：启动黑屏
- react-virtuoso 在 WebView2 中与 React 18 不兼容，触发 error #321 导致黑屏
- 回退到 DOM 原生滚动方案（@tanstack/react-virtual 保留为构建依赖）

### 🧠 会话记忆升级
- 新增 `promote_session_facts` 工具：临时会话记忆一键提升为永久存储
- remember(session=true) 记忆跨轮次存活

### 🎨 前端优化
- JumpBar/MessageNavigator 解耦 threadEl DOM 引用
- store.ts ensureAssistant 移除，流式 text/reasoning 事件优化
- ToolCard 紧凑化（节省 30-40% 垂直空间）

---

## V10.14.0 (2026-06-29) — 自我进化迭代

### 🧠 Reasonix 吸收
- **成功循环检测**：写工具重复成功 ≥2 次自动阻止（repeatedSuccessBlock + 7 辅助函数）
- **参数修复提示**：非法 JSON 参数时附带工具 schema（schema echo）
- **Grace Round 守卫**：防止 MaxSteps 限制下无限工具调用循环

### ⚡ 速度优化
- 流式 batcher maxBytes 8→32，Wails IPC 减少 75%
- Precheck 读盘后写入 toolCache，消除重复文件 I/O

### 🔧 测试修复
- 7 个既有测试全部修复

---

## V10.13.0 (2026-06-29) — 体验打磨迭代

- 删除流式输出闪烁光标 + 修复布局抖动
- 修复同一阶段多思考卡（reasoning 同步 dispatch）
- 清除"计划模式"概念 → "只读模式"
- 底栏模式子状态（探索·只读 / 开发·可写 / 编排）
- 思考卡默认折叠 + 工具卡空间紧缩

---

## V10.12.0 (2026-06-29) — 综合优化迭代

### Bug修复
- session_route_features: FilesModified 永久为0 → Pro模型自动升级失效
- 回到底部按钮不可见 + 滚动不到位（虚拟列表 scrollHeight 阈值修复）

### 流式输出流畅度
- text/reasoning setTimeout(0) 绕过 React 18 批处理
- stream_batcher 换行感知 flush
- useItems() 分离订阅

### auto_router 增强
- HasWrittenFiles / HasUsedSubAgent + TurnCount 10→5 + 关键词外部化

### grep 增强
- context_lines + highlight(>>>匹配<<<)

### Bash 智能截断
- JSON 模式独立截断 + bash_output tail_lines

### 配色系统重设计
- :root 暗色重设计 + 4主题(light/warm/ice/forest)
- 删除无效主题 midnight/neon/mono

### UI 紧凑化
- 思考卡 + ToolCard 缩小间距/字号/边距

### MemoryPanel 重构
- 5子组件提取 + React.memo + useCallback

### 测试
- 新增 58 用例，agent 包 242 测试
- SHA256: 832585cb1fb5c7a0981abacf34412d7c97a1515c177ba88d9471e6f43ec8aa48

## V10.10.0 (2026-06-28) — 综合性优化迭代
(详见 release/v10.10.0/RELEASE.md)

## V10.9.0 (2026-06-28) — 🧠 记忆建议引擎 + 多标签页骨架 + UI 增强

### 记忆建议引擎（借鉴 DeepSeek-Reasonix V1.13）
- **自动检测记忆候选**: 16 个中英文关键词（记住/always/偏好/约定 等）从用户消息中自动提取，纯本地运算不消耗 token
- **工作流技能自动生成**: 3 个模板（code-review/refactor/bug-fix）从历史检测重复模式→自动生成 SKILL.md
- **一键采纳**: AcceptMemorySuggestion / AcceptSkillSuggestion，记忆→Store.Save，技能→skill.CreateWithContent
- **归档记忆列表**: ListArchived() + ArchivedMemory 类型，store.go +80 行
- **[[wiki-link]] 内联渲染**: 记忆正文中 [[name]] 渲染为可点击跳转链接，死链接灰色删除线提示

### 多标签页系统（骨架）
- **WorkspaceTab 类型**: 独立 ID/Scope/WorkspaceRoot/SessionPath/Ctrl，为多标签准备
- **App.tabs map**: 所有绑定方法统一改用 ctrlByTabID("") 路由（20+ 方法重构），完全向后兼容
- **tabEventSink + toWireTab**: 事件注入 tabId 供前端路由，全局 eventSink 自动注入活跃 tabID
- **TabBar 前端组件 + 持久化**: desktop-tabs.json 保存恢复，SelectTab/TabMeta API

### UI 增强
- **PromptShelf 组件**: 共享架子（头部+进度条+折叠体+按钮），TodoPanel 重构使用
- **快速添加路径提示**: MemoryPanel 显示"保存至: ~/.tianxuan/..."路径
- **FactCard 增强**: wiki-link 内联渲染、编辑/删除/确认删除交互

### 借鉴来源
- DeepSeek-Reasonix V1.13.0 桌面端代码深度分析
- 记忆建议引擎 (memory_suggestions.go, 440行) ← Reasonix
- 多标签页骨架 (tabs.go + 路由重构) ← Reasonix
- PromptShelf ← Reasonix

## V10.8.0 (2026-06-28) — 🔵 智能化

- **compact 保留 todo**: 压缩前读取 .tianxuan/progress.md 注入指令，防止进度丢失
- **增强 commit message**: autoCommitMessage 包含文件名摘要（≤3 列出名字）
- **grep 相关性排序**: sort_by=relevance 按匹配密度排序

## V10.7.0 (2026-06-28) — 🟢 工作流支持

- **git_worktree 工具**: 新增 add/remove/list 操作，支持隔离并行开发
- **计划进度持久化**: todo_write 同步写入 .tianxuan/progress.md（Markdown 表格）
- **main/master 分支检测**: git_commit 在主分支上拒绝提交

## V10.6.0 (2026-06-28) — 🟡 可靠性增强

- **web_fetch 自动重试**: retries 参数 + 指数退避 1s→2s→4s + isTransientError 智能判断
- **bash stdout/stderr 分离**: json 模式返回独立 stdout/stderr 字段
- **子代理超时部分结果**: extractLastAssistantMessage + "(partial result returned)" 标签

## V10.5.0 (2026-06-28) — 🔴 编辑体验革命

- **edit_file 自动行尾适配**: CRLF/LF 自动检测和转换，multi_edit 同步适配
- **edit_lines 工具**: 按行号编辑（1-based），自动保留原始行尾格式
- **read_file 无行号输出**: line_numbers=false 输出纯文本

## V10.4.0 (2026-06-26) — Superpowers 融合 + 工具精简

- AGENTS.md: 7 条编码铁律 + 8 条推辞识别表
- 技能 10→4: 仅保留 explore/research/review/security-review（子代理）
- 工具 28→24: 移除 doctor/time/verify/design_session
- bash 超时 2→5min + output_format=json
- grep 200→500 + max_matches 参数
- edit_file: old_string not found 诊断增强
- 前端: 记忆面板中文化 + Transcript 滚动修复 + web 工具摘要

## V10.3.0 (2026-06-26)

- 统计面板合并: 子代理和主模型统计统一
- MessageNavigator: 右侧面板第5标签，消息列表+跳转
- 外观重设计: 9 主题配色 + 字体设置
- Plan Mode: explore/research/review/security_review 在只读模式下可用

## V10.2.0 (2026-06-24~25)

- UI 优化 + app.go 拆分为 5 个子模块 + 空间清理

## V10.1.0 (2026-06-26)

- 全量蒸馏: 13 commits, 40+ files, 2500+ lines
- Go 后端 6 机制 + React 前端 11 组件 + CSS 4 组 token

---

## 构建产物索引

| 版本 | 路径 | 大小 | SHA256 |
|------|------|------|--------|
| V10.2.0 | release/v10.2.0/tianxuan-desktop.exe | 16 MB | — |
| V10.4.0 | release/v10.4.0/tianxuan-desktop.exe | 16 MB | — |
| V10.5.0 | release/v10.5.0/tianxuan-desktop.exe | 16 MB | — |
| V10.6.0 | release/v10.6.0/tianxuan-desktop.exe | 16 MB | — |
| V10.7.0 | release/v10.7.0/tianxuan-desktop.exe | 16 MB | — |
| V10.8.0 | release/v10.8.0/tianxuan-desktop.exe | 16 MB | `b9671ae8f408…` |
