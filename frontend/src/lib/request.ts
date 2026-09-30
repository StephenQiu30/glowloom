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
    return "画布已被其他页面修改，请载入最新版本后重新操作。";
  if (code === "must_change_password")
    return "请先修改初始密码，再打开真实画布。";
  if (code === "project_archived") return "项目已归档，当前画布只可查看。";
  if (code === "invalid_credentials" || code === "login_failed")
    return "账号或密码不正确。";
  if (status === 401) return "会话已失效，请重新登录。";
  if (status === 403) return "当前账号没有此操作的权限。";
  if (status === 404) return "项目或画布不存在，或当前账号不可访问。";
  if (status === 409) return "操作与当前服务端状态冲突，请重新读取后操作。";
  if (status === 429) return "操作过于频繁，请稍后重试。";
  if (status === 400 || status === 422)
    return "输入未通过服务端校验，请检查备注和布局。";
  if (status === 0 || status >= 500)
    return "服务或依赖暂时不可用。修改尚未确认保存，请稍后重试。";
  return "请求未完成，请稍后重试。";
}

function csrfCookie() {
  if (typeof document === "undefined") return undefined;
  const value = document.cookie
    .split(";")
    .map((item) => item.trim())
    .find((item) => item.startsWith("lv_csrf="));
  if (!value) return undefined;
  try {
    return decodeURIComponent(value.slice("lv_csrf=".length));
  } catch {
    return undefined;
  }
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
    delete params["X-CSRF-Token"];
  }
  if (backend && !["GET", "HEAD", "OPTIONS"].includes(method)) {
    headers.set("Content-Type", "application/json");
    if (target.pathname !== "/api/auth/login") {
      const csrf = csrfCookie();
      if (csrf) headers.set("X-CSRF-Token", csrf);
    }
  }
  if (!backend || target.pathname === "/api/auth/login")
    headers.delete("X-CSRF-Token");
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
      withCredentials: backend,
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
