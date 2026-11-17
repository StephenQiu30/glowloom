import path from "node:path";
import { imageConfigDefault } from "next/dist/shared/lib/image-config";
import base from "./assets.vite.config";
import { canvasFixtureCache } from "./canvas-host";
const config = {
  ...base,
  define: {
    ...base.define,
    "process.env.__NEXT_IMAGE_OPTS": JSON.stringify({
      ...imageConfigDefault,
      unoptimized: true,
    }),
  },
  cacheDir: canvasFixtureCache("notifications"),
  resolve: {
    ...base.resolve,
    alias: {
      ...base.resolve.alias,
      "next/navigation": path.resolve(
        import.meta.dirname,
        "notifications-navigation.ts",
      ),
      "next/dynamic": path.resolve(
        import.meta.dirname,
        "ai-presets-next-dynamic.tsx",
      ),
    },
  },
  server: { host: "127.0.0.1", port: 3136, strictPort: true },
};

export default config;
