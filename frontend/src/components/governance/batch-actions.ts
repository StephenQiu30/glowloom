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
  previewAdminProviderDeletion,
  deleteAdminProvider,
  getAdminModel,
  setAdminModelStatus,
  getConfigReceipt,
} from "@/api/settings";
import { RequestError } from "@/lib/request";
import { providerConnectionName } from "@/lib/provider-connection-name";
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
        ? "删除后无法登录，作品保留。此操作不可恢复。"
        : kind === "disable"
          ? "禁用后无法登录，可随时重新启用。"
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
        return { message: "账号已删除" };
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
export type ConnectionRow = API.ProviderListItem & {
  id: string;
  deletionToken?: string;
};
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
const connectionDeletionReasons: Record<string, string> = {
  active: "连接已启用，请先在连接详情停用。",
  task_history: "关联在途或历史任务，可在连接详情归档。",
  check_history: "存在进行中或已执行的服务检查或模型调用，可在连接详情查看。",
  model_reference: "关联发布、默认配置或模型验证，可在连接详情归档。",
};

export const connectionDeleteAction: BatchAction<ConnectionRow> = {
  title: "删除连接",
  effect: "连接及列出的草稿模型将从管理目录移除。删除后无法恢复。",
  confirmLabel: (count) => `删除 ${count} 个连接`,
  destructive: true,
  label: (row) =>
    `${providerConnectionName(row.provider) ?? "连接"} · ${row.credential?.status === "active" ? "已配置" : "未配置"} · ${row.id.slice(0, 8)}`,
  prepare: async (row, signal, refresh) => {
    const preview = await previewAdminProviderDeletion(
      { id: row.id },
      { signal },
    );
    if (
      !refresh &&
      (preview.revision !== row.provider?.revision ||
        (row.deletionToken && row.deletionToken !== preview.token))
    ) {
      throw new RequestError("revision_conflict", 409);
    }
    const next = {
      ...row,
      provider: { ...row.provider, revision: preview.revision },
      deletionToken: preview.token,
    };
    if (!preview.token) {
      return {
        row: next,
        skip:
          connectionDeletionReasons[preview.blocked_reason ?? ""] ??
          "连接暂时无法删除。",
      };
    }
    const names =
      preview.drafts?.map((draft) => draft.name).filter(Boolean) ?? [];
    return {
      row: next,
      preview: names.length
        ? `一并移除模型草稿：${names.join("、")}`
        : undefined,
    };
  },
  execute: async (entry, _reason, signal) => {
    const result = await deleteAdminProvider(
      { id: entry.id },
      { token: entry.row.deletionToken! },
      { signal, headers: { "Idempotency-Key": entry.requestId } },
    );
    if (result.id !== entry.id) throw new RequestError("request_unconfirmed");
    return { message: "连接已删除" };
  },
  recover: async (entry, signal) => {
    const result = await receipt(
      "provider.delete",
      entry.id,
      entry.requestId,
      signal,
    );
    if (result) return { message: "连接已删除" };
  },
};
export const modelBatchAction: BatchAction<API.CreatedModel> = {
  title: "停用模型",
  effect: "停用后不能创建新任务，已提交任务不受影响。",
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
