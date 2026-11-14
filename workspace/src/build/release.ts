import {
  readdir,
  readFile,
  writeFile,
  readlink,
  realpath,
  rename,
} from "node:fs/promises";
import { resolve, relative } from "node:path";
import { randomUUID } from "node:crypto";
import { digest } from "../content/parse";
type Asset = { path: string; sha256: string; kind: "file" | "symlink" };
async function inventory(root: string): Promise<Asset[]> {
  const result: Asset[] = [];
  const canonical = await realpath(root);
  async function walk(dir: string) {
    for (const entry of await readdir(dir, { withFileTypes: true })) {
      const file = resolve(dir, entry.name);
      const path = relative(root, file);
      if (path === "release.json") continue;
      if (entry.isDirectory()) await walk(file);
      else if (entry.isSymbolicLink()) {
        const target = await realpath(file);
        if (!target.startsWith(canonical + "/"))
          throw new Error("release_integrity_symlink_escape");
        result.push({
          path,
          kind: "symlink",
          sha256: digest(await readlink(file)),
        });
      } else if (entry.isFile())
        result.push({
          path,
          kind: "file",
          sha256: digest(await readFile(file)),
        });
      else throw new Error("release_integrity_special_file");
    }
  }
  await walk(root);
  return result.sort((a, b) => a.path.localeCompare(b.path));
}
export async function sealRelease(
  root: string,
  metadata: Record<string, unknown>,
) {
  await writeFile(
    resolve(root, "release.json"),
    JSON.stringify({ ...metadata, assets: await inventory(root) }, null, 2),
  );
}
export async function verifyRelease(root: string, mode: string) {
  const metadata = JSON.parse(
    await readFile(resolve(root, "release.json"), "utf8"),
  );
  if (metadata.mode !== mode) throw new Error("release_mode_mismatch");
  if (JSON.stringify(metadata.assets) !== JSON.stringify(await inventory(root)))
    throw new Error("release_integrity_failed");
  return metadata;
}
export async function selectRelease(root: string, id: string, mode: string) {
  if (!/^[a-zA-Z0-9_-]+$/.test(id)) throw new Error("invalid_release");
  const metadata = await verifyRelease(resolve(root, id), mode);
  if (metadata.releaseId !== id) throw new Error("release_integrity_identity");
  const temp = resolve(root, "current.json." + randomUUID());
  await writeFile(temp, JSON.stringify({ releaseId: id }));
  await rename(temp, resolve(root, "current.json"));
}
