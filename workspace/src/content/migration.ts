import { mkdir, readFile, rename, stat, writeFile } from "node:fs/promises";
import { dirname, relative, resolve } from "node:path";
import { digest } from "./parse";
import { rewriteMarkdownLinks } from "./catalog";

export async function migrateKnowledge(
  root: string,
  files: string[],
  entrypoints: string[],
) {
  const mapping = new Map(
    files.map((p) => [
      resolve(root, p),
      resolve(root, "workspace/content", relative("docs", p)),
    ]),
  );
  const rows: { from: string; to: string; before: string; after: string }[] =
    [];
  const originals = new Map<string, Buffer>();
  for (const [from, to] of mapping) {
    if (
      !from.startsWith(resolve(root, "docs") + "/") ||
      !to.startsWith(resolve(root, "workspace/content") + "/")
    )
      throw new Error("Invalid migration path");
    try {
      await stat(to);
      throw new Error(`Target already exists: ${to}`);
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
    }
    originals.set(from, await readFile(from));
  }
  for (const name of entrypoints)
    originals.set(resolve(root, name), await readFile(resolve(root, name)));
  const rewritten = new Map<string, Buffer>();
  for (const [from, bytes] of originals) {
    const to = mapping.get(from) ?? from;
    let next = bytes;
    if (from.endsWith(".md")) {
      next = Buffer.from(
        rewriteMarkdownLinks(bytes.toString(), (href) => {
          if (/^(?:[a-z][a-z\d+.-]*:|\/\/|#|\/)/i.test(href)) return href;
          const hashAt = href.indexOf("#");
          const queryAt = href.indexOf("?");
          const cut = Math.min(
            ...[hashAt, queryAt, href.length].filter((i) => i >= 0),
          );
          const old = href.slice(0, cut);
          const target = resolve(dirname(from), decodeURIComponent(old));
          const moved = mapping.get(target) ?? target;
          if (relative(dirname(from), target) === relative(dirname(to), moved))
            return href;
          const replacement = relative(dirname(to), moved) + href.slice(cut);
          return old.includes("%")
            ? encodeURI(relative(dirname(to), moved)) + href.slice(cut)
            : replacement;
        }),
      );
    }
    rewritten.set(from, next);
  }
  for (const [from, bytes] of originals)
    if (!bytes.equals(await readFile(from)))
      throw new Error(`Concurrent edit: ${from}`);
  for (const [from, bytes] of originals) {
    const to = mapping.get(from) ?? from;
    const next = rewritten.get(from)!;
    if (mapping.has(from)) {
      await mkdir(dirname(to), { recursive: true });
      await rename(from, to);
    }
    if (!next.equals(bytes)) await writeFile(to, next);
    if (mapping.has(from))
      rows.push({
        from: relative(root, from),
        to: relative(root, to),
        before: digest(bytes),
        after: digest(next),
      });
  }
  return rows;
}
