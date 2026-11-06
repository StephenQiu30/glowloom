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
it("business modules avoid browser durable storage outside approved view, search history and recovery stores", () => {
  const allowed = new Set([
    "components/canvas/hooks/use-canvas-view-state.ts",
    "components/canvas/director/director-view-preferences.ts",
    "components/canvas/timeline/timeline-view-preferences.ts",
    // PRD14 AC-CJC-21/23 explicitly requires the canvas pending-recovery journal.
    "components/canvas/lib/canvas-yjs-pending-journal.ts",
    // Design28 §7.9 requires account-isolated local search history and clearing.
    "components/layout/explore-search.tsx",
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
