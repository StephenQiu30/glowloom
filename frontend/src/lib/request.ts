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

let csrfToken: string | null = null;
let sessionGeneration = 0;
const sessionInvalidationListeners = new Set<(generation: number) => void>();
export function getSessionGeneration() {
  return sessionGeneration;
}
export function subscribeSessionInvalidation(
  listener: (generation: number) => void,
) {
  sessionInvalidationListeners.add(listener);
  return () => {
    sessionInvalidationListeners.delete(listener);
  };
}
/** A delayed response can invalidate only the exact identity generation that sent it. */
export function notifySessionInvalidation(generation: number) {
  if (generation !== sessionGeneration || !csrfToken) return;
  for (const listener of [...sessionInvalidationListeners])
    listener(generation);
}

let creationScope: string | null = null;
let scopeController = new AbortController();
let scopeSwitching = false;
const scopeDeniedListeners = new Set<() => void>();
export function setScopeSwitching(value: boolean) {
  scopeSwitching = value;
}
export function subscribeScopeDenied(listener: () => void) {
  scopeDeniedListeners.add(listener);
  return () => {
    scopeDeniedListeners.delete(listener);
  };
}
export function getCreationScope() {
  return creationScope;
}
export function setCreationScope(scope: string | null) {
  if (creationScope !== scope) {
    scopeController.abort();
    scopeController = new AbortController();
  }
  creationScope = scope;
}
/** Session-bound proof stays in memory and is cleared on identity loss. */
export function setCSRFToken(token: string | null) {
  if (csrfToken !== token) {
    setCreationScope(null);
    scopeSwitching = false;
    sessionGeneration++;
  }
  csrfToken = token;
}

const client = axios.create({
  timeout: 15_000,
  // Same-origin cookies work without enabling cross-origin credentials.
  withCredentials: false,
});

client.interceptors.request.use((config) => {
  if (config.data instanceof FormData) config.headers.delete("Content-Type");
  const method = config.method?.toUpperCase() ?? "GET";
  const snapshot = (
    config as typeof config & {
      identityProof?: { csrf: string | null; scope: string | null };
    }
  ).identityProof;
  if (snapshot?.scope) config.headers.set("X-Creation-Scope", snapshot.scope);
  if (snapshot?.csrf && !["GET", "HEAD", "OPTIONS"].includes(method))
    config.headers.set("X-CSRF-Token", snapshot.csrf);
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
  const method = config.method?.toUpperCase() ?? "GET";
  if (scopeSwitching && !["GET", "HEAD", "OPTIONS"].includes(method))
    throw new RequestError("scope_switch_pending", 409);
  const scopeSignal = scopeController.signal;
  const signal = config.signal
    ? AbortSignal.any([config.signal as AbortSignal, scopeSignal])
    : scopeSignal;
  const generation = sessionGeneration;
  const proof = csrfToken;
  try {
    const outgoing = {
      ...config,
      url,
      signal,
      identityProof: { csrf: proof, scope: creationScope },
    };
    const response = await client.request<T>(outgoing);
    if (scopeSignal.aborted || generation !== sessionGeneration)
      throw new axios.CanceledError("identity_or_scope_changed");
    return response.status === 204 ? (undefined as T) : response.data;
  } catch (error) {
    if (
      creationScope &&
      !scopeSignal.aborted &&
      generation === sessionGeneration &&
      url !== "/api/me/spaces" &&
      error instanceof RequestError &&
      error.status === 403
    )
      for (const listener of [...scopeDeniedListeners]) listener();
    const publicAuth =
      /^\/api\/(auth-policy|invitations\/preview|registrations(?:\/|$)|password-resets(?:\/|$))/.test(
        url,
      ) ||
      (url === "/api/sessions" && config.method?.toUpperCase() === "POST");
    const rejectedPasswordProof =
      url === "/api/me/reauthentication" &&
      error instanceof RequestError &&
      error.code === "invalid_credentials";
    if (
      proof &&
      !publicAuth &&
      !rejectedPasswordProof &&
      error instanceof RequestError &&
      error.status === 401
    )
      notifySessionInvalidation(generation);
    throw error;
  }
}
