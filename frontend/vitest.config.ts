import path from "node:path";

import { defineConfig } from "vitest/config";

export default defineConfig({
  define: {
    "process.env.NODE_ENV": JSON.stringify("test"),
  },
  resolve: {
    alias: { "@": path.resolve(__dirname, "src") },
  },
  test: {
    environment: "jsdom",
    include: ["tests/**/*.test.ts", "tests/**/*.test.tsx"],
    setupFiles: ["./tests/setup.ts"],
  },
});
