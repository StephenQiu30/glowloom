import axios, { type AxiosRequestConfig } from "axios";

export type RequestOptions = Omit<AxiosRequestConfig, "url"> & {
  /** Emitted by @umijs/openapi for multipart uploads. */
  requestType?: "form";
};

export class RequestError extends Error {
  constructor(
    public readonly code: string,
    public readonly status = 0,
    public readonly meta?: Record<string, unknown>,
  ) {
    super(code);
    this.name = "RequestError";
  }
}

const client = axios.create({
  timeout: 15_000,
  // Same-origin cookies work without enabling cross-origin credentials.
  withCredentials: false,
});

client.interceptors.request.use((config) => {
  if (config.data instanceof FormData) config.headers.delete("Content-Type");
  const method = config.method?.toUpperCase() ?? "GET";
  if (
    !["GET", "HEAD", "OPTIONS"].includes(method) &&
    !config.headers.has("Idempotency-Key")
  ) {
    config.headers.set("Idempotency-Key", crypto.randomUUID());
  }
  return config;
});

client.interceptors.response.use(undefined, async (error: unknown) => {
  // Preserve Axios cancellation so callers can discard obsolete work.
  if (axios.isCancel(error)) throw error;
  if (!axios.isAxiosError(error)) throw error;
  let data: unknown = error.response?.data;
  if (typeof Blob !== "undefined" && data instanceof Blob) {
    data = await data.text();
  }
  if (typeof data === "string") {
    try {
      data = JSON.parse(data);
    } catch {
      data = null;
    }
  }
  const problem = data as Partial<API.Problem> | null;
  throw new RequestError(
    typeof problem?.code === "string" ? problem.code : "request_unconfirmed",
    error.response?.status ?? 0,
    problem?.meta && typeof problem.meta === "object"
      ? problem.meta
      : undefined,
  );
});

/** Transport for generated clients; returns the backend payload unchanged. */
export async function request<T>(
  url: string,
  options: RequestOptions = {},
): Promise<T> {
  const { requestType, ...config } = options;
  if (requestType === "form") {
    config.timeout ??= 120_000;
  }
  const response = await client.request<T>({ ...config, url });
  return response.status === 204 ? (undefined as T) : response.data;
}
