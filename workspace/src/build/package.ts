import { cp } from "node:fs/promises";
export async function copyPortableTree(from: string, to: string) {
  await cp(from, to, { recursive: true, verbatimSymlinks: true });
}
