import { canvasHostAliases } from "./canvas-host";
import path from "node:path";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      ...canvasHostAliases,
      "@": path.resolve(import.meta.dirname, "src"),
    },
    dedupe: ["react", "react-dom", "yjs"],
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./tests/setup.ts"],
    include: ["tests/unit/**/*.test.{ts,tsx}"],
  },
});
