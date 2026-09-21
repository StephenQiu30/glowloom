import { execFile } from "node:child_process";
import { createServer } from "node:http";
import { promisify } from "node:util";

import { expect, it } from "vitest";

const execute = promisify(execFile);
const generate = (url: string) =>
  execute(process.execPath, ["--import", "tsx", "openapi2ts.config.ts"], {
    env: { ...process.env, OPENAPI_SCHEMA_URL: url },
    timeout: 10_000,
  });

it("fails without an online schema URL instead of using a stale local schema", async () => {
  await expect(generate("")).rejects.toMatchObject({ code: 1 });
});

it("fails on an HTTP error even if the body looks like an OpenAPI document", async () => {
  const server = createServer((_, response) => {
    response.writeHead(503, { "Content-Type": "application/json" });
    response.end(JSON.stringify({ openapi: "3.0.3", paths: {} }));
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const address = server.address();
    if (!address || typeof address === "string") throw new Error("Missing test server address");
    await expect(generate(`http://127.0.0.1:${address.port}/openapi.json`)).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining("HTTP 503"),
    });
  } finally {
    await new Promise<void>((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    );
  }
});
