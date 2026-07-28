import axios, { type AxiosRequestConfig } from "axios";

export type RequestOptions = AxiosRequestConfig;

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
  baseURL: process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://127.0.0.1:8000",
  timeout: 10_000,
});

export default async function request<T>(
  url: string,
  options: RequestOptions = {},
): Promise<T> {
  try {
    const response = await client.request<T>({ url, ...options });
    return response.data;
  } catch (cause: unknown) {
    if (axios.isAxiosError(cause)) {
      const envelope = cause.response?.data as
        | { error?: { code?: string; message?: string; next_action?: string } }
        | undefined;
      throw new ApiClientError(
        envelope?.error?.message ?? "服务暂时不可用，请稍后重试。",
        envelope?.error?.code,
        envelope?.error?.next_action,
      );
    }
    throw cause;
  }
}
