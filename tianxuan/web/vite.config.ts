import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "path";

// Web 版本：base="/" (非 Wails "./")，通过 HTTP/SSE 连接 tianxuan serve
export default defineConfig({
  plugins: [tailwindcss(), react()],
  base: "/",
  resolve: {
    alias: {
      "@shared": resolve(__dirname, "../desktop/frontend/src"),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2021",
    rollupOptions: {
      output: {
        manualChunks: {
          "vendor-markdown": ["react-markdown", "remark-gfm", "remark-math", "rehype-katex"],
          "vendor-ui": ["lucide-react", "@tanstack/react-virtual"],
        },
      },
    },
  },
  server: {
    port: 5174,
    proxy: {
      "/events": "http://127.0.0.1:8080",
      "/submit": "http://127.0.0.1:8080",
      "/cancel": "http://127.0.0.1:8080",
      "/approve": "http://127.0.0.1:8080",
      "/plan": "http://127.0.0.1:8080",
      "/compact": "http://127.0.0.1:8080",
      "/new": "http://127.0.0.1:8080",
      "/history": "http://127.0.0.1:8080",
      "/context": "http://127.0.0.1:8080",
      "/health": "http://127.0.0.1:8080",
      "/meta": "http://127.0.0.1:8080",
      "/memory": "http://127.0.0.1:8080",
      "/remember": "http://127.0.0.1:8080",
      "/forget": "http://127.0.0.1:8080",
      "/save-doc": "http://127.0.0.1:8080",
      "/answer": "http://127.0.0.1:8080",
      "/models": "http://127.0.0.1:8080",
      "/sessions": "http://127.0.0.1:8080",
      "/delete-session": "http://127.0.0.1:8080",
      "/resume-session": "http://127.0.0.1:8080",
      "/files": "http://127.0.0.1:8080",
      "/file": "http://127.0.0.1:8080",
      "/balance": "http://127.0.0.1:8080",
      "/jobs": "http://127.0.0.1:8080",
      "/commands": "http://127.0.0.1:8080",
      "/capabilities": "http://127.0.0.1:8080",
      "/settings": "http://127.0.0.1:8080",
      "/mcp": "http://127.0.0.1:8080",
      "/checkpoints": "http://127.0.0.1:8080",
      "/rename-session": "http://127.0.0.1:8080",
      "/slash-args": "http://127.0.0.1:8080",
      "/tcca-report": "http://127.0.0.1:8080",
      "/rebuild": "http://127.0.0.1:8080",
    },
  },
});
