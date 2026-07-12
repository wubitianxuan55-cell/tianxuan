import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "path";

export default defineConfig({
  plugins: [tailwindcss(), react()],
  base: "/mobile/",
  resolve: {
    alias: {
      "@shared": resolve(__dirname, "../desktop/frontend/src"),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2021",
  },
  server: {
    port: 5175,
    proxy: {
      "/events": "http://127.0.0.1:8080",
      "/submit": "http://127.0.0.1:8080",
      "/cancel": "http://127.0.0.1:8080",
      "/approve": "http://127.0.0.1:8080",
      "/history": "http://127.0.0.1:8080",
      "/meta": "http://127.0.0.1:8080",
      "/sessions": "http://127.0.0.1:8080",
      "/new": "http://127.0.0.1:8080",
      "/health": "http://127.0.0.1:8080",
      "/answer": "http://127.0.0.1:8080",
      "/context": "http://127.0.0.1:8080",
      "/models": "http://127.0.0.1:8080",
      "/settings": "http://127.0.0.1:8080",
      "/mcp": "http://127.0.0.1:8080",
    },
  },
});
