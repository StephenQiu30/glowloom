import { readFileSync } from "node:fs";
import path from "node:path";
import type { Alias } from "vite";
import { canvasHostAliases } from "../../canvas-host";

export { canvasHostAliases };

export function canvasFixtureCache(name: string) {
  return path.resolve(
    import.meta.dirname,
    "../../node_modules/.cache/browser-fixtures",
    name,
  );
}

/** Apply fixture overrides to both application and editor host imports. */
export function canvasFixtureAliases(aliases: Alias[]) {
  return [
    ...aliases.flatMap((alias) =>
      typeof alias.find === "string" && alias.find.startsWith("@/")
        ? [
            { ...alias, find: "@glowloom/canvas-host/" + alias.find.slice(2) },
            alias,
          ]
        : [alias],
    ),
    ...Object.entries(canvasHostAliases).map(([find, replacement]) => ({
      find,
      replacement,
    })),
  ];
}

// Linked browser sources must use Vite's CJS conversion for the Redux bridge.
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
    "@glowloom/canvas > swr",
    "swr/infinite",
    "@glowloom/canvas > file-saver",
    "@glowloom/canvas > cn",
    "@tiptap/react > use-sync-external-store/shim/index.js",
    "@tiptap/react > use-sync-external-store/shim/with-selector.js",
  ],
};

/** Keep public JSON metadata in its existing single source, as Next.js does. */
export function canvasHostPublicJson() {
  const publicRoot = canvasHostAliases["@glowloom/canvas-host-public"];
  const prefix = "\0canvas-host-public-json:";
  return {
    name: "canvas-host-public-json",
    enforce: "pre" as const,
    resolveId(source: string) {
      const file = source.startsWith("@glowloom/canvas-host-public/")
        ? path.resolve(
            publicRoot,
            source.slice("@glowloom/canvas-host-public/".length),
          )
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
