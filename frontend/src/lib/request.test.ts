// @vitest-environment node
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import axios from "axios";
import { afterAll, beforeAll, expect, it } from "vitest";
import {
  downloadLibraryMediaAsset,
  uploadPersonalMediaAsset,
} from "@/api/media";
import { request, RequestError } from "./request";

let server: Server;
let baseURL: string;
beforeAll(async () => {
  server = createServer(async (incoming, response) => {
    const chunks: Buffer[] = [];
    for await (const chunk of incoming) chunks.push(Buffer.from(chunk));
    const path = incoming.url ?? "";
    if (path === "/slow") return;
    if (path === "/empty") {
      response.writeHead(204).end();
      return;
    }
    if (path === "/problem") {
      response.writeHead(409, { "Content-Type": "application/json" });
      response.end(
        JSON.stringify({ code: "revision_conflict", meta: { revision: 2 } }),
      );
      return;
    }
    if (path.includes("/download")) {
      if (path.includes("denied")) {
        response.writeHead(403, { "Content-Type": "application/json" });
        response.end(JSON.stringify({ code: "forbidden" }));
      } else {
        response.writeHead(200, { "Content-Type": "application/octet-stream" });
        response.end(Buffer.from([0, 255, 128, 42]));
      }
      return;
    }
    response.setHeader("Content-Type", "application/json");
    response.end(
      JSON.stringify({
        url: path,
        method: incoming.method,
        headers: incoming.headers,
        body: Buffer.concat(chunks).toString(),
      }),
    );
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  baseURL = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
});
afterAll(async () => {
  server.closeAllConnections();
  await new Promise<void>((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
});

type Echo = {
  url: string;
  method: string;
  headers: Record<string, string>;
  body: string;
};

it("Axios 序列化 JSON 和查询，保留调用方幂等键并返回原始 payload", async () => {
  const result = await request<Echo>("/echo", {
    baseURL,
    method: "POST",
    data: { name: "test" },
    params: { limit: 50, deleted: false, cursor: undefined },
    headers: { "Idempotency-Key": "original-key" },
  });
  expect(result.url).toBe("/echo?limit=50&deleted=false");
  expect(JSON.parse(result.body)).toEqual({ name: "test" });
  expect(result.headers["content-type"]).toContain("application/json");
  expect(result.headers["idempotency-key"]).toBe("original-key");
  expect(result.headers.cookie).toBeUndefined();
});

it("写请求缺少幂等键时生成 UUID，读取不生成", async () => {
  const write = await request<Echo>("/echo", {
    baseURL,
    method: "PATCH",
    data: {},
  });
  expect(write.headers["idempotency-key"]).toMatch(/^[0-9a-f-]{36}$/);
  const read = await request<Echo>("/echo", { baseURL });
  expect(read.headers["idempotency-key"]).toBeUndefined();
});

it("生成上传函数通过 Axios 发送包含实际边界的 multipart", async () => {
  const result = await uploadPersonalMediaAsset(
    { local_review_confirmed: true },
    new File(["original bytes"], "flower.png", { type: "image/png" }),
    { baseURL, headers: { "Idempotency-Key": "same-upload" } },
  );
  const echoed = result as Echo;
  expect(echoed.url).toBe("/api/media/library/uploads");
  expect(echoed.headers["content-type"]).toMatch(
    /^multipart\/form-data; boundary=/,
  );
  expect(echoed.headers["idempotency-key"]).toBe("same-upload");
  expect(echoed.body).toContain('filename="flower.png"');
  expect(echoed.body).toContain("original bytes");
  expect(echoed.body).toContain('name="local_review_confirmed"\r\n\r\ntrue');
});

it("生成下载函数使用显式 Axios responseType 保留原始字节", async () => {
  const blob = await downloadLibraryMediaAsset(
    { item_id: "asset-id", scope: "personal" },
    {
      baseURL,
      adapter: "fetch",
      responseType: "blob",
      headers: { "X-Test": "download" },
    },
  );
  expect(blob).toBeInstanceOf(Blob);
  expect([...new Uint8Array(await blob.arrayBuffer())]).toEqual([
    0, 255, 128, 42,
  ]);
  await expect(
    downloadLibraryMediaAsset(
      { item_id: "denied", scope: "personal" },
      { baseURL, adapter: "fetch", responseType: "blob" },
    ),
  ).rejects.toEqual(new RequestError("forbidden", 403));
});

it("统一保留 HTTP 错误码、状态和元数据，支持 204", async () => {
  await expect(request("/problem", { baseURL })).rejects.toMatchObject({
    code: "revision_conflict",
    status: 409,
    meta: { revision: 2 },
  });
  await expect(request("/empty", { baseURL })).resolves.toBeUndefined();
});

it("超时转换为未确认错误，主动取消保持 Axios 取消语义", async () => {
  await expect(request("/slow", { baseURL, timeout: 20 })).rejects.toEqual(
    new RequestError("request_unconfirmed"),
  );
  const controller = new AbortController();
  const pending = request("/slow", { baseURL, signal: controller.signal });
  controller.abort();
  const error = await pending.catch((error) => error);
  expect(axios.isCancel(error)).toBe(true);
  expect(error).not.toBeInstanceOf(RequestError);
});

it("401 invalidation is bound to the sending session generation and keeps the original proof", async () => {
  const { setCSRFToken, subscribeSessionInvalidation } =
    await import("./request");
  const notifications: number[] = [];
  const unsubscribe = subscribeSessionInvalidation((generation) =>
    notifications.push(generation),
  );
  let rejectOld!: () => void;
  let originalProof = "";
  setCSRFToken("old-proof");
  const pending = request("/api/me/password", {
    method: "POST",
    adapter: async (config) =>
      new Promise((_resolve, reject) => {
        originalProof = String(config.headers.get("X-CSRF-Token"));
        rejectOld = () =>
          reject(
            new axios.AxiosError(
              "unauthenticated",
              undefined,
              config,
              undefined,
              {
                status: 401,
                statusText: "Unauthorized",
                headers: {},
                config,
                data: { code: "unauthenticated" },
              },
            ),
          );
      }),
  });
  await new Promise((resolve) => setTimeout(resolve, 0));
  setCSRFToken("new-proof");
  rejectOld();
  await expect(pending).rejects.toEqual(
    new RequestError("unauthenticated", 401),
  );
  expect(originalProof).toBe("old-proof");
  expect(notifications).toEqual([]);
  await expect(
    request("/api/me", {
      adapter: async (config) => {
        throw new axios.AxiosError(
          "unauthenticated",
          undefined,
          config,
          undefined,
          {
            status: 401,
            statusText: "Unauthorized",
            headers: {},
            config,
            data: { code: "unauthenticated" },
          },
        );
      },
    }),
  ).rejects.toEqual(new RequestError("unauthenticated", 401));
  expect(notifications).toHaveLength(1);
  unsubscribe();
  setCSRFToken(null);
});

it("wrong reauthentication password preserves the current session, other private invalid_credentials does not", async () => {
  const { setCSRFToken, subscribeSessionInvalidation } =
    await import("./request");
  const calls: number[] = [];
  const stop = subscribeSessionInvalidation((generation) =>
    calls.push(generation),
  );
  setCSRFToken("proof");
  const adapter: import("axios").AxiosAdapter = async (config) => {
    throw new axios.AxiosError(
      "invalid_credentials",
      undefined,
      config,
      undefined,
      {
        status: 401,
        statusText: "Unauthorized",
        headers: {},
        config,
        data: { code: "invalid_credentials" },
      },
    );
  };
  await expect(
    request("/api/me/reauthentication", { method: "POST", adapter }),
  ).rejects.toEqual(new RequestError("invalid_credentials", 401));
  expect(calls).toEqual([]);
  await expect(request("/api/me", { adapter })).rejects.toEqual(
    new RequestError("invalid_credentials", 401),
  );
  expect(calls).toHaveLength(1);
  stop();
  setCSRFToken(null);
});

it("切换空间取消旧读取，切换期间阻止新写入，完成后只发送新范围", async () => {
  const { setCreationScope, setScopeSwitching } = await import("./request");
  setCreationScope("old");
  const pending = request("/slow", { baseURL });
  setScopeSwitching(true);
  setCreationScope("new");
  expect(axios.isCancel(await pending.catch((e) => e))).toBe(true);
  await expect(request("/echo", { baseURL, method: "POST" })).rejects.toEqual(
    new RequestError("scope_switch_pending", 409),
  );
  setScopeSwitching(false);
  const read = await request<Echo>("/echo", { baseURL });
  expect(read.headers["x-creation-scope"]).toBe("new");
  setCreationScope(null);
});

it("旧身份的成功响应在新身份激活后同样作废", async () => {
  const { setCSRFToken } = await import("./request");
  setCSRFToken("old-success-proof");
  let finish!: () => void;
  const pending = request("/api/me", {
    adapter: async (config) =>
      new Promise((resolve) => {
        finish = () =>
          resolve({
            data: { email: "old@example.com" },
            status: 200,
            statusText: "OK",
            headers: {},
            config,
          });
      }),
  });
  await new Promise((resolve) => setTimeout(resolve, 0));
  setCSRFToken("new-success-proof");
  finish();
  const result = await pending.catch((e) => e);
  expect(axios.isCancel(result)).toBe(true);
  setCSRFToken(null);
});
