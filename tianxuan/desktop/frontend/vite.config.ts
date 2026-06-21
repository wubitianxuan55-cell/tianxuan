import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Strip legacy KaTeX .woff/.ttf fonts from the build — all modern WebView
// engines (Windows/macOS/Linux) support woff2 natively.  Saves ~876 KB.
function stripLegacyFonts() {
  return {
    name: "strip-legacy-fonts",
    generateBundle(_opts: any, bundle: any) {
      for (const [key, chunk] of Object.entries(bundle)) {
        if (
          (key.endsWith(".woff") && !key.endsWith(".woff2")) ||
          key.endsWith(".ttf")
        ) {
          delete bundle[key];
        }
      }
    },
  };
}

// base: "./" so built asset URLs are relative. Wails serves the embedded dist from
// the app root over the wails:// scheme, where absolute "/assets/..." URLs 404.
export default defineConfig({
  plugins: [tailwindcss(), react(), stripLegacyFonts()],
  base: "./",
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
    // Bind IPv4 — unset host listens on ::1, and the Wails dev proxy's [::1]
    // dial fails on Windows hosts where IPv6 loopback is filtered.
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
  },
});
