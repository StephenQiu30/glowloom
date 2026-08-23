import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

const projectRoot = resolve(import.meta.dirname, "../..");

describe("create-next-app baseline", () => {
  it("keeps the official TypeScript App Router scaffold contract", () => {
    const packageJson = JSON.parse(
      readFileSync(resolve(projectRoot, "package.json"), "utf8"),
    ) as {
      scripts: Record<string, string>;
      dependencies: Record<string, string>;
    };
    const tsconfig = JSON.parse(
      readFileSync(resolve(projectRoot, "tsconfig.json"), "utf8"),
    ) as {
      compilerOptions: {
        strict: boolean;
        plugins: Array<{ name: string }>;
        paths: Record<string, string[]>;
      };
    };

    expect(packageJson.scripts).toMatchObject({
      dev: "next dev --hostname 127.0.0.1 --port 8123",
      build: "next build",
      start: "next start --hostname 0.0.0.0 --port 8123",
    });
    expect(packageJson.dependencies).toMatchObject({
      next: "16.2.12",
      react: "19.2.4",
      "react-dom": "19.2.4",
    });
    expect(tsconfig.compilerOptions.strict).toBe(true);
    expect(tsconfig.compilerOptions.plugins).toContainEqual({ name: "next" });
    expect(tsconfig.compilerOptions.paths["@/*"]).toEqual(["./src/*"]);
  });
});
