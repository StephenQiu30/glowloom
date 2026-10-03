import { z } from "zod";
import { ApiError } from "@/lib/request";
import {
  canonicalPlainText,
  richDocumentSchema,
  scriptScalarText,
  SCRIPT_MAX_DOCUMENT_BYTES,
  SCRIPT_MAX_SCALARS,
} from "./rich-document";

export const scriptUUID = z
  .string()
  .uuid()
  .refine((value) => value !== "00000000-0000-0000-0000-000000000000");
const revision = z.number().int().min(0).max(Number.MAX_SAFE_INTEGER);
const positiveRevision = revision.refine((value) => value > 0);
const scalarPosition = z.number().int().min(0).max(SCRIPT_MAX_SCALARS);
const sha = z.string().regex(/^[0-9a-f]{64}$/);
export const sourceKindSchema = z.enum(["chapter", "episode", "document"]);
export const sourceStatusSchema = z.enum(["draft", "ready", "completed"]);
export const sourceTitleSchema = scriptScalarText
  .refine((value) => value.trim().length > 0, "请填写来源标题。")
  .refine((value) => Array.from(value).length <= 512, "标题最多 512 个字符。");
export const scriptWorkspaceSchema = z
  .object({
    current_actor_id: scriptUUID,
    current_org_id: scriptUUID,
    state: z
      .object({
        project_id: scriptUUID,
        org_id: scriptUUID,
        revision,
        draft_version_id: scriptUUID.optional(),
        adopted_version_id: scriptUUID.optional(),
        updated_at: z.iso.datetime({ offset: true }),
      })
      .strict(),
  })
  .strict()
  .refine((value) => value.current_org_id === value.state.org_id)
  .refine(
    (value) =>
      value.state.revision > 0 ||
      (!value.state.draft_version_id && !value.state.adopted_version_id),
  );
export type ScriptWorkspace = z.infer<typeof scriptWorkspaceSchema>;
export const sourceSummarySchema = z
  .object({
    id: scriptUUID,
    source_lineage_id: scriptUUID,
    previous_source_id: scriptUUID.optional(),
    source_revision: positiveRevision,
    source_kind: sourceKindSchema,
    title: sourceTitleSchema,
    status: sourceStatusSchema,
    origin: z.enum(["file", "manual", "beeftv"]),
    position: z.number().int().min(0),
    char_count: scalarPosition,
    content_hash: sha,
    rich_sha256: sha,
    media_asset_id: scriptUUID.optional(),
  })
  .strict();
export type SourceSummary = z.infer<typeof sourceSummarySchema>;
export const sourcePageSchema = z
  .object({
    version_id: scriptUUID.optional(),
    items: z.array(sourceSummarySchema).max(100),
    next_position: z.number().int().positive().optional(),
  })
  .strict()
  .refine(
    (page) =>
      Boolean(page.version_id) ||
      (!page.items.length && page.next_position === undefined),
  )
  .refine(
    (page) =>
      new Set(page.items.map((item) => item.id)).size === page.items.length &&
      new Set(page.items.map((item) => item.source_lineage_id)).size ===
        page.items.length,
  );
const externalLabels = {
  external_id: scriptUUID.optional(),
  external_position: z.number().int().nonnegative().optional(),
};
const mappingSchema = z
  .object({
    part: scriptScalarText,
    paragraph: z.number().int().nonnegative().optional(),
    decoded_start: scalarPosition.optional(),
    decoded_end: scalarPosition.optional(),
    span_start: scalarPosition,
    span_end: scalarPosition,
  })
  .strict()
  .refine(
    (mapping) =>
      mapping.span_end >= mapping.span_start &&
      (mapping.decoded_start === undefined) ===
        (mapping.decoded_end === undefined) &&
      (mapping.decoded_start === undefined ||
        mapping.decoded_end! >= mapping.decoded_start),
  );
export const sourceExtractionWarningSchema = z
  .object({
    code: z.enum([
      "unsupported_document_element",
      "table_layout_flattened",
      "document_layout_omitted",
      "unsupported_heading_style",
      "paragraph_style_omitted",
      "unsupported_alignment",
      "paragraph_layout_omitted",
      "unsupported_paragraph_content",
      "unsupported_run_content",
      "embedded_media_omitted",
      "unsupported_hyperlink",
      "document_revision_omitted",
      "underline_style_omitted",
      "unsupported_text_color",
      "unsupported_highlight_color",
      "run_format_omitted",
      "duplicate_run_format",
      "unsupported_numbering_format",
      "numbering_missing_parent",
      "numbered_heading_as_paragraph",
    ]),
    count: z.number().int().min(1).max(200000),
    paragraph: z.number().int().min(0).max(199999).optional(),
  })
  .strict();
export type SourceExtractionWarning = z.infer<
  typeof sourceExtractionWarningSchema
>;
export const sourceProvenanceSchema = z
  .object({
    ...externalLabels,
    encoding: scriptScalarText.optional(),
    mapping: z.array(mappingSchema).optional(),
    warnings: z
      .array(sourceExtractionWarningSchema)
      .max(64)
      .refine(
        (items) =>
          new Set(items.map((item) => item.code)).size === items.length,
      )
      .optional(),
  })
  .strict();
export const sourceDetailSchema = sourceSummarySchema
  .extend({
    document: richDocumentSchema,
    plain_text: scriptScalarText,
    source_span: z
      .object({
        source_id: scriptUUID,
        source_lineage_id: scriptUUID,
        position: z.number().int().nonnegative(),
        start: scalarPosition,
        end: scalarPosition,
      })
      .strict(),
    provenance: sourceProvenanceSchema,
  })
  .superRefine((source, ctx) => {
    const span = source.source_span;
    if (
      span.source_id !== source.id ||
      span.source_lineage_id !== source.source_lineage_id ||
      span.position !== source.position ||
      (span.end - span.start !== source.char_count &&
        (source.char_count === 0 ||
          span.end - span.start !== source.char_count + 2)) ||
      canonicalPlainText(source.document) !== source.plain_text ||
      Array.from(source.plain_text).length !== source.char_count
    )
      ctx.addIssue({
        code: "custom",
        message: "来源身份、正文或坐标事实不一致。",
      });
  });
export type SourceDetail = z.infer<typeof sourceDetailSchema>;
export const sourceEditLabelsSchema = z.object(externalLabels).strict();
const originalHTML = scriptScalarText.refine(
  (value) =>
    new TextEncoder().encode(value).length <= SCRIPT_MAX_DOCUMENT_BYTES,
  "原 HTML 超过 8MiB。",
);
const itemFields = {
  source_kind: sourceKindSchema,
  title: sourceTitleSchema,
  status: sourceStatusSchema,
  document: richDocumentSchema.optional(),
  original_html: originalHTML.optional(),
  provenance: sourceEditLabelsSchema,
};
const oneBody = (value: { document?: unknown; original_html?: string }) =>
  (value.document !== undefined) !== (value.original_html !== undefined);
export const sourceImportItemSchema = z
  .object(itemFields)
  .strict()
  .refine(oneBody, "请选择完整富文本或原 HTML 中的一种输入。");
const cas = {
  expected_revision: revision,
  base_version_id: scriptUUID.optional(),
};
export const sourceWriteSchema = z
  .object({
    ...cas,
    ...itemFields,
    rights_confirmed: z.literal(true, { error: "请确认已获授权使用此正文。" }),
  })
  .strict()
  .refine(oneBody);
const bulkWrite = z
  .object({
    ...cas,
    rights_confirmed: z.literal(true),
    sources: z.array(sourceImportItemSchema).min(1).max(2500),
  })
  .strict()
  .refine(
    (value) =>
      new TextEncoder().encode(JSON.stringify(value.sources)).length <=
      32 * 1024 * 1024,
    "章节合计超过 32MiB。",
  );
export const sourceCommandSchema = z.discriminatedUnion("action", [
  z.object({ action: z.literal("create"), body: sourceWriteSchema }).strict(),
  z
    .object({
      action: z.literal("update"),
      lineageId: scriptUUID,
      sourceId: scriptUUID,
      body: sourceWriteSchema,
    })
    .strict(),
  z
    .object({
      action: z.literal("delete"),
      lineageId: scriptUUID,
      sourceId: scriptUUID,
      body: z.object(cas).strict(),
    })
    .strict(),
  z.object({ action: z.literal("import"), body: bulkWrite }).strict(),
  z
    .object({
      action: z.literal("reorder"),
      body: z
        .object({
          ...cas,
          source_lineage_ids: z
            .array(scriptUUID)
            .refine(
              (ids) => new Set(ids).size === ids.length,
              "来源顺序包含重复身份。",
            ),
        })
        .strict(),
    })
    .strict(),
]);
export type SourceCommand = z.infer<typeof sourceCommandSchema>;
const changeSchema = z
  .object({
    source_lineage_id: scriptUUID,
    old_source_id: scriptUUID.optional(),
    new_source_id: scriptUUID.optional(),
  })
  .strict()
  .refine(
    (change) =>
      Boolean(change.old_source_id || change.new_source_id) &&
      change.old_source_id !== change.new_source_id,
  );
export const sourceReceiptSchema = z
  .object({
    script_revision: positiveRevision,
    project_revision: positiveRevision,
    version_id: scriptUUID,
    split_set_id: scriptUUID,
    source_mappings: z.array(changeSchema),
    changed: z.boolean(),
    duplicate: z.boolean(),
  })
  .strict()
  .refine(
    (value) =>
      new Set(value.source_mappings.map((item) => item.source_lineage_id))
        .size === value.source_mappings.length &&
      new Set(
        value.source_mappings.flatMap((item) =>
          [item.old_source_id, item.new_source_id].filter(Boolean),
        ),
      ).size ===
        value.source_mappings.flatMap((item) =>
          [item.old_source_id, item.new_source_id].filter(Boolean),
        ).length,
  );
export type SourceReceipt = z.infer<typeof sourceReceiptSchema>;
export function readScriptValue<T>(schema: z.ZodType<T>, value: unknown): T {
  const checked = schema.safeParse(value);
  if (!checked.success) throw new ApiError(502, "invalid_response");
  return checked.data;
}
export function readScriptWorkspace(value: unknown, projectId: string) {
  const result = readScriptValue(scriptWorkspaceSchema, value);
  if (result.state.project_id !== projectId)
    throw new ApiError(502, "invalid_response");
  return result;
}
export function readSourcePage(
  value: unknown,
  request: { after: number; limit: number; versionId?: string },
) {
  const page = readScriptValue(sourcePageSchema, value);
  if (
    page.items.length > request.limit ||
    (request.after > 0 && !request.versionId) ||
    (request.versionId !== undefined &&
      page.version_id !== request.versionId) ||
    page.items.some((item, index) => item.position !== request.after + index) ||
    (page.next_position !== undefined &&
      (!page.items.length ||
        page.next_position !== request.after + page.items.length))
  )
    throw new ApiError(502, "invalid_response");
  return page;
}
export function readSourceDetail(value: unknown, lineageId: string) {
  const result = readScriptValue(sourceDetailSchema, value);
  if (result.source_lineage_id !== lineageId)
    throw new ApiError(502, "invalid_response");
  return result;
}
export function readSourceReceipt(command: SourceCommand, value: unknown) {
  const result = readScriptValue(sourceReceiptSchema, value);
  const changes = result.source_mappings;
  let valid =
    result.script_revision >=
    command.body.expected_revision + (result.changed ? 1 : 0);
  if (command.action === "create" || command.action === "import")
    valid &&=
      changes.length ===
        (command.action === "create" ? 1 : command.body.sources.length) &&
      changes.every(
        (item) => !item.old_source_id && Boolean(item.new_source_id),
      );
  if (command.action === "update" || command.action === "delete")
    valid &&=
      changes.length === 1 &&
      changes[0].source_lineage_id === command.lineageId &&
      changes[0].old_source_id === command.sourceId &&
      (command.action === "update"
        ? Boolean(changes[0].new_source_id)
        : !changes[0].new_source_id);
  if (command.action === "reorder") valid &&= !changes.length;
  if (!valid) throw new ApiError(502, "invalid_response");
  return result;
}
