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
  expect(axios.isCancel(await pending.catch((error) => error))).toBe(true);
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

it.each([
  { boundary: "scope", status: 401 },
  { boundary: "caller", status: 401 },
  { boundary: "scope", status: 403 },
  { boundary: "caller", status: 403 },
])(
  "迟到Blob$status解析期间$boundary取消后没有会话失效或权限刷新副作用",
  async ({ boundary, status }) => {
    const {
      setCSRFToken,
      setCreationScope,
      subscribeSessionInvalidation,
      subscribeScopeDenied,
    } = await import("./request");
    const invalidations: number[] = [];
    let denials = 0;
    const stop = subscribeSessionInvalidation((n) => invalidations.push(n));
    const stopDenied = subscribeScopeDenied(() => denials++);
    setCSRFToken("blob-proof");
    setCreationScope("old");
    let finish!: (value: string) => void;
    let started!: () => void;
    const parsing = new Promise<void>((resolve) => (started = resolve));
    const problem = new Blob();
    problem.text = () => {
      started();
      return new Promise<string>((resolve) => (finish = resolve));
    };
    const controller = new AbortController();
    const pending = request("/api/platform/user-exports/export/download", {
      signal: controller.signal,
      adapter: async (config) => {
        throw new axios.AxiosError(
          "unauthenticated",
          undefined,
          config,
          undefined,
          {
            status,
            statusText: status === 401 ? "Unauthorized" : "Forbidden",
            headers: {},
            config,
            data: problem,
          },
        );
      },
    });
    await parsing;
    if (boundary === "scope") setCreationScope("new");
    else controller.abort();
    finish(
      JSON.stringify({
        code: status === 401 ? "unauthenticated" : "forbidden",
      }),
    );
    const result = await pending.catch((e) => e);
    expect(axios.isCancel(result)).toBe(true);
    expect(invalidations).toEqual([]);
    expect(denials).toBe(0);
    stop();
    stopDenied();
    setCSRFToken(null);
  },
);

it("refreshIsSingleFlightAcrossTwentyReads", async () => {
  const { setCSRFToken, setSessionRecovery } = await import("./request");
  setCSRFToken("stable");
  let recovered = false;
  let refreshes = 0;
  setSessionRecovery(async () => {
    refreshes++;
    await new Promise((r) => setTimeout(r, 5));
    recovered = true;
  });
  const adapter: import("axios").AxiosAdapter = async (config) => {
    expect(config).not.toHaveProperty("sessionRecovery");
    if (!recovered)
      throw new axios.AxiosError("expired", undefined, config, undefined, {
        status: 401,
        statusText: "Unauthorized",
        headers: {},
        config,
        data: { code: "access_expired" },
      });
    return { status: 200, statusText: "OK", headers: {}, config, data: "kept" };
  };
  try {
    expect(
      await Promise.all(
        Array.from({ length: 20 }, () => request("/api/me", { adapter })),
      ),
    ).toEqual(Array(20).fill("kept"));
    expect(refreshes).toBe(1);
  } finally {
    setSessionRecovery(null);
    setCSRFToken(null);
  }
});

it("alreadySentMutationIsNeverReplayed", async () => {
  const { setCSRFToken, setSessionRecovery, subscribeSessionInvalidation } =
    await import("./request");
  setCSRFToken("stable");
  let sends = 0;
  let invalidations = 0;
  const stop = subscribeSessionInvalidation(() => invalidations++);
  setSessionRecovery(async () => {});
  try {
    await expect(
      request("/api/generate", {
        method: "POST",
        adapter: async (config) => {
          sends++;
          throw new axios.AxiosError("expired", undefined, config, undefined, {
            status: 401,
            statusText: "Unauthorized",
            headers: {},
            config,
            data: { code: "access_expired" },
          });
        },
      }),
    ).rejects.toMatchObject({ code: "access_expired" });
    expect(sends).toBe(1);
    expect(invalidations).toBe(0);
  } finally {
    stop();
    setSessionRecovery(null);
    setCSRFToken(null);
  }
});

it.each(["invalid_credentials", "current_password_invalid"])(
  "wrong current password %s preserves identity",
  async (code) => {
    const { setCSRFToken, subscribeSessionInvalidation } =
      await import("./request");
    setCSRFToken("stable");
    let invalidations = 0;
    const stop = subscribeSessionInvalidation(() => invalidations++);
    try {
      await expect(
        request("/api/me/password", {
          method: "POST",
          adapter: async (config) => {
            throw new axios.AxiosError(code, undefined, config, undefined, {
              status: 401,
              statusText: "Unauthorized",
              headers: {},
              config,
              data: { code },
            });
          },
        }),
      ).rejects.toMatchObject({ code });
      expect(invalidations).toBe(0);
    } finally {
      stop();
      setCSRFToken(null);
    }
  },
);
it("late expired reads share the completed recovery without a second rotation", async () => {
  const { setCSRFToken, setSessionRecovery } = await import("./request");
  setCSRFToken("stable");
  let refreshes = 0;
  let recovered = false;
  let release!: () => void;
  setSessionRecovery(async () => {
    refreshes++;
    recovered = true;
  });
  const adapter: import("axios").AxiosAdapter = async (config) => {
    const expired = !recovered;
    if (config.url === "/api/slow" && expired)
      await new Promise<void>((r) => (release = r));
    if (expired)
      throw new axios.AxiosError("expired", undefined, config, undefined, {
        status: 401,
        statusText: "Unauthorized",
        headers: {},
        config,
        data: { code: "access_expired" },
      });
    return { status: 200, statusText: "OK", headers: {}, config, data: "kept" };
  };
  try {
    const slow = request("/api/slow", { adapter });
    await request("/api/fast", { adapter });
    release();
    await slow;
    expect(refreshes).toBe(1);
  } finally {
    setSessionRecovery(null);
    setCSRFToken(null);
  }
});
it("a write waits for pending recovery and canceled waiters preserve Axios semantics", async () => {
  const { setCSRFToken, setSessionRecovery } = await import("./request");
  setCSRFToken("stable");
  let recovered = false;
  let finish!: () => void;
  setSessionRecovery(
    () =>
      new Promise<void>(
        (r) =>
          (finish = () => {
            recovered = true;
            r();
          }),
      ),
  );
  const adapter: import("axios").AxiosAdapter = async (config) => {
    if (!recovered)
      throw new axios.AxiosError("expired", undefined, config, undefined, {
        status: 401,
        statusText: "Unauthorized",
        headers: {},
        config,
        data: { code: "access_expired" },
      });
    return {
      status: 200,
      statusText: "OK",
      headers: {},
      config,
      data: config.method,
    };
  };
  try {
    const read = request("/api/me", { adapter });
    await new Promise((r) => setTimeout(r, 0));
    let sends = 0;
    const write = request("/api/write", {
      method: "POST",
      adapter: async (config) => {
        sends++;
        return adapter(config);
      },
    });
    const controller = new AbortController();
    const canceled = request("/api/read", {
      signal: controller.signal,
      adapter,
    }).catch((e) => e);
    controller.abort();
    expect(axios.isCancel(await canceled)).toBe(true);
    expect(sends).toBe(0);
    finish();
    expect(await write).toBe("post");
    await read;
    expect(sends).toBe(1);
  } finally {
    setSessionRecovery(null);
    setCSRFToken(null);
  }
});

it.each([
  "origin_forbidden",
  "origin_mismatch",
  "csrf_invalid",
  "session_invalid",
  "session_cookie_conflict",
])("%s never revalidates business spaces", async (code) => {
  const { setCSRFToken, setCreationScope, subscribeScopeDenied } =
    await import("./request");
  setCSRFToken("stable");
  setCreationScope("team");
  let denials = 0;
  const stop = subscribeScopeDenied(() => denials++);
  try {
    await expect(
      request("/api/write", {
        method: "POST",
        adapter: async (config) => {
          throw new axios.AxiosError(code, undefined, config, undefined, {
            status: 403,
            statusText: "Forbidden",
            headers: {},
            config,
            data: { code },
          });
        },
      }),
    ).rejects.toMatchObject({ code });
    expect(denials).toBe(0);
  } finally {
    stop();
    setCSRFToken(null);
  }
});
it("scope changes while recovery waits cancel reads and prevent queued writes", async () => {
  const { setCSRFToken, setCreationScope, setSessionRecovery } =
    await import("./request");
  setCSRFToken("stable");
  setCreationScope("old");
  let finish!: () => void;
  setSessionRecovery(() => new Promise<void>((r) => (finish = r)));
  const adapter: import("axios").AxiosAdapter = async (config) => {
    throw new axios.AxiosError("expired", undefined, config, undefined, {
      status: 401,
      statusText: "Unauthorized",
      headers: {},
      config,
      data: { code: "access_expired" },
    });
  };
  try {
    const read = request("/api/me", { adapter }).catch((e) => e);
    await new Promise((r) => setTimeout(r, 0));
    let sends = 0;
    const write = request("/api/write", {
      method: "POST",
      adapter: async (config) => {
        sends++;
        return adapter(config);
      },
    }).catch((e) => e);
    setCreationScope("new");
    finish();
    expect(axios.isCancel(await read)).toBe(true);
    expect(axios.isCancel(await write)).toBe(true);
    expect(sends).toBe(0);
  } finally {
    setSessionRecovery(null);
    setCSRFToken(null);
  }
});
