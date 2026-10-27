import {
  platformTeam,
  restorePlatformTeam,
  suspendPlatformTeam,
} from "@/api/identity";
import { RequestError } from "@/lib/request";
import type { BatchAction } from "@/components/governance/use-batch-operation";

// Only team lifecycle commands belong here; global account state is never changed.
export function teamBatchAction(
  kind: "restore" | "suspend",
): BatchAction<API.PlatformTeam> {
  return {
    title: kind === "restore" ? "恢复团队" : "停用团队",
    effect:
      kind === "restore"
        ? "恢复停用前的团队状态，治理冻结独立保留。"
        : "停止团队访问与创作，保留资料和作品，不禁用成员账号。",
    reason: true,
    password: true,
    label: (row) => row.name ?? "团队",
    prepare: async (row, signal, refresh) => {
      const next = await platformTeam({ id: row.id! }, { signal });
      if (!refresh && next.revision !== row.revision)
        throw new RequestError("revision_conflict", 409);
      return {
        row: refresh ? next : row,
        skip: (
          kind === "restore"
            ? next.status === "suspended"
            : next.status === "active" || next.status === "archived"
        )
          ? undefined
          : "团队当前状态无需此操作",
      };
    },
    execute: async (entry, reason, signal) => {
      await (kind === "restore" ? restorePlatformTeam : suspendPlatformTeam)(
        { id: entry.id },
        { expected_revision: entry.row.revision, reason },
        { signal },
      );
      return {
        message:
          kind === "restore" ? "团队已恢复" : "团队已停用，成员账号保持原状态",
      };
    },
  };
}
