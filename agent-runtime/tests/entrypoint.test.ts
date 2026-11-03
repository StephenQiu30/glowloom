import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { expect, it } from "vitest";
const cwd = fileURLToPath(new URL("..", import.meta.url));
it("loads the packaged Host without starting it as an import", () => {
  const result = spawnSync(
    process.execPath,
    ["--input-type=module", "-e", 'await import("./dist/host.js")'],
    { cwd, env: {}, encoding: "utf8", timeout: 5000 },
  );
  expect(result.status).toBe(0);
  expect(result.stderr).toBe("");
});
it("refuses startup without explicit internal configuration and never discovers ambient provider keys", () => {
  const result = spawnSync(process.execPath, ["dist/host.js"], {
    cwd,
    env: { OPENAI_API_KEY: "unused-provider-fixture" },
    encoding: "utf8",
    timeout: 5000,
  });
  expect(result.status).toBe(1);
  expect(result.stderr).toContain("agent runtime startup failed");
  expect(result.stderr).not.toContain("unused-provider-fixture");
});
