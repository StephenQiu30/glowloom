import { z } from "zod";
import * as script from "@/api/script";
import { ApiError } from "@/lib/request";
import { scriptScalarText } from "./rich-document";
import { scriptUUID } from "./source-model";
import {
  episodeSchema,
  parseStructureDocument,
  readEpisodeView,
  readSourceHistory,
  readSourceSnapshot,
  readVersionPage,
  reviewRevision,
  scriptPosition,
  scriptSHA,
  structureDocumentSchema,
  type ScriptEpisode,
} from "./review-model";
import {
  readSourceControlReceipt,
  readSourceWrite,
  readSourceWritePage,
  type SourceControlCommand,
} from "./source-write-model";
import type { ScriptScope } from "./source-intent";
import { scriptScopeKey } from "./source-queries";
import {
  readReviewReceipt,
  reviewIntentSchema,
  type ReviewIntent,
} from "./review-intent";

export const reviewScopeKey = (scope: ScriptScope) =>
  [...scriptScopeKey(scope), "review"] as const;
export async function runReviewIntent(
  value: ReviewIntent,
  signal?: AbortSignal,
) {
  const intent = reviewIntentSchema.parse(value);
  const options = {
    signal,
    headers: {
      "Idempotency-Key": intent.key,
      Origin: intent.origin,
      "X-Request-ID": crypto.randomUUID(),
    },
  };
  const response =
    intent.action === "adopt"
      ? await script.adoptScriptVersion(
          { pid: intent.projectId, vid: intent.versionId },
          intent.body,
          options,
        )
      : intent.action === "resplit"
        ? await script.resplitScriptEpisodes(
            { pid: intent.projectId },
            intent.body,
            options,
          )
        : intent.action === "confirm_split"
          ? await script.confirmScriptEpisodeSplit(
              { pid: intent.projectId },
              intent.body,
              options,
            )
          : intent.action === "save_structure"
            ? await script.saveScriptEpisodeStructure(
                { eid: intent.episodeId },
                intent.body,
                options,
              )
            : await script.confirmScriptEpisodeStructure(
                { eid: intent.episodeId },
                intent.body,
                options,
              );
  return readReviewReceipt(response, intent);
}
function bad(): never {
  throw new ApiError(502, "invalid_response");
}
export async function listVersions(
  scope: ScriptScope,
  before = 0,
  signal?: AbortSignal,
) {
  reviewRevision.parse(before);
  return readVersionPage(
    await script.listScriptVersions(
      { pid: scope.projectId, before_version_no: before, limit: 25 },
      { signal },
    ),
    before,
    25,
  );
}
export async function listSourceHistory(
  scope: ScriptScope,
  lineage: string,
  before = 0,
  signal?: AbortSignal,
) {
  scriptUUID.parse(lineage);
  reviewRevision.parse(before);
  return readSourceHistory(
    await script.listScriptSourceHistory(
      { pid: scope.projectId, lineage, before_revision: before, limit: 25 },
      { signal },
    ),
    lineage,
    before,
    25,
  );
}
export async function getSourceSnapshot(
  scope: ScriptScope,
  id: string,
  signal?: AbortSignal,
) {
  scriptUUID.parse(id);
  return readSourceSnapshot(
    await script.getScriptSourceSnapshot(
      { pid: scope.projectId, sid: id },
      { signal },
    ),
    id,
  );
}
export async function getEpisodes(
  scope: ScriptScope,
  versionId: string,
  charCount?: number,
  signal?: AbortSignal,
) {
  scriptUUID.parse(versionId);
  return readEpisodeView(
    await script.listScriptEpisodes(
      { pid: scope.projectId, version_id: versionId },
      { signal },
    ),
    scope,
    versionId,
    charCount,
  );
}
export async function episodeSourceText(
  scope: ScriptScope,
  episode: ScriptEpisode,
  from: number,
  to: number,
  signal?: AbortSignal,
) {
  scriptPosition.parse(from);
  scriptPosition.parse(to);
  if (
    episode.project_id !== scope.projectId ||
    episode.org_id !== scope.orgId ||
    from < episode.span_start ||
    to > episode.span_end ||
    to <= from ||
    to - from > 65536
  )
    throw new ApiError(422, "invalid_request");
  const result = z
    .object({
      episode_id: scriptUUID,
      script_version_id: scriptUUID,
      span_start: scriptPosition,
      span_end: scriptPosition,
      content_hash: scriptSHA,
      text: scriptScalarText,
    })
    .strict()
    .safeParse(
      await script.getScriptEpisodeSourceText(
        { eid: episode.id, from, to },
        { signal },
      ),
    );
  if (
    !result.success ||
    result.data.episode_id !== episode.id ||
    result.data.script_version_id !== episode.script_version_id ||
    result.data.span_start !== from ||
    result.data.span_end !== to ||
    Array.from(result.data.text).length !== to - from
  )
    bad();
  return result.data;
}
export async function getStructure(
  scope: ScriptScope,
  episodeId: string,
  version?: number,
  signal?: AbortSignal,
) {
  scriptUUID.parse(episodeId);
  if (version !== undefined)
    reviewRevision.refine((value) => value > 0).parse(version);
  const value =
    version === undefined
      ? await script.getScriptEpisodeStructure({ eid: episodeId }, { signal })
      : await script.getScriptEpisodeStructureVersion(
          { eid: episodeId, version },
          { signal },
        );
  const result = z
    .object({
      episode: episodeSchema,
      structure: z
        .object({
          id: scriptUUID,
          org_id: scriptUUID,
          project_id: scriptUUID,
          episode_id: scriptUUID,
          version_no: reviewRevision.refine((value) => value > 0),
          source_hash: scriptSHA,
          document: structureDocumentSchema,
          actor_id: scriptUUID,
          created_at: z.iso.datetime({ offset: true }),
        })
        .strict(),
    })
    .strict()
    .safeParse(value);
  if (!result.success) bad();
  const { episode, structure } = result.data;
  if (
    episode.id !== episodeId ||
    episode.project_id !== scope.projectId ||
    episode.org_id !== scope.orgId ||
    structure.project_id !== scope.projectId ||
    structure.org_id !== scope.orgId ||
    structure.episode_id !== episodeId ||
    (version !== undefined && structure.version_no !== version) ||
    (version === undefined && structure.id !== episode.current_structure_id)
  )
    bad();
  try {
    parseStructureDocument(
      structure.document,
      episode.span_start,
      episode.span_end,
    );
  } catch {
    bad();
  }
  return result.data;
}
export async function listStructureVersions(
  scope: ScriptScope,
  episodeId: string,
  before = 0,
  signal?: AbortSignal,
) {
  scriptUUID.parse(episodeId);
  reviewRevision.parse(before);
  const result = z
    .object({
      items: z
        .array(
          z
            .object({
              id: scriptUUID,
              episode_id: scriptUUID,
              version_no: reviewRevision.refine((value) => value > 0),
              source_hash: scriptSHA,
              created_at: z.iso.datetime({ offset: true }),
            })
            .strict(),
        )
        .max(25),
      next_version_no: reviewRevision.refine((value) => value > 0).optional(),
    })
    .strict()
    .safeParse(
      await script.listScriptEpisodeStructureVersions(
        { eid: episodeId, before_version_no: before, limit: 25 },
        { signal },
      ),
    );
  if (
    !result.success ||
    new Set(result.data.items.map((item) => item.id)).size !==
      result.data.items.length ||
    result.data.items.some(
      (item, index) =>
        item.episode_id !== episodeId ||
        (before > 0 && item.version_no >= before) ||
        (index > 0 &&
          item.version_no >= result.data.items[index - 1].version_no),
    ) ||
    (result.data.next_version_no !== undefined &&
      result.data.next_version_no !== result.data.items.at(-1)?.version_no)
  )
    bad();
  return result.data;
}
export async function listSourceWrites(
  scope: ScriptScope,
  after = 0,
  signal?: AbortSignal,
) {
  reviewRevision.parse(after);
  return readSourceWritePage(
    await script.listScriptSourceWrites(
      { pid: scope.projectId, after, limit: 25 },
      { signal },
    ),
    scope,
    after,
    25,
  );
}
export async function getSourceWrite(
  scope: ScriptScope,
  id: string,
  signal?: AbortSignal,
) {
  scriptUUID.parse(id);
  return readSourceWrite(
    await script.getScriptSourceWrite(
      { pid: scope.projectId, wid: id },
      { signal },
    ),
    id,
  );
}
export async function runSourceControl(
  scope: ScriptScope,
  command: SourceControlCommand,
  key: string,
) {
  scriptUUID.parse(key);
  const options = {
    headers: {
      "Idempotency-Key": key,
      Origin: scope.origin,
      "X-Request-ID": crypto.randomUUID(),
    },
  };
  const params = { pid: scope.projectId, wid: command.intentId };
  return readSourceControlReceipt(
    await (command.action === "cancel"
      ? script.cancelScriptSourceWrite(params, command.body, options)
      : script.reconcileScriptSourceWrite(params, command.body, options)),
    command,
  );
}
