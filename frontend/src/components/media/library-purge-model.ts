import { z } from "zod";
import { ApiError } from "@/lib/request";
import {
  libraryIdentitySchema,
  libraryScopeSchema,
  libraryUUID,
  sameLibraryScope,
  type LibraryIdentity,
} from "./library-model";
const revision = z.number().int().min(0).max(2147483647);
export const purgeItemsSchema = z
  .array(
    z
      .object({ id: libraryUUID, revision: revision.refine((n) => n > 0) })
      .strict(),
  )
  .min(1)
  .max(200)
  .refine(
    (items) => new Set(items.map((item) => item.id)).size === items.length,
  );
export const purgeInputSchema = z
  .object({
    scope: libraryScopeSchema,
    items: purgeItemsSchema,
    expected_revision: revision,
    expected_project_revision: revision,
    permanent_delete_confirmed: z.literal(true),
  })
  .strict()
  .refine((input) =>
    input.scope.kind === "personal"
      ? input.expected_project_revision === 0
      : input.expected_project_revision > 0,
  );
export type PurgeInput = z.infer<typeof purgeInputSchema>;
export const purgeJobSchema = z
  .object({
    id: libraryUUID,
    current_actor_id: libraryUUID,
    current_org_id: libraryUUID,
    scope: libraryScopeSchema,
    status: z.enum([
      "queued",
      "running",
      "needs_reconciliation",
      "succeeded",
      "partial_failed",
      "failed",
      "cancel_requested",
      "cancelled",
    ]),
    stage: z.enum(["frozen", "verifying", "removing", "completed"]),
    revision: revision.refine((n) => n > 0),
    attempt: z.number().int().min(1),
    cancellation_requested: z.boolean(),
    needs_reconciliation: z.boolean(),
    execution_unconfirmed: z.boolean(),
    items: z
      .array(
        z
          .object({
            index: z.number().int().nonnegative(),
            item_id: libraryUUID,
            asset_id: libraryUUID.nullable(),
            status: z.enum([
              "blocked",
              "queued",
              "running",
              "needs_reconciliation",
              "succeeded",
              "cancelled",
            ]),
            failure_code: z
              .enum([
                "in_use",
                "source_unavailable",
                "object_mismatch",
                "object_remove_unknown",
                "worker_interrupted",
                "cancelled",
              ])
              .nullable(),
          })
          .strict(),
      )
      .min(1)
      .max(200),
    created_at: z.iso.datetime({ offset: true }),
    updated_at: z.iso.datetime({ offset: true }),
  })
  .strict()
  .refine(
    (job) =>
      job.items.every((item, index) => item.index === index) &&
      new Set(job.items.map((item) => item.item_id)).size ===
        job.items.length &&
      Date.parse(job.updated_at) >= Date.parse(job.created_at) &&
      (!["succeeded", "partial_failed", "failed", "cancelled"].includes(
        job.status,
      ) ||
        (job.stage === "completed" &&
          !job.needs_reconciliation &&
          !job.execution_unconfirmed &&
          job.items.every((item) =>
            ["succeeded", "blocked", "cancelled"].includes(item.status),
          ))) &&
      (job.status !== "succeeded" ||
        job.items.every((item) => item.status === "succeeded")),
  );
export type PurgeJob = z.infer<typeof purgeJobSchema>;
export type PurgeReview = {
  mode: "selected" | "all";
  revision: number;
  projectRevision: number;
  items: { id: string; revision: number; title: string }[];
};
export const purgePlanTargetsSchema = z
  .array(
    z
      .object({ id: libraryUUID, revision: revision.refine((n) => n > 0) })
      .strict(),
  )
  .min(1)
  .max(2000)
  .refine(
    (items) => new Set(items.map((item) => item.id)).size === items.length,
  );
const intentCommon = {
  ...libraryIdentitySchema.shape,
  version: z.literal(1),
  key: libraryUUID,
};
export const purgeIntentSchema = z
  .discriminatedUnion("action", [
    z
      .object({
        ...intentCommon,
        action: z.literal("create"),
        body: purgeInputSchema,
      })
      .strict(),
    z
      .object({
        ...intentCommon,
        action: z.enum(["cancel", "reconcile"]),
        jobId: libraryUUID,
        body: z.object({ revision: revision.refine((n) => n > 0) }).strict(),
      })
      .strict(),
  ])
  .refine(
    (intent) =>
      intent.action !== "create" ||
      sameLibraryScope(intent.scope, intent.body.scope),
  );
export type PurgeIntent = z.infer<typeof purgeIntentSchema>;
export function requirePurgeRecoveryBudget(plan: unknown, intent: unknown) {
  const bytes = new TextEncoder().encode(
    JSON.stringify({ plan, intent }),
  ).length;
  if (bytes > 1024 * 1024)
    throw new Error(
      "完整清理计划与原键正文的整体JSON超过1MiB，已停止且保留原存储。请结束可确认的旧计划后拆分操作，不能截断目标或自动继续。",
    );
}
export function readPurgeJob(
  value: unknown,
  identity: LibraryIdentity,
  id?: string,
) {
  const result = purgeJobSchema.safeParse(value);
  if (
    !result.success ||
    result.data.current_actor_id !== identity.actorId ||
    result.data.current_org_id !== identity.orgId ||
    !sameLibraryScope(result.data.scope, identity.scope) ||
    (id && result.data.id !== id)
  )
    throw new ApiError(502, "invalid_response");
  return result.data;
}
export function purgeTerminal(job: PurgeJob) {
  return (
    ["succeeded", "partial_failed", "failed", "cancelled"].includes(
      job.status,
    ) &&
    job.stage === "completed" &&
    !job.needs_reconciliation &&
    !job.execution_unconfirmed
  );
}
export const purgeStatusLabels: Record<
  PurgeJob["status"] | PurgeJob["items"][number]["status"],
  string
> = {
  queued: "等待清理",
  running: "正在清理",
  needs_reconciliation: "结果未知，待对账",
  succeeded: "已永久清理",
  partial_failed: "部分条目未完成",
  failed: "未完成",
  cancel_requested: "已请求取消，尚未确认停止",
  cancelled: "已取消",
  blocked: "引用保护阻断",
};
