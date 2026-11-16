import { canvasFixtureCache, canvasHostAliases } from "./canvas-host";
import path from "node:path";
import base from "./assets.vite.config";
const config = {
  ...base,
  cacheDir: canvasFixtureCache("insights"),
  resolve: {
    dedupe: ["react", "react-dom", "yjs"],
    alias: {
      ...canvasHostAliases,
      ...base.resolve.alias,
      "next/navigation": path.resolve(
        import.meta.dirname,
        "insights-navigation.ts",
      ),
    },
  },
  server: { host: "127.0.0.1", port: 3134, strictPort: true },
};

export default config;
