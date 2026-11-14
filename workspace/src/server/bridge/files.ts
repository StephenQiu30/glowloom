import { open, mkdir, rename, readFile, link, rm } from "node:fs/promises";
import { dirname } from "node:path";
import { randomUUID } from "node:crypto";
export async function atomicWrite(path: string, contents: string | Uint8Array) {
  await mkdir(dirname(path), { recursive: true, mode: 0o700 });
  const temp = path + ".tmp-" + randomUUID();
  const file = await open(temp, "wx", 0o600);
  try {
    await file.writeFile(contents);
    await file.sync();
  } finally {
    await file.close();
  }
  await rename(temp, path);
}
export async function readJson<T>(path: string): Promise<T | null> {
  try {
    return JSON.parse(await readFile(path, "utf8")) as T;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return null;
    throw error;
  }
}
export async function claimJson(path: string, value: unknown) {
  await mkdir(dirname(path), { recursive: true, mode: 0o700 });
  const temp = path + ".claim-" + randomUUID();
  await atomicWrite(temp, JSON.stringify(value));
  try {
    await link(temp, path);
    return true;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "EEXIST") return false;
    throw error;
  } finally {
    await rm(temp, { force: true });
  }
}
