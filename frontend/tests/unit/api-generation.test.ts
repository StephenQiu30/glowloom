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
    expect(actualOperations).toEqual(
      expect.arrayContaining(["refreshSession", "upgradeSession"]),
    );
    const schema = JSON.parse(
      readFileSync(
        path.join(frontend, "node_modules/.cache/openapi/swagger.json"),
        "utf8",
      ),
    );
    const canvasDocument =
      schema.definitions[
        "github_com_StephenQiu30_glowloom-server_backend_internal_canvas_application.DocumentDTO"
      ];
    expect(canvasDocument.properties.revision.type).toBe("string");
    expect(canvasDocument.properties.read_only_reason["x-nullable"]).toBe(true);
    expect(canvasDocument.properties.schema_version.type).toBe("integer");
    const canvasNode =
      schema.definitions[
        "github_com_StephenQiu30_glowloom-server_backend_internal_canvas_application.NodeDTO"
      ];
    expect(canvasNode.properties.content_version.type).toBe("string");
    expect(canvasNode.properties.parent_id["x-nullable"]).toBe(true);
    for (const action of ["refresh", "upgrade"]) {
      const operation = schema.paths[`/api/session/${action}`].post;
      expect(operation.operationId).toBe(`${action}Session`);
      expect(operation.parameters ?? []).not.toEqual(
        expect.arrayContaining([expect.objectContaining({ in: "body" })]),
      );
    }
    const sessionProperties =
      schema.definitions["internal_identity_adapter_http.EmailSessionView"]
        .properties;
    expect(Object.keys(sessionProperties)).toEqual(
      expect.arrayContaining([
        "access_expires_at",
        "refresh_after_at",
        "server_time",
        "session_protocol",
        "upgrade_available",
        "idle_expires_at",
        "absolute_expires_at",
      ]),
    );
    expect(sessionProperties).not.toHaveProperty("access_token");
    expect(sessionProperties).not.toHaveProperty("refresh_token");

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
