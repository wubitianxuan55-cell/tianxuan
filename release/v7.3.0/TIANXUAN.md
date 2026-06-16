# tianxuan project memory

> V7.3.0 — 统计面板修复 · DSR 收敛 · 2026-06-14

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

### V7.3 变更

| 模块 | 变更 |
|------|------|
| StatsPanel | 重排为上下文→会话→本轮→当前步→命中率趋势→Token累计→工具统计；Token 图改为按轮累计 |
| `compact.go` | 交换 fallback 优先级（Legacy 优先）；compactStuck 降级为纯截断；删除 digest marker |
| `prefix_volatility.go` | 🗑️ 删除（只检测不修复） |
| `pressure_flush.go` | 🗑️ 删除（与轮内压缩策略矛盾） |
| `rebuild.go` | 🗑️ 删除（buildCheckpointRebuild 死代码） |

### 关键约束
- **单模型**: `Run()` 直接调用 `runDirect()`，无 Planner 分支
- **零额外 LLM 调用**: 无 compact 摘要，无 Learner 反馈
- **前缀稳定**: L1 Identity 字节不变，SHA-256 跨会话验证
- **工具描述免费**: L3 在 prefix cache 中，100% 命中不计费
