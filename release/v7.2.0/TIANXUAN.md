# tianxuan project memory

> V7.2.0 — DSR 压缩自适应 · 三闸门停止 · 会话归档 · Bug 修复 · 2026-06-13

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
- **L3 Skill** (~1,200 tok): 17 工具紧凑描述，prefix cache 完全命中
- **L4 Flow**: 对话历史，HistoryHygiene 三维压缩

### V5.22 新增（V5.10 → V5.22 12版本合并）

#### 缓存守卫 (Kun 源码移植)
| 模块 | 功能 |
|------|------|
| ImmutablePrefix | SHA256 校验 L1+L2+tools，漂移→panic |
| HistoryHygiene | 三维压缩替代 32KB 硬截断 |
| TokenEconomy | 按工具策略 (bash 180行/read 320行/grep 30条) |
| PrefixVolatility | 检测 UUID/ISO8601/hex/JWT 破坏缓存前缀 |
| ToolFingerprint | 工具集漂移检测 (additive/breaking) |
| CompactionDigest | 压缩摘要 SHA256 标记 |

#### 成本优化
| 模块 | 功能 |
|------|------|
| ParamStormBreaker | 参数级重复调用检测 (窗口8阈值3) |
| 三级压缩 | normal/aggressive/force，动态调整保留消息数 |
| BudgetGate | 80%警告/100%阻断 |
| ModelContextProfile | flash 128K / pro 1M |
| ToolCallRepair | 展平+捞取JSON+截断超大参数 |
| AutoRouter | 关键词+长度匹配 flash/pro |

#### GUI 升级
- 快捷任务卡片 (Kun ChatStarterGrid 风格)
- PlanPanel (Markdown 渲染 + 计划内容提取)
- 会话归档 API

#### 桌面端优化 (V5.23-dev)
- App.tsx 976→785 行，提取 6 个 hooks
- ToolCard + MemoMarkdown React.memo
- 5 个抽屉 React.lazy 懒加载
- Vite vendor 分包

### 关键约束
- **单模型**: `Run()` 直接调用 `runDirect()`，无 Planner 分支
- **零额外 LLM 调用**: 无 compact 摘要，无 Learner 反馈
- **前缀稳定**: L1 Identity 字节不变，SHA-256 跨会话验证
- **工具描述免费**: L3 在 prefix cache 中，100% 命中不计费

### V6.1 新增 (2026-06-12)

#### 运行时增强
| 模块 | 功能 |
|------|------|
| 并行工具 | getConflictKey 分组，编辑不同文件可并行执行 |
| 子代理轻量 | TemplatePrefix + ActiveSchemas 共享前缀缓存 |
| 3 Agent 模式 | `/mode explore/develop/orchestrate` 三种运行模式 |
| 缓存友好 | pressure_flush + output_continue + repeat_detect + stop_gate |

#### 智能进化
| 模块 | 功能 |
|------|------|
| Budgeted 注入 | 4 组件重要性权重预算：checkpoint 0.9 / memory 0.7 / tasks 0.5 / recent 1.0 |
| Dream & Distill | `/dream [scan\|extract]` 跨会话分析、`/distill [scan\|create]` 模式检测 + skill 生成 |
| Goal/Stop | `/goal` 设置目标 + stopGate 强制验证 + **Judge 模型独立验证** |
| 工具合并 | `[tools] compact = true` 隐藏冗余工具 ~41→~25 |
| Checkpoint-Writer | 预算重建后自动 QueueMemory 摘要 |
| 树形任务 | nudge 注入 T1/T1.1 格式提示（不修改 L3 schema） |
| 会话归档 | JSONL archive 跨会话查询 + 工具使用统计 |

#### 新增命令
| 命令 | 功能 |
|------|------|
| `/mode e/dev/o` | 切换 explore/develop/orchestrate 模式 |
| `/goal <desc>` | 设置会话停止条件（Judge 模型验证） |
| `/dream [scan\|extract]` | 跨会话知识提取 |
| `/distill [scan\|create]` | 模式发现 + skill 生成 |
| `/memories [query]` | 搜索持久记忆 |

**缓存安全：全部改动严格遵守 L1/L2/L3 前缀缓存不变原则**

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

- `internal/agent/` — AgentRunner, stream, executeBatch, compact, 缓存守卫 (param_storm, prefix_volatility, tool_call_repair, budget_gate, auto_router)
- `internal/boot/` — Build() 装配工厂
- `internal/cache/` — 四域管理 (Identity/Runtime/Skill/Spawn)
- `internal/context/` — TCCA 内核 (ContextManager)
- `internal/control/` — Controller 会话驱动
- `internal/tool/` — Tool 接口 + CompactDescriptor
- `desktop/frontend/src/hooks/` — 桌面端 6 个自定义 hooks

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
- `git -C tianxuan checkout v5.23.0` 回到当前版本
- Config: tianxuan.toml, secrets in .env, API key 在 ~/.env
- 桌面端: Wails v2, React 18, Vite 6, Zustand 5

## 编码原则

1. **Think Before Coding** — State assumptions. Surface tradeoffs.
2. **Simplicity First** — Minimum code. No speculative features.
3. **Surgical Changes** — Touch only what you must.
4. **Goal-Driven Execution** — Define verifiable success criteria. Loop until verified.
