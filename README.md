<p align="center">
  <h1 align="center">天璇 · Tianxuan</h1>
  <p align="center"><strong>面向 DeepSeek 的极简 AI 编程助手</strong> — 单 Go 二进制，CLI + 桌面端</p>
</p>

<p align="center">
  <a href="./tianxuan/LICENSE"><img src="https://img.shields.io/badge/license-MIT-8b949e?style=flat-square&labelColor=161b22" alt="license"/></a>
  <a href="https://github.com/wubitianxuan55-cell/tianxuan/stargazers"><img src="https://img.shields.io/github/stars/wubitianxuan55-cell/tianxuan?style=flat-square&color=dbab09&labelColor=161b22" alt="stars"/></a>
  <a href="https://github.com/wubitianxuan55-cell/tianxuan/commits/main"><img src="https://img.shields.io/github/last-commit/wubitianxuan55-cell/tianxuan?style=flat-square&color=3fb950&labelColor=161b22" alt="last commit"/></a>
</p>

---

## 这是什么

天璇是一个**只对 DeepSeek 优化的 AI 编程助手**。核心思路：围绕 DeepSeek 的前缀缓存机制精心设计消息结构，让长会话的 token 成本压到极低。实测命中率常年 90%+。

- **CLI** — 终端里用，`tianxuan chat` 或 `tianxuan run "任务"`
- **桌面端** — Wails 套壳的 GUI，带系统托盘，点 X 隐藏到托盘不退出

## 快速开始

```bash
# 构建 CLI
cd tianxuan && go build -o tianxuan.exe ./cmd/tianxuan/

# 配置 API Key
export DEEPSEEK_API_KEY=sk-...

# 开始使用
./tianxuan.exe chat          # 交互对话
./tianxuan.exe run "你的任务"  # 单次执行
```

桌面端构建见 [`tianxuan/`](tianxuan/)。

## 核心设计

| 层级 | 内容 | 缓存策略 |
|------|------|----------|
| **L1 Identity** | 系统提示词 (~300 tok) | SHA-256 校验，不可变 |
| **L2 Runtime** | 项目/语言/环境 (~100 tok) | 首轮锁定，后续不变 |
| **L3 Skills** | 工具紧凑描述 (~1200 tok) | 100% 命中，不计费 |
| **L4 Flow** | 对话历史 | 三维压缩（HistoryHygiene） |

> ⚠️ 缓存是命脉。L1 改 1 字节 → 整轮 cache miss → 费用翻倍。所有改动必须先过缓存安全审计。

## 主要特性

- **30+ 内置工具** — 文件读写、bash、Git、LSP、Web 搜索、MCP 客户端
- **计划模式** — 复杂任务先生成只读计划，批准后才执行
- **权限沙盒** — allow/ask/deny 三级 + 文件写限制在项目内
- **MCP 插件** — stdio + Streamable HTTP，兼容 Claude Code `.mcp.json`
- **会话持久化** — 对话分支、Checkpoint 回滚、跨会话恢复
- **双模型协同** — 执行器 + 规划器，各自独立的缓存稳定会话
- **系统托盘** — 桌面端点 X 隐藏，右键菜单显示/退出

## 项目结构

```
├── tianxuan/              ← 主项目（Go 内核）
│   ├── cmd/tianxuan/      ← CLI 入口
│   ├── internal/          ← 核心包（agent/cache/context/control/tool/lsp）
│   ├── desktop/           ← 桌面端（Wails + React）
│   └── release/           ← 发布产物
├── build-desktop.bat      ← 桌面端构建脚本
├── build-wails.bat        ← Wails 构建脚本
└── AGENTS.md              ← 项目开发规范
```

## 版本

当前 **V8.16.1** — 详见 [`tianxuan/CHANGELOG.md`](tianxuan/CHANGELOG.md)

## 许可

MIT — 见 [`tianxuan/LICENSE`](tianxuan/LICENSE)
