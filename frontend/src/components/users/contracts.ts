import { AuthError } from "@/components/auth/request";
import { isProjectID } from "@/components/projects/project";
export type User = CreationAPI.AdminUser & {
  id: string;
  login_name: string;
  display_name: string;
  role: "admin" | "creator";
  status: "active" | "disabled";
  revision: number;
};
export function requireUser(v: CreationAPI.AdminUser): User {
  if (
    !v ||
    !isProjectID(v.id) ||
    typeof v.login_name !== "string" ||
    typeof v.display_name !== "string" ||
    !["admin", "creator"].includes(v.role ?? "") ||
    !["active", "disabled"].includes(v.status ?? "") ||
    !Number.isSafeInteger(v.revision) ||
    v.revision! < 1
  )
    throw new AuthError("invalid_receipt");
  return v as User;
}
export function requireUserPage(p: CreationAPI.AdminUserPage) {
  if (!p || !Array.isArray(p.items)) throw new AuthError("invalid_receipt");
  return { items: p.items.map(requireUser), next_cursor: p.next_cursor };
}
export function requireUserMutation(
  r: CreationAPI.AdminMutationReceipt,
  id?: string,
  revision?: number,
) {
  if (
    !r ||
    !isProjectID(r.id) ||
    (id && r.id !== id) ||
    !Number.isSafeInteger(r.revision) ||
    r.revision! < 1 ||
    (revision !== undefined && r.revision !== revision + 1)
  )
    throw new AuthError("invalid_receipt");
  return r;
}
export function userError(e: unknown) {
  if (e instanceof AuthError) {
    const m: Record<string, string> = {
      login_name_taken: "登录名已存在，请更换登录名。",
      weak_password: "密码需至少 10 个字符，包含字母和数字，且不超过 72 字节。",
      password_reused: "新密码不能与当前密码相同。",
      self_management_forbidden:
        "不能对当前账号执行此操作，请到账号设置管理自己的密码。",
      last_active_admin: "至少需要保留一位启用的管理员。",
      revision_conflict:
        "用户资料已被其他管理员更新。请核对最新资料后重新操作。",
      account_state_conflict: "账号状态已经变化，请刷新后核对。",
      invalid_request: "请检查名称、角色与密码是否符合要求。",
    };
    if (m[e.code]) return m[e.code];
    if (e.status === 403) return "当前账号已无管理权限，请重新登录确认。";
    if (e.status === 401) return "登录已失效，请重新登录。";
    if (e.status === 404) return "该用户已不存在或不在你的组织中。";
  }
  return "请求结果未确认。请先核对最新用户信息，不要重复提交。";
}
