# tianxuan VS Code Extension

基于 ACP 协议 (JSON-RPC 2.0 over stdio) 的 VS Code 扩展。

## 架构

```
VS Code Extension (TypeScript) ──stdio JSON-RPC──▶ tianxuan serve (Go)
                                                          │
                                                  AgentRunner (零修改)
```

- **缓存安全**: 纯外壳层，不触及 TCCA 四域（L1/L2/L3/L4）
- **通信**: VS Code扩展通过 `tianxuan serve` 子进程启动 Go 内核，Webview 通过 HTTP/SSE 连接
- **前端**: 复用 `desktop/frontend/` 的 React 组件

## 开发

```bash
cd vscode
npm install
npm run compile
# 按 F5 在 VS Code 中启动调试
```

## 打包

```bash
npm run package  # 生成 .vsix
```
