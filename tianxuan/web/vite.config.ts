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
      "/history": "http://127.0.0.1:8080",
      "/context": "http://127.0.0.1:8080",
      "/health": "http://127.0.0.1:8080",
    },
  },
});
