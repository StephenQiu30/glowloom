import { accountActionRestriction } from "./action-restrictions";
import {
  getPlatformUser,
  platformDisable,
  platformDeleteUser,
  platformEnable,
  platformRevokeSessions,
  getUserGovernanceReceipt,
} from "@/api/identity";
import {
  getAdminProvider,
  updateAdminProvider,
  testAdminCredential,
  getAdminModel,
  setAdminModelStatus,
  getConfigReceipt,
} from "@/api/settings";
import { RequestError } from "@/lib/request";
import type { BatchAction } from "./use-batch-operation";
export function userBatchAction(
  kind: "enable" | "disable" | "revoke_sessions" | "delete",
  actorID: string,
): BatchAction<API.PlatformUserView> {
  const titles = {
    enable: "启用账号",
    disable: "禁用账号",
    delete: "删除账号",
    revoke_sessions: "撤销会话",
  };
  return {
    title: titles[kind],
    effect:
      kind === "delete"
        ? "删除后无法登录且不可恢复，账号资料、作品与团队数据保留。"
        : kind === "disable"
          ? "撤销所选账号的会话并阻止登录；资料和作品保留。权限或状态不适用的账号不执行。"
          : kind === "enable"
            ? "恢复账号访问资格，不恢复旧会话或自动解冻团队。"
            : "撤销所选账号全部会话，用户需要重新登录。",
    reason: kind !== "disable" && kind !== "delete",
    password: kind !== "disable" && kind !== "delete",
    compact: kind === "disable" || kind === "delete",
    destructive: kind === "delete",
    label: (r) => `${r.display_name} · ${r.email}`,
    prepare: async (row, signal, refresh) => {
      if (row.id === actorID)
        return { row, skip: "不能对当前管理员账号执行批量操作" };
      const detail = await getPlatformUser({ id: row.id }, { signal });
      if (!refresh && detail.user.revision !== row.revision)
        throw new RequestError("revision_conflict", 409);
      return {
        row: refresh ? detail.user : row,
        skip: detail.allowed_actions.includes(kind)
          ? undefined
          : accountActionRestriction(detail.blocked_actions?.[kind]),
      };
    },
    execute: async (entry, reason, signal) => {
      if (kind === "delete") {
        await platformDeleteUser(
          { id: entry.id },
          { expected_revision: entry.row.revision },
          { signal, headers: { "Idempotency-Key": entry.requestId } },
        );
        return { message: "账号已删除，作品与团队数据保留" };
      }
      await {
        enable: platformEnable,
        disable: platformDisable,
        revoke_sessions: platformRevokeSessions,
      }[kind](
        { id: entry.id },
        { expected_revision: entry.row.revision, reason },
        { signal, headers: { "Idempotency-Key": entry.requestId } },
      );
      return { message: "操作已完成" };
    },
    recover: async (entry, signal) => {
      try {
        const r = await getUserGovernanceReceipt(
          { id: entry.id, request: entry.requestId },
          { signal },
        );
        if (
          r.action !== kind ||
          r.target_id !== entry.id ||
          r.request_id !== entry.requestId
        )
          throw new RequestError("request_unconfirmed");
        return { message: "已找到原操作成功回执" };
      } catch (e) {
        if (e instanceof RequestError && e.status === 404) return;
        throw e;
      }
    },
  };
}
export type ConnectionRow = API.ProviderListItem & { id: string };
async function receipt(
  action: string,
  id: string,
  request: string,
  signal: AbortSignal,
) {
  try {
    const response = await getConfigReceipt(
      { id: request, action, target_id: id },
      { signal },
    );
    if (
      response.action !== action ||
      response.target_id !== id ||
      response.request_id !== request
    )
      throw new RequestError("request_unconfirmed");
    return response;
  } catch (e) {
    if (e instanceof RequestError && e.status === 404) return;
    throw e;
  }
}
export function connectionBatchAction(
  kind: "disable" | "check",
): BatchAction<ConnectionRow> {
  const check = kind === "check";
  return {
    title: check ? "检查连接" : "停用连接",
    effect: check
      ? "仅提交只读连接检查，不生成内容或发布模型。已提交不代表检查通过，实际结果可在连接详情查看。"
      : "停止通过所选连接创建新任务，已有任务继续使用绑定的配置和 Key。",
    destructive: !check,
    label: (r) => r.provider?.name ?? r.id,
    prepare: async (row, signal, refresh) => {
      const detail = await getAdminProvider({ id: row.id }, { signal });
      if (
        !refresh &&
        (detail.provider?.revision !== row.provider?.revision ||
          detail.credential?.id !== row.credential?.id)
      )
        throw new RequestError("revision_conflict", 409);
      const next = refresh ? { ...row, ...detail } : row;
      return {
        row: next,
        skip: check
          ? detail.credential?.id && detail.credential.status === "active"
            ? undefined
            : "没有可检查的有效凭据"
          : detail.provider?.status === "active"
            ? undefined
            : "连接已停用",
      };
    },
    execute: async (entry, _reason, signal) => {
      if (check) {
        const result = await testAdminCredential(
          { id: entry.id, credential_id: entry.row.credential!.id! },
          { signal, headers: { "Idempotency-Key": entry.requestId } },
        );
        if (!result?.check_id) throw new RequestError("request_unconfirmed");
        return {
          status: "submitted",
          message: "只读检查已提交，请在连接详情查看实际结果",
        };
      }
      await updateAdminProvider(
        { id: entry.id },
        { expected_revision: entry.row.provider!.revision, status: "disabled" },
        { signal, headers: { "Idempotency-Key": entry.requestId } },
      );
      return { message: "已停用，不再接受新任务" };
    },
    recover: async (entry, signal) => {
      const r = await receipt(
        check ? "credential.test" : "provider.update",
        entry.id,
        entry.requestId,
        signal,
      );
      if (!r) return;
      return check
        ? {
            status: "submitted",
            message: "原检查已提交，请在连接详情查看实际结果",
          }
        : { message: "已恢复原停用回执" };
    },
  };
}
export const modelBatchAction: BatchAction<API.CreatedModel> = {
  title: "停用模型",
  effect:
    "停止通过所选模型创建新任务，不取消已提交任务。发布和激活仍需逐项验证。",
  destructive: true,
  label: (r) => r.display_name ?? r.id ?? "模型",
  prepare: async (row, signal, refresh) => {
    const d = await getAdminModel({ id: row.id! }, { signal });
    if (!d.model) throw new RequestError("not_found", 404);
    if (!refresh && d.model.revision !== row.revision)
      throw new RequestError("revision_conflict", 409);
    return {
      row: refresh ? d.model : row,
      skip: d.model.status === "active" ? undefined : "草稿或已停用模型不执行",
    };
  },
  execute: async (entry, _reason, signal) => {
    await setAdminModelStatus(
      { id: entry.id },
      { status: "disabled", expected_revision: entry.row.revision },
      { signal, headers: { "Idempotency-Key": entry.requestId } },
    );
    return { message: "模型已停用" };
  },
  recover: async (entry, signal) => {
    const r = await receipt("model.status", entry.id, entry.requestId, signal);
    return r ? { message: "已恢复原模型停用回执" } : undefined;
  },
};
