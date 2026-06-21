// web/src/main.tsx — Web UI entry point.
// 直接导入桌面端 App 组件，仅替换 bridge 层（HTTP/SSE 替代 Wails IPC）。
// 缓存安全: 纯前端入口，不触及任何 Go 核心逻辑。

import React from "react";
import ReactDOM from "react-dom/client";
import App from "@shared/App";
import "@shared/styles.css";
import "@shared/tailwind.css";

// 注入 web bridge 到 window（桌面端用 Wails，Web 端用 HTTP/SSE）
import { app, onEvent } from "./bridge";
(window as any).go = { main: { App: app } };
(window as any).runtime = {
  EventsOn: (_name: string, cb: (...args: unknown[]) => void) => onEvent((e) => cb(e)),
  BrowserOpenURL: (url: string) => window.open(url, "_blank"),
};

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
