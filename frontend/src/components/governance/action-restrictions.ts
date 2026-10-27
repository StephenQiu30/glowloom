/** Account action eligibility comes from the server, not inferred from team summaries. */
export function accountActionRestriction(code?: string): string {
  return (
    (
      {
        current_account: "不能对当前登录账号执行此操作",
        account_deleted: "账号已删除",
        last_platform_admin: "需保留至少一位可用的平台管理员",
        deletion_duties: "请先完成团队管理员交接，再删除账号",
      } as Record<string, string>
    )[code ?? ""] ?? "当前状态或权限不允许此操作"
  );
}
