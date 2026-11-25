import { readFileSync } from "node:fs";
import path from "node:path";
import type { Alias } from "vite";

const frontendRoot = path.resolve(import.meta.dirname, "../..");
const srcRoot = path.join(frontendRoot, "src");
const publicRoot = path.join(frontendRoot, "public");

export const canvasHostAliases = {
  "@": srcRoot,
  "@public": publicRoot,
};

export function canvasFixtureCache(name: string) {
  return path.resolve(
    frontendRoot,
    "node_modules/.cache/browser-fixtures",
    name,
  );
}

/** Apply fixture overrides to frontend application and canvas editor imports. */
export function canvasFixtureAliases(aliases: Alias[]) {
  return [
    ...aliases,
    { find: "@public", replacement: publicRoot },
    { find: "@", replacement: srcRoot },
  ];
}

export const canvasFixtureOptimizeDeps = {
  exclude: [
    "@glowloom/canvas",
    "@tiptap/core",
    "@tiptap/pm",
    "yjs",
    "prosemirror-model",
    "prosemirror-state",
    "prosemirror-view",
    "prosemirror-transform",
  ],
  include: [
    "react-redux",
    "swr",
    "swr/infinite",
    "file-saver",
    "cn",
    "@tiptap/react > use-sync-external-store/shim/index.js",
    "@tiptap/react > use-sync-external-store/shim/with-selector.js",
  ],
};

/** Keep public JSON metadata in its existing single source, as Next.js does. */
export function canvasHostPublicJson() {
  const prefix = "\0canvas-host-public-json:";
  return {
    name: "canvas-host-public-json",
    enforce: "pre" as const,
    resolveId(source: string) {
      const file = source.startsWith("@public/")
        ? path.resolve(publicRoot, source.slice("@public/".length))
        : source;
      if (!path.isAbsolute(file) || !file.endsWith(".json")) return;
      const relative = path.relative(publicRoot, file);
      if (relative.startsWith("..") || path.isAbsolute(relative)) return;
      return prefix + file + ".js";
    },
    load(id: string) {
      if (!id.startsWith(prefix)) return;
      return `export default ${JSON.stringify(JSON.parse(readFileSync(id.slice(prefix.length, -3), "utf8")))};`;
    },
  };
}
