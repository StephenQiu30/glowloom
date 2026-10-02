import { z } from "zod";
import { ApiError } from "@/lib/request";
import {
  canonicalPlainText,
  richDocumentSchema,
  SCRIPT_MAX_SCALARS,
  scriptScalarText,
} from "./rich-document";
import {
  scriptUUID,
  sourceProvenanceSchema,
  sourceSummarySchema,
  sourceTitleSchema,
} from "./source-model";

export const reviewRevision = z
  .number()
  .int()
  .nonnegative()
  .max(Number.MAX_SAFE_INTEGER);
const positiveRevision = reviewRevision.refine((value) => value > 0);
export const scriptPosition = z
  .number()
  .int()
  .nonnegative()
  .max(SCRIPT_MAX_SCALARS);
export const scriptSHA = z.string().regex(/^[0-9a-f]{64}$/);
const timestamp = z.iso.datetime({ offset: true });
function bad(): never {
  throw new ApiError(502, "invalid_response");
}
export const scalarSpanSchema = z
  .object({ start: scriptPosition, end: scriptPosition })
  .strict()
  .refine((span) => span.end > span.start);
export const episodeBoundarySchema = z
  .object({
    seq_no: z.number().int().positive(),
    title: sourceTitleSchema,
    span_start: scriptPosition,
    span_end: scriptPosition,
    source_lineage_id: scriptUUID.optional(),
  })
  .strict()
  .refine((span) => span.span_end > span.span_start);
export type EpisodeBoundary = z.infer<typeof episodeBoundarySchema>;
export type ScalarSpan = z.infer<typeof scalarSpanSchema>;
export function validateReviewedBoundaries(
  value: unknown,
  preface: ScalarSpan | undefined,
  count: number,
) {
  scriptPosition.refine((value) => value > 0).parse(count);
  const boundaries = z
    .array(episodeBoundarySchema)
    .min(1)
    .max(2500)
    .parse(value);
  let next = 0;
  if (preface) {
    scalarSpanSchema.parse(preface);
    if (preface.start !== 0 || preface.end >= count)
      throw new Error("序言必须从正文起点到第一集前结束。");
    next = preface.end;
  }
  for (const [index, boundary] of boundaries.entries()) {
    if (
      boundary.seq_no !== index + 1 ||
      boundary.span_start !== next ||
      boundary.span_end > count
    )
      throw new Error(
        "分集必须从第 1 集起连续覆盖正文，不能有遗漏、重叠或零宽区间。",
      );
    next = boundary.span_end;
  }
  if (next !== count) throw new Error("完整分集必须覆盖正文终点。");
  return boundaries;
}
export function scalarSnippet(text: string, from: number, to: number) {
  scriptScalarText.parse(text);
  scriptPosition.parse(from);
  scriptPosition.parse(to);
  const characters = Array.from(text);
  if (to < from || to > characters.length)
    throw new Error("片段超出规范 Unicode 字符范围。");
  return characters.slice(from, to).join("");
}
export const versionSummarySchema = z
  .object({
    id: scriptUUID,
    version_no: positiveRevision,
    content_hash: scriptSHA,
    document_sha256: scriptSHA,
    source_manifest_sha256: scriptSHA,
    char_count: scriptPosition,
    source_count: z.number().int().nonnegative(),
    created_at: timestamp,
  })
  .strict();
export type ScriptVersionSummary = z.infer<typeof versionSummarySchema>;
export function readVersionPage(value: unknown, before: number, limit: number) {
  const result = z
    .object({
      items: z.array(versionSummarySchema).max(limit),
      next_version_no: positiveRevision.optional(),
    })
    .strict()
    .safeParse(value);
  if (!result.success) bad();
  const { items, next_version_no: next } = result.data;
  if (
    new Set(items.map((item) => item.id)).size !== items.length ||
    items.some(
      (item, index) =>
        (before > 0 && item.version_no >= before) ||
        (index > 0 && item.version_no >= items[index - 1].version_no),
    ) ||
    (next !== undefined && next !== items.at(-1)?.version_no)
  )
    bad();
  return result.data;
}
export function readSourceHistory(
  value: unknown,
  lineage: string,
  before: number,
  limit: number,
) {
  const result = z
    .object({
      items: z.array(sourceSummarySchema).max(limit),
      next_revision: positiveRevision.optional(),
    })
    .strict()
    .safeParse(value);
  if (!result.success) bad();
  const { items, next_revision: next } = result.data;
  if (
    new Set(items.map((item) => item.id)).size !== items.length ||
    items.some(
      (item, index) =>
        item.source_lineage_id !== lineage ||
        (before > 0 && item.source_revision >= before) ||
        (index > 0 && item.source_revision >= items[index - 1].source_revision),
    ) ||
    (next !== undefined && next !== items.at(-1)?.source_revision)
  )
    bad();
  return result.data;
}
export function readSourceSnapshot(value: unknown, id: string) {
  const result = sourceSummarySchema
    .extend({
      document: richDocumentSchema,
      plain_text: scriptScalarText,
      provenance: sourceProvenanceSchema,
      original_html: scriptScalarText.optional(),
    })
    .strict()
    .safeParse(value);
  if (
    !result.success ||
    result.data.id !== id ||
    canonicalPlainText(result.data.document) !== result.data.plain_text ||
    Array.from(result.data.plain_text).length !== result.data.char_count
  )
    bad();
  return result.data;
}
export type SourceSnapshot = ReturnType<typeof readSourceSnapshot>;
export const episodeSchema = z
  .object({
    id: scriptUUID,
    org_id: scriptUUID,
    project_id: scriptUUID,
    script_version_id: scriptUUID,
    split_set_id: scriptUUID,
    seq_no: z.number().int().positive(),
    title: sourceTitleSchema,
    span_start: scriptPosition,
    span_end: scriptPosition,
    revision: positiveRevision,
    current_structure_id: scriptUUID.optional(),
    confirmed_structure_id: scriptUUID.optional(),
    previous_episode_id: scriptUUID.optional(),
    inherit_status: z.string().min(1).max(128),
    is_delete: z.boolean(),
  })
  .strict()
  .refine((episode) => episode.span_end > episode.span_start);
export type ScriptEpisode = z.infer<typeof episodeSchema>;
export const episodeViewSchema = z
  .object({
    version_id: scriptUUID,
    head: z
      .object({
        version_id: scriptUUID,
        split_revision: reviewRevision,
        candidate_split_set_id: scriptUUID,
        confirmed_split_set_id: scriptUUID.optional(),
      })
      .strict(),
    candidate: z
      .object({
        id: scriptUUID,
        version_id: scriptUUID,
        org_id: scriptUUID,
        project_id: scriptUUID,
        kind: z.literal("candidate"),
        origin: z.enum(["sources", "rules"]),
        preface: scalarSpanSchema.optional(),
        boundaries: z.array(episodeBoundarySchema).max(2500),
        warnings: z.array(z.string()).optional(),
        created_at: timestamp,
      })
      .strict(),
    episodes: z.array(episodeSchema).max(2500),
  })
  .strict();
export function readEpisodeView(
  value: unknown,
  scope: { projectId: string; orgId: string },
  versionId: string,
  charCount?: number,
) {
  const result = episodeViewSchema.safeParse(value);
  if (!result.success) bad();
  const view = result.data;
  if (
    view.version_id !== versionId ||
    view.head.version_id !== versionId ||
    view.candidate.version_id !== versionId ||
    view.candidate.id !== view.head.candidate_split_set_id ||
    view.candidate.project_id !== scope.projectId ||
    view.candidate.org_id !== scope.orgId ||
    new Set(view.episodes.map((episode) => episode.id)).size !==
      view.episodes.length ||
    view.episodes.some(
      (episode, index) =>
        episode.project_id !== scope.projectId ||
        episode.org_id !== scope.orgId ||
        episode.script_version_id !== versionId ||
        episode.is_delete ||
        episode.split_set_id !== view.head.confirmed_split_set_id ||
        episode.seq_no !== index + 1 ||
        (index > 0 && episode.span_start < view.episodes[index - 1].span_end) ||
        (charCount !== undefined && episode.span_end > charCount),
    )
  )
    bad();
  if (charCount !== undefined && view.candidate.boundaries.length) {
    try {
      validateReviewedBoundaries(
        view.candidate.boundaries,
        view.candidate.preface,
        charCount,
      );
    } catch {
      bad();
    }
  }
  return view;
}
export type EpisodeView = ReturnType<typeof readEpisodeView>;

const actionSchema = z
  .object({
    type: z.literal("action"),
    line_key: scriptUUID,
    content: scriptScalarText.refine((value) => value.length > 0),
    span_start: scriptPosition,
    span_end: scriptPosition,
  })
  .strict();
const lineSchema = z
  .object({
    ...actionSchema.shape,
    type: z.literal("line"),
    kind: z.enum(["dialogue", "voiceover", "inner"]),
    speaker_text: scriptScalarText.optional(),
    character_id: scriptUUID.optional(),
    character_version_id: scriptUUID.optional(),
    emotion: scriptScalarText.optional(),
  })
  .strict();
export const structureItemSchema = z.discriminatedUnion("type", [
  actionSchema,
  lineSchema,
]);
export type StructureItem = z.infer<typeof structureItemSchema>;
export const structureSceneSchema = z
  .object({
    scene_key: scriptUUID,
    seq_no: z.number().int().positive(),
    heading: scriptScalarText,
    location_text: scriptScalarText,
    time_of_day: scriptScalarText,
    span_start: scriptPosition,
    span_end: scriptPosition,
    items: z
      .array(structureItemSchema)
      .nullable()
      .transform((items) => items ?? []),
  })
  .strict();
export const unassignedLineSchema = z
  .object({
    line_key: scriptUUID,
    content: scriptScalarText.refine((value) => value.length > 0),
    span_start: scriptPosition,
    span_end: scriptPosition,
  })
  .strict();
export const structureDocumentSchema = z
  .object({
    scenes: z
      .array(structureSceneSchema)
      .nullable()
      .transform((items) => items ?? []),
    unassigned_lines: z
      .array(unassignedLineSchema)
      .nullable()
      .transform((items) => items ?? []),
  })
  .strict()
  .refine(
    (document) =>
      document.scenes.every((scene) =>
        scene.items.every(
          (item) =>
            item.type !== "line" ||
            !item.character_version_id ||
            !!item.character_id,
        ),
      ),
    "角色版本必须与正式角色身份一并保留。",
  );
export type StructureDocument = z.infer<typeof structureDocumentSchema>;
export function parseStructureDocument(
  value: unknown,
  start: number,
  end: number,
) {
  scriptPosition.parse(start);
  scriptPosition.parse(end);
  const document = structureDocumentSchema.parse(value);
  if (
    end <= start ||
    (!document.scenes.length && !document.unassigned_lines.length) ||
    new TextEncoder().encode(JSON.stringify(document)).length > 1024 * 1024
  )
    throw new Error("手工结构不能为空或超过 1 MiB，正文区间必须有效。");
  const keys = new Set<string>();
  const assigned: { span_start: number; span_end: number }[] = [];
  function unique(key: string) {
    if (keys.has(key)) throw new Error("场景与行的稳定身份不能重复。");
    keys.add(key);
  }
  let sceneEnd = start;
  for (const [index, scene] of document.scenes.entries()) {
    unique(scene.scene_key);
    if (
      scene.seq_no !== index + 1 ||
      scene.span_start < sceneEnd ||
      scene.span_end <= scene.span_start ||
      scene.span_end > end
    )
      throw new Error("场景顺序或正文区间无效。");
    sceneEnd = scene.span_end;
    let itemEnd = scene.span_start;
    for (const item of scene.items) {
      unique(item.line_key);
      if (
        item.span_start < itemEnd ||
        item.span_end <= item.span_start ||
        item.span_end > scene.span_end
      )
        throw new Error("行动与台词必须在场景内按原顺序排列。");
      itemEnd = item.span_end;
      assigned.push(item);
    }
  }
  for (const line of document.unassigned_lines) {
    unique(line.line_key);
    if (
      line.span_start < start ||
      line.span_end <= line.span_start ||
      line.span_end > end
    )
      throw new Error("未归属行超出本集正文区间。");
    assigned.push(line);
  }
  assigned.sort((left, right) => left.span_start - right.span_start);
  if (
    assigned.some(
      (span, index) =>
        index > 0 && span.span_start < assigned[index - 1].span_end,
    )
  )
    throw new Error("结构中的源行区间不能重叠。");
  return document;
}
