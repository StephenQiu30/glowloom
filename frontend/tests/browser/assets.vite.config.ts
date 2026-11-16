import {
  canvasFixtureCache,
  canvasHostAliases,
  canvasHostPublicJson,
  canvasFixtureOptimizeDeps,
} from "./canvas-host";
import path from "node:path";
import react from "@vitejs/plugin-react";
const config = {
  cacheDir: canvasFixtureCache("assets"),
  root: path.resolve(import.meta.dirname, "../.."),
  optimizeDeps: canvasFixtureOptimizeDeps,
  plugins: [canvasHostPublicJson(), react()],
  define: { "process.env": "{}" },
  resolve: {
    dedupe: ["react", "react-dom", "yjs"],
    alias: {
      ...canvasHostAliases,
      "@": path.resolve(import.meta.dirname, "../../src"),
      "next/navigation": path.resolve(
        import.meta.dirname,
        "assets-navigation.ts",
      ),
    },
  },
  server: { host: "127.0.0.1", port: 5196, strictPort: true },
};
export default config;
