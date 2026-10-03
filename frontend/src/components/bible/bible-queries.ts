import {
  listBibleEntries,
  getBibleEntry,
  listBibleVersions,
  getBibleVersion,
  listBibleVoices,
  createBibleEntry,
  updateBibleEntry,
  confirmBibleEntry,
  deleteBibleEntry,
  restoreBibleEntry,
  mergeBibleCharacter,
  splitBibleCharacter,
  createBibleLook,
  updateBibleLook,
  deleteBibleLook,
  setBibleDefaultLook,
  replaceBibleReferences,
  bindBibleVoice,
  unbindBibleVoice,
  adoptBibleResult,
  createBibleEntryFromResult,
} from "@/api/bible";
import { ApiError } from "@/lib/request";
import { queryModels, type Model } from "@/components/catalog/queries";
import {
  bibleIdentitySchema,
  bibleUUID,
  readBiblePage,
  readBibleDetail,
  readBibleHistory,
  readBibleVersion,
  readBibleVoices,
  readBibleReceipt,
  type BibleIdentity,
  type BibleKind,
  type BibleVoiceChoice,
} from "./bible-model";
import { bibleIntentSchema, type BibleIntent } from "./bible-intent";
import { z } from "zod";
import { listMediaAssets, getMediaPreview } from "@/api/media";
import {
  getWorkspace,
  requireScriptScope,
} from "@/components/script/source-queries";
import {
  getEpisodes,
  getStructure,
  listStructureVersions,
} from "@/components/script/review-queries";
import { queryTask, queryTasks } from "@/components/operation/queries";
const referenceCandidate = z
  .object({
    id: bibleUUID,
    project_id: bibleUUID,
    kind: z.enum(["image", "audio"]),
    file_name: z.string(),
    mime_type: z.string(),
    byte_size: z.number().int().positive(),
    width: z.number().int().positive().optional(),
    height: z.number().int().positive().optional(),
    duration_ms: z.number().int().nonnegative().optional(),
    revision: z.number().int().positive(),
  })
  .strict();

export function bibleScopeKey(identity: BibleIdentity) {
  return [
    "bible",
    identity.origin,
    identity.actorId,
    identity.orgId,
    identity.projectId,
  ] as const;
}
export function bibleContextKey(origin: string, projectId: string) {
  return ["bible-context", origin, projectId] as const;
}
export async function listBible(
  projectId: string,
  kind: BibleKind,
  page: { cursor?: string; limit?: number } = {},
  signal?: AbortSignal,
  identity?: BibleIdentity,
) {
  bibleUUID.parse(projectId);
  return readBiblePage(
    await listBibleEntries(
      { pid: projectId, kind, ...page, limit: page.limit ?? 25 },
      { signal },
    ),
    projectId,
    kind,
    identity,
  );
}
export async function freshBibleScope(
  identity: BibleIdentity,
  signal?: AbortSignal,
) {
  bibleIdentitySchema.parse(identity);
  if (
    typeof window !== "undefined" &&
    window.location.origin !== identity.origin
  )
    throw new ApiError(403, "bible_scope_changed");
  const page = await listBible(
    identity.projectId,
    "character",
    { limit: 1 },
    signal,
  );
  if (
    page.current_actor_id !== identity.actorId ||
    page.current_org_id !== identity.orgId
  )
    throw new ApiError(403, "bible_scope_changed");
  return page;
}
export async function getBibleDetail(
  identity: BibleIdentity,
  kind: BibleKind,
  id: string,
  signal?: AbortSignal,
) {
  bibleUUID.parse(id);
  await freshBibleScope(identity, signal);
  return readBibleDetail(
    await getBibleEntry({ pid: identity.projectId, kind, id }, { signal }),
    identity,
    kind,
    id,
  );
}
export async function getBibleHistory(
  identity: BibleIdentity,
  kind: BibleKind,
  id: string,
  cursor?: string,
  signal?: AbortSignal,
) {
  bibleUUID.parse(id);
  await freshBibleScope(identity, signal);
  return readBibleHistory(
    await listBibleVersions(
      { pid: identity.projectId, kind, id, limit: 10, cursor },
      { signal },
    ),
    identity,
    kind,
    id,
  );
}
export async function getBibleSnapshot(
  identity: BibleIdentity,
  kind: BibleKind,
  id: string,
  version: string,
  signal?: AbortSignal,
) {
  bibleUUID.parse(id);
  bibleUUID.parse(version);
  await freshBibleScope(identity, signal);
  return readBibleVersion(
    await getBibleVersion(
      { pid: identity.projectId, kind, id, version },
      { signal },
    ),
    identity,
    kind,
    id,
    version,
  );
}
export async function getBibleVoices(
  identity: BibleIdentity,
  cursor?: string,
  signal?: AbortSignal,
) {
  await freshBibleScope(identity, signal);
  return readBibleVoices(
    await listBibleVoices(
      { pid: identity.projectId, limit: 50, cursor },
      { signal },
    ),
  );
}
export async function getBibleVoiceModel(
  identity: BibleIdentity,
  choice: BibleVoiceChoice,
  signal?: AbortSignal,
): Promise<Model> {
  await freshBibleScope(identity, signal);
  let cursor: string | undefined;
  const seen = new Set<string>();
  do {
    const page = await queryModels(
      identity.projectId,
      "audio.tts",
      signal,
      cursor,
    );
    const model = page.items.find((item) => item.key === choice.model_key);
    if (model) {
      const schema = model.current_version?.param_schema,
        voices = schema?.filter(
          (parameter) =>
            parameter.field === "voice_id" &&
            parameter.type === "string" &&
            parameter.component === "voice",
        );
      if (
        !model.current_version ||
        model.current_version.version_no !== choice.model_version ||
        !voices ||
        voices.length !== 1 ||
        !voices[0].enum?.includes(choice.voice_key)
      )
        throw new ApiError(409, "bible_voice_changed");
      return model;
    }
    cursor = page.next_cursor ?? undefined;
    if (cursor) {
      if (seen.has(cursor)) throw new ApiError(502, "invalid_model_cursor");
      seen.add(cursor);
    }
  } while (cursor);
  throw new ApiError(404, "bible_voice_model_unavailable");
}
export async function applyBibleIntent(input: BibleIntent) {
  const intent = bibleIntentSchema.parse(input),
    command = intent.command;
  await freshBibleScope({
    origin: intent.origin,
    actorId: intent.actorId,
    orgId: intent.orgId,
    projectId: intent.projectId,
  });
  const options = {
    headers: { "Idempotency-Key": intent.key, Origin: intent.origin },
  };
  const base = { pid: intent.projectId, kind: command.kind };
  let result: unknown;
  switch (command.action) {
    case "create":
      result = await createBibleEntry(base, command.body, options);
      break;
    case "update":
      result = await updateBibleEntry(
        { ...base, id: command.id! },
        command.body,
        options,
      );
      break;
    case "confirm":
      result = await confirmBibleEntry(
        { ...base, id: command.id },
        command.body,
        options,
      );
      break;
    case "delete":
      result = await deleteBibleEntry(
        { ...base, id: command.id },
        command.body,
        options,
      );
      break;
    case "restore":
      result = await restoreBibleEntry(
        { ...base, id: command.id },
        command.body,
        options,
      );
      break;
    case "merge":
      result = await mergeBibleCharacter(
        { ...base, kind: "character", id: command.id },
        command.body,
        options,
      );
      break;
    case "split":
      result = await splitBibleCharacter(
        { ...base, kind: "character", id: command.id },
        command.body,
        options,
      );
      break;
    case "look_create":
      result = await createBibleLook(
        { ...base, id: command.id },
        command.body,
        options,
      );
      break;
    case "look_update":
      result = await updateBibleLook(
        { ...base, id: command.id, look: command.lookId! },
        command.body,
        options,
      );
      break;
    case "look_delete":
      result = await deleteBibleLook(
        { ...base, id: command.id, look: command.lookId },
        command.body,
        options,
      );
      break;
    case "look_default":
      result = await setBibleDefaultLook(
        { ...base, id: command.id, look: command.lookId },
        command.body,
        options,
      );
      break;
    case "references":
      result = await replaceBibleReferences(
        { ...base, id: command.id, look: command.lookId },
        command.body,
        options,
      );
      break;
    case "voice_bind":
      result = await bindBibleVoice(
        { ...base, id: command.id },
        command.body,
        options,
      );
      break;
    case "voice_unbind":
      result = await unbindBibleVoice(
        { ...base, id: command.id },
        command.body,
        options,
      );
      break;
    case "adopt_result":
      result = await adoptBibleResult(
        { ...base, id: command.id! },
        command.body,
        options,
      );
      break;
    case "create_result":
      result = await createBibleEntryFromResult(base, command.body, options);
      break;
  }
  return readBibleReceipt(result, command);
}
export async function getBibleMediaCandidates(
  identity: BibleIdentity,
  kind: "image" | "audio",
  cursor?: string,
  signal?: AbortSignal,
) {
  await freshBibleScope(identity, signal);
  const result = z
    .object({
      items: z.array(referenceCandidate).max(50),
      next_cursor: z.string().min(1).nullable(),
    })
    .strict()
    .safeParse(
      await listMediaAssets(
        { pid: identity.projectId, kind, limit: 50, cursor },
        { signal },
      ),
    );
  if (
    !result.success ||
    result.data.items.some(
      (asset) => asset.project_id !== identity.projectId || asset.kind !== kind,
    ) ||
    new Set(result.data.items.map((asset) => asset.id)).size !==
      result.data.items.length
  )
    throw new ApiError(502, "invalid_bible_media");
  return result.data;
}
export async function getBiblePreview(
  identity: BibleIdentity,
  id: string,
  kind: "image" | "audio",
  revision?: number,
  signal?: AbortSignal,
) {
  bibleUUID.parse(id);
  await freshBibleScope(identity, signal);
  const parsed = z
    .object({
      asset: referenceCandidate,
      url: z.url().refine((value) => {
        const url = new URL(value);
        return (
          ["http:", "https:"].includes(url.protocol) &&
          !url.username &&
          !url.password
        );
      }),
      expires_at: z.iso.datetime({ offset: true }),
    })
    .strict()
    .safeParse(
      await getMediaPreview(
        { pid: identity.projectId, asset_id: id },
        { signal },
      ),
    );
  if (!parsed.success) throw new ApiError(502, "invalid_bible_media");
  const value = parsed.data;
  if (
    value.asset.id !== id ||
    value.asset.project_id !== identity.projectId ||
    value.asset.kind !== kind ||
    (revision !== undefined && value.asset.revision !== revision)
  )
    throw new ApiError(409, "bible_media_changed");
  return value;
}
export async function getBibleFormalEpisodes(
  identity: BibleIdentity,
  signal?: AbortSignal,
) {
  await freshBibleScope(identity, signal);
  const workspace = await getWorkspace(identity.projectId, signal);
  requireScriptScope(workspace, identity);
  const version =
    workspace.state.adopted_version_id ?? workspace.state.draft_version_id;
  return version
    ? (await getEpisodes(identity, version, undefined, signal)).episodes
    : [];
}
export async function getBibleFormalScenes(
  identity: BibleIdentity,
  episodeId: string,
  signal?: AbortSignal,
) {
  const episode = (await getBibleFormalEpisodes(identity, signal)).find(
    (item) => item.id === episodeId,
  );
  if (!episode) throw new ApiError(404, "bible_scope_unavailable");
  if (!episode.confirmed_structure_id) return [];
  let before = 0;
  for (;;) {
    const page = await listStructureVersions(
        identity,
        episodeId,
        before,
        signal,
      ),
      confirmed = page.items.find(
        (item) => item.id === episode.confirmed_structure_id,
      );
    if (confirmed)
      return (
        await getStructure(identity, episodeId, confirmed.version_no, signal)
      ).structure.document.scenes;
    if (!page.next_version_no) return [];
    if (before && page.next_version_no >= before)
      throw new ApiError(502, "invalid_bible_scope_cursor");
    before = page.next_version_no;
  }
}
export async function getBibleResults(
  identity: BibleIdentity,
  cursor?: string,
  signal?: AbortSignal,
) {
  await freshBibleScope(identity, signal);
  const page = await queryTasks(
    identity.projectId,
    { status: "completed", capability: "text.generate", cursor },
    signal,
  );
  if (
    page.items.some(
      (item) =>
        item.project_id !== identity.projectId ||
        item.status !== "completed" ||
        item.capability !== "text.generate",
    )
  )
    throw new ApiError(502, "invalid_bible_result");
  return page;
}
export async function getBibleResult(
  identity: BibleIdentity,
  id: string,
  signal?: AbortSignal,
) {
  bibleUUID.parse(id);
  await freshBibleScope(identity, signal);
  const task = await queryTask(identity.projectId, id, signal);
  if (
    task.id !== id ||
    task.project_id !== identity.projectId ||
    task.status !== "completed" ||
    task.capability !== "text.generate"
  )
    throw new ApiError(409, "bible_result_not_completed");
  return task;
}
