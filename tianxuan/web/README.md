# tianxuan Web UI

基于 HTTP/SSE 的 Web 界面，通过 `tianxuan serve` 连接 Go 内核。

## 架构

```
Browser ──HTTP/SSE──▶ tianxuan serve (Go) ──▶ AgentRunner (零修改)
```

- **缓存安全**: 纯展示层，不触及 TCCA 四域
- **组件**: 100% 复用 `desktop/frontend/src/` React 组件，仅替换 bridge.ts 通信层

## 开发

```bash
cd web
npm install
npm run dev       # 启动 dev server :5174，代理到 tianxuan serve :8080
```

需要先启动后端：
```bash
tianxuan serve --port 8080
```

## 构建

```bash
npm run build     # 产出 dist/，嵌入 Go binary 的 serve 模式
```
