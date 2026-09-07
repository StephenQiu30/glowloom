import { readdirSync } from "node:fs";
import { resolve } from "node:path";
import { ESLint } from "eslint";
import { describe, expect, it } from "vitest";

const projectRoot = resolve(import.meta.dirname, "../..");

describe("API generation boundary", () => {
  it("lints handwritten transport and exempts only generated declarations", async () => {
    const eslint = new ESLint({ cwd: projectRoot });
    const generated = new Set(["schema.d.ts", "typings.d.ts"]);
    const files = readdirSync(resolve(projectRoot, "src/api"));

    for (const file of files.filter((name) => name.endsWith(".ts"))) {
      expect(
        await eslint.isPathIgnored(resolve(projectRoot, "src/api", file)),
        file,
      ).toBe(generated.has(file));
    }
  });
});
