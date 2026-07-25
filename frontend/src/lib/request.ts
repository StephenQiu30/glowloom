export type RequestParameter = string | number | boolean | null | undefined;

export interface RequestOptions extends Omit<RequestInit, "body" | "method"> {
  method?: "GET" | "POST" | "PUT";
  params?: Record<string, RequestParameter | RequestParameter[]>;
  data?: unknown;
}

export class ApiRequestError extends Error {
  constructor(
    readonly status: number,
    readonly problem: unknown,
  ) {
    super(`API request failed with status ${status}`);
    this.name = "ApiRequestError";
  }
}

function apiUrl(path: string, params?: RequestOptions["params"]) {
  const baseUrl = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://127.0.0.1:8000";
  const url = new URL(path, baseUrl);
  for (const [name, raw] of Object.entries(params ?? {})) {
    const values = Array.isArray(raw) ? raw : [raw];
    for (const value of values) {
      if (value !== null && value !== undefined) {
        url.searchParams.append(name, String(value));
      }
    }
  }
  return url;
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { data, params, headers: inputHeaders, ...init } = options;
  const headers = new Headers(inputHeaders);
  if (data !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(apiUrl(path, params), {
    ...init,
    headers,
    body: data === undefined ? undefined : JSON.stringify(data),
    cache: "no-store",
  });
  const isJson = response.headers.get("content-type")?.includes("json") ?? false;
  const payload: unknown = isJson ? await response.json() : await response.text();
  if (!response.ok) {
    throw new ApiRequestError(response.status, payload);
  }
  return payload as T;
}
