import path from "node:path";
import type { IncomingMessage, ServerResponse } from "node:http";
import react from "@vitejs/plugin-react";

/** Explicit test-only aliases: never import this config from Next.js. */
const config = {
  root: path.resolve(import.meta.dirname, "../.."),
  plugins: [
    react(),
    {
      name: "ai-trial-artifact-fixture",
      configureServer(server: {
        middlewares: {
          use: (
            handler: (
              req: IncomingMessage,
              res: ServerResponse,
              next: () => void,
            ) => void,
          ) => void;
        };
      }) {
        server.middlewares.use((req, res, next) => {
          if (
            req.url?.startsWith("/api/admin/ai-trials/") &&
            req.url.includes("/artifacts/1")
          ) {
            res.setHeader("Content-Type", "image/png");
            res.end(
              Buffer.from(
                "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a7S8AAAAASUVORK5CYII=",
                "base64",
              ),
            );
            return;
          }
          next();
        });
      },
    },
  ],
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
