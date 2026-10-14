// @vitest-environment node
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, readdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { expect, it } from "vitest";
import { format, resolveConfig } from "prettier";

it("所有 API 都可从 Swagger 重现，禁止手改、遗漏或额外接口文件", async () => {
  const frontend = path.resolve(import.meta.dirname, "../..");
  const temporary = mkdtempSync(path.join(tmpdir(), "glowloom-api-check-"));
  try {
    execFileSync("pnpm", ["run", "api:schema"], {
      cwd: frontend,
      timeout: 120_000,
      stdio: "pipe",
    });
    // Use the API directly: the upstream CLI logs generation errors but exits 0.
    execFileSync(
      process.execPath,
      [
        "-e",
        `
      const { generateService } = require('@umijs/openapi');
      const fs = require('node:fs');
      const config = require('./openapi2ts.config.ts').default;
      const schema = JSON.parse(fs.readFileSync(config.schemaPath, 'utf8'));
      const ids = Object.values(schema.paths).flatMap(item =>
        Object.values(item).flatMap(operation => operation.operationId ? [operation.operationId] : []));
      fs.writeFileSync(process.argv[1] + '/operations.json', JSON.stringify(ids));
      generateService({ ...config, serversPath: process.argv[1] + '/api' }).catch(error => {
        console.error(error);
        process.exitCode = 1;
      });
    `,
        temporary,
      ],
      { cwd: frontend, timeout: 120_000, stdio: "pipe" },
    );
    const generated = path.join(temporary, "api");
    const checkedIn = path.join(frontend, "src/api");
    const files = readdirSync(generated).sort();
    expect(readdirSync(checkedIn).sort()).toEqual(files);
    const actualOperations: string[] = [];
    for (const file of files) {
      const actual = readFileSync(path.join(checkedIn, file), "utf8");
      expect(actual, `${file}: 请运行 pnpm api:generate`).toBe(
        await format(readFileSync(path.join(generated, file), "utf8"), {
          ...(await resolveConfig(path.join(checkedIn, file))),
          parser: "typescript",
        }),
      );
      actualOperations.push(
        ...Array.from(
          actual.matchAll(/export async function (\w+)\(/g),
          (match) => match[1],
        ),
      );
    }
    const expectedOperations = JSON.parse(
      readFileSync(path.join(temporary, "operations.json"), "utf8"),
    );
    expect(actualOperations.sort()).toEqual(expectedOperations.sort());
    expect(new Set(actualOperations).size).toBe(actualOperations.length);
    const sourceFiles = readdirSync(path.join(frontend, "src/components"), {
      recursive: true,
    });
    expect(
      sourceFiles.filter((file) =>
        /(^|[/\\])generated([/\\]|$)/.test(String(file)),
      ),
    ).toEqual([]);
  } finally {
    rmSync(temporary, { recursive: true, force: true });
  }
}, 150_000);
