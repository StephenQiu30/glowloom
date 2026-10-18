/** Run with node tests/browser/serve-ai-presets.mjs from frontend/. */
import { createRequire } from "node:module";
import { fileURLToPath, pathToFileURL } from "node:url";

const require = createRequire(import.meta.url);
// Vite is the pinned dependency already supplied by Vitest; add no second toolchain.
const vitestRequire = createRequire(require.resolve("vitest/package.json"));
const { createServer } = await import(
  pathToFileURL(vitestRequire.resolve("vite"))
);
const server = await createServer({
  configFile: fileURLToPath(
    new URL("./ai-presets.vite.config.ts", import.meta.url),
  ),
});
await server.listen();
server.printUrls();
