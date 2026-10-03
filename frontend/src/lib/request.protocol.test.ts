// @vitest-environment node

import axios, {
  AxiosError,
  AxiosHeaders,
  type AxiosAdapter,
  type AxiosResponse,
} from "axios";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, request } from "./request";

afterEach(() => vi.restoreAllMocks());

// Keep Axios real: its body/header transforms run before the custom adapter.
function successfulAdapter() {
  return vi.fn<AxiosAdapter>(async (config) => ({
    config,
    status: 200,
    statusText: "OK",
    headers: new AxiosHeaders(),
    data: "received",
  }));
}

function conflictAdapter(data: unknown, contentType: string): AxiosAdapter {
  return async (config) => {
    const response: AxiosResponse<unknown> = {
      config,
      status: 409,
      statusText: "Conflict",
      headers: new AxiosHeaders({ "Content-Type": contentType }),
      data,
    };
    throw new AxiosError(
      "private adapter diagnostic",
      AxiosError.ERR_BAD_REQUEST,
      config,
      undefined,
      response,
    );
  };
}

const problem = {
  status: 409,
  code: "revision_conflict",
  request_id: "protocol-conflict-request",
  meta: { current_revision: 7 },
  detail: "private server diagnostic",
};
const binaryFormats = [
  {
    label: "Blob",
    responseType: "blob",
    body: (text: string, type: string) => new Blob([text], { type }),
  },
  {
    label: "ArrayBuffer",
    responseType: "arraybuffer",
    body: (text: string) => new TextEncoder().encode(text).buffer,
  },
] as const;

describe("真实 Axios 请求协议", () => {
  it("显式 text/plain 保持 Content-Type 与原文字节，不变成 JSON 字符串", async () => {
    const adapter = successfulAdapter();
    const body = "第一行：原文\n第二行=不转换";
    await request("/api/protocol/text", {
      method: "POST",
      data: body,
      headers: { "Content-Type": "text/plain; charset=utf-8" },
      adapter,
    });
    const config = adapter.mock.calls[0][0];
    expect
      .soft(config.headers.getContentType())
      .toBe("text/plain; charset=utf-8");
    expect(config.data).toBe(body);
  });

  it("URLSearchParams 由 Axios 编码并设置 x-www-form-urlencoded", async () => {
    const adapter = successfulAdapter();
    const body = new URLSearchParams({ text: "中文 与空格", cursor: "a&b" });
    await request("/api/protocol/form", {
      method: "POST",
      data: body,
      adapter,
    });
    const config = adapter.mock.calls[0][0];
    expect(config.headers.getContentType()).toBe(
      "application/x-www-form-urlencoded;charset=utf-8",
    );
    expect(config.data).toBe(body.toString());
  });

  it("清理外部请求凭据不修改调用方共享的 AxiosHeaders", async () => {
    const shared = new AxiosHeaders({
      Authorization: "Bearer synthetic-test-token",
      Cookie: "synthetic-session=test",
      "Idempotency-Key": "synthetic-operation-key",
      "X-Client-Marker": "preserve-me",
    });
    const before = shared.toJSON();
    const adapter = successfulAdapter();
    await request("https://objects.example.test/original", {
      headers: shared,
      adapter,
    });
    const sent = adapter.mock.calls[0][0].headers;
    for (const name of ["Authorization", "Cookie", "Idempotency-Key"])
      expect.soft(sent.has(name)).toBe(false);
    expect.soft(sent.get("X-Client-Marker")).toBe("preserve-me");
    expect(shared.toJSON()).toEqual(before);
  });

  it("相对 /api URL 使用外部 baseURL 时不携带后端凭据", async () => {
    const adapter = successfulAdapter();
    await request("/api/protocol/external", {
      baseURL: "https://objects.example.test",
      headers: {
        Authorization: "Bearer synthetic-test-token",
        Cookie: "synthetic-session=test",
        "Idempotency-Key": "synthetic-operation-key",
      },
      adapter,
    });
    const config = adapter.mock.calls[0][0];
    expect(axios.getUri(config)).toBe(
      "https://objects.example.test/api/protocol/external",
    );
    for (const name of ["Authorization", "Cookie", "Idempotency-Key"])
      expect.soft(config.headers.has(name)).toBe(false);
  });

  it("响应回调自身抛错时保留原错误，不伪报依赖不可用", async () => {
    const failure = new Error("调用方响应处理失败");
    await expect(
      request("/api/protocol/response", {
        adapter: successfulAdapter(),
        onResponse: () => {
          throw failure;
        },
      }),
    ).rejects.toBe(failure);
  });
});

describe("真实 Axios 二进制错误响应", () => {
  it.each([false, true])(
    "真实 fetch adapter 的 503 错误流取消并保留请求 ID，传入signal：%s",
    async (withSignal) => {
      const cancel = vi.fn();
      const controller = new AbortController();
      const fetch = vi.fn(
        async () =>
          new Response(new ReadableStream<Uint8Array>({ cancel }), {
            status: 503,
            headers: {
              "Content-Type": "application/problem+json",
              "X-Request-Id": "protocol-error-stream-request",
            },
          }),
      );
      await expect(
        request("https://objects.example.test/original", {
          adapter: "fetch",
          timeout: 0,
          responseType: "stream",
          env: { fetch },
          ...(withSignal ? { signal: controller.signal } : {}),
        }),
      ).rejects.toMatchObject({
        name: "ApiError",
        status: 503,
        code: "request_failed",
        requestId: "protocol-error-stream-request",
      });
      expect(fetch).toHaveBeenCalledOnce();
      expect(cancel).toHaveBeenCalledOnce();
    },
  );

  it("自定义 validateStatus 接受 404 流只判定一次，正文交给消费者取消", async () => {
    const cancel = vi.fn();
    const body = new ReadableStream<Uint8Array>({ cancel });
    const fetch = vi.fn(async () => new Response(body, { status: 404 }));
    const validateStatus = vi.fn((status: number) => status === 404);
    const result = await request<ReadableStream<Uint8Array>>(
      "https://objects.example.test/optional-original",
      {
        adapter: "fetch",
        timeout: 0,
        responseType: "stream",
        env: { fetch },
        validateStatus,
      },
    );
    expect(fetch).toHaveBeenCalledOnce();
    expect(validateStatus).toHaveBeenCalledExactlyOnceWith(404);
    expect(cancel).not.toHaveBeenCalled();
    await result.cancel();
    expect(cancel).toHaveBeenCalledOnce();
  });

  it("原始 fetch 接收原 redirect 配置，不接收传输层内部 Symbol 上下文", async () => {
    const originalFetch = vi.fn<typeof globalThis.fetch>(
      async () => new Response("原始资源正文"),
    );
    const result = await request<ReadableStream<Uint8Array>>(
      "https://objects.example.test/original",
      {
        adapter: "fetch",
        timeout: 0,
        responseType: "stream",
        env: { fetch: originalFetch },
        fetchOptions: { redirect: "error", cache: "no-store" },
      },
    );
    expect(originalFetch).toHaveBeenCalledOnce();
    const [input, init] = originalFetch.mock.calls[0];
    expect(init).toMatchObject({ redirect: "error", cache: "no-store" });
    expect(Object.getOwnPropertySymbols(init ?? {})).toEqual([]);
    expect(input).toBeInstanceOf(Request);
    expect((input as Request).redirect).toBe("error");
    expect((input as Request).cache).toBe("no-store");
    await result.cancel();
  });

  it.each(binaryFormats)(
    "$label 的 ProblemJSON 409 保留业务码、请求 ID 和修订 metadata",
    async (format) => {
      const failure: unknown = await request("/api/protocol/download", {
        responseType: format.responseType,
        adapter: conflictAdapter(
          format.body(JSON.stringify(problem), "application/problem+json"),
          "application/problem+json; charset=utf-8",
        ),
      }).catch((error: unknown) => error);
      expect(failure).toBeInstanceOf(ApiError);
      expect(failure).toMatchObject({
        status: 409,
        code: "revision_conflict",
        requestId: problem.request_id,
        meta: problem.meta,
      });
      expect((failure as Error).message).not.toContain("private");
    },
  );

  it.each(binaryFormats)(
    "$label 的非 JSON 正文不解码或暴露内部 Problem 字段",
    async (format) => {
      const data = format.body(JSON.stringify(problem), "text/plain");
      const blobDecode = vi.spyOn(Blob.prototype, "text");
      const bufferDecode = vi.spyOn(TextDecoder.prototype, "decode");
      const failure: unknown = await request("/api/protocol/download", {
        responseType: format.responseType,
        adapter: conflictAdapter(data, "text/plain"),
      }).catch((error: unknown) => error);
      expect(failure).toBeInstanceOf(ApiError);
      expect(failure).toMatchObject({ status: 409, code: "request_failed" });
      expect.soft(failure).not.toHaveProperty("requestId", problem.request_id);
      expect.soft(failure).not.toHaveProperty("meta", problem.meta);
      expect.soft(blobDecode).not.toHaveBeenCalled();
      expect(bufferDecode).not.toHaveBeenCalled();
      expect((failure as Error).message).not.toContain("private");
    },
  );

  it.each(binaryFormats)(
    "$label 的超大 ProblemJSON 不整件解码，保持通用 409 错误",
    async (format) => {
      const data = format.body(
        JSON.stringify({ ...problem, detail: "x".repeat(2 * 1024 * 1024) }),
        "application/problem+json",
      );
      const blobDecode = vi.spyOn(Blob.prototype, "text");
      const bufferDecode = vi.spyOn(TextDecoder.prototype, "decode");
      const failure: unknown = await request("/api/protocol/download", {
        responseType: format.responseType,
        adapter: conflictAdapter(data, "application/problem+json"),
      }).catch((error: unknown) => error);
      expect(failure).toBeInstanceOf(ApiError);
      expect(failure).toMatchObject({ status: 409, code: "request_failed" });
      expect.soft(failure).not.toHaveProperty("requestId", problem.request_id);
      expect.soft(blobDecode).not.toHaveBeenCalled();
      expect(bufferDecode).not.toHaveBeenCalled();
    },
  );
});
