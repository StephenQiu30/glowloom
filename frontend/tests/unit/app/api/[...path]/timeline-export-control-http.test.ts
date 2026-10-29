// @vitest-environment node
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { afterAll, beforeAll, expect, it, vi } from "vitest";
import { cancelMediaExport, retryMediaExport } from "@/api/mediaExports";
import { POST } from "@/app/api/[...path]/route";

const jobID = "11111111-1111-4111-8111-111111111111";
const projectID = "22222222-2222-4222-8222-222222222222";
const key = "33333333-3333-4333-8333-333333333333";
const calls: { path: string; type?: string; key?: string; body: unknown }[] =
  [];
let bff: Server;
let upstream: Server;
let baseURL: string;
const listen = async (server: Server) => {
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  return `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
};
beforeAll(async () => {
  upstream = createServer(async (req, res) => {
    const chunks: Buffer[] = [];
    for await (const chunk of req) chunks.push(Buffer.from(chunk));
    calls.push({
      path: req.url!,
      type: req.headers["content-type"],
      key: req.headers["idempotency-key"] as string,
      body: JSON.parse(Buffer.concat(chunks).toString()),
    });
    res.writeHead(202, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ status: "cancel_requested" }));
  });
  vi.stubEnv("GLOWLOOM_BACKEND_ORIGIN", await listen(upstream));
  bff = createServer(async (req, res) => {
    const chunks: Buffer[] = [];
    for await (const chunk of req) chunks.push(Buffer.from(chunk));
    const url = new URL(req.url!, "http://localhost");
    const headers = new Headers();
    for (const [name, value] of Object.entries(req.headers))
      if (value)
        headers.set(name, Array.isArray(value) ? value.join(",") : value);
    const result = await POST(
      new Request(url, {
        method: "POST",
        headers,
        body: Buffer.concat(chunks),
      }),
      { params: Promise.resolve({ path: url.pathname.slice(5).split("/") }) },
    );
    res.writeHead(result.status, Object.fromEntries(result.headers));
    res.end(Buffer.from(await result.arrayBuffer()));
  });
  baseURL = await listen(bff);
});
afterAll(async () => {
  vi.unstubAllEnvs();
  for (const server of [bff, upstream]) {
    server.closeAllConnections();
    await new Promise<void>((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    );
  }
});

it("真实生成客户端经HTTP和BFF保留取消/重试正文及幂等键，拒绝误传GET项目query", async () => {
  const body = { project_id: projectID, revision: 9 };
  const options = { baseURL, headers: { "Idempotency-Key": key } };
  // Reproduce the observed UI defect: structural typing admits an extra GET field.
  const wrongParams = { job_id: jobID, project_id: projectID };
  await expect(
    cancelMediaExport(wrongParams, body, options),
  ).rejects.toMatchObject({
    status: 422,
  });
  expect(calls).toHaveLength(0);
  for (const [action, client] of [
    ["cancel", cancelMediaExport],
    ["retry", retryMediaExport],
  ] as const) {
    await expect(client({ job_id: jobID }, body, options)).resolves.toEqual({
      status: "cancel_requested",
    });
    expect(calls.at(-1)).toEqual({
      path: `/api/media-exports/${jobID}/${action}`,
      type: "application/json",
      key,
      body,
    });
  }
});
