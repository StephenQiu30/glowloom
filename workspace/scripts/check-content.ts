import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
import { loadSnapshot, validateLinks } from "../src/content/catalog";

const root = resolve(fileURLToPath(new URL("..", import.meta.url)));
const scope = process.argv.includes("--public") ? "public" : "internal";
const snapshot = await loadSnapshot({ workspaceRoot: root, scope });
const errors = await validateLinks(snapshot);
if (errors.length) {
  console.error(JSON.stringify(errors, null, 2));
  process.exitCode = 1;
} else
  console.log(
    `${snapshot.documents.length} documents; ${snapshot.contentDigest}; links valid (${scope})`,
  );
