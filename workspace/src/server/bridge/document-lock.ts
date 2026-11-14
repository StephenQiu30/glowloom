import { readFile, rm } from "node:fs/promises";
import { randomUUID } from "node:crypto";
import { claimJson } from "./files";
const active = new Set<string>();
export async function lockDocument(path: string) {
  if (active.has(path)) throw new Error("document_busy");
  active.add(path);
  const owner = { pid: process.pid, nonce: randomUUID() };
  try {
    let claimed = await claimJson(path, owner);
    if (!claimed) {
      let prior;
      try {
        prior = JSON.parse(await readFile(path, "utf8")) as {
          pid: number;
          nonce?: string;
        };
      } catch {
        throw new Error("document_busy_unknown_owner");
      }
      if (!Number.isSafeInteger(prior.pid) || prior.pid <= 1)
        throw new Error("document_busy_unknown_owner");
      let dead = false;
      try {
        process.kill(prior.pid, 0);
      } catch (error) {
        if ((error as NodeJS.ErrnoException).code === "ESRCH") dead = true;
        else throw new Error("document_busy");
      }
      if (!dead) throw new Error("document_busy");
      await rm(path);
      claimed = await claimJson(path, owner);
    }
    if (!claimed) throw new Error("document_busy");
    return async () => {
      try {
        const current = JSON.parse(await readFile(path, "utf8"));
        if (current.nonce === owner.nonce) await rm(path);
      } finally {
        active.delete(path);
      }
    };
  } catch (error) {
    active.delete(path);
    throw error;
  }
}
