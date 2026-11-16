import path from "node:path";
import base from "./canvas.vite.config";
import { canvasFixtureCache } from "./canvas-host";

const config = {
  ...base,
  cacheDir: canvasFixtureCache("canvas-responsive"),
  resolve: {
    ...base.resolve,
    alias: {
      ...base.resolve.alias,
      "@glowloom/canvas/editor/canvas.css": path.resolve(
        import.meta.dirname,
        "../../../canvas/editor/canvas.css",
      ),
    },
  },
  server: {
    host: "127.0.0.1",
    port: 5205,
    strictPort: true,
    fs: { allow: [path.resolve(import.meta.dirname, "../../..")] },
  },
};
export default config;
