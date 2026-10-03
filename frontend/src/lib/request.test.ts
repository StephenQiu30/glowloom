import axios, {
  AxiosError,
  AxiosHeaders,
  type AxiosAdapter,
  type InternalAxiosRequestConfig,
} from "axios";
import { afterEach, describe, expect, it, vi } from "vitest";

import { createProject, listProjects } from "@/api/projects";
import { ApiError, readResourceStream, request } from "./request";

// The adapter only replaces transport: Axios still merges config, transforms
// request/response bodies and checks cancellation before dispatching it.
function captureAdapter(
  data: unknown = {},
  status = 200,
  headers: Record<string, string> = {},
) {
  const requests: InternalAxiosRequestConfig[] = [];
  const adapter: AxiosAdapter = async (config) => {
    requests.push(config);
    const response = {
      data,
      status,
      statusText: "Synthetic response",
      headers: new AxiosHeaders(headers),
      config,
    };
    if (config.validateStatus && !config.validateStatus(status)) {
      throw new AxiosError(
        "Synthetic upstream failure with private details",
        status >= 500 ? "ERR_BAD_RESPONSE" : "ERR_BAD_REQUEST",
        config,
        undefined,
        response,
      );
    }
    return response;
  };
  return { adapter, requests };
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("公共请求入口", () => {
  it("计数后的上传流保留原multipart头并直接返回上游流和元数据", async () => {
    const incoming = new ReadableStream<Uint8Array>();
    const outgoing = new ReadableStream<Uint8Array>();
    const transport = captureAdapter(outgoing, 422, {
      "content-type": "application/problem+json",
    });
    const onResponse = vi.fn();
    const result = await request("/api/projects/one/media/uploads", {
      adapter: transport.adapter,
      method: "POST",
      data: incoming,
      requestType: "form",
      responseType: "stream",
      headers: {
        "Content-Type": "multipart/form-data; boundary=original",
        Accept: false,
      },
      validateStatus: () => true,
      onResponse,
    });
    expect(result).toBe(outgoing);
    const config = transport.requests[0];
    expect(config.data).toBe(incoming);
    expect(config.headers.get("Content-Type")).toBe(
      "multipart/form-data; boundary=original",
    );
    expect(config.headers.get("Accept")).toBe(false);
    expect(config).not.toHaveProperty("onResponse");
    expect(onResponse).toHaveBeenCalledOnce();
    const response = onResponse.mock.calls[0][0];
    expect(response.status).toBe(422);
    expect(response.headers.get("Content-Type")).toBe(
      "application/problem+json",
    );
  });

  it("授权原件使用Axios流而非整份缓冲，不携带凭据也不跟随重定向", async () => {
    const bytes = new Uint8Array([1, 2, 3]);
    const upstream = vi.fn<typeof fetch>(async () => {
      return new Response(
        new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(bytes);
            controller.close();
          },
        }),
      );
    });
    vi.stubGlobal("fetch", upstream);
    const result = await readResourceStream(
      "https://objects.example.test/model",
      new AbortController().signal,
    );
    expect(result).toBeInstanceOf(ReadableStream);
    const reader = result.getReader();
    expect(await reader.read()).toEqual({ done: false, value: bytes });
    expect(await reader.read()).toEqual({ done: true, value: undefined });
    reader.releaseLock();
    expect(upstream).toHaveBeenCalledOnce();
    const sent = upstream.mock.calls[0][0] as Request;
    expect(sent.url).toBe("https://objects.example.test/model");
    expect(sent.redirect).toBe("error");
    expect(sent.credentials).toBe("omit");
    expect(sent.headers.has("Authorization")).toBe(false);
    expect(sent.headers.has("Cookie")).toBe(false);
  });

  it("取消保留Axios取消错误供资源生命周期识别且不启动传输", async () => {
    const upstream = vi.fn();
    vi.stubGlobal("fetch", upstream);
    const controller = new AbortController();
    controller.abort();
    await expect(
      readResourceStream(
        "https://objects.example.test/model",
        controller.signal,
      ),
    ).rejects.toMatchObject({ name: "CanceledError", code: "ERR_CANCELED" });
    expect(upstream).not.toHaveBeenCalled();
    const cancellation = new axios.CanceledError("canceled");
    await expect(
      request("/api/probe", {
        adapter: async () => {
          throw cancellation;
        },
      }),
    ).rejects.toBe(cancellation);
  });

  it("multipart文件保持FormData并保留取消/进度/幂等，不转换为JSON", async () => {
    const transport = captureAdapter();
    const body = new FormData();
    body.append("file", new File(["bytes"], "file.png", { type: "image/png" }));
    const signal = new AbortController().signal;
    const onUploadProgress = vi.fn();
    await request("/api/projects/one/media/uploads", {
      adapter: transport.adapter,
      method: "POST",
      data: body,
      signal,
      onUploadProgress,
      headers: {
        "Content-Type": "multipart/form-data",
        "Idempotency-Key": "file-key",
      },
    });
    const config = transport.requests[0];
    expect(config.data).toBe(body);
    expect(config.data.get("file")).toBeInstanceOf(File);
    expect(config.signal).toBe(signal);
    expect(config.onUploadProgress).toBe(onUploadProgress);
    expect(config.headers.get("Content-Type")).not.toContain(
      "application/json",
    );
    expect(config.headers.get("Idempotency-Key")).toBe("file-key");
  });

  it("同源写请求返回业务data并由Axios序列化JSON和保留稳定幂等键", async () => {
    const data = { revision: 2 };
    const transport = captureAdapter(data);
    expect(
      await request("/api/canvases/one/commands", {
        adapter: transport.adapter,
        method: "POST",
        data: { commands: [] },
        headers: { "Idempotency-Key": "same-request" },
      }),
    ).toBe(data);
    const config = transport.requests[0];
    expect(config.url).toBe("/api/canvases/one/commands");
    expect(config.withCredentials).toBe(false);
    expect(config.withXSRFToken).toBe(false);
    expect(config.timeout).toBe(15_000);
    expect(config.headers.get("Idempotency-Key")).toBe("same-request");
    expect(config.headers.get("Content-Type")).toBe("application/json");
    expect(config.data).toBe('{"commands":[]}');
  });

  it("预签名上传不发送后端凭据头或幂等键且不修改共享AxiosHeaders", async () => {
    const transport = captureAdapter();
    const headers = new AxiosHeaders({
      Authorization: "synthetic-authorization",
      Cookie: "synthetic-cookie",
      "Idempotency-Key": "synthetic-key",
    });
    await request("https://objects.example.test/upload", {
      adapter: transport.adapter,
      method: "PUT",
      data: "file",
      headers,
    });
    const config = transport.requests[0];
    expect(config.withCredentials).toBe(false);
    expect(config.headers.has("Cookie")).toBe(false);
    expect(config.headers.has("Authorization")).toBe(false);
    expect(config.headers.has("Idempotency-Key")).toBe(false);
    expect(headers.get("Authorization")).toBe("synthetic-authorization");
    expect(headers.get("Cookie")).toBe("synthetic-cookie");
    expect(headers.get("Idempotency-Key")).toBe("synthetic-key");
  });

  it("真实生成SDK通过options.headers传幂等键并保留独立query参数", async () => {
    const transport = captureAdapter({ revision: 1 });
    await createProject(
      { name: "请求边界项目", aspect_ratio: "16:9", style_type: "realistic" },
      {
        adapter: transport.adapter,
        headers: { "Idempotency-Key": "stable-key" },
      },
    );
    const write = transport.requests[0];
    expect(write.url).toBe("/api/projects");
    expect(write.headers.get("Idempotency-Key")).toBe("stable-key");
    expect(write.params).toBeUndefined();
    expect(JSON.parse(write.data)).toMatchObject({ name: "请求边界项目" });

    await listProjects(
      { limit: 20, cursor: "next-page" },
      { adapter: transport.adapter },
    );
    const read = transport.requests[1];
    expect(read.params).toEqual({ limit: 20, cursor: "next-page" });
    expect(read.params).not.toHaveProperty("Idempotency-Key");
  });

  it("解析problem+json状态/业务码/request_id，依赖失败不泄漏原始错误", async () => {
    const transport = captureAdapter(
      JSON.stringify({
        status: 409,
        code: "revision_conflict",
        request_id: "req-test",
        detail: "Synthetic private upstream detail",
        meta: { current_revision: 3 },
      }),
      409,
      { "content-type": "application/problem+json" },
    );
    const failure = await request("/api/canvases/one", {
      adapter: transport.adapter,
    }).catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(ApiError);
    expect(failure).toMatchObject({
      status: 409,
      code: "revision_conflict",
      requestId: "req-test",
      meta: { current_revision: 3 },
    });
    expect((failure as Error).message).not.toContain("private");
  });

  it("网络不可达明确失败，不返回样例或空文档", async () => {
    await expect(
      request("/api/canvases/one", {
        adapter: async () => {
          throw new AxiosError("Synthetic ECONNREFUSED", "ERR_NETWORK");
        },
      }),
    ).rejects.toMatchObject({ status: 0, code: "dependency_unavailable" });
  });
});
