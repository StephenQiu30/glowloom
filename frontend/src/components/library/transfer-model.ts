import { z } from "zod";
import { ApiError } from "@/lib/request";
import {
  libraryScopeSchema,
  libraryUUID,
  sameLibraryScope,
  type LibraryIdentity,
} from "@/components/media/library-model";
const revision = z.number().int().min(0).max(2147483647);
const positive = revision.refine((n) => n > 0);
const itemRevision = z.object({ id: libraryUUID, revision }).strict();
export const transferInputSchema = z
  .object({
    source: libraryScopeSchema,
    target: libraryScopeSchema,
    items: z.array(itemRevision).min(1).max(200),
    expected_source_revision: revision,
    expected_target_revision: revision,
    expected_project_revision: positive,
    target_folder_id: libraryUUID.nullable(),
    expected_folder_revision: revision,
  })
  .strict()
  .refine(
    (input) =>
      input.source.kind !== input.target.kind &&
      new Set(input.items.map((item) => item.id)).size === input.items.length &&
      (input.target_folder_id === null
        ? input.expected_folder_revision === 0
        : input.expected_folder_revision > 0),
  );
export type TransferInput = z.infer<typeof transferInputSchema>;
export const transferCommandSchema = z.discriminatedUnion("action", [
  z.object({ action: z.literal("create"), body: transferInputSchema }).strict(),
  z
    .object({
      action: z.enum(["cancel", "retry", "reconcile"]),
      jobId: libraryUUID,
      attempt: positive,
      source: libraryScopeSchema,
      target: libraryScopeSchema,
      targetFolderId: libraryUUID.nullable(),
      body: z.object({ revision: positive }).strict(),
    })
    .strict()
    .refine((c) => c.source.kind !== c.target.kind),
]);
export type TransferCommand = z.infer<typeof transferCommandSchema>;
const statuses = [
  "queued",
  "running",
  "needs_reconciliation",
  "succeeded",
  "partial_failed",
  "failed",
  "cancel_requested",
  "cancelled",
] as const;
const jobSchema = z
  .object({
    id: libraryUUID,
    current_actor_id: libraryUUID,
    current_org_id: libraryUUID,
    source: libraryScopeSchema,
    target: libraryScopeSchema,
    target_folder_id: libraryUUID.nullable(),
    status: z.enum(statuses),
    stage: z.enum(["frozen", "copying", "registering", "cleanup", "completed"]),
    attempt: positive,
    revision: positive,
    needs_reconciliation: z.boolean(),
    cancellation_requested: z.boolean(),
    execution_unconfirmed: z.boolean(),
    items: z
      .array(
        z
          .object({
            index: z.number().int().min(0).max(199),
            source_item_id: libraryUUID,
            target_item_id: libraryUUID,
            target_asset_id: libraryUUID.nullable(),
            status: z.enum([
              "queued",
              "running",
              "succeeded",
              "failed",
              "needs_reconciliation",
              "cancelled",
            ]),
            failure_code: z.string().min(1).max(128).nullable(),
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
      job.source.kind !== job.target.kind &&
      job.items.every(
        (item, i) =>
          item.index === i &&
          item.source_item_id !== item.target_item_id &&
          (item.target_asset_id === null ||
            item.target_asset_id === item.target_item_id),
      ) &&
      new Set(job.items.map((item) => item.source_item_id)).size ===
        job.items.length &&
      new Set(job.items.map((item) => item.target_item_id)).size ===
        job.items.length,
  );
export type TransferJob = z.infer<typeof jobSchema>;
const pageSchema = z
  .object({
    current_actor_id: libraryUUID,
    current_org_id: libraryUUID,
    items: z.array(jobSchema).max(100),
    page: z.number().int().min(1).max(10000),
    page_size: z.number().int().min(1).max(100),
  })
  .strict();
function invalid(): never {
  throw new ApiError(502, "invalid_response");
}
function actorMatches(
  job: { current_actor_id: string; current_org_id: string },
  identity: LibraryIdentity,
) {
  return (
    job.current_actor_id === identity.actorId &&
    job.current_org_id === identity.orgId
  );
}
function inScope(job: TransferJob, identity: LibraryIdentity) {
  return (
    identity.scope.kind === "personal" ||
    sameLibraryScope(job.source, identity.scope) ||
    sameLibraryScope(job.target, identity.scope)
  );
}
export function readTransfer(
  value: unknown,
  identity: LibraryIdentity,
  id?: string,
) {
  const parsed = jobSchema.safeParse(value);
  if (
    !parsed.success ||
    !actorMatches(parsed.data, identity) ||
    !inScope(parsed.data, identity) ||
    (id && parsed.data.id !== id)
  )
    invalid();
  return parsed.data;
}
export function readTransferPage(
  value: unknown,
  identity: LibraryIdentity,
  page: number,
  size = 20,
) {
  const parsed = pageSchema.safeParse(value);
  if (
    !parsed.success ||
    !actorMatches(parsed.data, identity) ||
    parsed.data.page !== page ||
    parsed.data.page_size !== size ||
    parsed.data.items.length > size ||
    new Set(parsed.data.items.map((job) => job.id)).size !==
      parsed.data.items.length ||
    parsed.data.items.some(
      (job) => !actorMatches(job, identity) || !inScope(job, identity),
    )
  )
    invalid();
  return parsed.data;
}
export function readTransferReceipt(
  value: unknown,
  intent: TransferCommand & LibraryIdentity,
) {
  const job = readTransfer(
    value,
    intent,
    intent.action === "create" ? undefined : intent.jobId,
  );
  if (intent.action === "create") {
    const b = intent.body;
    if (
      !sameLibraryScope(job.source, b.source) ||
      !sameLibraryScope(job.target, b.target) ||
      job.target_folder_id !== b.target_folder_id ||
      job.revision !== 1 ||
      job.attempt !== 1 ||
      job.status !== "queued" ||
      job.stage !== "frozen" ||
      job.needs_reconciliation ||
      job.execution_unconfirmed ||
      job.cancellation_requested ||
      job.items.length !== b.items.length ||
      job.items.some(
        (item, index) =>
          item.source_item_id !== b.items[index].id ||
          item.status !== "queued" ||
          item.failure_code !== null,
      )
    )
      invalid();
  } else {
    if (
      !sameLibraryScope(job.source, intent.source) ||
      !sameLibraryScope(job.target, intent.target) ||
      job.target_folder_id !== intent.targetFolderId
    )
      invalid();
    if (intent.action === "cancel") {
      if (
        ![intent.body.revision, intent.body.revision + 1].includes(
          job.revision,
        ) ||
        job.attempt !== intent.attempt ||
        !job.cancellation_requested ||
        !["cancel_requested", "cancelled"].includes(job.status)
      )
        invalid();
    } else if (
      job.revision !== intent.body.revision + 1 ||
      job.attempt !== intent.attempt + 1 ||
      job.status !== "queued" ||
      job.stage !== "frozen" ||
      job.needs_reconciliation ||
      job.execution_unconfirmed ||
      (intent.action === "retry" && job.cancellation_requested)
    )
      invalid();
  }
  return job;
}
export function transferActions(job: TransferJob) {
  return {
    cancel:
      [
        "queued",
        "running",
        "needs_reconciliation",
        "cancel_requested",
      ].includes(job.status) && !job.cancellation_requested,
    retry:
      ["failed", "partial_failed"].includes(job.status) &&
      !job.needs_reconciliation &&
      !job.execution_unconfirmed &&
      !job.cancellation_requested &&
      job.items.some((item) => item.status === "failed"),
    reconcile:
      job.status === "needs_reconciliation" &&
      job.needs_reconciliation &&
      !job.execution_unconfirmed,
  };
}
export function transferActive(job: TransferJob) {
  return ["queued", "running", "cancel_requested"].includes(job.status);
}
export function transferControl(
  job: TransferJob,
  action: "cancel" | "retry" | "reconcile",
): TransferCommand {
  return {
    action,
    jobId: job.id,
    attempt: job.attempt,
    source: job.source,
    target: job.target,
    targetFolderId: job.target_folder_id,
    body: { revision: job.revision },
  };
}
