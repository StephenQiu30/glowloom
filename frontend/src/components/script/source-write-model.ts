import { z } from "zod";
import { ApiError } from "@/lib/request";
import { scriptUUID } from "./source-model";
import { reviewRevision } from "./review-model";
export const sourceWriteSchema = z
  .object({
    id: scriptUUID,
    revision: reviewRevision.refine((value) => value > 0),
    action: z.enum(["create", "update", "delete", "import", "reorder"]),
    status: z.enum(["pending", "completed", "cancelled"]),
    expected_script_revision: reviewRevision,
    base_version_id: scriptUUID.optional(),
    object_count: z.number().int().nonnegative(),
    confirmed_object_count: z.number().int().nonnegative(),
    cancellation_requested: z.boolean(),
    needs_reconciliation: z.boolean(),
    active_io: z.boolean(),
    can_control: z.boolean(),
    created_at: z.iso.datetime({ offset: true }),
    updated_at: z.iso.datetime({ offset: true }),
  })
  .strict()
  .refine(
    (item) =>
      item.confirmed_object_count <= item.object_count &&
      (item.status === "pending" || !item.can_control),
  );
export type SourceWrite = z.infer<typeof sourceWriteSchema>;
export function readSourceWritePage(
  value: unknown,
  scope: { actorId: string; orgId: string },
  after: number,
  limit: number,
) {
  const result = z
    .object({
      current_actor_id: scriptUUID,
      current_org_id: scriptUUID,
      items: z.array(sourceWriteSchema).max(limit),
      next_after: z.number().int().positive().optional(),
    })
    .strict()
    .safeParse(value);
  if (
    !result.success ||
    result.data.current_actor_id !== scope.actorId ||
    result.data.current_org_id !== scope.orgId ||
    new Set(result.data.items.map((item) => item.id)).size !==
      result.data.items.length ||
    (result.data.next_after !== undefined && result.data.next_after <= after)
  )
    throw new ApiError(502, "invalid_response");
  return result.data;
}
export function readSourceWrite(value: unknown, id: string) {
  const result = sourceWriteSchema.safeParse(value);
  if (!result.success || result.data.id !== id)
    throw new ApiError(502, "invalid_response");
  return result.data;
}
export function sourceWriteActions(item: SourceWrite) {
  const available = item.status === "pending" && item.can_control;
  return {
    cancel: available && !item.cancellation_requested,
    reconcile:
      available && (item.cancellation_requested || item.needs_reconciliation),
  };
}
export const sourceControlCommandSchema = z
  .object({
    intentId: scriptUUID,
    action: z.enum(["cancel", "reconcile"]),
    body: z
      .object({
        expected_revision: reviewRevision.refine((value) => value > 0),
      })
      .strict(),
  })
  .strict();
export type SourceControlCommand = z.infer<typeof sourceControlCommandSchema>;
export function readSourceControlReceipt(
  value: unknown,
  command: SourceControlCommand,
) {
  const result = z
    .object({
      intent_id: scriptUUID,
      revision: reviewRevision.refine((value) => value > 0),
      action: z.enum(["cancel", "reconcile"]),
      accepted: z.literal(true),
    })
    .strict()
    .safeParse(value);
  if (
    !result.success ||
    result.data.intent_id !== command.intentId ||
    result.data.action !== command.action ||
    result.data.revision < command.body.expected_revision
  )
    throw new ApiError(502, "invalid_response");
  return result.data;
}
