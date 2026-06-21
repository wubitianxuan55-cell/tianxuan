# tianxuan project memory

> V8.14.0 — VS Code 智能编码动作体系 · 2026-06-21
## V8.14.0 发布摘要 (2026-06-21)
**基于**: V8.13.0 · **二进制**: CLI 20MB · **变更**: 2源文件 +262/-35行
⚡ Quick Fix: CodeActionProvider — 诊断错误→AI修复→diff预览
📝 文档注释: 右键菜单+CodeLens — 生成JSDoc/GoDoc/docstring
🧪 单元测试: 右键菜单+CodeLens — 推断框架→生成测试→diff预览
🔍 CodeLens: 函数上方 $(hubot)解释 · $(beaker)测试 · $(book)文档
🖥️ 终端解释: 粘贴错误→AI中文解释+修复建议
🎯 补全增强: 300ms防抖(真实setTimeout) + 语义边界触发(. ( 空格)
🔒 缓存安全: 全部L4域 — L1/L2/L3零变更，不新增Go端点

> V8.13.0 — VS Code 原生 AI 能力 · 编辑器深度集成 · 2026-06-21
## V8.13.0 发布摘要 (2026-06-21)
**基于**: V8.12.0 · **二进制**: CLI 20MB · **变更**: 8源文件 +520/-3900行
🤖 InlineCompletionProvider: 代码补全(300ms防抖+2s节流) + POST /complete 端点
💡 HoverProvider: 鼠标悬停标识符→AI解释
🔌 编辑器集成: 行号定位/Diff预览/applyEdit/diagnostic注入
🎛️ 右键菜单: 解释/审查/修复代码 3 命令
⌨️ 快捷键: Ctrl+Shift+T / Ctrl+Shift+Enter
📊 状态栏 + 主题同步 + submitSelection 修复
🧹 构建清理: 删除过期产物 + vscode/webview/dist + build:webview 干净重建
🔒 缓存安全: 全部L4域 — L1/L2/L3零变更，命中率维持 67%-93%

> V8.12.0 — 全线桥接打通 · VS Code 全连通 · 2026-06-21
## V8.12.0 发布摘要 (2026-06-21)
**基于**: V8.11.0 · **变更**: 2文件 +285/-230行
🔌 Web bridge 双传输层: 浏览器→fetch, VS Code→postMessage 透明切换
🔌 extension.ts HTTP/SSE 代理: proxyFetch + connectSSE 逐帧转发
🔒 CSP 收紧: 取消 connect-src 白名单，全走 postMessage 更安全

## V8.11.0 发布摘要 (2026-06-21)
**基于**: V8.10.0 · **变更**: 2文件 +244/-37行
🔌 VS Code postMessage 请求/响应通道 + 6个原生 API + CSP注入

## V8.10.0 发布摘要 (2026-06-21)
**基于**: V8.9.0 · **变更**: 233文件 +3869/-24行
🔌 50 serve 端点: Settings/MCP/Checkpoint/Session/Slash/TCCA/Rebuild
🌐 Web bridge 46/46 全接通 + 30桩替换 + 5缺失补全
🏗️ Server Rebuild 热重载: Snapshot→Carry→Rebuild→Resume

> V8.9.0 — 跨项目吸收+前端打通+技能升级 · 2026-06-21
## V8.9.0 发布摘要 (2026-06-21)
**基于**: V8.8.0 · **位置**: `release/v8.9.0/` · **变更**: 23文件 +~700/-34行
🔄 跨项目吸收：Whale(ToolEnvelope/CacheShape) + MiMo-Code(RecoverableError/截断/output_schema/Checkpoint)
🖥️ 前端打通：Web UI 28端点 + VS Code 扩展接入 + serve 内嵌 React
🎨 UI增强：ASK弹窗拖拽 + ToolCard可恢复错误灰度 + 截断tail scan
📝 技能升级：20个 superpowers-zh 中文技能替换
🔒 缓存安全：全部改动仅影响传输层/L4域 — L1/L2/Tools 零变动

> V8.8.0 — Context7内置集成 · 第三方库实时文档 · 2026-06-21
## V8.8.0 发布摘要 (2026-06-21)
**基于**: V8.7.0 · **位置**: `release/v8.8.0/` · **变更**: 5文件 +80/-0行
🧩 Context7：内置 /context7 技能 + CapabilitiesPanel 一键添加按钮 + 3语言 i18n
🔒 缓存安全：Skill→L4域 · MCP运行时不影响L3 hash · Go内核零变动


> V8.7.0 — UI优化: 智能主页+内联style消除+微交互 · 2026-06-21
## V8.7.0 发布摘要 (2026-06-21)
**基于**: V8.6.0 · **位置**: `release/v8.7.0/` · **变更**: 7文件 +63/-75行
🎨 UI: Welcome智能主页(6快捷命令+会话卡片+首次引导) + JumpBar/PlanPanel/StatsPanel/StreamingIndicator内联style消除(16→10) + Transcript滚动按钮动画增强
🔒 缓存安全: Go内核零变动


> V8.6.0 — God对象拆分+Web/VS Code骨架+死代码大扫除 · 2026-06-21
## V8.6.0 发布摘要 (2026-06-21)
**基于**: V8.4.1 · **二进制**: CLI / Desktop · **位置**: `release/v8.6.0/` · **变更**: 17文件 +96/-6629行
🏗️ Go核心: agent.go 1219→840 (-31%), compact.go 591→369 (-38%), 新增 agent_run/agent_stream/compact_summary 3文件
🧹 死代码: usePasteHandler.ts(-94) + package-lock.json(-4849) + flow.go墓碑(-2) + styles.css(-535)
🧩 复用: CloseButton+ToolbarButton 组件 + close-btn/toolbar-btn CSS utility
🌐 新平台: web/ Web UI (React SPA, HTTP/SSE) + vscode/ VS Code Extension (sidecar)
🏗️ 构建: KaTeX字体过滤节省876KB + bridge.ts→mock.ts拆分(658→215)
🔒 缓存安全: 全部通过 — L1/L2 hash一致, 命中率 0%→52%→67%, 峰值93%


> V8.4.1 — Token成本优化+统计修复+7组件优化 · 2026-06-21

## V8.4.1 发布摘要 (2026-06-21)
**基于**: V8.4.0 · **二进制**: CLI 18.6MB / Desktop 16.0MB · **位置**:  · **变更**: 24文件 +451/-275行
🔧 Go: itoa 4→1合并(包) + 编码修复 + SetSession计数器归零(3×Store(0))
🩺 统计修复: 命中率600%根因消除 + 分母对齐 + HitPct clamp + Y轴自适应
🎨 UI: JumpBar圆点→横条+键盘导航、StreamingIndicator sticky、SettingsPanel 9项修复、PlanPanel进度+todo、遮罩层统一pointer-events-none
📐 基础设施: ResizableDrawer三层遮罩 + 缓存红线文档扩展 + strutil测试

## V8.4.0 发布摘要 (2026-06-20)

**基于**: V8.3.1 (commit facab32) · **二进制**: CLI 13MB / Desktop 16MB (Wails v2.12.0) · **位置**: `release/v8.4.0/`

🎨 前端全量 UI 优化 — 22 轮，36 文件，+1858/-1994 行：
- P0 主题修复：补全 15+ 缺失 CSS 变量，浅色主题下 color-mix/diff/语法高亮恢复正常
- 紧凑模式：新增 `hooks/useCompact.tsx` React Context，一键切换 6 组件自适应字号/间距/图标
- CSS 清理：styles.css 删除 ~400 行死代码（`.ico/menu/toast/error-card/cap-*` 旧样式）
- 动画统一：`--transition-fast/normal/slow` 三档 token + `prefers-reduced-motion`
- ToolCard：ICONS 10→30+（含 MCP `mcp__*→Plug`），模板字面量 bug 修复，HljsCode/Diff 增强
- 居中弹窗：MemoryPanel/PlanPanel/ApprovalModal 从右侧抽屉重做为居中模态（`animate-[scaleIn]`）
- 费用面板：StatusBar 空闲态 → `Total + 💰¥0.52`（自动 DeepSeek 模型价格表计价）
- Composer：项目感知 placeholder、focus 发光环、底部快捷提示条、菜单动画、粘贴块 `<ActionBtn>` 组件化
- 全局统一：`✕` 字符→lucide `<X/>` 图标 12 处；硬编码色值→CSS 变量；`bg-black→bg-bg` 主题感知
- 侧边栏+文件树：4 按钮三态过渡、文件树选中色条、面包屑打磨
- 响应式：删除 820px 断点 macOS 硬编码 `topbar padding-left: 82px`

## V8.3.1 发布摘要 (2026-06-20)

**基于**: V8.2.3 (commit d5aeed1)

**Go 核心**: cacheBreakDetector L4 追踪激活（5 种断裂原因诊断）；maybeCompact 重新启用。
**测试**: 新增 3 个 L4 追踪测试，10 个 cacheBreakDetector 全绿。
**前端**: 统计面板按会话持久化（sessionKey 绑定 .jsonl 路径）；工具卡折叠修复（min-h-0）。

## V8.2.2 发布摘要 (2026-06-20)

**基于**: V8.2.1 (commit 6846c4e)

**🎨 前端 UI 优化 — 10轮渐进式 Tailwind 迁移：**

| 轮次 | 主题 | CSS 行 | gzip |
|------|------|--------|------|
| V8.2.1 | 基线 | 3773 | 26.84 KB |
| 1 | Tailwind 收尾 | 3236 | 26.02 |
| 2 | 消息气泡 | 2937 | 25.58 |
| 3 | 工具卡片 | 2910 | 25.52 |
| 4 | Composer | 2800 | 25.40 |
| 5 | Modal+Drawer | 2605 | 25.19 |
| 6 | 批量收尾 | 2193 | 24.26 |
| 7 | .btn 统一 | 2170 | 24.28 |
| 8 | chip+banner | 2098 | 24.13 |
| 9 | right-panel | 1835 | 23.61 |
| 10 | 最终收尾 | **1567** | **23.00** |
| **总计** | | **-2206 (-58.5%)** | **-3.84 KB (-14.3%)** |

**消除 `::after`/`::before` 伪元素**: 15处 → 7处（仅保留功能性 resizer 拖拽指示器）

**动画增强**: ToolCard/ToolGroup grid-rows 展开动画、thinking 折叠平滑过渡、按钮 scale 反馈

**桌面端构建**: Wails v2.12.0 + Vite 6 + React 18，注入版本 V8.2.2，二进制 ~15.75 MB

## Project

tianxuan 是一个面向 DeepSeek V4 的极简 Coding Agent。单 Go 二进制，零外部依赖。
核心目标：极低成本、极快响应。

## Architecture

**单模型直连** — 无 Planner、无 Learner、无 LLM Compact。

```
用户输入 → Controller → ContextManager(L1+L2+L4) → AgentRunner.runDirect()
                                                          │
                                              DeepSeek V4 API (1次调用)
                                                          │
                                              工具执行 (流式预执行 + 文件缓存)
                                                          │
                                              截断检查 (≥500K tok → 三级压缩)
```

### 四域前缀 (TCCA)
- **L1 Identity** (~300 tok): 身份 + 规则，SHA-256 不可变校验
- **L2 Runtime** (~100 tok): 项目/语言/入口，首轮锁定
- **L3 Skill** (~1,200 tok): 工具紧凑描述，prefix cache 完全命中
- **L4 Flow**: 对话历史，HistoryHygiene 三维压缩

### V7.3 新增 (2026-06-14)

#### 原生 Git 工具
| 工具 | 功能 | ReadOnly |
|------|------|:--------:|
| `git_status` | 结构化工作区状态：分支、暂存/未暂存/未跟踪/冲突 | ✅ |
| `git_diff` | 行级 diff，支持 `--staged` 和 `path` 过滤 | ✅ |
| `git_commit` | 提交暂存变更，支持 `stage_all`/`amend`/自动生成消息 | ❌ |
| `git_log` | 格式化提交历史，支持 `count`/`path`/`author` 过滤 | ✅ |

位置: `internal/tool/builtin/git.go`

#### LSP 扩展
| 工具 | 功能 | ReadOnly |
|------|------|:--------:|
| `lsp_completion` | 获取光标位置的代码补全建议 | ✅ |
| `lsp_rename` | 跨文件重命名符号（实际修改文件） | ❌ |

位置: `internal/lsp/tool.go` + `client.go` + `manager.go` + `results.go`

#### `/undo` 回滚命令
- `Controller.Submit()` 中新增 `/undo [N]` 命令
- 利用 Checkpoint 系统回滚最近 N 轮的代码修改 + 对话
- 桌面/HTTP 前端均可使用（TUI 另有更丰富的 `/rewind` 面板）
- 位置: `internal/control/controller.go`

#### 文档清理
- 删除根目录误导性 `design.md`（描述已废弃的 Rust 架构）
- V5/V6 过时文档归档到 `_archive/`

### 关键约束

- **单模型**: `Run()` 直接调用 `runDirect()`，无 Planner 分支
- **零额外 LLM 调用**: 无 compact 摘要，无 Learner 反馈
- **工具描述免费**: L3 在 prefix cache 中，100% 命中不计费

### 🔴 缓存保护红线（最高优先级，禁止违反）

DeepSeek 的前缀缓存是整个项目的成本命脉——缓存命中率每下降 1%，每轮多消耗约 12K prompt token。以下规则绝对不可违反：

| 规则 | 原因 | 违规案例 |
|------|------|----------|
| **L1 Identity 字节不可变** — 系统提示词一旦锁定，任何字符（含空格/换行）不能改 | `verifyPrefix` 用 SHA-256 校验；漂移 → panic | — |
| **tools 列表整个会话不可变** — 不能按输入动态增删工具 | DeepSeek 缓存 key 包含 tools 列表；变化 → cache miss 率 ~0% | V8.0.2 `filteredSchemas` 致命事故 |
| **L2 Runtime 首轮锁定** — 运行时上下文在第一轮后不可变 | 同上，缓存 key 包含 L2 | — |
| **不允许动态系统提示词注入** — 不能在用户消息前插入可变文本 | 会破坏 L1 → user 的固定前缀结构 | — |
| **工具描述不可热更新** — session 中途不能修改 CompactDescriptor | 工具描述是缓存前缀的一部分 | — |

> 💡 为什么这么严格？DeepSeek 的 prefix cache 按"前缀连续匹配"计费。如果某轮的前缀与上一轮有 1 字节差异，整个 prompt 全部按 miss 计费。V5.7 基线命中率 99%，V8.0.2 因 filteredSchemas 降为 0%——同一次对话，费用从 ¥0.008 → ¥0.020（2.5 倍）。

## 命令

```
# 构建 CLI（发布版）
set GOOS=windows&& set GOARCH=amd64&& go build -ldflags="-s -w" -o release/vX.Y.Z/tianxuan.exe ./cmd/tianxuan/

# 构建桌面端（发布版）—— 产物在 desktop/build/bin/tianxuan-desktop.exe，必须保留
cd desktop && wails build
cp build/bin/tianxuan-desktop.exe ../release/vX.Y.Z/tianxuan-desktop.exe

# 桌面端开发启动（wails dev —— 从 build/bin/ 启动）
.\wails.ps1

# Go 测试
go test ./internal/...

# Go vet
go vet ./internal/...

# 前端开发
cd desktop/frontend && pnpm dev

# 前端构建
cd desktop/frontend && pnpm build

# 前端类型检查
cd desktop/frontend && npx tsc --noEmit
```

## 关键模块

- `internal/agent/` — AgentRunner, stream, executeBatch, compact, 缓存守卫
- `internal/boot/` — Build() 装配工厂
- `internal/cache/` — 四域管理 (Identity/Runtime/Skill/Spawn)
- `internal/context/` — TCCA 内核 (ContextManager)
- `internal/control/` — Controller 会话驱动
- `internal/tool/` — Tool 接口 + CompactDescriptor
- `internal/tool/builtin/git.go` — 原生 Git 工具
- `internal/lsp/` — LSP 集成（含 completion + rename 扩展）
- `desktop/frontend/src/hooks/` — 桌面端 6 个自定义 hooks

## 内置工具一览 (自 V7.3.0+)

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
| `notebook_edit` | Jupyter 编辑 |
| `memory_search` | 持久记忆搜索 |
| `bash_output` / `kill_shell` / `wait` | 后台作业 |

## 实测 (DeepSeek V4, 30轮)

```
CLI 10轮命中率: 98.9%
Mock 无压缩14轮: 93%
Mock 小窗口30轮: 91% (10轮恢复)
真实API大前缀: 94%
```

## 约定

- Go kernel under internal/; each package owns one concern
- Transport-agnostic Controller behind every frontend
- Config: tianxuan.toml, secrets in .env, API key 在 ~/.env
- 桌面端: Wails v2, React 18, Vite 6, Zustand 5

## 编码原则

1. **Think Before Coding** — State assumptions. Surface tradeoffs.
2. **Simplicity First** — Minimum code. No speculative features.
3. **Surgical Changes** — Touch only what you must.
4. **Goal-Driven Execution** — Define verifiable success criteria. Loop until verified.
