import { RequestError } from "@/lib/request";

export function authErrorMessage(error: unknown) {
  const code =
    error instanceof RequestError ? error.code : "request_unconfirmed";
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
    dependency_unavailable:
      "登录服务暂时不可用，请稍后重试。你仍可继续浏览灵感。",
    request_unconfirmed: "暂时无法连接，请求结果未确认，请检查网络后重试。",
  };
  return messages[code] ?? "请求结果未确认，请稍后重试或重新登录。";
}

export function requireSession(value: unknown): API.SessionView {
  const session = value as Partial<API.SessionView> | null;
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
    throw new RequestError("invalid_receipt");
  return session as API.SessionView;
}
