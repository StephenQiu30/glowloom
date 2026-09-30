// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { POST } from "./route";

const projectId = "b33ab321-e124-4c48-9361-39294af0d404";
const key = "061ab42b-101e-42cd-b345-e5833a5e2530";
const requestId = "abebdcda-a9d3-41b0-9f5a-a7c8b5ec7fbd";
const maxBody = 500 * 1024 * 1024 + 64 * 1024;
const context = { params: Promise.resolve({ projectId }) };
const fetchMock = vi.fn<typeof fetch>();

function request(
  body: BodyInit | null = "multipart-body",
  headers?: HeadersInit,
  signal?: AbortSignal,
) {
  return new Request(
    `http://caller.invalid/api/projects/${projectId}/media/uploads?ignored=1`,
    {
      method: "POST",
      headers: {
        "Content-Type": "multipart/form-data; boundary=original",
        Origin: "http://localhost:3000",
        "Idempotency-Key": key,
        "X-Request-Id": requestId,
        ...headers,
      },
      body,
      signal,
      duplex: "half",
    } as RequestInit & { duplex: "half" },
  );
}

async function consume(body: BodyInit | null | undefined) {
  let bytes = 0;
  const reader = (body as ReadableStream<Uint8Array>).getReader();
  for (;;) {
    const { done, value } = await reader.read();
    if (done) return bytes;
    bytes += value.byteLength;
  }
}

describe("bounded streaming media upload route", () => {
  beforeEach(() => {
    vi.stubEnv("LV_API_BASE_URL", "http://127.0.0.1:18991");
    vi.stubGlobal("fetch", fetchMock);
    fetchMock.mockReset();
    fetchMock.mockImplementation(async (_url, options) => {
      await consume(options?.body);
      return Response.json({ asset: "actual-go-result" }, { status: 201 });
    });
  });
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
  });

  it("streams original bytes and transport identity to the fixed Go route without auth/hop headers", async () => {
    const response = await POST(
      request("original bytes", {
        Authorization: "Bearer synthetic",
        Cookie: "synthetic=1",
        Connection: "keep-alive",
        "X-Forwarded-Host": "evil.invalid",
        "Content-Length": "14",
      }),
      context,
    );
    expect(response.status).toBe(201);
    expect(await response.json()).toEqual({ asset: "actual-go-result" });
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe(
      `http://127.0.0.1:18991/api/projects/${projectId}/media/uploads`,
    );
    expect(options).toMatchObject({
      method: "POST",
      cache: "no-store",
      redirect: "manual",
      duplex: "half",
    });
    expect(options?.body).toBeInstanceOf(ReadableStream);
    const headers = new Headers(options?.headers);
    expect(Object.fromEntries(headers)).toEqual({
      "content-type": "multipart/form-data; boundary=original",
      "content-length": "14",
      origin: "http://localhost:3000",
      "idempotency-key": key,
      "x-request-id": requestId,
    });
    expect(response.headers.get("cache-control")).toBe("private, no-store");
  });

  it("does not create an Origin or idempotency key for callers missing them", async () => {
    const input = request();
    input.headers.delete("Origin");
    input.headers.delete("Idempotency-Key");
    await POST(input, context);
    const headers = new Headers(fetchMock.mock.calls[0][1]?.headers);
    expect(headers.has("origin")).toBe(false);
    expect(headers.has("idempotency-key")).toBe(false);
  });

  it.each([403, 409, 413, 415, 422, 429, 503])(
    "preserves Go status %i and problem bytes without leaking upstream cookies",
    async (status) => {
      const problem = JSON.stringify({
        code: "go_contract_error",
        request_id: requestId,
        meta: { retry: false },
      });
      fetchMock.mockResolvedValue(
        new Response(problem, {
          status,
          headers: {
            "Content-Type": "application/problem+json",
            "X-Request-Id": requestId,
            "Set-Cookie": "private=1",
            Connection: "close",
            "Content-Length": "123",
            "X-Internal-Secret": "hidden",
          },
        }),
      );
      const response = await POST(request(), context);
      expect(response.status).toBe(status);
      expect(await response.text()).toBe(problem);
      expect(Object.fromEntries(response.headers)).toEqual({
        "cache-control": "private, no-store",
        "content-type": "application/problem+json",
        "x-request-id": requestId,
      });
    },
  );

  it("rejects an oversized declared body before forwarding or reading it", async () => {
    const input = request("body", { "Content-Length": String(maxBody + 1) });
    const readerSpy = vi.spyOn(input, "arrayBuffer");
    const response = await POST(input, context);
    expect(response.status).toBe(413);
    expect(await response.json()).toMatchObject({
      code: "request_too_large",
      request_id: requestId,
    });
    expect(fetchMock).not.toHaveBeenCalled();
    expect(readerSpy).not.toHaveBeenCalled();
  });

  it.each(["-1", "12x", "9007199254740993"])(
    "rejects invalid Content-Length %s",
    async (length) => {
      expect(
        (await POST(request("body", { "Content-Length": length }), context))
          .status,
      ).toBe(422);
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it("accepts the exact actual-byte limit with a bounded reused chunk and rejects its next byte", async () => {
    const chunk = new Uint8Array(1024 * 1024);
    function stream(size: number) {
      return new ReadableStream<Uint8Array>({
        pull(controller) {
          if (size <= 0) {
            controller.close();
            return;
          }
          const count = Math.min(size, chunk.length);
          size -= count;
          controller.enqueue(chunk.subarray(0, count));
        },
      });
    }
    expect((await POST(request(stream(maxBody)), context)).status).toBe(201);
    const response = await POST(request(stream(maxBody + 1)), context);
    expect(response.status).toBe(413);
    expect(await response.json()).toMatchObject({ code: "request_too_large" });
  });

  it("sends the first chunk upstream before the caller finishes and never materializes or clones the body", async () => {
    let finish!: () => void;
    const barrier = new Promise<void>((resolve) => {
      finish = resolve;
    });
    let sent = false;
    const input = request(
      new ReadableStream<Uint8Array>({
        async pull(controller) {
          if (!sent) {
            sent = true;
            controller.enqueue(new Uint8Array([1, 2, 3]));
            return;
          }
          await barrier;
          controller.enqueue(new Uint8Array([4]));
          controller.close();
        },
      }),
    );
    const methods = [
      vi.spyOn(input, "clone"),
      vi.spyOn(input, "arrayBuffer"),
      vi.spyOn(input, "formData"),
      vi.spyOn(input, "text"),
    ];
    let firstChunk!: () => void;
    const observed = new Promise<void>((resolve) => {
      firstChunk = resolve;
    });
    fetchMock.mockImplementation(async (_url, options) => {
      const reader = (options?.body as ReadableStream<Uint8Array>).getReader();
      expect((await reader.read()).value).toEqual(new Uint8Array([1, 2, 3]));
      firstChunk();
      expect((await reader.read()).value).toEqual(new Uint8Array([4]));
      expect((await reader.read()).done).toBe(true);
      return new Response(null, { status: 201 });
    });
    const result = POST(input, context);
    await observed;
    finish();
    expect((await result).status).toBe(201);
    for (const method of methods) expect(method).not.toHaveBeenCalled();
  });

  it("propagates caller cancellation to body reading and upstream fetch", async () => {
    const controller = new AbortController();
    const input = request("body", undefined, controller.signal);
    fetchMock.mockImplementation(
      async (_url, options) =>
        new Promise((_resolve, reject) => {
          options?.signal?.addEventListener(
            "abort",
            () => reject(options.signal?.reason),
            { once: true },
          );
          controller.abort(new DOMException("synthetic cancel", "AbortError"));
        }),
    );
    const response = await POST(input, context);
    expect(response.status).toBe(408);
    expect(fetchMock.mock.calls[0][1]?.signal?.aborted).toBe(true);
  });

  it("uses exactly a five-minute abort deadline and reports a safe transport timeout", async () => {
    const timeoutController = new AbortController();
    const timeout = vi
      .spyOn(AbortSignal, "timeout")
      .mockReturnValue(timeoutController.signal);
    fetchMock.mockImplementation(
      async (_url, options) =>
        new Promise((_resolve, reject) => {
          options?.signal?.addEventListener(
            "abort",
            () => reject(options.signal?.reason),
            { once: true },
          );
          timeoutController.abort(
            new DOMException("synthetic timeout", "TimeoutError"),
          );
        }),
    );
    const response = await POST(request(), context);
    expect(timeout).toHaveBeenCalledWith(300_000);
    expect(response.status).toBe(504);
    expect(await response.json()).toMatchObject({
      code: "dependency_unavailable",
    });
  });

  it("rejects an invalid project path or upstream root without allowing caller URL control", async () => {
    expect(
      (
        await POST(request(), {
          params: Promise.resolve({ projectId: "../../private" }),
        })
      ).status,
    ).toBe(422);
    vi.stubEnv("LV_API_BASE_URL", "http://user:private-password@localhost/api");
    const response = await POST(request(), context);
    expect(response.status).toBe(503);
    expect(await response.text()).not.toContain("private-password");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("maps an unreachable upstream to the safe dependency problem", async () => {
    fetchMock.mockRejectedValue(
      new Error("private upstream implementation details"),
    );
    const response = await POST(request(), context);
    expect(response.status).toBe(502);
    expect(await response.text()).not.toContain("private upstream");
  });

  it("does not dispatch a request that was already cancelled", async () => {
    const cancelled = new AbortController();
    cancelled.abort();
    const response = await POST(
      request("body", undefined, cancelled.signal),
      context,
    );
    expect(response.status).toBe(408);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("does not follow or expose an upstream redirect to another origin", async () => {
    fetchMock.mockResolvedValue(
      new Response(null, {
        status: 307,
        headers: { Location: "https://other.invalid/private" },
      }),
    );
    const response = await POST(request(), context);
    expect(response.status).toBe(307);
    expect(response.headers.has("location")).toBe(false);
    expect(fetchMock.mock.calls[0][1]?.redirect).toBe("manual");
    expect(fetchMock).toHaveBeenCalledOnce();
  });
});
