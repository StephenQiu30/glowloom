import { z } from "zod";
import { ApiError } from "@/lib/request";
import { scriptUUID, sourceExtractionWarningSchema } from "./source-model";
import { reviewRevision } from "./review-model";
const positiveRevision = reviewRevision.refine((value) => value > 0);
const warnings = z
  .array(sourceExtractionWarningSchema)
  .max(64)
  .refine(
    (items) => new Set(items.map((item) => item.code)).size === items.length,
  );
export const fileImportFileSchema = z
  .object({
    position: z.number().int().min(0).max(199),
    asset_id: scriptUUID,
    file_name: z
      .string()
      .min(1)
      .refine(
        (value) =>
          new TextEncoder().encode(value).length <= 255 &&
          !/[/\\\p{Cc}]/u.test(value),
      ),
    status: z.enum(["queued", "succeeded", "failed"]),
    failure_code: z.string().min(1).max(128).optional(),
    warnings,
    source_id: scriptUUID.optional(),
    source_lineage_id: scriptUUID.optional(),
    attempt: z.number().int().min(1).max(100),
  })
  .strict()
  .refine(
    (file) =>
      Boolean(file.source_id) === Boolean(file.source_lineage_id) &&
      (!file.source_id || file.status === "succeeded"),
  );
export const fileImportJobSchema = z
  .object({
    id: scriptUUID,
    project_id: scriptUUID,
    revision: positiveRevision,
    attempt: z.number().int().min(1).max(100),
    status: z.enum([
      "queued",
      "running",
      "partial",
      "failed",
      "cancel_requested",
      "succeeded",
      "cancelled",
    ]),
    stage: z.enum([
      "queued",
      "extracting",
      "normalizing",
      "storing",
      "committing",
      "completed",
      "failed",
      "cancelling",
      "awaiting_reconciliation",
    ]),
    expected_script_revision: reviewRevision,
    latest_script_revision: reviewRevision,
    latest_version_id: scriptUUID.optional(),
    cancellation_requested: z.boolean(),
    reconciliation_requested: z.boolean(),
    needs_reconciliation: z.boolean(),
    active_io: z.boolean(),
    retryable: z.boolean(),
    can_control: z.boolean(),
    failure_code: z.string().min(1).max(128).optional(),
    files: z.array(fileImportFileSchema).min(1).max(200),
    created_at: z.iso.datetime({ offset: true }),
    updated_at: z.iso.datetime({ offset: true }),
  })
  .strict()
  .refine(
    (job) =>
      job.latest_script_revision >= job.expected_script_revision &&
      job.files.every(
        (file, index) => file.position === index && file.attempt <= job.attempt,
      ) &&
      new Set(job.files.map((file) => file.asset_id)).size ===
        job.files.length &&
      new Set(
        job.files.flatMap((file) => (file.source_id ? [file.source_id] : [])),
      ).size === job.files.filter((file) => file.source_id).length &&
      new Set(
        job.files.flatMap((file) =>
          file.source_lineage_id ? [file.source_lineage_id] : [],
        ),
      ).size === job.files.filter((file) => file.source_lineage_id).length &&
      (!job.files.some((file) => file.source_id) ||
        Boolean(job.latest_version_id)),
  );
export type FileImportJob = z.infer<typeof fileImportJobSchema>;
export const fileImportCreateSchema = z
  .object({
    action: z.literal("create"),
    body: z
      .object({
        expected_revision: reviewRevision,
        base_version_id: scriptUUID.optional(),
        rights_confirmed: z.literal(true),
        asset_ids: z
          .array(scriptUUID)
          .min(1)
          .max(200)
          .refine((ids) => new Set(ids).size === ids.length),
      })
      .strict(),
  })
  .strict();
export const fileImportControlSchema = z
  .object({
    action: z.enum(["cancel", "retry", "reconcile"]),
    jobId: scriptUUID,
    body: z.object({ expected_revision: positiveRevision }).strict(),
  })
  .strict();
export const fileImportCommandSchema = z.union([
  fileImportCreateSchema,
  fileImportControlSchema,
]);
export type FileImportCommand = z.infer<typeof fileImportCommandSchema>;
function invalid(): never {
  throw new ApiError(502, "invalid_response");
}
export function readFileImport(
  value: unknown,
  projectId: string,
  jobId?: string,
): FileImportJob {
  const result = fileImportJobSchema.safeParse(value);
  if (
    !result.success ||
    result.data.project_id !== projectId ||
    (jobId && result.data.id !== jobId)
  )
    invalid();
  return result.data;
}
export function readFileImportPage(
  value: unknown,
  scope: { projectId: string; actorId: string; orgId: string },
  after = 0,
) {
  const result = z
    .object({
      current_actor_id: scriptUUID,
      current_org_id: scriptUUID,
      items: z.array(fileImportJobSchema).max(25),
      next_after: z.number().int().min(1).max(100000).optional(),
    })
    .strict()
    .safeParse(value);
  if (
    !result.success ||
    result.data.current_actor_id !== scope.actorId ||
    result.data.current_org_id !== scope.orgId ||
    result.data.items.some((job) => job.project_id !== scope.projectId) ||
    new Set(result.data.items.map((job) => job.id)).size !==
      result.data.items.length ||
    (result.data.next_after !== undefined && result.data.next_after <= after)
  )
    invalid();
  return result.data;
}
export function fileImportActions(job: FileImportJob) {
  const controllable =
    job.can_control && !["succeeded", "cancelled"].includes(job.status);
  return {
    cancel: controllable && !job.cancellation_requested,
    retry:
      controllable &&
      ["partial", "failed"].includes(job.status) &&
      job.retryable &&
      !job.cancellation_requested &&
      !job.needs_reconciliation &&
      !job.reconciliation_requested &&
      !job.active_io,
    reconcile:
      controllable &&
      (job.needs_reconciliation || job.cancellation_requested) &&
      !job.reconciliation_requested,
  };
}
export function readFileImportReceipt(
  value: unknown,
  projectId: string,
  command: FileImportCommand,
) {
  const job = readFileImport(
    value,
    projectId,
    command.action === "create" ? undefined : command.jobId,
  );
  if (command.action === "create") {
    if (
      job.expected_script_revision !== command.body.expected_revision ||
      job.latest_script_revision !== command.body.expected_revision ||
      job.latest_version_id !== command.body.base_version_id ||
      job.status !== "queued" ||
      job.stage !== "queued" ||
      job.revision !== 1 ||
      job.attempt !== 1 ||
      job.cancellation_requested ||
      job.reconciliation_requested ||
      job.needs_reconciliation ||
      job.active_io ||
      job.files.some(
        (file) => file.status !== "queued" || Boolean(file.source_id),
      ) ||
      job.files.length !== command.body.asset_ids.length ||
      job.files.some(
        (file, index) => file.asset_id !== command.body.asset_ids[index],
      )
    )
      invalid();
  } else if (
    job.revision !== command.body.expected_revision + 1 ||
    (command.action === "cancel" &&
      (!job.cancellation_requested ||
        job.status !== "cancel_requested" ||
        job.stage !== "cancelling")) ||
    (command.action === "reconcile" && !job.reconciliation_requested) ||
    (command.action === "retry" &&
      (job.status !== "queued" ||
        job.stage !== "queued" ||
        job.attempt < 2 ||
        job.cancellation_requested ||
        job.reconciliation_requested))
  )
    invalid();
  return job;
}
