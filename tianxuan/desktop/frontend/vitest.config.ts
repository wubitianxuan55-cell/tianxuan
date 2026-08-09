import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "node",
    // globals:true 让 @testing-library/react 检测到全局 afterEach 并自动
    // cleanup DOM——否则组件测试间渲染残留互相污染（auto-cleanup 失效）
    globals: true,
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
