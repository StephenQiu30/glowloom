import { readFile, realpath, stat } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { dirname, relative, resolve, sep } from "node:path";
import { unified } from "unified";
import remarkParse from "remark-parse";
import { visit } from "unist-util-visit";
import { toString } from "mdast-util-to-string";
import type { ParsedDocument } from "./types";
import { digest, parseMarkdown } from "./parse";

export type CatalogEntry = {
  id: string;
  path: string;
  visibility: "public" | "internal";
  aliases?: string[];
};
export type Document = ParsedDocument &
  CatalogEntry & {
    source: string;
    revision: string;
    category: string;
    sourcePath: string;
  };
export type Snapshot = {
  documents: Document[];
  contentDigest: string;
  repoCommit: string;
  scope: "public" | "internal";
  workspaceRoot: string;
  preview: boolean;
};
const rootFiles = new Set([
  "AGENTS.md",
  "PROJECT.md",
  "DESIGN.md",
  "BACKLOG.md",
  "README.md",
  "backend/AGENTS.md",
  "frontend/AGENTS.md",
  "agent/AGENTS.md",
  "canvas/AGENTS.md",
  "workspace/README.md",
  "workspace/AGENTS.md",
]);
const isExternal = (href: string) => /^(?:[a-z][a-z\d+.-]*:|\/\/)/i.test(href);
export const documentHref = (id: string, anchor = "") =>
  `/read/${encodeURIComponent(id)}/${anchor ? `#${encodeURIComponent(anchor)}` : ""}`;

export function rewriteMarkdownLinks(
  source: string,
  rewrite: (url: string) => string | null,
  missing: (label: string, href: string) => string = () => "受限内容",
): string {
  const tree = unified().use(remarkParse).parse(source);
  const edits: { start: number; end: number; value: string }[] = [];
  const definitions = new Map<string, string>();
  visit(tree, "definition", (node) => {
    definitions.set(node.identifier.toLowerCase(), node.url);
  });
  visit(tree, (node) => {
    if (node.type !== "linkReference" && node.type !== "imageReference") return;
    const url = definitions.get(node.identifier.toLowerCase());
    if (
      url &&
      rewrite(url) === null &&
      node.position?.start.offset !== undefined &&
      node.position.end.offset !== undefined
    )
      edits.push({
        start: node.position.start.offset,
        end: node.position.end.offset,
        value: missing(toString(node), url),
      });
  });
  visit(tree, (node) => {
    if (
      node.type !== "link" &&
      node.type !== "image" &&
      node.type !== "definition"
    )
      return;
    const url = rewrite(node.url);
    const start = node.position?.start.offset;
    const end = node.position?.end.offset;
    if (start === undefined || end === undefined || url === node.url) return;
    const raw = source.slice(start, end);
    const urlStart =
      node.type === "definition"
        ? raw.indexOf(":") + 1
        : raw.lastIndexOf("](") + 2;
    edits.push({
      start,
      end,
      value:
        url === null
          ? missing(toString(node), node.url)
          : raw.slice(0, urlStart) + raw.slice(urlStart).replace(node.url, url),
    });
  });
  for (const edit of edits.sort((a, b) => b.start - a.start))
    source = source.slice(0, edit.start) + edit.value + source.slice(edit.end);
  return source;
}

export async function loadSnapshot(options: {
  workspaceRoot: string;
  scope: "public" | "internal";
}): Promise<Snapshot> {
  const workspaceRoot = await realpath(options.workspaceRoot);
  const repoRoot = dirname(workspaceRoot);
  const catalog = JSON.parse(
    await readFile(resolve(workspaceRoot, "catalog.json"), "utf8"),
  ) as CatalogEntry[];
  const ids = new Set<string>();
  const paths = new Map<string, CatalogEntry>();
  for (const entry of catalog) {
    if (!entry.id || !/^[\w-]+$/.test(entry.id) || ids.has(entry.id))
      throw new Error(`Duplicate or invalid document identity: ${entry.id}`);
    if (!["public", "internal"].includes(entry.visibility))
      throw new Error("Invalid visibility");
    ids.add(entry.id);
    const path = resolve(workspaceRoot, entry.path);
    const repoPath = relative(repoRoot, path).split(sep).join("/");
    if (!(
      path.startsWith(workspaceRoot + "/content/") ||
      path.startsWith(workspaceRoot + "/skills/") ||
      path.startsWith(workspaceRoot + "/workflows/") ||
      rootFiles.has(repoPath)
    ))
      throw new Error(`Disallowed path: ${entry.path}`);
    if (paths.has(path)) throw new Error(`Duplicate path: ${entry.path}`);
    paths.set(path, entry);
  }
  const visible = catalog.filter(
    (e) => options.scope === "internal" || e.visibility === "public",
  );
  const canRead = (entry: CatalogEntry) =>
    options.scope === "internal" || entry.visibility === "public";
  const documents: Document[] = [];
  for (const entry of visible) {
    const path = resolve(workspaceRoot, entry.path);
    if ((await realpath(path)) !== path)
      throw new Error(`Symlink path escape: ${entry.path}`);
    let source = await readFile(path, "utf8");
    const targetEntry = (href: string) => {
      if (isExternal(href) || href.startsWith("#")) return undefined;
      return paths.get(
        resolve(dirname(path), decodeURIComponent(href.split(/[?#]/)[0])),
      );
    };
    if (options.scope === "public") {
      const publicTree = unified().use(remarkParse).parse(source);
      visit(publicTree, "html", (node) => {
        if (
          !/^\s*(?:<a\s+id=(?:"[^"]+"|'[^']+')\s*>\s*<\/a>\s*)+$/.test(
            node.value,
          )
        )
          throw new Error("public HTML requires explicit review: " + entry.id);
      });
      source = rewriteMarkdownLinks(source, (href) => {
        const target = targetEntry(href);
        return target && !canRead(target) ? null : href;
      });
    }
    const parsed = await parseMarkdown(source, (href) => {
      if (isExternal(href) || href.startsWith("#")) return href;
      const target = targetEntry(href);
      const anchor = decodeURIComponent(href.split("#").slice(1).join("#"));
      if (target)
        return canRead(target)
          ? documentHref(target.id, anchor)
          : "#restricted";
      // Engineering references are explicit references, never arbitrary source downloads.
      const engineeringPath = relative(
        repoRoot,
        resolve(dirname(path), decodeURIComponent(href.split(/[?#]/)[0])),
      )
        .split(sep)
        .join("/");
      return `/engineering/?path=${encodeURIComponent(engineeringPath)}`;
    });
    if (entry.path.endsWith(".csv")) {
      const csv = await parseMarkdown(
        `# ${entry.path.split("/").at(-1)}\n\n\`\`\`csv\n${source}\n\`\`\``,
      );
      Object.assign(parsed, csv);
    }
    documents.push({
      ...entry,
      ...parsed,
      source,
      revision: digest(source),
      category: entry.path.startsWith("content/")
        ? entry.path.split("/")[1].replace(/\.md$/, "")
        : entry.path.startsWith("skills/") ||
            entry.path.startsWith("workflows/")
          ? entry.path.split("/")[0]
          : "工程规范",
      sourcePath: relative(repoRoot, path).split(sep).join("/"),
    });
  }
  let repoCommit = "unversioned";
  let preview = true;
  try {
    repoCommit = execFileSync("git", ["rev-parse", "HEAD"], {
      cwd: repoRoot,
      stdio: ["ignore", "pipe", "ignore"],
    })
      .toString()
      .trim();
    preview = !!execFileSync(
      "git",
      ["status", "--porcelain", "--untracked-files=normal"],
      { cwd: repoRoot, stdio: ["ignore", "pipe", "ignore"] },
    )
      .toString()
      .trim();
  } catch {
    /* Filesystem fixtures and exported source trees have no Git metadata. */
  }
  const snapshot = {
    documents,
    contentDigest: digest(
      JSON.stringify(
        documents.map((d) => [
          d.id,
          d.path,
          d.category,
          d.revision,
          d.visibility,
          d.aliases,
        ]),
      ),
    ),
    repoCommit,
    scope: options.scope,
    preview,
  } as Snapshot;
  Object.defineProperty(snapshot, "workspaceRoot", {
    value: workspaceRoot,
    enumerable: false,
  });
  return snapshot;
}

export async function validateLinks(
  snapshot: Snapshot,
): Promise<{ documentId: string; target: string; reason: string }[]> {
  const errors: { documentId: string; target: string; reason: string }[] = [];
  const docs = new Map(
    snapshot.documents.map((d) => [resolve(snapshot.workspaceRoot, d.path), d]),
  );
  for (const doc of snapshot.documents) {
    for (const target of doc.links) {
      if (isExternal(target)) continue;
      const [pathname, anchor] = target.split("#");
      if (decodeURIComponent(pathname).startsWith("/")) {
        errors.push({
          documentId: doc.id,
          target,
          reason: "nonportable_local_link",
        });
        continue;
      }
      const path = pathname
        ? resolve(
            dirname(resolve(snapshot.workspaceRoot, doc.path)),
            decodeURIComponent(pathname.split("?")[0]),
          )
        : resolve(snapshot.workspaceRoot, doc.path);
      let exists = false;
      try {
        const info = await stat(path);
        exists = info.isFile() || info.isDirectory();
      } catch {
        /* Report broken links with their source. */
      }
      if (!exists)
        errors.push({ documentId: doc.id, target, reason: "file_missing" });
      else if (anchor && docs.has(path)) {
        const html = docs.get(path)!.html;
        const id = decodeURIComponent(anchor);
        if (!html.includes(`id="${id.replaceAll('"', "&quot;")}"`))
          errors.push({ documentId: doc.id, target, reason: "anchor_missing" });
      }
    }
  }
  return errors;
}
