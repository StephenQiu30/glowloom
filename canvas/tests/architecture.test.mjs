import test from "node:test";
import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { loadConfig } from "../dist/config.js";
import {
  ConcurrencyGate,
  classifyRuntimeError,
} from "../dist/transport/concurrency.js";

const sourceRoot = fileURLToPath(new URL("../src/", import.meta.url));

async function listSourceFiles(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await listSourceFiles(full)));
    } else if (entry.isFile() && entry.name.endsWith(".ts")) {
      files.push(full);
    }
  }
  return files;
}

test("canvas/src enforces Google TypeScript flat two-level package layout and naming", async () => {
  const files = await listSourceFiles(sourceRoot);
  const violations = [];
  for (const file of files) {
    const rel = relative(sourceRoot, file).replaceAll("\\", "/");
    const segments = rel.split("/");
    if (segments.length > 2) {
      violations.push(`${rel}: exceeds two-level package depth`);
    }
    const filename = segments.at(-1) ?? "";
    if (filename === "index.ts") {
      violations.push(`${rel}: handwritten barrel index.ts is forbidden`);
    }
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*\.ts$/.test(filename)) {
      violations.push(`${rel}: filename must be lowercase kebab-case`);
    }
    const body = await readFile(file, "utf8");
    if (/^\s*export\s+default\b/m.test(body)) {
      violations.push(`${rel}: export default is forbidden by Google TS`);
    }
    if (/(?:from|import\s*\()\s*['"]\.\.\/\.\.\/(?!types\/)/.test(body)) {
      violations.push(`${rel}: deep ../../ relative import is forbidden`);
    }
  }
  assert.deepEqual(violations, []);
});

test("canvas config validation and shared HTTP/gRPC ConcurrencyGate enforce limits", () => {
  assert.throws(() => loadConfig({}), /RUNTIME_TOKEN_REQUIRED/);
  assert.throws(
    () =>
      loadConfig({
        GLOWLOOM_COLLAB_RUNTIME_TOKEN: "a".repeat(32),
        PORT: "not-a-port",
      }),
    /INVALID_PORT/,
  );
  const cfg = loadConfig({
    GLOWLOOM_COLLAB_RUNTIME_TOKEN: "a".repeat(32),
    PORT: "4400",
    GLOWLOOM_CANVAS_MAX_CONCURRENT: "6",
  });
  assert.equal(cfg.port, 4400);
  assert.equal(cfg.maxConcurrent, 6);

  const gate = new ConcurrencyGate(2);
  const r1 = gate.tryAcquire();
  const r2 = gate.tryAcquire();
  assert.ok(r1);
  assert.ok(r2);
  assert.equal(gate.tryAcquire(), null);
  r1();
  r1(); // idempotent release
  assert.equal(gate.inFlight, 1);
  r2();
  assert.equal(gate.inFlight, 0);

  assert.deepEqual(
    classifyRuntimeError({ code: "BAD_NODE", definitive: true }),
    { code: "BAD_NODE", definitive: true },
  );
  assert.deepEqual(classifyRuntimeError(new Error("boom")), {
    code: "RUNTIME_UNAVAILABLE",
    definitive: false,
  });
});
