import axios, { type AxiosRequestConfig } from "axios";

export type RequestOptions = AxiosRequestConfig;

type ErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
    next_action?: string;
  };
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
  timeout: 15_000,
  headers: { "Content-Type": "application/json" },
});

export default async function request<T>(
  url: string,
  options: RequestOptions = {},
): Promise<T> {
  try {
    const response = await client.request<T>({ ...options, url });
    return response.data;
  } catch (cause: unknown) {
    if (axios.isAxiosError<ErrorEnvelope>(cause)) {
      const error = cause.response?.data.error;
      throw new ApiClientError(
        error?.message ?? "服务暂时不可用，请确认 API 和 Worker 已启动。",
        error?.code,
        error?.next_action,
      );
    }
    throw cause;
  }
}
