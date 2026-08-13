# Tianxuan · 核心

<p align="center"><strong>面向 DeepSeek 的极简 AI 编程助手</strong> — 单 Go 二进制，CLI + 桌面端。</p>

## 这是什么

Tianxuan 是一个 AI 编程助手，核心思路是围绕 DeepSeek 的前缀缓存机制精心设计消息结构，
让长会话的 token 成本压到极低（实测命中率常年在 90%+）。

- **CLI** — 终端里用 `tianxuan chat` 或 `tianxuan run "任务"`
- **桌面端** — Wails 套壳的 GUI，带系统托盘（点 X 隐藏到托盘，托盘菜单退出）

## 快速开始

```bash
# 构建 CLI
go build -o tianxuan.exe ./cmd/tianxuan/

# 配置 API Key
export DEEPSEEK_API_KEY=sk-...

# 开始使用
./tianxuan.exe chat          # 交互对话
./tianxuan.exe run "你的任务" # 单次执行
```

## 核心设计

| 层级 | 内容 | 缓存策略 |
|------|------|----------|
| **L1 Identity** | 系统提示词（~300 tok） | SHA-256 校验，不可变 |
| **L2 Runtime** | 项目/语言/环境（~100 tok） | 首轮锁定，后续不可变 |
| **L3 Skills** | 工具紧凑描述（~1200 tok） | 100% 命中，不计费 |
| **L4 Flow** | 对话历史 | 三维压缩（HistoryHygiene） |

> 缓存是命脉：L1 任何字节变化都会导致整轮前缀失效 → 全量 cache miss → 约 2.5 倍费用。
> 所有改动必须先过缓存安全检查。

## 主要特性

- **30+ 内置工具** — 文件读写、bash、git、LSP、Web 搜索、MCP 客户端
- **计划模式** — 复杂任务先生成只读计划，批准后才执行
- **权限沙箱** — allow/ask/deny 三级 + 项目内文件写限制
- **MCP 插件** — stdio + Streamable HTTP，兼容 Claude Code `.mcp.json`
- **会话持久化** — 会话分支、checkpoint 回滚、跨会话恢复
- **双模型协作** — 执行器 + 规划器，各自独立的缓存稳定会话
- **系统托盘** — 桌面端关闭时隐藏到托盘

## 仓库结构

```
cmd/tianxuan/       → CLI 入口
internal/           → 核心包（agent/cache/context/control/tool/lsp/…）
desktop/            → 桌面端（Wails + React，独立 Go module）
scripts/            → 发布/构建/缓存守卫脚本
docs/               → 工程规格与迁移指南
_archive/           → 历史架构文档
```

开发工作流见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 版本

当前 **V10.177.0** — 详见 [CHANGELOG.md](CHANGELOG.md)。

## 许可

MIT — 见 [LICENSE](LICENSE)。
