# Reasonix V1.15 → Tianxuan 蒸馏实施记录

> 基于 Reasonix V1.15 本地源码 (D:/AI/reasonix-v1.15) 逐文件对比 + KunAgent 设计分析

---

## ✅ 已完成（2026-07-04 会话）

### 第一批：Kun 蒸馏
| # | 特性 | 来源 | 文件 |
|---|------|------|------|
| 1 | 结构化搜索结果 JSON (title/url/snippet/source) | Kun | `websearch.go`, `compact.go` |
| 2 | web_fetch 域名策略 allow/deny | Kun | `webfetch.go`, `config.go` |
| 3 | task 子代理 retry_until | Kun | `task.go` |
| 4 | 后台任务启停循环检测器 | 原创 | `agent.go`, `agent_run.go`, `execute_one.go` |

### 第二批：Reasonix 核心 (🥇+🥈+🥉)
| # | 特性 | 文件 | 代码量 |
|---|------|------|--------|
| 1.1 | 文件编码检测管线 (8种编码) | `fileutil/encoding/encoding.go`, `encoding_helpers.go`, `readfile.go`, `editfile.go`, `multiedit.go` | ~615 行 |
| 1.2 | 模糊编辑匹配 (尾空格/tab↔space/read_file前缀) | `encoding_helpers.go`, `editfile.go`, `multiedit.go` | ~330 行 |
| 1.3 | 大括号完整性校验 | `delete_range.go` | ~100 行 |
| 1.4 | complete_step 增强 (step_index + commandHints) | `completestep.go`, `compact.go`, `evidence/evidence.go` | ~40 行 |
| 1.5 | Stale anchor 编辑守卫 | `agent.go`, `agent_run.go`, `execute_one.go` | ~65 行 |
| 2.1 | ripgrep 委托 | `grep.go`, `boot/boot.go` | ~65 行 |
| 2.2 | Tokenizer HTML 提取 (net/html 替换正则) | `webfetch.go` | ~250 行 |
| 3.1 | code_index 符号索引 (Go AST + 多语言 regex) | `codeindex.go`, `compact.go` | ~310 行 |
| 3.4 | move_file 工具 | `movefile.go`, `compact.go`, `config/config.go` | ~70 行 |

**总计：17 个特性，~2,085 行新增代码，全量测试通过**

---

## 🔴 未完成（下个会话继续）

### 编码管线遗留 ✅ 已完成（2026-07-04 第二部分）
| 特性 | 文件 | 状态 |
|------|------|------|
| **delete_range 编码保存** | `delete_range.go` | ✅ Execute() 使用 readFileEncoded + writeFileEncoded |
| **delete_symbol 编码保存** | `delete_symbol.go` | ✅ Execute/Preview 使用编码感知读，Execute 编码感知写 |
| **editlines 编码保存** | `editlines.go` | ✅ 使用 readFileEncoded + writeFileEncoded |
| **writefile 编码保存** | `writefile.go` | ✅ 覆盖已有文件时保留原编码，新文件 UTF-8 |

### 子代理增强
| 特性 | 说明 |
|------|------|
| **子代理 transcript 持久化 (continue_from)** | ✅ 已完成（2026-07-04 第三部分）。新建 `internal/agent/subagent_store.go` (~240行) + 修改 `task.go` (~65行)。SubagentStore/SubagentRun 精简版：PrepareFresh/PrepareContinue/SaveCompleted/SaveFailed，支持跨轮次续跑。

### 架构升级（独立 PR）
| 特性 | 说明 |
|------|------|
| **双模型协调器 (planner + executor)** | ✅ 已完成（2026-07-04 第四部分）。新建 `internal/agent/coordinator.go` (~260行)，修改 `boot.go` 集成。当 `planner_model` 配置时自动启用。

### 其他优化（优先级低）
| 特性 | 说明 |
|------|------|
| web_fetch 代理支持 (HTTP CONNECT + SOCKS5) | 对 GFW 用户重要 |
| grep .gitignore 精确行走（纯 Go 扫描器用） | ripgrep 委托已覆盖大部分场景 |
| delete_range 编码保存 | ✅ 已完成 (2026-07-04 第二部分) |
| delete_symbol 编码保存 | ✅ 已完成 (2026-07-04 第二部分) |

---

## 📁 关键文件清单（已修改）

```
internal/fileutil/encoding/encoding.go    # 新增：8种编码检测+转换
internal/tool/builtin/encoding_helpers.go # 新增：编码读写 + 模糊匹配引擎
internal/tool/builtin/readfile.go         # 重写：完整编码管线
internal/tool/builtin/editfile.go         # 编码读写 + applyOldStringEdit
internal/tool/builtin/multiedit.go        # 编码读写 + applyOldStringEdit
internal/tool/builtin/delete_range.go     # 大括号完整性校验
internal/tool/builtin/completestep.go     # step_index + commandHints
internal/tool/builtin/codeindex.go        # 新增：符号索引工具
internal/tool/builtin/movefile.go         # 新增：move_file 工具
internal/tool/builtin/grep.go             # ripgrep 委托
internal/tool/builtin/webfetch.go         # Tokenizer HTML + 域名策略
internal/tool/builtin/websearch.go        # 结构化 JSON 输出
internal/tool/builtin/compact.go          # 多处 schema 更新
internal/evidence/evidence.go             # SuccessfulCommands()
internal/agent/agent.go                   # stale anchor 追踪 + bg start-kill
internal/agent/agent_run.go               # 复位逻辑
internal/agent/execute_one.go             # stale anchor 守卫 + bg 标志
internal/agent/task.go                    # retry_until + continue_from + SubagentStore 集成
internal/agent/subagent_store.go          # 新增：SubagentStore/SubagentRun (continue_from)
internal/config/config.go                 # AllowDomains/DenyDomains + move_file
internal/boot/boot.go                     # ResolveRgPath() + Coordinator 集成
internal/agent/coordinator.go              # 新增：双模型协调器 (planner + executor)
```

---

## 📚 参考源文件

```
D:/AI/reasonix-v1.15/    # Reasonix V1.15 本地源码 (git clone)
docs/specs/2026-07-04-reasonix-distillation-plan.md  # 原始蒸馏计划
```
