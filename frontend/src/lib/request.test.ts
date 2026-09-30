import axios, { AxiosError } from "axios";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, request } from "./request";

afterEach(() => {
  vi.restoreAllMocks();
});
describe("公共请求入口", () => {
  it("同源写请求无需认证并保留调用方的稳定幂等键", async () => {
    const send = vi
      .spyOn(axios, "request")
      .mockResolvedValue({ data: { revision: 2 } });
    await request("/api/canvases/one/commands", {
      method: "POST",
      data: { commands: [] },
      headers: { "Idempotency-Key": "same-request" },
    });
    expect(send).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/canvases/one/commands",
        withCredentials: false,
        headers: expect.objectContaining({
          "Idempotency-Key": "same-request",
          "Content-Type": "application/json",
        }),
      }),
    );
  });
  it("预签名上传不发送后端凭据头", async () => {
    const send = vi.spyOn(axios, "request").mockResolvedValue({ data: {} });
    await request("https://objects.example.test/upload", {
      method: "PUT",
      data: "file",
      headers: { Authorization: "bad", Cookie: "bad" },
    });
    expect(send.mock.calls[0][0].withCredentials).toBe(false);
    expect(send.mock.calls[0][0].headers).not.toHaveProperty("Cookie");
    expect(send.mock.calls[0][0].headers).not.toHaveProperty("Authorization");
  });
  it("生成器的幂等键参数归位，不进入query", async () => {
    const send = vi.spyOn(axios, "request").mockResolvedValue({ data: {} });
    await request("/api/canvases/one/commands", {
      method: "POST",
      params: {
        "Idempotency-Key": "stable-key",
        cursor: "next",
      },
      data: {},
    });
    expect(send.mock.calls[0][0].headers).toMatchObject({
      "Idempotency-Key": "stable-key",
    });
    expect(send.mock.calls[0][0].params).toEqual({ cursor: "next" });
  });
  it("解析problem+json状态/业务码/request_id，依赖失败不泄漏原始错误", async () => {
    vi.spyOn(axios, "request").mockRejectedValue(
      new AxiosError(
        "sql password is private",
        "ERR_BAD_RESPONSE",
        undefined,
        undefined,
        {
          status: 409,
          statusText: "Conflict",
          headers: {},
          config: {} as never,
          data: {
            status: 409,
            code: "revision_conflict",
            request_id: "req-test",
            detail: "sensitive internal details",
            meta: { current_revision: 3 },
          },
        },
      ),
    );
    await expect(request("/api/canvases/one")).rejects.toMatchObject({
      status: 409,
      code: "revision_conflict",
      requestId: "req-test",
      meta: { current_revision: 3 },
    });
    try {
      await request("/api/canvases/one");
    } catch (error) {
      expect(error).toBeInstanceOf(ApiError);
      expect((error as Error).message).not.toContain("sensitive");
    }
  });
  it("网络不可达明确失败，不返回样例或空文档", async () => {
    vi.spyOn(axios, "request").mockRejectedValue(
      new AxiosError("ECONNREFUSED", "ERR_NETWORK"),
    );
    await expect(request("/api/canvases/one")).rejects.toMatchObject({
      status: 0,
      code: "dependency_unavailable",
    });
  });
});
