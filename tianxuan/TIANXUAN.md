# tianxuan project memory

> V7.7.1 — 8个内置技能 · 双重 JSON Repair 消除 · toolcache O(1) · 2026-06-18

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
- **前缀稳定**: L1 Identity 字节不变，SHA-256 跨会话验证
- **工具描述免费**: L3 在 prefix cache 中，100% 命中不计费

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
