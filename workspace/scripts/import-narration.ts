import {
  readFile,
  realpath,
  mkdir,
  cp,
  stat,
  rename,
  rm,
} from "node:fs/promises";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import { loadSnapshot } from "../src/content/catalog";
import { digest } from "../src/content/parse";
import {
  validateManifest,
  type NarrationManifest,
} from "../src/narration/manifest";
import { atomicWrite } from "../src/server/bridge/files";
const workspace = resolve(fileURLToPath(new URL("..", import.meta.url)));
if (!process.argv[2])
  throw new Error(
    "Usage: pnpm import:narration /absolute/path/manifest.json --authorized",
  );
if (!process.argv.includes("--authorized"))
  throw new Error(
    "Confirm rights to the audio and voice with --authorized; no synthesis or voice upload is performed",
  );
const input = await realpath(resolve(process.argv[2]));
const manifest = JSON.parse(await readFile(input, "utf8")) as NarrationManifest;
const snapshot = await loadSnapshot({
  workspaceRoot: workspace,
  scope: "internal",
});
const doc = snapshot.documents.find((d) => d.id === manifest.documentId);
if (!doc) throw new Error("Unknown document");
validateManifest(manifest, doc);
const final = resolve(workspace, ".local/narration", doc.id);
const output = final + ".import-" + process.pid;
await mkdir(output, { recursive: true, mode: 0o700 });
for (const segment of manifest.segments) {
  const path = resolve(dirname(input), segment.src);
  if ((await realpath(path)) !== path) throw new Error("Audio path escape");
  if ((await stat(path)).size > 100_000_000) throw new Error("Audio too large");
  const bytes = await readFile(path);
  if (bytes.byteLength > 100_000_000 || digest(bytes) !== segment.sha256)
    throw new Error("Audio size or digest mismatch");
  const duration =
    Number(
      execFileSync(
        "ffprobe",
        [
          "-protocol_whitelist",
          "file,pipe",
          "-v",
          "error",
          "-show_entries",
          "format=duration",
          "-of",
          "default=noprint_wrappers=1:nokey=1",
          path,
        ],
        { encoding: "utf8" },
      ).trim(),
    ) * 1000;
  if (
    !Number.isFinite(duration) ||
    Math.abs(duration - segment.durationMs) > 150
  )
    throw new Error("Declared duration does not match actual audio");
  await mkdir(dirname(resolve(output, segment.src)), { recursive: true });
  await cp(path, resolve(output, segment.src));
}
await atomicWrite(
  resolve(output, "manifest.json"),
  JSON.stringify(manifest, null, 2),
);
const previous = final + ".previous-" + process.pid;
try {
  await rename(final, previous);
} catch (error) {
  if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
}
try {
  await rename(output, final);
} catch (error) {
  await rename(previous, final).catch(() => {});
  throw error;
}
await rm(previous, { recursive: true, force: true });
console.log(
  "Imported authorized existing narration. Rebuild the matching release to publish it; no TTS called.",
);
