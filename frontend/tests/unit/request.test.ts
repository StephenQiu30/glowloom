import {
  AxiosError,
  type AxiosAdapter,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from "axios";
import { afterEach, describe, expect, it } from "vitest";

import { clearAccessToken, getAccessToken, setAccessToken } from "@/lib/auth-session";
import request from "@/lib/request";

function response<T>(
  config: InternalAxiosRequestConfig,
  data: T,
  status = 200,
): AxiosResponse<T> {
  return { config, data, headers: {}, status, statusText: String(status) };
}

afterEach(() => clearAccessToken());

describe("request", () => {
  it("uses the API instance and forwards native Axios options", async () => {
    setAccessToken("session-token");
    const controller = new AbortController();
    const adapter: AxiosAdapter = async (config) => {
      expect(config.baseURL).toBe("http://127.0.0.1:8686");
      expect(config.headers.get("Authorization")).toBe("Bearer session-token");
      expect(config.withCredentials).toBe(true);
      expect(config.signal).toBe(controller.signal);
      expect(config.timeout).toBe(12_345);
      return response(config, { ok: true });
    };

    await expect(
      request("/api/v1/example", {
        adapter,
        signal: controller.signal,
        timeout: 12_345,
      }),
    ).resolves.toEqual({ ok: true });
  });

  it("maps the backend error envelope and clears an unauthorized session", async () => {
    setAccessToken("expired-token");
    const adapter: AxiosAdapter = async (config) => {
      throw new AxiosError(
        "Unauthorized",
        AxiosError.ERR_BAD_REQUEST,
        config,
        undefined,
        response(
          config,
          {
            error: {
              code: "unauthenticated",
              message: "登录状态已失效。",
              next_action: "login",
              details: { review_decision_id: "decision-1" },
            },
          },
          401,
        ),
      );
    };

    await expect(request("/api/v1/me", { adapter })).rejects.toMatchObject({
      code: "unauthenticated",
      message: "登录状态已失效。",
      nextAction: "login",
      details: { review_decision_id: "decision-1" },
    });
    expect(getAccessToken()).toBeNull();
  });
});
