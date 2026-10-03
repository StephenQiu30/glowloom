import axios, {
  AxiosHeaders,
  type AxiosRequestConfig,
  type AxiosResponse,
} from "axios";
import { backendApiOrigin } from "./backend-api-origin";

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

export type RequestOptions = AxiosRequestConfig<unknown> & {
  // Umi OpenAPI 的模板保留此标记；实际 FormData 交由 Axios 处理。
  requestType?: "json" | "form";
  onResponse?: (
    response: Pick<AxiosResponse<unknown>, "status" | "headers">,
  ) => void;
};

const streamContextKey = Symbol("stream-response-context");
type StreamContext = {
  fetch: NonNullable<NonNullable<RequestOptions["env"]>["fetch"]>;
  validateStatus: RequestOptions["validateStatus"];
  accepted?: boolean;
};
type StreamFetchOptions = RequestInit & {
  [streamContextKey]?: () => StreamContext;
};

// Axios 缓存 fetch adapter；固定函数身份，避免每次请求留下一个缓存实例。
async function fetchStreamResponse(
  input: URL | Request | string,
  init?: StreamFetchOptions,
): Promise<Response> {
  const { [streamContextKey]: getContext, ...fetchOptions } = init ?? {};
  const context = getContext?.();
  const response = await (context?.fetch ?? globalThis.fetch)(
    input,
    fetchOptions,
  );
  if (response.status && context?.validateStatus) {
    context.accepted = context.validateStatus(response.status);
    // Axios 包装流之后取消可能等待未完成的读取，先释放原始错误正文。
    if (!context.accepted) void response.body?.cancel().catch(() => {});
  }
  return response;
}

const client = axios.create({
  timeout: 15_000,
  withCredentials: false,
  withXSRFToken: false,
});

// Axios 合并配置后再处理请求头，避免修改调用方的 AxiosHeaders 或漏掉默认头。
client.interceptors.request.use((config) => {
  const origin =
    typeof window === "undefined"
      ? backendApiOrigin(process.env.LV_API_BASE_URL)
      : window.location.origin;
  const target = new URL(
    client.getUri({
      url: config.url,
      baseURL: config.baseURL,
      allowAbsoluteUrls: config.allowAbsoluteUrls,
    }),
    origin,
  );
  const backend =
    target.origin === origin && target.pathname.startsWith("/api/");
  if (!backend) {
    if (target.username || target.password)
      throw new ApiError(0, "dependency_unavailable");
    for (const name of [
      "Authorization",
      "Cookie",
      "Idempotency-Key",
      "X-CSRF-Token",
    ])
      config.headers.delete(name);
    config.auth = undefined;
  }

  if (config.adapter === "fetch" && config.responseType === "stream") {
    const validateStatus = config.validateStatus;
    const context: StreamContext = {
      fetch: config.env?.fetch ?? globalThis.fetch,
      validateStatus,
    };
    const fetchOptions: StreamFetchOptions = {
      ...config.fetchOptions,
      // Axios 会深复制配置中的普通对象；函数保留同一请求上下文。
      [streamContextKey]: () => context,
    };
    config.fetchOptions = fetchOptions;
    config.env = {
      ...config.env,
      fetch: fetchStreamResponse,
    };
    if (validateStatus)
      config.validateStatus = (status) =>
        context.accepted ?? validateStatus(status);
  }
  return config;
});

const maxProblemBytes = 64 * 1024;

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function responseHeader(
  response: AxiosResponse<unknown> | undefined,
  name: string,
): string | undefined {
  const headers = response?.headers;
  const value =
    headers instanceof AxiosHeaders
      ? headers.get(name)
      : Object.entries(headers ?? {}).find(
          ([key]) => key.toLowerCase() === name.toLowerCase(),
        )?.[1];
  return typeof value === "string" ? value : undefined;
}

// 二进制下载失败仍可能返回 Problem JSON，只解码有界的 JSON 错误正文。
async function readProblem(response: AxiosResponse<unknown> | undefined) {
  const data: unknown = response?.data;
  if (typeof ReadableStream !== "undefined" && data instanceof ReadableStream) {
    // 错误流不会交给消费者；释放正文，避免等待或积累无界的错误响应。
    void data.cancel().catch(() => {});
    return undefined;
  }
  if (
    isRecord(data) &&
    !(typeof Blob !== "undefined" && data instanceof Blob) &&
    !(data instanceof ArrayBuffer) &&
    !ArrayBuffer.isView(data)
  )
    return data;
  const contentType = responseHeader(response, "Content-Type")
    ?.split(";", 1)[0]
    .trim()
    .toLowerCase();
  if (contentType !== "application/json" && !contentType?.endsWith("+json"))
    return undefined;

  let text: string;
  try {
    if (typeof Blob !== "undefined" && data instanceof Blob) {
      if (data.size > maxProblemBytes) return undefined;
      text = await data.text();
    } else if (data instanceof ArrayBuffer || ArrayBuffer.isView(data)) {
      if (data.byteLength > maxProblemBytes) return undefined;
      const bytes =
        data instanceof ArrayBuffer
          ? new Uint8Array(data)
          : new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
      text = new TextDecoder().decode(bytes);
    } else if (typeof data === "string") {
      if (
        data.length > maxProblemBytes ||
        new TextEncoder().encode(data).byteLength > maxProblemBytes
      )
        return undefined;
      text = data;
    } else return undefined;
    const parsed: unknown = JSON.parse(text);
    return isRecord(parsed) ? parsed : undefined;
  } catch {
    return undefined;
  }
}

// 生成客户端调用此 TypeScript 服务；业务代码只调用 src/api 中的生成函数。
export async function request<T>(
  url: string,
  options: RequestOptions = {},
): Promise<T> {
  const { requestType: _requestType, onResponse, ...config } = options;
  void _requestType;
  let response: AxiosResponse<T>;
  try {
    response = await client.request<T, AxiosResponse<T>, unknown>({
      ...config,
      url,
      withCredentials: false,
      withXSRFToken: false,
    });
  } catch (error) {
    if (axios.isCancel(error) || error instanceof ApiError) throw error;
    if (!axios.isAxiosError<unknown>(error))
      throw new ApiError(0, "dependency_unavailable");
    const problem = await readProblem(error.response);
    const status = error.response?.status ?? 0;
    const requestId = responseHeader(error.response, "X-Request-Id");
    throw new ApiError(
      status,
      typeof problem?.code === "string"
        ? problem.code
        : status
          ? "request_failed"
          : "dependency_unavailable",
      typeof problem?.request_id === "string"
        ? problem.request_id
        : typeof requestId === "string"
          ? requestId
          : undefined,
      isRecord(problem?.meta) ? problem.meta : undefined,
    );
  }
  onResponse?.({ status: response.status, headers: response.headers });
  return response.data;
}

// 对象存储原件使用 Axios fetch adapter 返回 Web ReadableStream，取消由消费者管理。
export function readResourceStream(url: string, signal?: AbortSignal) {
  return request<ReadableStream<Uint8Array>>(url, {
    method: "GET",
    adapter: "fetch",
    timeout: 0,
    responseType: "stream",
    fetchOptions: { redirect: "error" },
    signal,
  });
}
