import axios, { type AxiosRequestConfig } from "axios";

import { clearAccessToken, getAccessToken } from "@/lib/auth-session";

export type RequestOptions = AxiosRequestConfig;

type ApiErrorEnvelope = {
  error?: { code?: string; message?: string; next_action?: string };
};

export class ApiClientError extends Error {
  readonly code: string;
  readonly nextAction?: string;

  constructor(message: string, code = "request_failed", nextAction?: string) {
    super(message);
    this.name = "ApiClientError";
    this.code = code;
    this.nextAction = nextAction;
  }
}

const client = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://127.0.0.1:8686",
  timeout: 10_000,
});

export default async function request<T>(
  url: string,
  options: RequestOptions = {},
): Promise<T> {
  try {
    const accessToken = getAccessToken();
    const response = await client.request<T>({
      ...options,
      url,
      headers: accessToken
        ? { ...options.headers, Authorization: `Bearer ${accessToken}` }
        : options.headers,
    });
    return response.data;
  } catch (cause: unknown) {
    if (axios.isAxiosError<ApiErrorEnvelope>(cause)) {
      if (cause.response?.status === 401) clearAccessToken();
      const error = cause.response?.data.error;
      throw new ApiClientError(
        error?.message ?? "服务暂时不可用，请稍后重试。",
        error?.code,
        error?.next_action,
      );
    }
    throw cause;
  }
}
