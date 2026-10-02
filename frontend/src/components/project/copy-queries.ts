import { z } from "zod";
import * as copies from "@/gen/api/projectCopies";
import { ApiError } from "@/lib/request";
import { copyIntentSchema, type CopyIntent } from "./copy-intent";

export const COPY_KEY = ["project-copies"] as const;
export const copyUUID = z
  .string()
  .uuid()
  .refine((value) => value !== "00000000-0000-0000-0000-000000000000");
const count = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER);
const scriptCount = z.number().int().nonnegative().max(2147483647);
const scriptCountsSchema = z
  .object({
    sources: scriptCount,
    versions: scriptCount,
    version_sources: scriptCount,
    project_states: scriptCount.max(1),
    version_heads: scriptCount,
    split_sets: scriptCount,
    split_confirmations: scriptCount,
    episodes: scriptCount,
    structures: scriptCount,
    scenes: scriptCount,
    dialogue_lines: scriptCount,
    action_lines: scriptCount,
    objects: scriptCount,
  })
  .strict()
  .refine((counts) => counts.version_heads === counts.versions);
type ScriptCounts = z.infer<typeof scriptCountsSchema>;
function sameScriptCounts(a: ScriptCounts, b: ScriptCounts) {
  return (Object.keys(a) as (keyof ScriptCounts)[]).every(
    (key) => a[key] === b[key],
  );
}
const scriptProgressSchema = z
  .object({
    counts: scriptCountsSchema,
    completed_counts: scriptCountsSchema.optional(),
  })
  .strict()
  .refine(
    (script) =>
      !script.completed_counts ||
      sameScriptCounts(script.counts, script.completed_counts),
  );
const copyJobSchema = z
  .object({
    id: copyUUID,
    source_project_id: copyUUID,
    source_revision: z.number().int().positive(),
    target_project_id: copyUUID,
    target_name: z.string().min(1),
    status: z.enum([
      "queued",
      "running",
      "failed",
      "cancel_requested",
      "cancelled",
      "succeeded",
    ]),
    stage: z.enum([
      "media",
      "script",
      "canvases",
      "finalizing",
      "cleanup",
      "complete",
    ]),
    revision: z.number().int().positive(),
    attempt: count,
    documents: count,
    assets: count,
    renditions: count,
    completed_documents: count,
    completed_assets: count,
    completed_renditions: count,
    retryable: z.boolean(),
    needs_reconciliation: z.boolean(),
    reconciliation_requested: z.boolean(),
    execution_unconfirmed: z.boolean(),
    cancellation_requested: z.boolean(),
    failure_code: z.string().max(200).optional(),
    script: scriptProgressSchema.optional(),
  })
  .strict()
  .refine(
    (job) =>
      job.source_project_id !== job.target_project_id &&
      job.completed_documents <= job.documents &&
      job.completed_assets <= job.assets &&
      job.completed_renditions <= job.renditions &&
      (job.stage !== "script" || !!job.script) &&
      (!job.script ||
        !["canvases", "finalizing", "complete"].includes(job.stage) ||
        !!job.script.completed_counts) &&
      (job.status !== "succeeded" ||
        (job.stage === "complete" &&
          job.completed_documents === job.documents &&
          job.completed_assets === job.assets &&
          job.completed_renditions === job.renditions)),
  );
export type CopyJob = z.infer<typeof copyJobSchema>;
function parse<T>(schema: z.ZodType<T>, response: unknown): T {
  const checked = schema.safeParse(response);
  if (!checked.success) throw new ApiError(502, "invalid_response");
  return checked.data;
}
function jobFor(sourceId: string, jobId?: string) {
  return copyJobSchema.refine(
    (job) => job.source_project_id === sourceId && (!jobId || job.id === jobId),
  );
}
export async function listCopies(
  sourceId: string,
  cursor?: string,
  signal?: AbortSignal,
) {
  const schema = z
    .object({
      copies: z.array(jobFor(sourceId)).max(25),
      next_cursor: z.string().min(1).nullable(),
      current_actor_id: copyUUID,
      current_org_id: copyUUID,
    })
    .strict();
  return parse(
    schema,
    await copies.listProjectCopies(
      { pid: sourceId, limit: 25, cursor },
      { signal },
    ),
  );
}
export async function getCopy(
  sourceId: string,
  jobId: string,
  signal?: AbortSignal,
) {
  return parse(
    jobFor(sourceId, jobId),
    await copies.getProjectCopy({ id: jobId }, { signal }),
  );
}
export async function runCopyIntent(intent: CopyIntent) {
  const checked = copyIntentSchema.parse(intent);
  const options = {
    headers: { "Idempotency-Key": checked.key, Origin: checked.origin },
  };
  let response: unknown;
  if (checked.action === "create")
    response = await copies.createProjectCopy(
      { pid: checked.sourceId },
      checked.body,
      options,
    );
  else {
    const command = {
      cancel: copies.cancelProjectCopy,
      retry: copies.retryProjectCopy,
      reconcile: copies.reconcileProjectCopy,
    }[checked.action];
    response = await command({ id: checked.jobId }, checked.body, options);
  }
  const receipt = parse(
    jobFor(
      checked.sourceId,
      checked.action === "create" ? undefined : checked.jobId,
    ),
    response,
  );
  if (
    checked.action === "create" &&
    (receipt.source_revision !== checked.body.expected_revision ||
      receipt.target_name !== checked.body.target_name)
  )
    throw new ApiError(502, "invalid_response");
  return receipt;
}
export function newerCopy(current: CopyJob | undefined, incoming: CopyJob) {
  if (
    current?.id === incoming.id &&
    (current.source_project_id !== incoming.source_project_id ||
      current.target_project_id !== incoming.target_project_id ||
      current.source_revision !== incoming.source_revision ||
      !!current.script !== !!incoming.script ||
      (current.script &&
        incoming.script &&
        !sameScriptCounts(current.script.counts, incoming.script.counts)))
  )
    throw new ApiError(502, "invalid_response");
  // A repeated 202 is the original admission receipt, often older than GET facts.
  return current &&
    current.id === incoming.id &&
    current.revision >= incoming.revision
    ? current
    : incoming;
}
export function copyActions(job: CopyJob) {
  const stoppedFailure = job.status === "failed" && !job.execution_unconfirmed;
  return {
    cancel:
      !["succeeded", "cancelled", "cancel_requested"].includes(job.status) &&
      !job.cancellation_requested,
    retry:
      stoppedFailure &&
      job.retryable &&
      !job.needs_reconciliation &&
      !job.reconciliation_requested &&
      !job.cancellation_requested,
    reconcile:
      stoppedFailure &&
      job.needs_reconciliation &&
      !job.reconciliation_requested,
  };
}
