import axios, { AxiosError } from "axios";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, request } from "./request";

afterEach(() => {
  vi.restoreAllMocks();
  document.cookie = "lv_csrf=; Max-Age=0; Path=/";
});
describe("公共请求入口", () => {
  it("同源写请求附会话/CSRF并保留调用方的稳定幂等键", async () => {
    document.cookie = "lv_csrf=test-csrf; Path=/";
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
        withCredentials: true,
        headers: expect.objectContaining({
          "X-CSRF-Token": "test-csrf",
          "Idempotency-Key": "same-request",
          "Content-Type": "application/json",
        }),
      }),
    );
  });
  it("登录豁免CSRF，预签名上传不发送后端凭据头", async () => {
    document.cookie = "lv_csrf=test-csrf; Path=/";
    const send = vi.spyOn(axios, "request").mockResolvedValue({ data: {} });
    await request("/api/auth/login", {
      method: "POST",
      data: { login_name: "test" },
    });
    expect(send.mock.calls[0][0].headers).not.toHaveProperty("X-CSRF-Token");
    await request("https://objects.example.test/upload", {
      method: "PUT",
      data: "file",
      headers: { "X-CSRF-Token": "bad", Authorization: "bad" },
    });
    expect(send.mock.calls[1][0].withCredentials).toBe(false);
    expect(send.mock.calls[1][0].headers).not.toHaveProperty("X-CSRF-Token");
    expect(send.mock.calls[1][0].headers).not.toHaveProperty("Authorization");
  });
  it("生成器的header参数归位，不让CSRF或幂等键进入query", async () => {
    document.cookie = "lv_csrf=test-csrf; Path=/";
    const send = vi.spyOn(axios, "request").mockResolvedValue({ data: {} });
    await request("/api/canvases/one/commands", {
      method: "POST",
      params: {
        "Idempotency-Key": "stable-key",
        "X-CSRF-Token": "",
        cursor: "next",
      },
      data: {},
    });
    expect(send.mock.calls[0][0].headers).toMatchObject({
      "Idempotency-Key": "stable-key",
      "X-CSRF-Token": "test-csrf",
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
