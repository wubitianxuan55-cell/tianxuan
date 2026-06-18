# tianxuan V7.7.1

tianxuan 是一个面向 DeepSeek V4 的极简 Coding Agent。单 Go 二进制，零外部依赖。

## 内置技能 (8个)

| 技能 | 方式 | 职责 |
|------|:--:|------|
| `init` | inline | 初始化 AGENTS.md |
| `explore` | subagent | 代码库探索 — 7 个 CodeGraph 工具 + 3 个 LSP 查询 |
| `research` | subagent | explore + web 搜索 |
| `review` | subagent | Git 原生工具 + CodeGraph 影响分析 + LSP 诊断 |
| `security-review` | subagent | 安全视角审查 |
| `tdd` | inline | RED（隔离 + 写失败测试）→ GREEN（最小修复）→ REFACTOR（回归测试 + 清理）|
| `lsp` | inline | 诊断 → 理解 → 修复 → 验证 |
| `debug` | inline | 4 阶段调试：Reproduce → Isolate → Fix → Prevent |

## 命令

```
tianxuan chat    交互式对话
tianxuan run     单任务执行
tianxuan version 版本信息
```

## 配置

复制 `tianxuan.example.toml` 为 `tianxuan.toml` 并编辑。
API 密钥通过环境变量 `.env` 或 `~/.env` 设置。

## 回退

如遇问题，回退到 V7.7.0：`git checkout v7.7.0` 或替换 `release/v7.7.0/` 下的二进制。
