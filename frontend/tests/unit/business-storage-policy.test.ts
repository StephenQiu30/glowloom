import { readFileSync, readdirSync } from "node:fs";
import { join, relative } from "node:path";
import { expect, it } from "vitest";

const files = (path: string): string[] =>
  readdirSync(path, { withFileTypes: true }).flatMap((entry) =>
    entry.isDirectory()
      ? files(join(path, entry.name))
      : /\.[cm]?[jt]sx?$/.test(entry.name)
        ? [join(path, entry.name)]
        : [],
  );
it("all application business modules avoid browser durable storage; only the view preference whitelist remains", () => {
  const allowed = new Set([
    "components/canvas/hooks/use-canvas-view-state.ts",
    "components/canvas/director/director-view-preferences.ts",
  ]);
  const violations = files("src")
    .filter((path) => !allowed.has(relative("src", path)))
    .filter((path) =>
      /\b(localStorage|sessionStorage|indexedDB)\s*[?.]|\b(caches\.(open|match|put)|serviceWorker\.register)/.test(
        readFileSync(path, "utf8"),
      ),
    );
  expect(violations).toEqual([]);
});
