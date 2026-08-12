import { existsSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

const projectRoot = resolve(import.meta.dirname, "../..");

describe("semantic source names", () => {
  it("does not reintroduce generic route groups without a layout boundary", () => {
    expect(existsSync(resolve(projectRoot, "src/app/(main)"))).toBe(false);
    expect(existsSync(resolve(projectRoot, "src/app/(routes)"))).toBe(false);
  });

  it("uses purpose-specific names for shared frontend infrastructure", () => {
    for (const genericName of ["utils.ts", "store.ts", "app-api.ts"]) {
      expect(existsSync(resolve(projectRoot, "src/lib", genericName))).toBe(false);
    }

    for (const semanticName of [
      "request.ts",
      "auth-session.ts",
      "class-names.ts",
      "redux-store.ts",
      "server-state.ts",
    ]) {
      expect(existsSync(resolve(projectRoot, "src/lib", semanticName))).toBe(true);
    }
  });
});
