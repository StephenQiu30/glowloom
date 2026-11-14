import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { selectRelease } from "../src/build/release";
const mode = process.argv[2],
  id = process.argv[3];
if (!["static", "online"].includes(mode) || !id)
  throw new Error("Usage: pnpm release:select static|online <release-id>");
await selectRelease(
  resolve(fileURLToPath(new URL("..", import.meta.url)), "dist", mode),
  id,
  mode,
);
console.log(
  `Selected ${mode} release ${id}; restart its reader/runtime to use it.`,
);
