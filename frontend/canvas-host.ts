import path from "node:path";

/** Application-owned capabilities supplied to the browser editor at build time. */
export const canvasHostAliases = {
  "@glowloom/canvas-host-public": path.join(import.meta.dirname, "public"),
  "@glowloom/canvas-host": path.join(import.meta.dirname, "src"),
};
