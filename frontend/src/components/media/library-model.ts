import { z } from "zod";
import { ApiError } from "@/lib/request";

export const libraryUUID = z
  .uuid()
  .refine((id) => id !== "00000000-0000-0000-0000-000000000000");
const revision = z.number().int().min(0).max(2147483647);
const timestamp = z.iso.datetime({ offset: true });
export const libraryKinds = [
  "text",
  "image",
  "video",
  "audio",
  "document",
  "model",
] as const;
export const libraryCategories = [
  "character",
  "environment",
  "prop",
  "material",
  "other",
] as const;
export const categoryLabels = {
  character: "角色",
  environment: "场景",
  prop: "道具",
  material: "素材",
  other: "其他",
};
export const kindLabels = {
  text: "文本",
  image: "图片",
  video: "视频",
  audio: "音频",
  document: "文档",
  model: "3D 模型",
};
export const folderStyles = [
  "glass",
  "stacked",
  "midnight",
  "paper",
  "cinema",
  "compact",
] as const;
export const folderThemes = ["aurora", "obsidian", "ember", "pearl"] as const;
export const folderStyleLabels = {
  glass: "流光玻璃",
  stacked: "内容陈列",
  midnight: "午夜封面",
  paper: "纸感收藏",
  cinema: "电影胶片",
  compact: "紧凑资料",
};
export const folderThemeLabels = {
  aurora: "极光",
  obsidian: "曜石",
  ember: "余烬",
  pearl: "珍珠",
};
export function libraryText(maximum: number, required = false) {
  return z
    .string()
    .refine(
      (value) =>
        value.isWellFormed() &&
        !/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/.test(value) &&
        Array.from(value).length <= maximum &&
        (!required || value.trim().length > 0),
      `最多 ${maximum} 个字符，不能包含无效字符。`,
    );
}
export const libraryScopeSchema = z.discriminatedUnion("kind", [
  z.object({ kind: z.literal("personal") }).strict(),
  z.object({ kind: z.literal("project"), project_id: libraryUUID }).strict(),
]);
export type LibraryScope = z.infer<typeof libraryScopeSchema>;
export const libraryIdentitySchema = z
  .object({
    origin: z.url().refine((value) => {
      const url = new URL(value);
      return ["http:", "https:"].includes(url.protocol) && url.origin === value;
    }),
    actorId: libraryUUID,
    orgId: libraryUUID,
    libraryId: libraryUUID,
    scope: libraryScopeSchema,
  })
  .strict();
export type LibraryIdentity = z.infer<typeof libraryIdentitySchema>;
export function sameLibraryScope(a: LibraryScope, b: LibraryScope) {
  return (
    a.kind === b.kind &&
    (a.kind === "personal" ||
      (b.kind === "project" && a.project_id === b.project_id))
  );
}
export function sameLibraryIdentity(a: LibraryIdentity, b: LibraryIdentity) {
  return (
    a.origin === b.origin &&
    a.actorId === b.actorId &&
    a.orgId === b.orgId &&
    a.libraryId === b.libraryId &&
    sameLibraryScope(a.scope, b.scope)
  );
}
export const libraryFilterSchema = z
  .object({
    page: z.number().int().min(1).max(100000),
    page_size: z.number().int().min(1).max(120),
    kind: z.enum(["", ...libraryKinds]),
    category: z.enum(["", ...libraryCategories]),
    folder: z.union([z.literal("all"), z.literal("root"), libraryUUID]),
    favorite_only: z.boolean(),
    recent_only: z.boolean(),
    catalog_state: z.enum(["active", "trashed"]),
    search: libraryText(512),
    order: z.enum(["updated_desc", "updated_asc", "name_asc"]),
  })
  .strict();
export type LibraryFilter = z.infer<typeof libraryFilterSchema>;
export function defaultLibraryFilter(scope: LibraryScope): LibraryFilter {
  return {
    page: 1,
    page_size: scope.kind === "personal" ? 40 : 20,
    kind: "",
    category: "",
    folder: "all",
    favorite_only: false,
    recent_only: false,
    catalog_state: "active",
    search: "",
    order: "updated_desc",
  };
}
const folderSchema = z
  .object({
    id: libraryUUID,
    library_id: libraryUUID,
    library_kind: z.enum(["personal", "project"]),
    parent_id: libraryUUID.nullable(),
    name: libraryText(60, true),
    position: revision,
    style: z.string(),
    theme: z.string(),
    revision: revision.refine((n) => n > 0),
    created_at: timestamp,
    updated_at: timestamp,
  })
  .strict()
  .superRefine((folder, ctx) => {
    if (
      folder.name !== folder.name.trim() ||
      folder.parent_id === folder.id ||
      (folder.library_kind === "personal" &&
        (folder.parent_id !== null ||
          folder.style !== "" ||
          folder.theme !== "" ||
          Array.from(folder.name).length > 40)) ||
      (folder.library_kind === "project" &&
        (!folderStyles.some((style) => style === folder.style) ||
          !folderThemes.some((theme) => theme === folder.theme)))
    )
      ctx.addIssue({ code: "custom", message: "目录归属或样式无效。" });
  });
export type LibraryFolder = z.infer<typeof folderSchema>;
export function folderAncestry(
  folders: readonly LibraryFolder[],
  id: string,
): LibraryFolder[] {
  const indexed = new Map(folders.map((folder) => [folder.id, folder]));
  return indexedAncestry(indexed, id);
}
function indexedAncestry(
  indexed: ReadonlyMap<string, LibraryFolder>,
  id: string,
): LibraryFolder[] {
  const ancestors: LibraryFolder[] = [];
  let current: string | null = id;
  while (current !== null) {
    const folder = indexed.get(current);
    if (
      !folder ||
      ancestors.some((ancestor) => ancestor.id === current) ||
      ancestors.length >= 8
    )
      throw new ApiError(502, "invalid_response");
    ancestors.unshift(folder);
    current = folder.parent_id;
  }
  return ancestors;
}
export function canReparentLibraryFolder(
  folders: readonly LibraryFolder[],
  currentID: string | undefined,
  parentID: string | null,
) {
  if (parentID === null) return true;
  return (
    libraryFolderMoveTargets(folders, currentID).find(
      (target) => target.folder.id === parentID,
    )?.allowed === true
  );
}
export function libraryFolderMoveTargets(
  folders: readonly LibraryFolder[],
  currentID?: string,
) {
  const indexed = new Map(folders.map((folder) => [folder.id, folder]));
  const paths = folders.map((folder) => ({
    folder,
    path: indexedAncestry(indexed, folder.id),
  }));
  let height = 1;
  if (currentID)
    for (const { path } of paths) {
      const index = path.findIndex((ancestor) => ancestor.id === currentID);
      if (index >= 0) height = Math.max(height, path.length - index);
    }
  return paths.map(({ folder, path }) => ({
    folder,
    path,
    allowed:
      path.length + height <= 8 &&
      (!currentID || !path.some((ancestor) => ancestor.id === currentID)),
  }));
}
export const libraryAssetSchema = z
  .object({
    id: libraryUUID,
    kind: z.enum(["image", "video", "audio", "document", "model"]),
    file_name: z
      .string()
      .min(1)
      .refine(
        (value) =>
          value.isWellFormed() &&
          value === value.trim() &&
          !/[/\\\p{Cc}]/u.test(value) &&
          new TextEncoder().encode(value).length <= 255,
      ),
    mime_type: z.string().min(1).max(255),
    byte_size: z
      .number()
      .int()
      .positive()
      .max(500 * 1024 * 1024),
    width: z.number().int().positive().nullable(),
    height: z.number().int().positive().nullable(),
    duration_ms: z.number().int().positive().nullable(),
    revision: revision.refine((n) => n > 0),
  })
  .strict();
export type LibraryAsset = z.infer<typeof libraryAssetSchema>;
const tagsSchema = z
  .array(libraryText(64, true).refine((tag) => tag === tag.trim()))
  .max(32)
  .refine((tags) => new Set(tags).size === tags.length, "标签不能重复。");
export const libraryMetadataSchema = z
  .object({
    plain_text: z
      .string()
      .refine(
        (value) =>
          value.isWellFormed() &&
          !value.includes("\0") &&
          new TextEncoder().encode(value).length <= 65536,
        "文本最多 64 KiB。",
      )
      .optional(),
    folder_id: libraryUUID.nullable(),
    title: libraryText(240, true),
    category: z.enum(libraryCategories),
    tags: tagsSchema,
    source_label: libraryText(240),
    note: libraryText(8000),
    favorite: z.boolean(),
  })
  .strict();
export type LibraryMetadata = z.infer<typeof libraryMetadataSchema>;
const itemShape = {
  id: libraryUUID,
  asset_id: libraryUUID.nullable(),
  folder_id: libraryUUID.nullable(),
  kind: z.enum(libraryKinds),
  title: libraryText(240, true),
  category: z.enum(libraryCategories),
  tags: tagsSchema,
  source_label: libraryText(240),
  note: libraryText(8000),
  favorite: z.boolean(),
  catalog_state: z.enum(["active", "trashed"]),
  trashed_at: timestamp.nullable(),
  position: revision,
  revision,
  created_at: timestamp,
  updated_at: timestamp,
  media: libraryAssetSchema.nullable().optional(),
};
const itemSchema = z
  .object(itemShape)
  .strict()
  .refine(
    (item) =>
      (item.catalog_state === "trashed") === (item.trashed_at !== null) &&
      (item.kind === "text"
        ? item.asset_id === null && !item.media && item.revision > 0
        : item.asset_id !== null &&
          item.id === item.asset_id &&
          item.media?.id === item.asset_id &&
          item.media.kind === item.kind),
  );
export type LibraryItem = z.infer<typeof itemSchema>;
const detailSchema = z
  .object({
    ...itemShape,
    plain_text: libraryMetadataSchema.shape.plain_text.nullable().optional(),
  })
  .strict()
  .refine(
    (item) =>
      itemSchema.safeParse(
        Object.fromEntries(
          Object.entries(item).filter(([key]) => key !== "plain_text"),
        ),
      ).success &&
      (item.kind === "text"
        ? typeof item.plain_text === "string"
        : item.plain_text == null),
  );
export type LibraryDetail = z.infer<typeof detailSchema>;
const folderInputSchema = z
  .object({
    parent_id: libraryUUID.nullable(),
    name: libraryText(60, true).refine((name) => name === name.trim()),
    style: z.string(),
    theme: z.string(),
  })
  .strict();
const commandBase = {
  scope: libraryScopeSchema,
  expected_revision: revision.refine((n) => n < 2147483647),
};
const itemRevisions = z
  .array(z.object({ id: libraryUUID, revision }).strict())
  .min(1)
  .max(200)
  .refine(
    (items) => new Set(items.map((item) => item.id)).size === items.length,
  );
export const libraryCommandSchema = z
  .discriminatedUnion("action", [
    z
      .object({
        ...commandBase,
        action: z.literal("create_folder"),
        folder: folderInputSchema,
      })
      .strict(),
    z
      .object({
        ...commandBase,
        action: z.literal("update_folder"),
        folder_id: libraryUUID,
        expected_folder_revision: revision.refine((n) => n > 0),
        folder: folderInputSchema,
      })
      .strict(),
    z
      .object({
        ...commandBase,
        action: z.literal("delete_folder"),
        folder_id: libraryUUID,
        expected_folder_revision: revision.refine((n) => n > 0),
      })
      .strict(),
    z
      .object({
        ...commandBase,
        action: z.literal("create_text"),
        metadata: libraryMetadataSchema.extend({
          plain_text: libraryMetadataSchema.shape.plain_text.unwrap(),
        }),
      })
      .strict(),
    z
      .object({
        ...commandBase,
        action: z.literal("update_item"),
        item_id: libraryUUID,
        expected_item_revision: revision,
        metadata: libraryMetadataSchema,
      })
      .strict(),
    z
      .object({
        ...commandBase,
        action: z.literal("move_items"),
        items: itemRevisions,
        target_folder_id: libraryUUID.nullable(),
      })
      .strict(),
    z
      .object({
        ...commandBase,
        action: z.literal("recycle_items"),
        items: itemRevisions,
      })
      .strict(),
    z
      .object({
        ...commandBase,
        action: z.literal("restore_items"),
        items: itemRevisions,
      })
      .strict(),
    z
      .object({
        ...commandBase,
        action: z.literal("remove_items"),
        items: itemRevisions,
      })
      .strict(),
  ])
  .superRefine((command, ctx) => {
    if (
      (command.action === "remove_items" && command.scope.kind !== "project") ||
      ("metadata" in command &&
        command.scope.kind !== "personal" &&
        command.metadata.favorite)
    )
      ctx.addIssue({ code: "custom", message: "此动作不适用于当前素材库。" });
    if ("folder" in command) {
      const folder = command.folder;
      if (
        command.scope.kind === "personal"
          ? folder.parent_id !== null ||
            folder.style !== "" ||
            folder.theme !== "" ||
            Array.from(folder.name).length > 40
          : !folderStyles.some((style) => style === folder.style) ||
            !folderThemes.some((theme) => theme === folder.theme)
      )
        ctx.addIssue({ code: "custom", message: "目录归属或样式无效。" });
    }
  });
export type LibraryCommand = z.infer<typeof libraryCommandSchema>;
const countSchema = z.number().int().min(0).max(Number.MAX_SAFE_INTEGER);
const pageSchema = z
  .object({
    current_actor_id: libraryUUID,
    current_org_id: libraryUUID,
    library_id: libraryUUID,
    scope: libraryScopeSchema,
    revision,
    page: z.number().int().positive().max(100000),
    page_size: z.number().int().positive().max(120),
    total: countSchema,
    items: z.array(itemSchema).max(120),
    category_counts: z.partialRecord(z.enum(libraryCategories), countSchema),
    folder_counts: z.record(z.string(), countSchema),
    folders: z.array(folderSchema).max(4096),
  })
  .strict();
export type LibraryPage = z.infer<typeof pageSchema>;
function read<T>(schema: z.ZodType<T>, value: unknown): T {
  const result = schema.safeParse(value);
  if (!result.success) throw new ApiError(502, "invalid_response");
  return result.data;
}
export function requireLibraryIdentity(
  page: Pick<
    LibraryPage,
    "scope" | "library_id" | "current_actor_id" | "current_org_id"
  >,
  identity: LibraryIdentity,
) {
  if (
    !sameLibraryScope(page.scope, identity.scope) ||
    page.library_id !== identity.libraryId ||
    page.current_actor_id !== identity.actorId ||
    page.current_org_id !== identity.orgId
  )
    throw new ApiError(409, "scope_changed");
}
export function readLibraryPage(
  value: unknown,
  scope: LibraryScope,
  query: Pick<LibraryFilter, "page" | "page_size">,
  identity?: LibraryIdentity,
): LibraryPage {
  const page = read(pageSchema, value);
  if (
    !sameLibraryScope(page.scope, scope) ||
    page.page !== query.page ||
    page.page_size !== query.page_size ||
    page.items.length > page.page_size ||
    page.items.length > page.total ||
    new Set(page.items.map((item) => item.id)).size !== page.items.length ||
    new Set(page.folders.map((folder) => folder.id)).size !==
      page.folders.length
  )
    throw new ApiError(502, "invalid_response");
  const folderIds = new Set(page.folders.map((folder) => folder.id));
  const indexed = new Map(page.folders.map((folder) => [folder.id, folder]));
  const siblings = new Set<string>();
  for (const folder of page.folders) {
    const sibling = `${folder.parent_id ?? "root"}/${folder.name.toLowerCase()}`;
    if (
      folder.library_id !== page.library_id ||
      folder.library_kind !== scope.kind ||
      siblings.has(sibling)
    )
      throw new ApiError(502, "invalid_response");
    siblings.add(sibling);
    indexedAncestry(indexed, folder.id);
  }
  if (
    page.items.some(
      (item) =>
        (item.folder_id !== null && !folderIds.has(item.folder_id)) ||
        (scope.kind === "project" && item.favorite),
    ) ||
    Object.keys(page.folder_counts).some(
      (id) => id !== "root" && !folderIds.has(id),
    )
  )
    throw new ApiError(502, "invalid_response");
  if (identity) requireLibraryIdentity(page, identity);
  return page;
}
export function readLibraryDetail(value: unknown, id: string) {
  const item = read(detailSchema, value);
  if (item.id !== id) throw new ApiError(502, "invalid_response");
  return item;
}
const changeSchema = z
  .object({
    id: libraryUUID,
    asset_id: libraryUUID.nullable(),
    folder_id: libraryUUID.nullable(),
    kind: z.enum(libraryKinds),
    catalog_state: z.enum(["active", "trashed", "removed"]),
    trashed_at: timestamp.nullable(),
    revision: revision.refine((n) => n > 0),
  })
  .strict()
  .refine(
    (item) =>
      (item.kind === "text") === (item.asset_id === null) &&
      (item.catalog_state === "trashed") === (item.trashed_at !== null),
  );
const receiptSchema = z
  .object({
    current_actor_id: libraryUUID,
    current_org_id: libraryUUID,
    library_id: libraryUUID,
    scope: libraryScopeSchema,
    revision: revision.refine((n) => n > 0),
    project_revision: revision
      .refine((n) => n > 0)
      .nullable()
      .optional(),
    folder: folderSchema.nullable().optional(),
    items: z.array(changeSchema).max(200),
  })
  .strict();
export type LibraryReceipt = z.infer<typeof receiptSchema>;
export function readLibraryReceipt(
  value: unknown,
  identity: LibraryIdentity,
  body: LibraryCommand,
): LibraryReceipt {
  const receipt = read(receiptSchema, value);
  requireLibraryIdentity(receipt, identity);
  let valid =
    receipt.revision === body.expected_revision + 1 &&
    (body.scope.kind === "project"
      ? typeof receipt.project_revision === "number"
      : receipt.project_revision == null) &&
    new Set(receipt.items.map((item) => item.id)).size === receipt.items.length;
  if (body.action === "create_folder" || body.action === "update_folder") {
    const folder = receipt.folder;
    valid &&= Boolean(
      folder &&
      folder.library_id === identity.libraryId &&
      folder.library_kind === body.scope.kind &&
      folder.name === body.folder.name &&
      folder.parent_id === body.folder.parent_id &&
      folder.style === body.folder.style &&
      folder.theme === body.folder.theme &&
      folder.revision ===
        (body.action === "create_folder"
          ? 1
          : body.expected_folder_revision + 1) &&
      (body.action === "create_folder" || folder.id === body.folder_id) &&
      receipt.items.length === 0,
    );
  } else if (body.action === "delete_folder")
    valid &&= !receipt.folder && receipt.items.length === 0;
  else {
    valid &&= !receipt.folder;
    if (body.action === "create_text")
      valid &&=
        receipt.items.length === 1 &&
        receipt.items[0].kind === "text" &&
        receipt.items[0].revision === 1 &&
        receipt.items[0].folder_id === body.metadata.folder_id &&
        receipt.items[0].catalog_state === "active";
    else {
      const requested =
        body.action === "update_item"
          ? [{ id: body.item_id, revision: body.expected_item_revision }]
          : body.items;
      valid &&=
        receipt.items.length === requested.length &&
        requested.every((item) =>
          receipt.items.some(
            (changed) =>
              changed.id === item.id && changed.revision === item.revision + 1,
          ),
        );
      if (body.action === "move_items")
        valid &&= receipt.items.every(
          (item) =>
            item.folder_id === body.target_folder_id &&
            item.catalog_state === "active",
        );
      if (body.action === "recycle_items")
        valid &&= receipt.items.every(
          (item) => item.catalog_state === "trashed",
        );
      if (body.action === "restore_items")
        valid &&= receipt.items.every(
          (item) => item.catalog_state === "active",
        );
      if (body.action === "remove_items")
        valid &&= receipt.items.every(
          (item) => item.catalog_state === "removed" && item.folder_id === null,
        );
      if (body.action === "update_item")
        valid &&= receipt.items[0]?.folder_id === body.metadata.folder_id;
    }
  }
  if (!valid) throw new ApiError(502, "invalid_response");
  return receipt;
}
