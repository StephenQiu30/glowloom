/** Source-component acceptance only. Never starts or contacts a product backend. */
import { createRequire } from "node:module";
import { fileURLToPath, pathToFileURL } from "node:url";
const require = createRequire(import.meta.url);
const vitestRequire = createRequire(require.resolve("vitest/package.json"));
const { createServer } = await import(pathToFileURL(vitestRequire.resolve("vite")));
const server = await createServer({ configFile: fileURLToPath(new URL("./assets.vite.config.ts", import.meta.url)) });
await server.listen(); server.printUrls();
