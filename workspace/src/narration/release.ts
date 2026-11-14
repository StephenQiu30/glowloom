import { readFile, realpath } from "node:fs/promises";
import { resolve, dirname } from "node:path";
import { digest } from "../content/parse";
import { validateManifest, type NarrationManifest } from "./manifest";
export async function freezeNarration(
  root: string,
  documents: {
    id: string;
    revision: string;
    blocks: { id: string; text: string }[];
  }[],
  scope: string,
) {
  const items: { manifest: NarrationManifest; path: string }[] = [];
  let canonical: string;
  try {
    canonical = await realpath(root);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT")
      return { items, digest: digest("[]") };
    throw error;
  }
  for (const doc of documents) {
    const path = resolve(canonical, doc.id, "manifest.json");
    let raw: string;
    try {
      raw = await readFile(path, "utf8");
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === "ENOENT") continue;
      throw error;
    }
    if ((await realpath(path)) !== path)
      throw new Error("Audio symlink escape");
    const manifest = JSON.parse(raw) as NarrationManifest;
    if (manifest.scope !== scope) continue;
    validateManifest(manifest, doc, scope);
    for (const segment of manifest.segments) {
      const file = resolve(dirname(path), segment.src);
      if (
        (await realpath(file)) !== file ||
        digest(await readFile(file)) !== segment.sha256
      )
        throw new Error("Audio digest or path mismatch");
    }
    items.push({ manifest, path });
  }
  return {
    items,
    digest: digest(JSON.stringify(items.map((i) => i.manifest))),
  };
}
