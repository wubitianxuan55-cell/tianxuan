# tianxuan VS Code Extension

> V8.13.0 — 全面 AI 编码助手增强

## 架构

```
VS Code Extension (TypeScript) ──HTTP/SSE──▶ tianxuan serve (Go)
   │  ▲                                         │
   │  └── postMessage 代理 ──────────────────────┘
   │
   ├── InlineCompletionProvider (代码补全)
   ├── HoverProvider (悬停解释)
   ├── Webview Panel (完整聊天 UI)
   └── Sidebar View (侧边栏聊天)
```

- **缓存安全**: 纯外壳层，不触及 Go 核心（system prompt/tools/messages 全不变）
- **双传输层**: 浏览器→fetch+EventSource，VS Code→postMessage 代理（CSP 安全）
- **前端**: 复用 `desktop/frontend/src` 的 React 组件，通过 `@shared` alias 引用

## 功能

| 功能 | 快捷键 | 说明 |
|------|--------|------|
| 打开聊天面板 | `Ctrl+Shift+T` | 在编辑器旁打开 tianxuan 对话面板 |
| 发送选中内容 | `Ctrl+Shift+Enter` | 将选中代码发送给 tianxuan 处理 |
| 内联代码补全 | 自动 | 输入暂停 300ms 后自动触发，从 Go 端获取补全建议 |
| 悬停解释 | 悬停 | 鼠标悬停在标识符上时显示 AI 解释 |
| 解释代码 | 右键菜单 | 用 tianxuan 解释选中代码 |
| 审查代码 | 右键菜单 | 用 tianxuan 审查选中代码 |
| 修复问题 | 右键菜单 | 用 tianxuan 修复选中代码中的问题 |

## 开发

```bash
cd vscode
npm install
npm run compile          # TypeScript 编译检查
npm run build            # esbuild 打包 extension.js
npm run build:webview    # 构建 Web UI 并复制到 webview/
# 按 F5 在 VS Code 中启动调试
```

## 构建命令

| 命令 | 说明 |
|------|------|
| `npm run compile` | `tsc -p ./` — 类型检查 |
| `npm run build` | `esbuild` 打包 extension.js（~17KB）|
| `npm run build:webview` | 构建 Web 前端 + 复制到 webview 产物目录 |
| `npm run package` | 全量构建 + `vsce package` 生成 .vsix |

## 通信协议

### 请求/响应

Webview 通过 `postMessage` 发送 `tianxuan:request` 消息，扩展主进程转发到 Go serve 的 HTTP/SSE 端点：

```typescript
// Webview → Extension
{ type: "tianxuan:request", id: 1, method: "fetch", params: { method: "POST", path: "/submit", body: { input: "..." } } }

// Extension → Webview
{ type: "tianxuan:response", id: 1, result: { status: 200, body: "..." } }
```

### SSE 事件

```typescript
// Extension → Webview（逐帧转发）
{ type: "tianxuan:sse:event", data: "{...json...}" }
```

### 生命周期消息

Extension 主动发送的消息（不经过请求/响应通道）：

| 消息类型 | 方向 | 说明 |
|---------|------|------|
| `tianxuan:init` | Ext→Web | 初始化（端口、工作区、主题）|
| `tianxuan:submit-text` | Ext→Web | 外部命令提交文本到 Composer |
| `tianxuan:theme-changed` | Ext→Web | VS Code 主题变更 |
| `tianxuan:workspace-changed` | Ext→Web | 工作区文件夹变更 |

### 编辑器 API（VS Code 专有）

| method | 说明 |
|--------|------|
| `getDiagnostics` | 获取当前文件诊断信息 |
| `getEditorContext` | 获取当前编辑器上下文（文件/语言/选中文本/光标行）|
| `applyEdit` | 应用文本编辑到文件 |
| `diffPreview` | 在 VS Code diff 编辑器中预览代码变更 |
| `openWorkspacePath` | 打开文件（支持 `file.go:42` 行号定位）|
| `revealWorkspacePath` | 在系统文件管理器中显示文件 |

## 打包

```bash
npm run package  # 生成 .vsix
```

生成的 `.vsix` 文件可直接在 VS Code 中安装（Extensions → ... → Install from VSIX）。
