# tianxuan V8.0.6

tianxuan 是一个面向 DeepSeek V4 的极简 Coding Agent。单 Go 二进制，零外部依赖。

## 内置技能 (8个)

| 技能 | 方式 | 职责 |
|------|:--:|------|
| `init` | inline | 初始化 AGENTS.md |
| `explore` | subagent | 代码库探索 — 7 个 CodeGraph 工具 + 3 个 LSP 查询 |
| `research` | subagent | explore + web 搜索 |
| `review` | subagent | Git 原生工具 + CodeGraph 影响分析 + LSP 诊断 |
| `security-review` | subagent | 安全视角审查 |
| `tdd` | inline | RED → GREEN → REFACTOR |
| `lsp` | inline | 诊断 → 理解 → 修复 → 验证 |
| `debug` | inline | 4 阶段调试：Reproduce → Isolate → Fix → Prevent |

## V8.0 新特性

| 特性 | 说明 |
|------|------|
| 确定性结果剪枝 | 相同工具+参数+结果不重复发 token |
| Mid-turn Steer | 检测错误螺旋，注入纠偏提示 |
| Plan 模式智能澄清 | 模糊输入主动追问 |
| read_skill 工具 | Agent 可按需读取技能内容 |
| Plan 模式 bash 安全白名单 | 20+ 安全命令 + 元字符/重定向检测 |
| Context7 MCP | 环境变量 CONTEXT7_API_KEY 自动启用 |
| /goal 命令 | 大目标分解为子任务 |
| PermissionRequest Hook | 自定义审批策略 + 参数修改 |

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

`git checkout v7.7.1` 回退到 V8.0 之前的最后稳定版。
