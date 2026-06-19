# tianxuan project memory

> V8.3.0 — 记忆模块全量升级：GlobalDir + Archive + BM25 + 增量刷新 · 2026-06-21

## V8.3.0 发布摘要 (2026-06-21)

**参照**: DeepSeek-Reasonix v1.9.1 `internal/memory` + `internal/retrieval` 架构

**核心升级**:
- **双目录 Store**: `GlobalDir`（user/feedback 全局共享）+ `Dir`（project/reference 项目隔离）
- **Archive 归档**: 替代永久删除，移到 `.archive/<timestamp>-<name>.md`
- **BM25 搜索**: `retrieval` 包 — CJK 单字切分 + BM25(k1=1.2,b=0.75) + Snippet
- **memory 工具**: search/read/list 三合一，替代旧的 `memory_search`
- **增量刷新**: RefreshDocs/RefreshIndex 替代全量 Load()，O(N)→O(1)
- **forget 纠错**: Levenshtein 编辑距离建议候选
- **老化追踪**: Memory.Mtime + Block() 末尾 stale 计数

**变更统计**: 13文件，+537/-565行（新增 retrieval 214行 + recall 284行，删除 search 299行 + memory_search 106行）

## Project

tianxuan 是一个面向 DeepSeek V4 的极简 Coding Agent。单 Go 二进制，零外部依赖。
核心目标：极低成本、极快响应。

## Architecture

**单模型直连** — 无 Planner、无 Learner、无 LLM Compact。
| **L1 Identity 字节不可变** — 系统提示词一旦锁定，任何字符（含空格/换行）不能改 | `verifyPrefix` 用 SHA-256 校验；漂移 → panic |
Mock 无压缩14轮: 93%
Mock 小窗口30轮: 91% (10轮恢复)
真实API大前缀: 94%

### 核心包结构 (V8.3.0)

| 包 | 文件 | 职责 |
|----|------|------|
| `cmd/tianxuan/` | `main.go` (22行) | 入口：空白导入注册 → `cli.Run()` |
| `internal/cli/` | `cli.go`, `chat_tui.go` (70KB) | 子命令路由、Bubble Tea TUI |
| `internal/boot/` | `boot.go` (30KB) | **装配工厂**：唯一 Build() 入口 |
| `internal/control/` | `controller.go` (33KB) | **会话驱动器**：传输无关事件流 |
| `internal/agent/` | `agent.go` (51KB) | **AI 引擎**：模型调用、批量执行、缓存 |
| `internal/context/` | `flow.go`, `manager.go` | TCCA 缓存内核 L1/L2 |
| **`internal/memory/`** | **`store.go`** (18KB), **`memory.go`** (8KB), **`recall.go`** (8KB) | **双目录记忆 + BM25 搜索** |
| **`internal/retrieval/`** | **`bm25.go`** (5KB) | **通用 BM25 + CJK 分词 + Snippet** |
| `internal/learning/` | `patterns.go` | 跨会话错误学习 60天衰减 |
| `internal/lsp/` | `client.go`, `manager.go` | LSP 诊断/补全/跳转 |
| `internal/skill/` | `skill.go`, `builtins.go` | 技能/Playbook 系统 |

### 记忆模块架构 (V8.3.0)

```
记忆模块 = 分层文档记忆 (TIANXUAN.md/AGENTS.md 链条) + 双目录自动记忆仓库
                ↓
        boot.Load() → Compose() 注入系统前缀（缓存零成本）
                ↓
        中途变更 → QueueMemory → pendingMemory → 下一轮 turn-tail 注入
                ↓
        Store{Dir, GlobalDir}
        user/feedback → GlobalDir (跨项目共享)
        project/reference → Dir (项目隔离)
        forget → Archive() → .archive/<timestamp>-<name>.md
```

### 搜索架构 (V8.3.0)

```
memory 工具 (search/read/list)
    ↓
retrieval 包
    ├── Tokens(): CJK 单字切分 + 拉丁词小写
    ├── BM25Score(): k1=1.2, b=0.75, IDF归一化
    ├── KeepTopRelativeScore(): 相对分截断 (floor=0.15)
    └── MakeSnippet(): 260字符智能截取
```

## 约定

- Go kernel under internal/; each package owns one concern
- Transport-agnostic Controller behind every frontend
- Config: tianxuan.toml, secrets in .env, API key 在 ~/.env
- 桌面端: Wails v2, React 18, Vite 6, Zustand 5

## 编码原则

1. **Think Before Coding** — State assumptions。Surface tradeoffs。
2. **Simplicity First** — Minimum code。No speculative features。
3. **Surgical Changes** — Touch only what you must。
4. **Goal-Driven Execution** — Define verifiable success criteria。Loop until verified。
