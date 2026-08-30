import axios, { type AxiosRequestConfig } from "axios";

import {
  clearAccessToken,
  getAccessToken,
  setAccessToken,
} from "@/lib/auth-session";

export type RequestOptions = AxiosRequestConfig & {
  skipAuthRefresh?: boolean;
};

type ApiErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
    next_action?: string;
    details?: unknown;
  };
};

export class ApiClientError extends Error {
  readonly code: string;
  readonly nextAction?: string;
  readonly details?: unknown;

  constructor(
    message: string,
    code = "request_failed",
    nextAction?: string,
    details?: unknown,
  ) {
    super(message);
    this.name = "ApiClientError";
    this.code = code;
    this.nextAction = nextAction;
    this.details = details;
  }
}

const client = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://127.0.0.1:8686",
  timeout: 10_000,
  withCredentials: true,
});

let refreshPromise: Promise<string | null> | null = null;

export async function refreshAccessToken(): Promise<string | null> {
  if (refreshPromise) return refreshPromise;
  refreshPromise = client
    .post<{ data: API.AuthResponse }>(
      "/api/auth/refresh",
      undefined,
      { withCredentials: true },
    )
    .then((response) => {
      const token = response.data.data.access_token;
      setAccessToken(token);
      return token;
    })
    .catch(() => {
      clearAccessToken();
      return null;
    })
    .finally(() => {
      refreshPromise = null;
    });
  return refreshPromise;
}

const AUTH_REFRESH_EXCLUDED_PATHS = new Set([
  "/api/auth/login",
  "/api/auth/register",
  "/api/auth/refresh",
]);

export default async function request<T>(
  url: string,
  options: RequestOptions = {},
): Promise<T> {
  const { skipAuthRefresh = false, ...axiosOptions } = options;
  try {
    const accessToken = getAccessToken();
    const response = await client.request<T>({
      ...axiosOptions,
      url,
      headers: accessToken
        ? { ...axiosOptions.headers, Authorization: `Bearer ${accessToken}` }
        : axiosOptions.headers,
    });
    return response.data;
  } catch (cause: unknown) {
    if (axios.isAxiosError<ApiErrorEnvelope>(cause)) {
      if (
        cause.response?.status === 401 &&
        !skipAuthRefresh &&
        !AUTH_REFRESH_EXCLUDED_PATHS.has(url)
      ) {
        const refreshed = await refreshAccessToken();
        if (refreshed) {
          return request(url, { ...options, skipAuthRefresh: true });
        }
      }
      if (cause.response?.status === 401) clearAccessToken();
      const error = cause.response?.data.error;
      throw new ApiClientError(
        error?.message ?? "服务暂时不可用，请稍后重试。",
        error?.code,
        error?.next_action,
        error?.details,
      );
    }
    throw cause;
  }
}
