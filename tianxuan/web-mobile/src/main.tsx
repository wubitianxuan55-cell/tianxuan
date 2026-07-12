// web-mobile entry — 复用桌面端 store + 组件，只替换 bridge 层。
import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "@shared/styles.css";
import "@shared/tailwind.css";
import "./tailwind.css";

// 注入 HTTP/SSE bridge 到 window（模拟 Wails 环境）
import { app, onEvent } from "./bridge";
(window as any).go = { main: { App: app } };
(window as any).runtime = {
  EventsOn: (_name: string, cb: (...args: unknown[]) => void) => onEvent((e) => cb(e)),
  EventsOff: () => {},
  EventsOnce: () => {},
  BrowserOpenURL: (url: string) => window.open(url, "_blank"),
};

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
