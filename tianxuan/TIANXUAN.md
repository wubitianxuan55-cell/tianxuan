# tianxuan project memory

> V8.18.0 — 缓存架构重构: LLM摘要digest累积 · 2026-06-21

## V8.18.0 (2026-06-21)

### 缓存架构重构: 纯截断 → LLM 摘要 digest 累积
🔧 compact(): LLM摘要digest累积 — [system+firstUser+digest1...N] 固定前缀全量 cache hit
🔧 planCompaction+tailStart: 按 token 预算定 tail 边界（替代按消息数）
🔧 partitionFold+pinnedPrefixLen: 旧 digest + 小 user turn 永久保留不折叠
🔧 summarize(): 复用现有Provider(空tools), 失败降级 mechanical fold
🔧 PruneStaleToolResults: compact 前免费预处理 — 替换可恢复 tool_result
🔧 CacheShape 重写: PrefixShape + CompareShape (120行) 替代三套独立哈希
🔧 verifyPrefix: panic → Notice
🔧 删除: cacheBreakDetector (6个FNV-1a字段) + 2个测试文件(626行)
📁 compact.go 重写(570行) · prune.go 新 · cache_shape.go 重写 · 9文件 +594/-1011行

## V8.17.0 核心变更 (2026-06-21)
- provider/openai: DeepSeek thinking.type=enabled 自动注入
- 吸收 retrieval/sysproxy/netclient/proc 4个独立模块

## V8.16.2 (2026-06-21)
- 仓库公开: 首次 git push → github.com/wubitianxuan55-cell/tianxuan
- 根目录 README.md

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

### 🔴 缓存保护红线（最高优先级）

| 规则 | 原因 | 违规案例 |
|------|------|----------|
| **L1 Identity 字节不可变** | `verifyPrefixAndShape` Notice 守卫 | — |
| **tools 列表整个会话不可变** | DeepSeek 缓存 key 包含 tools 列表 | V8.0.2 `filteredSchemas` |
| **L2 Runtime 首轮锁定** | 缓存 key 包含 L2 | — |
| **不允许动态系统提示词注入** | 破坏 L1 → user 固定前缀结构 | — |
| **工具描述不可热更新** | 工具描述是缓存前缀的一部分 | — |

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
