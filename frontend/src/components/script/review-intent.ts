import { z } from "zod";
import { ApiError } from "@/lib/request";
import { scriptUUID } from "./source-model";
import {
  sameScriptScope,
  scriptScopeSchema,
  type ScriptScope,
} from "./source-intent";
import {
  episodeBoundarySchema,
  episodeSchema,
  parseStructureDocument,
  reviewRevision,
  scalarSpanSchema,
  scriptPosition,
  structureDocumentSchema,
  validateReviewedBoundaries,
} from "./review-model";
const splitBase = z
  .object({
    version_id: scriptUUID,
    expected_revision: reviewRevision,
    expected_split_revision: reviewRevision,
    candidate_set_id: scriptUUID,
    ack_invalidate: z.boolean(),
  })
  .strict();
const structureBase = z
  .object({
    expected_revision: reviewRevision,
    expected_episode_revision: reviewRevision.refine((n) => n > 0),
    base_structure_version_no: reviewRevision,
  })
  .strict();
const schemas = [
  z.object({ action: z.literal("resplit"), body: splitBase }).strict(),
  z
    .object({
      action: z.literal("confirm_split"),
      charCount: scriptPosition.refine((n) => n > 0),
      body: splitBase
        .extend({
          boundaries: z.array(episodeBoundarySchema).min(1).max(2500),
          preface: scalarSpanSchema.optional(),
        })
        .strict(),
    })
    .strict(),
  z
    .object({
      action: z.literal("save_structure"),
      episodeId: scriptUUID,
      spanStart: scriptPosition,
      spanEnd: scriptPosition,
      body: structureBase
        .extend({ document: structureDocumentSchema })
        .strict(),
    })
    .strict(),
  z
    .object({
      action: z.literal("confirm_structure"),
      episodeId: scriptUUID,
      structureId: scriptUUID,
      body: structureBase
        .extend({
          base_structure_version_no: reviewRevision.refine((n) => n > 0),
          ack_invalidate: z.boolean(),
        })
        .strict(),
    })
    .strict(),
  z
    .object({
      action: z.literal("adopt"),
      versionId: scriptUUID,
      splitSetId: scriptUUID,
      episodeIds: z
        .array(scriptUUID)
        .min(1)
        .max(2500)
        .refine((ids) => new Set(ids).size === ids.length),
      body: z
        .object({
          expected_revision: reviewRevision.refine((n) => n > 0),
          expected_split_revision: reviewRevision.refine((n) => n > 0),
          ack_invalidate: z.boolean(),
        })
        .strict(),
    })
    .strict(),
] as const;
function checkCommand(command: z.infer<(typeof schemas)[number]>) {
  if (command.action === "confirm_split")
    validateReviewedBoundaries(
      command.body.boundaries,
      command.body.preface,
      command.charCount,
    );
  if (command.action === "save_structure")
    parseStructureDocument(
      command.body.document,
      command.spanStart,
      command.spanEnd,
    );
}
export const reviewCommandSchema = z
  .discriminatedUnion("action", schemas)
  .superRefine((value, ctx) => {
    try {
      checkCommand(value);
    } catch (cause) {
      ctx.addIssue({
        code: "custom",
        message: cause instanceof Error ? cause.message : "审核正文无效。",
      });
    }
  });
export type ReviewCommand = z.infer<typeof reviewCommandSchema>;
const identity = {
  ...scriptScopeSchema.shape,
  version: z.literal(1),
  key: scriptUUID,
};
export const reviewIntentSchema = z
  .discriminatedUnion("action", [
    schemas[0].extend(identity).strict(),
    schemas[1].extend(identity).strict(),
    schemas[2].extend(identity).strict(),
    schemas[3].extend(identity).strict(),
    schemas[4].extend(identity).strict(),
  ])
  .superRefine((value, ctx) => {
    try {
      checkCommand(value);
    } catch (cause) {
      ctx.addIssue({
        code: "custom",
        message: cause instanceof Error ? cause.message : "原审核正文无效。",
      });
    }
  });
export type ReviewIntent = z.infer<typeof reviewIntentSchema>;
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
function storageKey(scope: ScriptScope) {
  return [
    "lanverse:script-review:v1",
    scope.origin,
    scope.actorId,
    scope.orgId,
    scope.projectId,
  ]
    .map(encodeURIComponent)
    .join(":");
}
function storageFailure() {
  return new Error(
    "审核原意图存储不可用或不一致。请恢复存储后人工核验原键，当前不会发送新审核。",
  );
}
export function loadReviewIntent(
  storage: IntentStorage,
  scope: ScriptScope,
): ReviewIntent | null {
  try {
    const body = storage.getItem(storageKey(scope));
    if (body === null) return null;
    const checked = reviewIntentSchema.parse(JSON.parse(body));
    if (!sameScriptScope(checked, scope)) throw storageFailure();
    return checked;
  } catch {
    throw storageFailure();
  }
}
export function saveReviewIntent(storage: IntentStorage, value: ReviewIntent) {
  try {
    const checked = reviewIntentSchema.parse(value);
    const encoded = JSON.stringify(checked);
    const previous = loadReviewIntent(storage, checked);
    if (previous && JSON.stringify(previous) !== encoded)
      throw storageFailure();
    storage.setItem(storageKey(checked), encoded);
    if (storage.getItem(storageKey(checked)) !== encoded)
      throw storageFailure();
  } catch {
    throw storageFailure();
  }
}
export function clearReviewIntent(storage: IntentStorage, scope: ScriptScope) {
  try {
    storage.removeItem(storageKey(scope));
    if (storage.getItem(storageKey(scope)) !== null) throw storageFailure();
  } catch {
    throw storageFailure();
  }
}
const positive = reviewRevision.refine((n) => n > 0);
const mappingSchema = z
  .object({
    previous_episode_id: scriptUUID.optional(),
    episode_id: scriptUUID.optional(),
    inherit_status: z.enum(["retained", "not_inherited", "invalidated"]),
  })
  .strict();
const array = <T extends z.ZodType>(item: T) =>
  z
    .array(item)
    .max(2500)
    .nullable()
    .transform((items) => items ?? []);
const splitReceiptSchema = z
  .object({
    script_revision: positive,
    project_revision: positive,
    split_revision: positive,
    split_set_id: scriptUUID,
    confirmation_id: scriptUUID.optional(),
    episodes: array(episodeSchema),
    episode_mappings: array(mappingSchema),
    renamed_episode_ids: array(scriptUUID),
  })
  .strict();
const structureReceiptSchema = z
  .object({
    script_revision: positive,
    project_revision: positive,
    episode_revision: positive,
    structure_id: scriptUUID,
    version_no: positive,
    review_status: z.enum(["candidate", "confirmed"]),
  })
  .strict();
function invalid(): never {
  throw new ApiError(502, "invalid_response");
}
export function readReviewReceipt(value: unknown, original: ReviewIntent) {
  if (original.action === "adopt") {
    const checked = z
      .object({
        script_revision: positive,
        project_revision: positive,
        version_id: scriptUUID,
        previous_version_id: scriptUUID.optional(),
        split_set_id: scriptUUID,
        episode_mappings: array(mappingSchema),
        changed: z.boolean(),
        duplicate: z.boolean(),
      })
      .strict()
      .safeParse(value);
    if (!checked.success) invalid();
    const receipt = checked.data;
    if (
      receipt.changed === receipt.duplicate ||
      receipt.script_revision !==
        original.body.expected_revision + (receipt.changed ? 1 : 0) ||
      receipt.version_id !== original.versionId ||
      receipt.split_set_id !== original.splitSetId ||
      (receipt.duplicate &&
        receipt.previous_version_id !== original.versionId) ||
      (receipt.changed && receipt.previous_version_id) ||
      receipt.episode_mappings.length !== original.episodeIds.length
    )
      invalid();
    const ids = new Set<string>();
    for (const mapping of receipt.episode_mappings) {
      if (
        !mapping.episode_id ||
        mapping.previous_episode_id ||
        !original.episodeIds.includes(mapping.episode_id) ||
        ids.has(mapping.episode_id) ||
        mapping.inherit_status !==
          (receipt.duplicate ? "retained" : "not_inherited")
      )
        invalid();
      ids.add(mapping.episode_id);
    }
    return receipt;
  }
  if (original.action === "resplit" || original.action === "confirm_split") {
    const checked = splitReceiptSchema.safeParse(value);
    if (!checked.success) invalid();
    const receipt = checked.data;
    if (
      receipt.script_revision !== original.body.expected_revision + 1 ||
      receipt.split_revision !== original.body.expected_split_revision + 1
    )
      invalid();
    if (original.action === "resplit") {
      if (
        receipt.confirmation_id ||
        receipt.episodes.length ||
        receipt.episode_mappings.length ||
        receipt.renamed_episode_ids.length
      )
        invalid();
    } else {
      const boundaries = original.body.boundaries;
      if (
        !receipt.confirmation_id ||
        receipt.episodes.length !== boundaries.length ||
        new Set(receipt.episodes.map((episode) => episode.id)).size !==
          receipt.episodes.length
      )
        invalid();
      for (const [index, episode] of receipt.episodes.entries()) {
        const boundary = boundaries[index];
        if (
          episode.org_id !== original.orgId ||
          episode.project_id !== original.projectId ||
          episode.script_version_id !== original.body.version_id ||
          episode.split_set_id !== receipt.split_set_id ||
          episode.is_delete ||
          episode.seq_no !== boundary.seq_no ||
          episode.title !== boundary.title ||
          episode.span_start !== boundary.span_start ||
          episode.span_end !== boundary.span_end
        )
          invalid();
      }
      const previous = new Set<string>();
      const next = new Set<string>();
      const episodeIDs = new Set(receipt.episodes.map((episode) => episode.id));
      for (const mapping of receipt.episode_mappings) {
        if (mapping.previous_episode_id) {
          if (previous.has(mapping.previous_episode_id)) invalid();
          previous.add(mapping.previous_episode_id);
        }
        if (mapping.episode_id) {
          if (
            next.has(mapping.episode_id) ||
            !episodeIDs.has(mapping.episode_id)
          )
            invalid();
          next.add(mapping.episode_id);
        }
        if (
          mapping.inherit_status === "retained"
            ? !mapping.previous_episode_id ||
              mapping.previous_episode_id !== mapping.episode_id
            : mapping.inherit_status === "invalidated"
              ? !mapping.previous_episode_id || Boolean(mapping.episode_id)
              : !mapping.episode_id || Boolean(mapping.previous_episode_id)
        )
          invalid();
      }
      if (
        next.size !== episodeIDs.size ||
        new Set(receipt.renamed_episode_ids).size !==
          receipt.renamed_episode_ids.length ||
        receipt.renamed_episode_ids.some((id) => !episodeIDs.has(id))
      )
        invalid();
    }
    return receipt;
  }
  const checked = structureReceiptSchema.safeParse(value);
  if (!checked.success) invalid();
  const receipt = checked.data;
  const confirming = original.action === "confirm_structure";
  if (
    receipt.script_revision !== original.body.expected_revision + 1 ||
    receipt.episode_revision !== original.body.expected_episode_revision + 1 ||
    receipt.version_no !==
      original.body.base_structure_version_no + (confirming ? 0 : 1) ||
    receipt.review_status !== (confirming ? "confirmed" : "candidate") ||
    (confirming && receipt.structure_id !== original.structureId)
  )
    invalid();
  return receipt;
}
export type ReviewReceipt = ReturnType<typeof readReviewReceipt>;
export const reviewImpactSchema = z
  .object({
    affected_episodes: z
      .array(
        z
          .object({
            episode_id: scriptUUID,
            confirmed_structure_id: scriptUUID,
            reason: z.enum([
              "boundary_changed",
              "structure_replaced",
              "adopted_version_changed",
            ]),
          })
          .strict(),
      )
      .min(1)
      .max(2500),
  })
  .strict()
  .refine(
    (value) =>
      new Set(value.affected_episodes.map((episode) => episode.episode_id))
        .size === value.affected_episodes.length,
  );
