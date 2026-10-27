"use client";
import { accountActionRestriction } from "@/components/governance/action-restrictions";
import { Checkbox } from "@/components/ui/checkbox";
import { FieldLabel } from "@/components/ui/field";
import { useState } from "react";
import {
  platformDisable,
  platformDeleteUser,
  platformEnable,
  platformUnlock,
  platformRevokeSessions,
  platformResetProfile,
  platformRevokeAdmin,
  platformSendPasswordReset,
  proposeAdmin,
} from "@/api/identity";
import type { ConfirmAction } from "@/components/governance/confirmation";
import { Button } from "@/components/ui/button";
const labels: Record<string, string> = {
  disable: "禁用账号",
  delete: "删除账号",
  enable: "启用账号",
  unlock: "解除登录锁定",
  revoke_sessions: "撤销全部会话",
  profile_reset: "重置公开资料",
  revoke_admin: "撤销平台管理员",
  password_reset: "发送密码恢复通知",
  propose_admin: "提议平台管理员授权",
};
export function UserActions({
  detail,
  onAction,
}: {
  detail: API.PlatformUserDetailView;
  onAction: (a: ConfirmAction) => void;
}) {
  const [resetName, setResetName] = useState(true);
  const [resetAvatar, setResetAvatar] = useState(true);
  function choose(kind: string) {
    const simple = kind === "disable" || kind === "delete";
    onAction({
      title: simple
        ? `${kind === "delete" ? "删除" : "禁用"}${detail.user.display_name}？`
        : labels[kind],
      confirmLabel: labels[kind],
      destructive: kind === "delete" || kind === "revoke_admin",
      effect:
        kind === "password_reset"
          ? "成功表示密码恢复通知已加入邮件队列。管理员不会设置或看到用户密码。"
          : kind === "propose_admin"
            ? "目标账号需主动接受授权，接受前平台角色保持原状态。"
            : kind === "delete"
              ? "删除后将无法登录，账号资料与作品保留，不会删除团队或团队作品。此操作不可恢复。"
              : kind === "disable"
                ? "禁用将撤销会话并阻止登录；资料和作品保留。"
                : kind === "profile_reset"
                  ? "仅重置所选公开资料；账号和作品保留。"
                  : "根据当前账号状态执行治理操作，并通知相关用户。",
      reason: !simple,
      password: !simple,
      run: (reason, signal, requestID) => {
        const params = { id: detail.user.id };
        const body = { expected_revision: detail.user.revision, reason };
        const options = { signal, headers: { "Idempotency-Key": requestID } };
        if (kind === "delete")
          return platformDeleteUser(
            params,
            { expected_revision: detail.user.revision },
            options,
          );
        if (kind === "profile_reset")
          return platformResetProfile(
            params,
            {
              ...body,
              reset_display_name: resetName,
              reset_avatar: resetAvatar,
            },
            options,
          );
        if (kind === "revoke_sessions")
          return platformRevokeSessions(params, body, options);
        if (kind === "propose_admin")
          return proposeAdmin(
            { ...body, target_user_id: detail.user.id },
            options,
          );
        return {
          disable: platformDisable,
          enable: platformEnable,
          unlock: platformUnlock,
          revoke_admin: platformRevokeAdmin,
          password_reset: platformSendPasswordReset,
        }[kind]!(params, body, options);
      },
    });
  }
  const actions =
    detail.user.status === "deleted"
      ? []
      : detail.allowed_actions.filter((a) => a in labels);
  return (
    <section className="grid gap-3">
      <h3>可用管理操作</h3>
      {actions.includes("profile_reset") ? (
        <div className="grid gap-2">
          <FieldLabel className="flex gap-2">
            <Checkbox
              checked={resetName}
              onCheckedChange={(value) => setResetName(value === true)}
            />
            重置显示名
          </FieldLabel>
          <FieldLabel className="flex gap-2">
            <Checkbox
              checked={resetAvatar}
              onCheckedChange={(value) => setResetAvatar(value === true)}
            />
            重置头像
          </FieldLabel>
        </div>
      ) : null}
      <div className="flex flex-wrap gap-2">
        {actions.map((a) => (
          <Button
            key={a}
            variant={
              a === "delete" || a === "revoke_admin" ? "destructive" : "outline"
            }
            disabled={a === "profile_reset" && !resetName && !resetAvatar}
            onClick={() => choose(a)}
          >
            {labels[a]}
          </Button>
        ))}
      </div>
      {detail.blocked_actions?.delete && detail.user.status !== "deleted" ? (
        <p className="text-sm text-muted-foreground">
          {accountActionRestriction(detail.blocked_actions.delete)}
        </p>
      ) : null}
      {actions.length === 0 ? <p>当前账号没有可用管理动作。</p> : null}
    </section>
  );
}
