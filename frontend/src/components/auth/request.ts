export type RequestOptions = Omit<RequestInit, "body"> & {
  data?: unknown;
  params?: Record<string, string>;
};

export class AuthError extends Error {
  constructor(
    public readonly code: string,
    public readonly status = 0,
  ) {
    super(authErrorMessage(code));
  }
}
export function authErrorMessage(code: string) {
  const messages: Record<string, string> = {
    login_name_taken: "登录名已被使用，请更换后重试。",
    invalid_credentials:
      "登录名或密码不正确。滚动 15 分钟内错误 5 次会锁定 15 分钟。",
    current_password_invalid: "当前密码不正确。",
    weak_password: "密码需至少 10 个字符，包含字母和数字，且不超过 72 字节。",
    password_reused: "新密码不能与当前密码相同。",
    credential_conflict: "账号凭据已更新，请重新登录。",
    invalid_request: "输入不符合规则，请检查登录名、显示名与密码。",
    session_invalid: "登录已失效，请重新登录。",
    forbidden: "当前账号无权执行此操作。",
    invalid_receipt: "服务端回执不完整，结果未确认，请刷新或重新登录。",
  };
  return messages[code] ?? "请求结果未确认，请稍后重试或重新登录。";
}

// Generated calls use one same-origin transport; credentials never enter storage.
export async function request<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { data, params, ...init } = options;
  const query = new URLSearchParams(params).toString();
  const headers = new Headers(init.headers);
  if (init.method && init.method !== "GET")
    headers.set("Idempotency-Key", crypto.randomUUID());
  if (data !== undefined) headers.set("Content-Type", "application/json");
  let response: Response;
  try {
    response = await fetch(path + (query ? `?${query}` : ""), {
      ...init,
      headers,
      body: data === undefined ? undefined : JSON.stringify(data),
      credentials: "same-origin",
      cache: "no-store",
      signal: init.signal
        ? AbortSignal.any([init.signal, AbortSignal.timeout(15000)])
        : AbortSignal.timeout(15000),
    });
  } catch (error) {
    if (init.signal?.aborted) throw error;
    throw new AuthError("request_unconfirmed");
  }
  if (response.status === 204) return undefined as T;
  const value: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    const problem = value as Partial<AuthAPI.Problem> | null;
    throw new AuthError(
      typeof problem?.code === "string" ? problem.code : "request_unconfirmed",
      response.status,
    );
  }
  return value as T;
}

export function requireSession(value: unknown): AuthAPI.SessionView {
  const session = value as Partial<AuthAPI.SessionView> | null;
  const actor = session?.actor;
  const uuid =
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
  if (
    !actor ||
    typeof actor.id !== "string" ||
    !uuid.test(actor.id) ||
    typeof session?.session_id !== "string" ||
    !uuid.test(session.session_id) ||
    typeof actor.login_name !== "string" ||
    typeof actor.display_name !== "string" ||
    (actor.role !== "creator" && actor.role !== "admin") ||
    typeof actor.must_change_password !== "boolean" ||
    !Number.isSafeInteger(actor.credential_revision) ||
    actor.credential_revision < 1 ||
    typeof session.persistent !== "boolean" ||
    typeof session.last_active_at !== "string" ||
    !Number.isFinite(Date.parse(session.last_active_at)) ||
    typeof session.absolute_expires_at !== "string" ||
    !Number.isFinite(Date.parse(session.absolute_expires_at))
  )
    throw new AuthError("invalid_receipt");
  return session as AuthAPI.SessionView;
}
