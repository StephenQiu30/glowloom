import axios, {
  AxiosHeaders,
  type AxiosRequestConfig,
  type RawAxiosHeaders,
} from "axios";

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    public readonly requestId?: string,
    public readonly meta?: Record<string, unknown>,
  ) {
    super(errorMessage(status, code));
    this.name = "ApiError";
  }
}

function errorMessage(status: number, code: string) {
  if (code === "revision_conflict")
    return "内容已被其他页面修改，请载入最新版本后重新操作。";
  if (code === "project_archived") return "项目已归档，当前内容只可查看。";
  if (code === "inflight_work")
    return "项目仍有正在处理或结果待确认的任务，请处理完成后重试。";
  if (code === "restore_expired") return "项目已超过恢复期限，当前不能恢复。";
  if (status === 401) return "服务拒绝了此请求，请重新读取后重试。";
  if (status === 403) return "当前账号没有此操作的权限。";
  if (status === 404) return "请求的内容不存在，或当前账号不可访问。";
  if (status === 409) return "操作与当前服务端状态冲突，请重新读取后操作。";
  if (status === 429) return "操作过于频繁，请稍后重试。";
  if (status === 400 || status === 422)
    return "输入未通过服务端校验，请检查填写内容。";
  if (status === 0 || status >= 500)
    return "服务或依赖暂时不可用。修改尚未确认保存，请稍后重试。";
  return "请求未完成，请稍后重试。";
}

export type RequestOptions = Omit<AxiosRequestConfig, "headers"> & {
  headers?: RawAxiosHeaders | AxiosHeaders;
  requestType?: string;
};

// 生成客户端和普通 HTTP 的唯一入口。浏览器仅通过同源 /api 连接 Go。
export async function request<T>(
  url: string,
  options: RequestOptions = {},
): Promise<T> {
  const method = (options.method ?? "GET").toUpperCase();
  const target = new URL(
    url,
    typeof window === "undefined" ? "http://localhost" : window.location.origin,
  );
  const sameOrigin =
    typeof window === "undefined"
      ? url.startsWith("/")
      : target.origin === window.location.origin;
  const backend = sameOrigin && target.pathname.startsWith("/api/");
  const headers = AxiosHeaders.from(options.headers);
  // openapi2ts 的标准模板把 header 参数合入 params；在唯一入口归位。
  const params =
    options.params && Object.getPrototypeOf(options.params) === Object.prototype
      ? ({ ...options.params } as Record<string, unknown>)
      : options.params;
  if (params && Object.getPrototypeOf(params) === Object.prototype) {
    if (
      backend &&
      typeof params["Idempotency-Key"] === "string" &&
      !headers.has("Idempotency-Key")
    )
      headers.set("Idempotency-Key", params["Idempotency-Key"]);
    delete params["Idempotency-Key"];
  }
  const formData =
    typeof FormData !== "undefined" && options.data instanceof FormData;
  if (formData) {
    // 浏览器/适配器负责与实际 body 一致的 boundary；生成器的无 boundary 头不能沿用。
    headers.delete("Content-Type");
  } else if (backend && !["GET", "HEAD", "OPTIONS"].includes(method)) {
    headers.set("Content-Type", "application/json");
  }
  if (!backend) {
    headers.delete("Authorization");
    headers.delete("Cookie");
    headers.delete("Idempotency-Key");
  }
  const { requestType: _requestType, ...config } = options;
  void _requestType;
  try {
    const response = await axios.request<T>({
      timeout: 15_000,
      ...config,
      url,
      method,
      headers: headers.toJSON(),
      params,
      withCredentials: false,
      withXSRFToken: false,
    });
    return response.data;
  } catch (error) {
    if (axios.isCancel(error)) throw error;
    if (!axios.isAxiosError(error))
      throw new ApiError(0, "dependency_unavailable");
    const response = error.response;
    const problem = response?.data as Record<string, unknown> | undefined;
    const status = response?.status ?? 0;
    throw new ApiError(
      status,
      typeof problem?.code === "string"
        ? problem.code
        : status
          ? "request_failed"
          : "dependency_unavailable",
      typeof problem?.request_id === "string" ? problem.request_id : undefined,
      problem?.meta && typeof problem.meta === "object"
        ? (problem.meta as Record<string, unknown>)
        : undefined,
    );
  }
}
