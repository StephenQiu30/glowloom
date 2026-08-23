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

type AuthEnvelope = {
  data?: {
    access_token?: string;
  };
};

type RetriableRequest = AxiosRequestConfig & {
  _lanverseRetried?: boolean;
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
  withCredentials: true,
  headers: { "Content-Type": "application/json" },
});

let accessToken: string | undefined;
let refreshPromise: Promise<string> | null = null;

export function setAccessToken(token?: string) {
  accessToken = token?.trim() || undefined;
}

client.interceptors.request.use((config) => {
  const requestID = config.headers?.["X-Request-Id"] ?? globalThis.crypto?.randomUUID?.();
  if (requestID) config.headers["X-Request-Id"] = requestID;
  if (accessToken) config.headers.Authorization = `Bearer ${accessToken}`;
  return config;
});

async function refreshAccessToken() {
  if (refreshPromise) return refreshPromise;
  const current = client
    .post<AuthEnvelope>("/api/auth/refresh")
    .then((response) => {
      const token = response.data.data?.access_token;
      if (!token) throw new ApiClientError("刷新响应缺少访问令牌", "session_invalid", 401);
      setAccessToken(token);
      return token;
    })
    .catch((cause) => {
      setAccessToken();
      throw cause;
    });
  refreshPromise = current;
  try {
    return await current;
  } finally {
    if (refreshPromise === current) refreshPromise = null;
  }
}

client.interceptors.response.use(undefined, async (cause: unknown) => {
  if (!axios.isAxiosError(cause) || cause.response?.status !== 401) throw cause;
  const config = cause.config as RetriableRequest | undefined;
  if (!config || config._lanverseRetried || config.url?.startsWith("/api/auth/")) throw cause;
  config._lanverseRetried = true;
  const token = await refreshAccessToken();
  config.headers = { ...config.headers, Authorization: `Bearer ${token}` };
  return client.request(config);
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
