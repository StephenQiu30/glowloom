import axios, { type AxiosRequestConfig } from "axios";
import { developmentResponse } from "./development-transport";

export type RequestOptions = Omit<AxiosRequestConfig, "url"> & {
  /** Emitted by @umijs/openapi for multipart uploads. */
  requestType?: "form";
  sessionRecovery?: "auto" | "never";
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
export function getCSRFToken() {
  return csrfToken;
}
let recoveryHandler: ((signal: AbortSignal) => Promise<void>) | null = null;
let recoveryFlight: Promise<void> | null = null;
let recoveryRevision = 0;
let pendingRenewal: Promise<unknown> | null = null;
/** Consumer port: every renewal source publishes its pending work before writes. */
export function trackSessionRenewal(pending: Promise<unknown> | null) {
  pendingRenewal = pending;
  if (!pending) return;
  void pending
    .then(
      () => {
        ++recoveryRevision;
      },
      () => {},
    )
    .finally(() => {
      if (pendingRenewal === pending) pendingRenewal = null;
    });
}
export function setSessionRecovery(handler: typeof recoveryHandler) {
  recoveryHandler = handler;
  recoveryFlight = null;
}
function recoverSession() {
  if (!recoveryFlight && recoveryHandler) {
    const pending = recoveryHandler(new AbortController().signal).then(() => {
      ++recoveryRevision;
    });
    recoveryFlight = pending;
    void pending
      .finally(() => {
        if (recoveryFlight === pending) recoveryFlight = null;
      })
      .catch(() => {});
  }
  return recoveryFlight;
}
export function waitForSession<T>(
  pending: Promise<T>,
  signal?: AbortSignal,
): Promise<T> {
  if (!signal) return pending;
  if (signal.aborted) return Promise.reject(new axios.CanceledError());
  return new Promise((resolve, reject) => {
    const cancel = () => reject(new axios.CanceledError());
    signal.addEventListener("abort", cancel, { once: true });
    pending
      .then(resolve, reject)
      .finally(() => signal.removeEventListener("abort", cancel));
  });
}
const contextChangeListeners = new Set<() => void>();
export function subscribeContextChange(listener: () => void) {
  contextChangeListeners.add(listener);
  return () => {
    contextChangeListeners.delete(listener);
  };
}
function releaseContextMemory() {
  for (const listener of contextChangeListeners) listener();
}
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
let scopeGeneration = 0;
export function getScopeGeneration() {
  return scopeGeneration;
}
export function sessionCancellation() {
  return new axios.CanceledError("identity_or_scope_changed");
}
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
    ++scopeGeneration;
    releaseContextMemory();
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
    releaseContextMemory();
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
  const local = developmentResponse(url, options);
  if (local) return (await local) as T;
  const { requestType, sessionRecovery = "auto", ...config } = options;
  if (requestType === "form") {
    config.timeout ??= 120_000;
  }
  const method = config.method?.toUpperCase() ?? "GET";
  if (scopeSwitching && !["GET", "HEAD", "OPTIONS"].includes(method))
    throw new RequestError("scope_switch_pending", 409);
  const publicAuth =
    /^\/api\/(auth-policy|invitations\/preview|registrations(?:\/|$)|password-resets(?:\/|$)|session\/(refresh|upgrade)(?:\/|$))/.test(
      url,
    ) ||
    (url === "/api/sessions" && method === "POST");
  const canRecover = sessionRecovery === "auto" && !publicAuth;
  const scopeSignal = scopeController.signal;
  const signal = config.signal
    ? AbortSignal.any([config.signal as AbortSignal, scopeSignal])
    : scopeSignal;
  const generation = sessionGeneration;
  const proof = csrfToken;
  const observedRecovery = recoveryRevision;
  try {
    const waiting = pendingRenewal ?? recoveryFlight;
    if (canRecover && waiting) {
      await waitForSession(waiting, signal);
      if (signal.aborted || generation !== sessionGeneration)
        throw new axios.CanceledError("identity_or_scope_changed");
    }
    const outgoing = {
      ...config,
      url,
      signal,
      identityProof: { csrf: proof, scope: creationScope },
    };
    const response = await client.request<T>(outgoing);
    if (
      signal.aborted ||
      scopeSignal.aborted ||
      generation !== sessionGeneration
    )
      throw new axios.CanceledError("identity_or_scope_changed");
    return response.status === 204 ? (undefined as T) : response.data;
  } catch (error) {
    if (
      signal.aborted ||
      scopeSignal.aborted ||
      generation !== sessionGeneration
    )
      throw new axios.CanceledError("identity_or_scope_changed");
    if (
      creationScope &&
      !scopeSignal.aborted &&
      generation === sessionGeneration &&
      url !== "/api/me/spaces" &&
      error instanceof RequestError &&
      error.status === 403 &&
      ![
        "origin_forbidden",
        "origin_mismatch",
        "csrf_invalid",
        "csrf_required",
        "session_invalid",
        "session_cookie_conflict",
      ].includes(error.code)
    )
      for (const listener of [...scopeDeniedListeners]) listener();
    if (
      canRecover &&
      error instanceof RequestError &&
      error.status === 401 &&
      error.code === "access_expired" &&
      recoveryHandler
    ) {
      if (observedRecovery === recoveryRevision)
        await waitForSession(recoverSession()!, signal);
      if (
        signal.aborted ||
        scopeSignal.aborted ||
        generation !== sessionGeneration
      )
        throw new axios.CanceledError("identity_or_scope_changed");
      if (["GET", "HEAD"].includes(method))
        return request<T>(url, { ...options, sessionRecovery: "never" });
      throw error;
    }
    const rejectedPasswordProof =
      ["/api/me/reauthentication", "/api/me/password"].includes(url) &&
      error instanceof RequestError &&
      ["invalid_credentials", "current_password_invalid"].includes(error.code);
    if (
      proof &&
      !publicAuth &&
      !rejectedPasswordProof &&
      error instanceof RequestError &&
      error.status === 401 &&
      error.code !== "access_expired"
    )
      notifySessionInvalidation(generation);
    throw error;
  }
}
