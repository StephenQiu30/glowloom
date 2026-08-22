import axios, { type AxiosRequestConfig } from "axios";

export type RequestOptions = AxiosRequestConfig;

type ErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
    next_action?: string;
    request_id?: string;
    details?: unknown;
    recovery_actions?: Array<{ code: string; label: string }>;
  };
};

export class ApiClientError extends Error {
  readonly code: string;
  readonly status?: number;
  readonly requestID?: string;
  readonly details?: unknown;
  readonly recoveryActions: Array<{ code: string; label: string }>;

  constructor(message: string, code = "request_failed", status?: number, requestID?: string, recoveryActions: Array<{ code: string; label: string }> = [], details?: unknown) {
    super(message);
    this.name = "ApiClientError";
    this.code = code;
    this.status = status;
    this.requestID = requestID;
    this.recoveryActions = recoveryActions;
    this.details = details;
  }
}

const client = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://127.0.0.1:8686",
  timeout: 15_000,
  headers: { "Content-Type": "application/json" },
});

client.interceptors.request.use((config) => {
	const requestID = config.headers?.["X-Request-Id"] ?? globalThis.crypto?.randomUUID?.();
	if (requestID) config.headers["X-Request-Id"] = requestID;
	if (typeof window !== "undefined") {
    const token = window.localStorage.getItem("lanverse.session_token");
    const workspaceID = window.localStorage.getItem("lanverse.workspace_id");
    if (token) config.headers.Authorization = `Bearer ${token}`;
    if (workspaceID) config.headers["X-Workspace-Id"] = workspaceID;
  }
  return config;
});

export default async function request<T>(url: string, options: RequestOptions = {}): Promise<T> {
  try {
    const response = await client.request<T>({ ...options, url });
    return response.data;
  } catch (cause: unknown) {
    if (axios.isAxiosError<ErrorEnvelope>(cause)) {
      const error = cause.response?.data.error;
      throw new ApiClientError(
        error?.message ?? "服务暂时不可用，请确认 API 和 Worker 已启动。",
        error?.code,
        cause.response?.status,
        error?.request_id,
        error?.recovery_actions,
        error?.details,
      );
    }
    throw cause;
  }
}
