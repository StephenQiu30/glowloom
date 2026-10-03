import { z } from "zod";
import * as script from "@/api/script";
import { ApiError } from "@/lib/request";
import { scriptUUID } from "./source-model";
import { scriptScalarText } from "./rich-document";
import {
  episodeSchema,
  reviewRevision,
  scalarSpanSchema,
  scriptPosition,
  scriptSHA,
  validateReviewedBoundaries,
  versionSummarySchema,
  type ScriptVersionSummary,
} from "./review-model";
import { listVersions } from "./review-queries";
import type { ScriptScope } from "./source-intent";
function invalid(): never {
  throw new ApiError(502, "invalid_response");
}
export async function findVersion(
  scope: ScriptScope,
  id: string,
  signal?: AbortSignal,
): Promise<ScriptVersionSummary> {
  scriptUUID.parse(id);
  let before = 0;
  const identities = new Set<string>();
  do {
    const page = await listVersions(scope, before, signal);
    for (const version of page.items) {
      if (identities.has(version.id)) invalid();
      identities.add(version.id);
    }
    const found = page.items.find((version) => version.id === id);
    if (found) return found;
    before = page.next_version_no ?? 0;
  } while (before > 0);
  throw new ApiError(404, "not_found");
}
export async function versionSourceText(
  scope: ScriptScope,
  version: ScriptVersionSummary,
  from: number,
  to: number,
  signal?: AbortSignal,
) {
  versionSummarySchema.parse(version);
  scriptPosition.parse(from);
  scriptPosition.parse(to);
  if (to <= from || to > version.char_count || to - from > 65536)
    throw new ApiError(422, "invalid_request");
  const result = z
    .object({
      script_version_id: scriptUUID,
      span_start: scriptPosition,
      span_end: scriptPosition,
      content_hash: scriptSHA,
      text: scriptScalarText,
    })
    .strict()
    .safeParse(
      await script.getScriptVersionSourceText(
        { pid: scope.projectId, vid: version.id, from, to },
        { signal },
      ),
    );
  if (
    !result.success ||
    result.data.script_version_id !== version.id ||
    result.data.span_start !== from ||
    result.data.span_end !== to ||
    result.data.content_hash !== version.content_hash ||
    result.data.text.includes("\r") ||
    Array.from(result.data.text).length !== to - from
  )
    invalid();
  return result.data;
}
const positive = reviewRevision.refine((n) => n > 0);
export const confirmationSummarySchema = z
  .object({
    id: scriptUUID,
    revision: positive,
    candidate_set_id: scriptUUID,
    formal_set_id: scriptUUID,
    episode_count: z.number().int().positive().max(2500),
    created_at: z.iso.datetime({ offset: true }),
  })
  .strict();
export async function listConfirmations(
  scope: ScriptScope,
  versionId: string,
  before = 0,
  signal?: AbortSignal,
) {
  scriptUUID.parse(versionId);
  reviewRevision.parse(before);
  const result = z
    .object({
      items: z.array(confirmationSummarySchema).max(25),
      next_revision: positive.optional(),
    })
    .strict()
    .safeParse(
      await script.listScriptSplitConfirmations(
        {
          pid: scope.projectId,
          vid: versionId,
          before_revision: before,
          limit: 25,
        },
        { signal },
      ),
    );
  if (
    !result.success ||
    new Set(result.data.items.map((item) => item.id)).size !==
      result.data.items.length ||
    result.data.items.some(
      (item, index, all) =>
        (before > 0 && item.revision >= before) ||
        (index > 0 && item.revision >= all[index - 1].revision),
    ) ||
    (result.data.next_revision !== undefined &&
      result.data.next_revision !== result.data.items.at(-1)?.revision)
  )
    invalid();
  return result.data;
}
export async function getConfirmation(
  scope: ScriptScope,
  version: ScriptVersionSummary,
  id: string,
  signal?: AbortSignal,
) {
  scriptUUID.parse(id);
  const result = z
    .object({
      id: scriptUUID,
      org_id: scriptUUID,
      project_id: scriptUUID,
      version_id: scriptUUID,
      candidate_set_id: scriptUUID,
      formal_set_id: scriptUUID,
      actor_id: scriptUUID,
      revision: positive,
      preface: scalarSpanSchema.optional(),
      episodes: z.array(episodeSchema).min(1).max(2500),
      created_at: z.iso.datetime({ offset: true }),
    })
    .strict()
    .safeParse(
      await script.getScriptSplitConfirmation(
        { pid: scope.projectId, vid: version.id, cid: id },
        { signal },
      ),
    );
  if (
    !result.success ||
    result.data.id !== id ||
    result.data.org_id !== scope.orgId ||
    result.data.project_id !== scope.projectId ||
    result.data.version_id !== version.id ||
    new Set(result.data.episodes.map((episode) => episode.id)).size !==
      result.data.episodes.length ||
    result.data.episodes.some(
      (episode) =>
        episode.org_id !== scope.orgId ||
        episode.project_id !== scope.projectId ||
        episode.script_version_id !== version.id ||
        episode.split_set_id !== result.data.formal_set_id,
    )
  )
    invalid();
  try {
    validateReviewedBoundaries(
      result.data.episodes.map((episode) => ({
        seq_no: episode.seq_no,
        title: episode.title,
        span_start: episode.span_start,
        span_end: episode.span_end,
      })),
      result.data.preface,
      version.char_count,
    );
  } catch {
    invalid();
  }
  return result.data;
}
