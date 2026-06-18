# tianxuan project memory

> V8.2.0 — 文件拆分 + Mock UI + Logo 重设计 + 桌面端布局升级 · 缓存红线固化 · 2026-06-18

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
# 构建 CLI
go build -ldflags "-s -w" -o bin/tianxuan.exe ./cmd/tianxuan

# 构建桌面端 (Wails)
cd desktop && wails build

# Go 测试
go test ./internal/...

# Go vet
go vet ./internal/...

# 前端校验
cd desktop/frontend && npx tsc --noEmit && npx vite build
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
