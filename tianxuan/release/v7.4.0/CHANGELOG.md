## [7.4.0] — 2026-06-16 智能增强版

### 新功能 — 原生 Git 工具
| 工具 | 功能 | ReadOnly |
|------|------|:--------:|
| `git_status` | 结构化工作区状态：分支、暂存/未暂存/未跟踪/冲突 | ✅ |
| `git_diff` | 行级 diff，支持 `--staged` 和 `path` 过滤 | ✅ |
| `git_commit` | 提交暂存变更，支持 `stage_all`/`amend`/自动生成消息 | ❌ |
| `git_log` | 格式化提交历史，支持 `count`/`path`/`author` 过滤 | ✅ |

位置: `internal/tool/builtin/git.go`（独立文件，零侵入）

### 新功能 — LSP 扩展
| 工具 | 功能 | ReadOnly |
|------|------|:--------:|
| `lsp_completion` | 获取光标位置的代码补全建议 | ✅ |
| `lsp_rename` | 跨文件重命名符号（实际修改文件） | ❌ |

### 新功能 — LSP 诊断自动注入
`edit_file`/`write_file`/`multi_edit`/`delete_range`/`delete_symbol` 执行后自动触发 LSP 诊断，
将编译/检查错误作为系统消息注入下一轮对话。模型无需主动请求诊断即可看到代码问题。

- 新接口: `AgentRunner.lspManager` + `SetLSPManager()`
- 新方法: `runPostToolDiagnostics()` — 执行在 `executeBatch` 之后
- 自动连线: `boot.go` LSP 初始化时连接 Agent

### 新功能 — Diff 注入模型上下文
`edit_file`/`write_file`/`multi_edit` 等 writer 工具执行后，自动收集文件变更的 unified diff，
作为系统消息注入下一轮对话。模型无需手动 `read_file` 确认变更结果——在下一轮回答中即可看到
自己修改了哪些文件、哪几行、具体做了什么改动。

- 新增: `AgentRunner.pendingDiffs` 收集器 + `runPostToolDiffPreview()` 方法
- 注入时机: `executeBatch` → `runPostToolDiagnostics` → `runPostToolDiffPreview`
- 位置: `internal/agent/agent.go`


### 新功能 — 动态 Provider 路由
模型选择从纯启发式（关键词+长度）升级为历史感知的贝叶斯路由：
- 每个任务被指纹化为任务桶（长度区间 + 关键词签名）
- 桶内统计 flash/pro 的历史调用次数
- 如果 flash 在相似任务上有 > 90% 的成功率，优先走 flash
- 样本不足（< 3 次）时回退到启发式决策
- 路由决策在会话内持续学习

新增文件: `internal/agent/auto_router_history.go`



### 新功能 — Constitution 宪法系统
结构化约束文件 `.tianxuan/constitution.toml`，声明：
- **权威链**: user_intent > constitution > project_memory > code_tests > convention > memory
- **6 条保护性不变式**: 不泄露密钥、不删除错误处理、不越界写入、不经批准运行 shell 等
- **验证策略**: 声明完成前必须运行测试、验证诊断、提供证据

位置: `.tianxuan/constitution.toml` + `internal/boot/constitution.go`

### 新功能 — 子 Agent 输出契约
子 agent（task / run_skill / explore / research / review / security_review）输出强制五段式：
SUMMARY / CHANGES / EVIDENCE / RISKS / BLOCKERS，让父模型能快速评估子任务结果。

注入在系统提示的 V7.4 段。

### 新功能 — `/undo` 回滚命令
- `Controller.Submit()` 中新增 `/undo [N]` 命令
- 利用 Checkpoint 系统回滚最近 N 轮的代码修改 + 对话
- 桌面/HTTP 前端均可使用

### 重构 — controller.go 拆分（2041→1469 行，-28%）
| 新文件 | 行数 | 内容 |
|--------|:----:|------|
| `controller_submit.go` | 270 | Submit + 全部 slash 命令分发 |
| `controller_memory.go` | 98 | QuickAdd / SaveDoc / ForgetMemory / QueueMemory |
| `controller_approval.go` | 202 | gateApprover / seedPlanTodos / PlanTodosJSON / requestApproval |

### 重构 — chat_tui.go 样式提取
| 新文件 | 行数 | 内容 |
|--------|:----:|------|
| `chat_tui_styles.go` | 27 | 4 个样式变量（原在 chat_tui.go 内联） |
| `chat_tui_view.go` | 311 | View + 11 个渲染辅助函数 |

### 文档清理
- `design.md` → `design.md.obsolete`（描述已废弃的 Rust tokio 架构，与 Go 代码完全不符）
- V5.0-ARCHITECTURE/V5.0-RELEASE/V6.0-ARCHITECTURE/V6.1-RELEASE 归档到 `_archive/`
- TIANXUAN.md 完整重写，增加工具一览表和 V7.4 特性

### Bug 修复
- `branch.go:72`: `renderTUIBanner` 调用传了 4 个参数但函数只接受 3 个，第 4 个 `m.sessionTitle` 在 struct 中不存在

### 发布
- CLI: `bin/tianxuan.exe` (13.2 MB)
- 桌面端: `desktop/build/bin/tianxuan-desktop.exe`
