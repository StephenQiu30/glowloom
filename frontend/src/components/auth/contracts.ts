import { RequestError } from "@/lib/request";

export const MIN_PASSWORD_LENGTH = 8;
export const MAX_PASSWORD_LENGTH = 128;
export const PASSWORD_LENGTH_HINT = `${MIN_PASSWORD_LENGTH}～${MAX_PASSWORD_LENGTH} 个字符`;

export function authErrorMessage(error: unknown) {
  const code =
    error instanceof RequestError ? error.code : "request_unconfirmed";
  const messages: Record<string, string> = {
    temporarily_unavailable: "服务暂时不可用，请稍后重试。",
    unauthenticated: "登录已失效，请重新登录。",
    request_too_large: "上传文件过大，请选择 5 MB 以内的头像图片。",
    unsupported_media_type: "请上传静态 JPEG、PNG 或 WebP 图片。",
    invalid_avatar_dimensions: "图片宽高不能超过 4096 像素，请缩小后上传。",
    scope_switch_pending: "正在切换创作空间，请稍后再操作。",
    email_unavailable: "该邮箱目前无法使用，请更换邮箱或重新检查。",
    team_revision_conflict: "团队资料已经变化，请重新读取后操作。",
    governance_conflict: "当前状态已发生变化，请重新读取后操作。",
    state_conflict: "当前状态不允许此操作，请刷新后检查。",
    team_creation_closed: "团队创建暂未开放。",
    team_capacity_exceeded: "您已达到创建或加入团队的数量限制。",
    last_platform_administrator:
      "平台必须保留一位可用管理员，请先完成管理交接。",
    export_limit_exceeded:
      "当前筛选超过导出上限，请缩小范围后重试（最多 10000 行）。",
    export_unavailable: "导出暂时不可用，请稍后重试。",
    not_found: "内容不存在或您已无法访问，请重新读取。",
    team_state_conflict: "团队当前状态不允许此操作，请重新读取。",
    last_team_administrator: "请先完成管理转交，团队必须保留一位可用管理员。",
    invitation_expired: "邀请已过期，请联系邀请人重新发送。",
    invitation_invalid: "邀请无效、已被撤销或已使用，请检查最新邮件。",
    proof_invalid: "验证码或恢复证明无效、过期或已使用，请重新申请。",
    rate_limited: "请求过于频繁，请等待后重试。",
    revision_conflict: "资料已在其他设备更新，请重新加载后再保存。",
    reauthentication_required: "请先验证当前密码，再继续操作。",
    administrator_transfer_required: "请先交接最后一位平台或团队管理员职责。",
    login_name_taken: "登录名已被使用，请更换后重试。",
    invalid_credentials: "邮箱或密码不正确。",
    current_password_invalid: "当前密码不正确。",
    weak_password: `请使用 ${PASSWORD_LENGTH_HINT}，并避免常见的简单密码。`,
    password_reused: "新密码不能与当前密码相同。",
    credential_conflict: "账号凭据已更新，请重新登录。",
    invalid_request: "输入不符合规则，请检查邮箱、显示名与密码。",
    session_invalid: "登录已失效，请重新登录。",
    forbidden: "当前账号无权执行此操作。",
    origin_forbidden:
      "当前访问地址不受信任，请使用系统配置的网站地址重新打开。",
    invalid_receipt: "暂时无法确认结果，请重试。",
    dependency_unavailable: "登录服务暂不可用，请稍后重试。",
    request_unconfirmed: "连接异常，请稍后重试。",
  };
  return messages[code] ?? "暂时无法完成操作，请稍后重试。";
}

export function requireSession(value: unknown): API.EmailSessionView {
  const session = value as Partial<API.EmailSessionView> | null;
  const actor = session?.actor;
  const uuid =
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
  if (
    !actor ||
    typeof actor.id !== "string" ||
    !uuid.test(actor.id) ||
    typeof session?.session_id !== "string" ||
    !uuid.test(session.session_id) ||
    typeof actor.email !== "string" ||
    !actor.email.includes("@") ||
    typeof actor.display_name !== "string" ||
    !["creator", "platform_admin"].includes(actor.platform_role) ||
    actor.status !== "active" ||
    typeof actor.revision !== "string" ||
    !/^[1-9][0-9]*$/.test(actor.revision) ||
    !Array.isArray(actor.capabilities) ||
    typeof session.credential_epoch !== "string" ||
    !/^[1-9][0-9]*$/.test(session.credential_epoch) ||
    typeof session.csrf_token !== "string" ||
    !session.csrf_token ||
    typeof session.persistent !== "boolean" ||
    !Number.isFinite(Date.parse(session.last_active_at ?? "")) ||
    !Number.isFinite(Date.parse(session.absolute_expires_at ?? ""))
  )
    throw new RequestError("invalid_receipt");
  return session as API.EmailSessionView;
}

/** Validate 202 mail acceptance without claiming delivery or inventing a flow. */
export function requireAccepted(value: unknown): API.AcceptedView {
  const result = value as Partial<API.AcceptedView> | null;
  if (
    !result ||
    typeof result.operation_id !== "string" ||
    result.operation_id === "00000000-0000-0000-0000-000000000000" ||
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
      result.operation_id,
    ) ||
    typeof result.retry_after_seconds !== "number" ||
    !Number.isInteger(result.retry_after_seconds) ||
    result.retry_after_seconds < 0 ||
    result.retry_after_seconds > 86400
  )
    throw new RequestError("invalid_receipt");
  return result as API.AcceptedView;
}
