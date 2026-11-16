import { canvasFixtureCache } from "./canvas-host";
import base from "./assets.vite.config";
const config = {
  ...base,
  cacheDir: canvasFixtureCache("composer-toolbar"),
  server: { host: "127.0.0.1", port: 5204, strictPort: true },
};
export default config;
