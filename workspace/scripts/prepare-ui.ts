import { cp, mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
const root = resolve(fileURLToPath(new URL("..", import.meta.url)));
const output = resolve(root, ".build/shared-ui");
await mkdir(output, { recursive: true });
for (const name of [
  "button",
  "badge",
  "input",
  "textarea",
  "dialog",
  "separator",
  "alert",
  "card",
  "empty",
  "label",
  "field",
])
  await cp(
    resolve(root, "../frontend/src/components/ui", name + ".tsx"),
    resolve(output, name + ".tsx"),
  );
