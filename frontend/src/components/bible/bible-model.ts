import { z } from "zod";
import { ApiError } from "@/lib/request";

export const bibleUUID = z
  .uuid()
  .refine((value) => value !== "00000000-0000-0000-0000-000000000000");
const nilUUID = "00000000-0000-0000-0000-000000000000";
export const bibleSHA = z.string().regex(/^[0-9a-f]{64}$/);
const revision = z.number().int().min(1).max(2147483647);
const count = z.number().int().positive().max(Number.MAX_SAFE_INTEGER);
const timestamp = z.iso.datetime({ offset: true });
function text(maximum: number) {
  return z.string().refine(
    (value) =>
      !value.includes("\0") &&
      Array.from(value).length <= maximum &&
      !Array.from(value).some((scalar) => {
        const n = scalar.codePointAt(0)!;
        return n >= 0xd800 && n <= 0xdfff;
      }),
  );
}
export const bibleName = text(512).refine(
  (value) => value.trim().length > 0,
  "请输入1..512个Unicode字符的名称。",
);
const description = text(8192);
const aliases = z
  .array(bibleName)
  .max(64)
  .refine((items) => new Set(items).size === items.length);
export const bibleKind = z.enum(["character", "location", "prop"]);
export type BibleKind = z.infer<typeof bibleKind>;
export const kindLabels: Record<BibleKind, string> = {
  character: "角色",
  location: "场景",
  prop: "道具",
};
export const imageRoles = [
  "primary",
  "front",
  "side",
  "back",
  "turnaround_sheet",
  "expression_sheet",
] as const;
export const imageRole = z.enum(imageRoles);
export const imageRoleLabels: Record<z.infer<typeof imageRole>, string> = {
  primary: "主要形象",
  front: "正面",
  side: "侧面",
  back: "背面",
  turnaround_sheet: "三视图",
  expression_sheet: "表情图",
};
export const definitionFields = [
  ["role", "剧情定位"],
  ["appearance", "外貌"],
  ["physique", "体型"],
  ["clothing", "默认服装造型"],
  ["personality", "性格与表演基线"],
  ["props", "固定道具"],
  ["consistency_prompt", "一致性约束"],
  ["multi_view_prompt", "三视图约束"],
  ["voice_language", "声音语言"],
  ["voice_age", "声音年龄"],
  ["voice_timbre", "声音音色描述"],
] as const;
export const characterDefinitionSchema = z
  .object({
    role: description.optional(),
    appearance: description.optional(),
    physique: description.optional(),
    clothing: description.optional(),
    personality: description.optional(),
    props: description.optional(),
    consistency_prompt: description.optional(),
    multi_view_prompt: description.optional(),
    voice_language: description.optional(),
    voice_age: description.optional(),
    voice_timbre: description.optional(),
  })
  .strict();
const labels = {
  name: bibleName,
  aliases: aliases.optional(),
  description: description.optional(),
};
export const characterInputSchema = z
  .object({ ...labels, definition: characterDefinitionSchema })
  .strict();
export const locationSchema = z
  .object({ ...labels, prompt: description.optional() })
  .strict();
export const propSchema = locationSchema;
export const lookScopeSchema = z
  .object({ episode_id: bibleUUID, scene_key: bibleUUID.optional() })
  .strict();
const appliesTo = z
  .array(lookScopeSchema)
  .max(2500)
  .refine(
    (items) =>
      new Set(items.map((item) => `${item.episode_id}:${item.scene_key ?? ""}`))
        .size === items.length,
  );
export const lookInputSchema = z
  .object({
    name: bibleName,
    description: description.optional(),
    default: z.boolean(),
    applies_to: appliesTo.optional(),
  })
  .strict();
const mediaFact = {
  asset_id: bibleUUID,
  revision,
  sha256: bibleSHA,
  byte_size: count,
};
export const imageFactSchema = z
  .object({
    ...mediaFact,
    kind: z.literal("image"),
    rendition_id: bibleUUID,
    rendition_sha256: bibleSHA,
  })
  .strict();
export const audioFactSchema = z
  .object({
    ...mediaFact,
    kind: z.literal("audio"),
    rendition_id: z.literal(nilUUID).optional(),
    rendition_sha256: z.literal("").optional(),
  })
  .strict();
export const imageReferenceSchema = z
  .object({ role: imageRole, media: imageFactSchema })
  .strict();
const references = z
  .array(imageReferenceSchema)
  .max(8)
  .refine(
    (items) => new Set(items.map((item) => item.role)).size === items.length,
  );
export const referenceInputSchema = z
  .object({ role: imageRole, asset_id: bibleUUID })
  .strict();
const referenceInputs = z
  .array(referenceInputSchema)
  .max(8)
  .refine(
    (items) => new Set(items.map((item) => item.role)).size === items.length,
  );
export const lookContentSchema = z
  .object({
    ...lookInputSchema.shape,
    id: bibleUUID,
    references: references.optional(),
  })
  .strict();
export const voiceParamsSchema = z
  .object({
    speed: z.number().finite().optional(),
    pitch: z.number().finite().optional(),
    volume: z.number().finite().optional(),
    emotion: text(512).optional(),
    language: text(512).optional(),
  })
  .strict();
export const voiceInputSchema = z.discriminatedUnion("kind", [
  z
    .object({
      kind: z.literal("sample"),
      instructions: description.optional(),
      sample: z.object({ name: bibleName, asset_id: bibleUUID }).strict(),
    })
    .strict(),
  z
    .object({
      kind: z.literal("catalog"),
      instructions: description.optional(),
      catalog: z
        .object({
          model_key: bibleName,
          expected_model_version: count,
          voice_key: bibleName,
          params: voiceParamsSchema,
        })
        .strict(),
    })
    .strict(),
]);
export const voiceContentSchema = z.discriminatedUnion("kind", [
  z
    .object({
      kind: z.literal("sample"),
      instructions: description.optional(),
      sample: z.object({ name: bibleName, media: audioFactSchema }).strict(),
    })
    .strict(),
  z
    .object({
      kind: z.literal("catalog"),
      instructions: description.optional(),
      catalog: z
        .object({
          model_key: bibleName,
          model_version_id: bibleUUID,
          model_version: count,
          voice_key: bibleName,
          param_schema_sha256: bibleSHA,
          params: voiceParamsSchema,
        })
        .strict(),
    })
    .strict(),
]);
export const characterContentSchema = z
  .object({
    ...characterInputSchema.shape,
    looks: z
      .array(lookContentSchema)
      .min(1)
      .max(200)
      .refine(
        (items) =>
          items.filter((item) => item.default).length === 1 &&
          new Set(items.map((item) => item.id)).size === items.length,
      ),
    voice: voiceContentSchema.optional(),
  })
  .strict();
export const bibleIdentitySchema = z
  .object({
    origin: z.string().refine((value) => {
      try {
        const url = new URL(value);
        return (
          ["http:", "https:"].includes(url.protocol) &&
          url.origin === value &&
          !url.username &&
          !url.password
        );
      } catch {
        return false;
      }
    }),
    actorId: bibleUUID,
    orgId: bibleUUID,
    projectId: bibleUUID,
  })
  .strict();
export type BibleIdentity = z.infer<typeof bibleIdentitySchema>;
export const bibleHeadSchema = z
  .object({
    id: bibleUUID,
    org_id: bibleUUID,
    project_id: bibleUUID,
    kind: bibleKind,
    revision,
    current_version_id: bibleUUID,
    confirmed_version_id: bibleUUID.optional(),
    redirect_id: bibleUUID.optional(),
    deleted: z.boolean(),
    created_at: timestamp,
    updated_at: timestamp,
  })
  .strict()
  .refine(
    (head) =>
      !head.redirect_id ||
      (head.kind === "character" && head.redirect_id !== head.id),
  );
const resultSource = z
  .object({
    operation_id: bibleUUID,
    output_id: bibleUUID,
    input_sha256: bibleSHA,
    output_sha256: bibleSHA,
  })
  .strict();
const versionBase = {
  id: bibleUUID,
  entry_id: bibleUUID,
  org_id: bibleUUID,
  project_id: bibleUUID,
  number: count,
  previous_id: bibleUUID.optional(),
  actor_id: bibleUUID,
  created_at: timestamp,
  origin: z.enum(["manual", "ai"]),
  result: resultSource.optional(),
  content_sha256: bibleSHA,
};
export const bibleVersionSchema = z
  .discriminatedUnion("kind", [
    z
      .object({
        ...versionBase,
        kind: z.literal("character"),
        character: characterContentSchema,
      })
      .strict(),
    z
      .object({
        ...versionBase,
        kind: z.literal("location"),
        location: locationSchema,
      })
      .strict(),
    z
      .object({ ...versionBase, kind: z.literal("prop"), prop: propSchema })
      .strict(),
  ])
  .refine(
    (version) =>
      (version.origin === "ai") === Boolean(version.result) &&
      version.previous_id !== version.id,
  )
  .refine((version) => {
    const content =
      version.kind === "character"
        ? version.character
        : version.kind === "location"
          ? version.location
          : version.prop;
    return (
      new TextEncoder().encode(JSON.stringify(content)).byteLength <= 4 << 20
    );
  });
export type BibleVersion = z.infer<typeof bibleVersionSchema>;
export type BibleLook = z.infer<typeof lookContentSchema>;
export type BibleReference = z.infer<typeof imageReferenceSchema>;
export type CharacterInput = z.infer<typeof characterInputSchema>;
const summarySchema = z
  .object({ head: bibleHeadSchema, name: bibleName, content_sha256: bibleSHA })
  .strict();
const pageSchema = z
  .object({
    entries: z.array(summarySchema).max(200),
    next_cursor: z.string().min(1).max(2048).optional(),
    current_actor_id: bibleUUID,
    current_org_id: bibleUUID,
  })
  .strict();
export type BiblePage = z.infer<typeof pageSchema>;
const detailSchema = z
  .object({
    head: bibleHeadSchema,
    current: bibleVersionSchema,
    resolved_id: bibleUUID,
  })
  .strict();
export type BibleDetail = z.infer<typeof detailSchema>;
export function versionInput(detail: BibleDetail) {
  const version = detail.current;
  if (version.kind !== "character")
    return version.kind === "location" ? version.location : version.prop;
  const { name, aliases, description, definition } = version.character;
  return characterInputSchema.parse({ name, aliases, description, definition });
}
const historySchema = z
  .object({
    versions: z.array(bibleVersionSchema).max(50),
    next_cursor: z.string().min(1).max(2048).optional(),
  })
  .strict();
export type BibleHistory = z.infer<typeof historySchema>;
const voiceChoiceSchema = z
  .object({
    model_key: bibleName,
    model_version: count,
    voice_key: bibleName,
    display_name: text(512),
  })
  .strict();
export const voicePageSchema = z
  .object({
    voices: z.array(voiceChoiceSchema).max(200),
    next_cursor: z.string().min(1).max(2048).optional(),
  })
  .strict();
export type BibleVoiceChoice = z.infer<typeof voiceChoiceSchema>;
export const impactSchema = z
  .object({
    sha256: bibleSHA,
    revision: z.number().int().min(0).max(Number.MAX_SAFE_INTEGER),
  })
  .strict();
const controlBody = z
  .object({
    expected_revision: revision,
    acknowledged_impact: impactSchema.optional(),
  })
  .strict();
const contentBody = z
  .object({
    expected_revision: z.number().int().min(0).max(2147483647),
    character: characterInputSchema.optional(),
    location: locationSchema.optional(),
    prop: propSchema.optional(),
  })
  .strict();
const entryBase = { kind: bibleKind, id: bibleUUID };
const resultBody = { operation_id: bibleUUID, output_id: bibleUUID };
export const bibleCommandSchema = z
  .union([
    z
      .object({
        action: z.enum(["create", "update"]),
        kind: bibleKind,
        id: bibleUUID.optional(),
        body: contentBody,
      })
      .strict(),
    z
      .object({
        action: z.enum(["confirm", "delete", "restore"]),
        ...entryBase,
        body: controlBody,
      })
      .strict(),
    z
      .object({
        action: z.enum(["look_create", "look_update"]),
        kind: z.literal("character"),
        id: bibleUUID,
        lookId: bibleUUID.optional(),
        body: z
          .object({ expected_revision: revision, look: lookInputSchema })
          .strict(),
      })
      .strict(),
    z
      .object({
        action: z.enum(["look_delete", "look_default"]),
        kind: z.literal("character"),
        id: bibleUUID,
        lookId: bibleUUID,
        body: controlBody,
      })
      .strict(),
    z
      .object({
        action: z.literal("references"),
        kind: z.literal("character"),
        id: bibleUUID,
        lookId: bibleUUID,
        body: z
          .object({ expected_revision: revision, references: referenceInputs })
          .strict(),
      })
      .strict(),
    z
      .object({
        action: z.literal("voice_bind"),
        kind: z.literal("character"),
        id: bibleUUID,
        body: z
          .object({ expected_revision: revision, voice: voiceInputSchema })
          .strict(),
      })
      .strict(),
    z
      .object({
        action: z.literal("voice_unbind"),
        kind: z.literal("character"),
        id: bibleUUID,
        body: z.object({ expected_revision: revision }).strict(),
      })
      .strict(),
    z
      .object({
        action: z.literal("merge"),
        kind: z.literal("character"),
        id: bibleUUID,
        body: z
          .object({
            expected_revision: revision,
            target_id: bibleUUID,
            expected_target_revision: revision,
            acknowledged_impact: impactSchema.optional(),
          })
          .strict(),
      })
      .strict(),
    z
      .object({
        action: z.literal("split"),
        kind: z.literal("character"),
        id: bibleUUID,
        body: z
          .object({
            expected_revision: revision,
            character: characterInputSchema,
          })
          .strict(),
      })
      .strict(),
    z
      .object({
        action: z.enum(["create_result", "adopt_result"]),
        kind: bibleKind,
        id: bibleUUID.optional(),
        body: z
          .object({
            expected_revision: z.number().int().min(0).max(2147483647),
            ...resultBody,
          })
          .strict(),
      })
      .strict(),
  ])
  .superRefine((command, context) => {
    const creates =
      command.action === "create" || command.action === "create_result";
    if (
      creates
        ? command.id !== undefined || command.body.expected_revision !== 0
        : !command.id || command.body.expected_revision < 1
    )
      context.addIssue({
        code: "custom",
        message: "命令身份与冻结版本不一致。",
      });
    if (command.action === "create" || command.action === "update") {
      const body = command.body;
      if (
        Number(Boolean(body.character)) +
          Number(Boolean(body.location)) +
          Number(Boolean(body.prop)) !==
          1 ||
        !body[command.kind]
      )
        context.addIssue({
          code: "custom",
          message: "只能提交此类型的完整设定。",
        });
    }
    if (command.action === "look_create" || command.action === "look_update") {
      if ((command.action === "look_update") !== Boolean(command.lookId))
        context.addIssue({ code: "custom", message: "造型身份不匹配。" });
    }
    if (command.action === "merge" && command.id === command.body.target_id)
      context.addIssue({ code: "custom", message: "角色不能合并到自身。" });
    if (new TextEncoder().encode(JSON.stringify(command)).byteLength > 1 << 20)
      context.addIssue({ code: "custom", message: "命令超过1MiB。" });
  });
export type BibleCommand = z.infer<typeof bibleCommandSchema>;
const receiptSchema = z
  .object({
    entry_id: bibleUUID,
    kind: bibleKind,
    revision,
    version_id: bibleUUID,
    version_number: count,
    confirmed_version_id: bibleUUID.optional(),
    project_revision: revision,
    content_sha256: bibleSHA,
    created_entry_id: bibleUUID.optional(),
    created_version_id: bibleUUID.optional(),
    redirect_id: bibleUUID.optional(),
  })
  .strict();
export type BibleReceipt = z.infer<typeof receiptSchema>;
function invalid(): never {
  throw new ApiError(502, "invalid_bible_response");
}
function parse<T>(schema: z.ZodType<T>, value: unknown): T {
  const result = schema.safeParse(value);
  return result.success ? result.data : invalid();
}
function requireOwner(
  value: { org_id: string; project_id: string; kind: BibleKind },
  scope: BibleIdentity,
  kind: BibleKind,
) {
  if (
    value.org_id !== scope.orgId ||
    value.project_id !== scope.projectId ||
    value.kind !== kind
  )
    invalid();
}
export function readBiblePage(
  value: unknown,
  projectId: string,
  kind: BibleKind,
  expected?: BibleIdentity,
): BiblePage {
  const result = parse(pageSchema, value);
  if (
    expected &&
    (expected.actorId !== result.current_actor_id ||
      expected.orgId !== result.current_org_id ||
      expected.projectId !== projectId)
  )
    invalid();
  const ids = new Set<string>();
  for (const item of result.entries) {
    if (
      item.head.project_id !== projectId ||
      item.head.org_id !== result.current_org_id ||
      item.head.kind !== kind ||
      item.head.deleted ||
      item.head.redirect_id ||
      ids.has(item.head.id)
    )
      invalid();
    ids.add(item.head.id);
  }
  return result;
}
export function readBibleVersion(
  value: unknown,
  scope: BibleIdentity,
  kind: BibleKind,
  entryId: string,
  versionId?: string,
): BibleVersion {
  const result = parse(bibleVersionSchema, value);
  requireOwner(result, scope, kind);
  if (result.entry_id !== entryId || (versionId && result.id !== versionId))
    invalid();
  return result;
}
export function readBibleDetail(
  value: unknown,
  scope: BibleIdentity,
  kind: BibleKind,
  entryId: string,
): BibleDetail {
  const result = parse(detailSchema, value);
  requireOwner(result.head, scope, kind);
  readBibleVersion(
    result.current,
    scope,
    kind,
    entryId,
    result.head.current_version_id,
  );
  if (
    result.head.id !== entryId ||
    (!result.head.redirect_id && result.resolved_id !== entryId) ||
    (result.head.redirect_id && result.resolved_id === entryId)
  )
    invalid();
  return result;
}
export function readBibleHistory(
  value: unknown,
  scope: BibleIdentity,
  kind: BibleKind,
  entryId: string,
): BibleHistory {
  const result = parse(historySchema, value),
    ids = new Set<string>();
  let previous = Infinity;
  for (const version of result.versions) {
    readBibleVersion(version, scope, kind, entryId);
    if (ids.has(version.id) || version.number >= previous) invalid();
    ids.add(version.id);
    previous = version.number;
  }
  return result;
}
export function readBibleVoices(value: unknown) {
  const result = parse(voicePageSchema, value);
  if (
    new Set(
      result.voices.map((voice) => `${voice.model_key}:${voice.voice_key}`),
    ).size !== result.voices.length
  )
    invalid();
  return result;
}
export function readBibleReceipt(
  value: unknown,
  command: BibleCommand,
): BibleReceipt {
  const result = parse(receiptSchema, value);
  if (
    result.kind !== command.kind ||
    result.revision !== command.body.expected_revision + 1 ||
    (command.id && result.entry_id !== command.id)
  )
    invalid();
  if (
    (command.action === "create" || command.action === "create_result") &&
    result.version_number !== 1
  )
    invalid();
  if (
    command.action === "confirm" &&
    result.confirmed_version_id !== result.version_id
  )
    invalid();
  if (
    command.action === "merge"
      ? result.redirect_id !== command.body.target_id
      : result.redirect_id !== undefined
  )
    invalid();
  if (command.action === "split") {
    if (
      !result.created_entry_id ||
      !result.created_version_id ||
      result.created_entry_id === result.entry_id ||
      result.created_version_id === result.version_id
    )
      invalid();
  } else if (result.created_entry_id || result.created_version_id) invalid();
  return result;
}
export function visuallyReady(
  references: readonly { role: string }[],
): boolean {
  const purposes = new Set(references.map((reference) => reference.role));
  return (
    purposes.has("turnaround_sheet") ||
    ["front", "side", "back"].every((role) => purposes.has(role))
  );
}
