import path from "node:path";
import react from "@vitejs/plugin-react";

/** Explicit test-only aliases: never import this config from Next.js. */
const config = {
  root: path.resolve(import.meta.dirname, "../.."),
  plugins: [react()],
  define: { "process.env": "{}" },
  resolve: {
    alias: [
      {
        find: "@/components/auth/session",
        replacement: path.resolve(import.meta.dirname, "ai-presets-session.ts"),
      },
      {
        find: "@",
        replacement: path.resolve(import.meta.dirname, "../../src"),
      },
    ],
  },
  server: { host: "127.0.0.1", port: 5197, strictPort: true },
};

export default config;
