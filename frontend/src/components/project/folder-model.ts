import { z } from "zod";
import { ApiError } from "@/lib/request";

export const folderUUID = z
  .string()
  .uuid()
  .refine((value) => value !== "00000000-0000-0000-0000-000000000000");
const revision = z.number().int().positive().max(2147483647);
const placementRevision = z.number().int().nonnegative().max(2147483647);
export const folderName = z
  .string()
  .transform((value) =>
    value.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, ""),
  )
  .refine((value) => value.length > 0, "请填写目录名称。")
  .refine(
    (value) => Array.from(value).length <= 160,
    "目录名称最多 160 个字符。",
  )
  .refine(
    (value) => !/[\uD800-\uDFFF\u0000]/u.test(value),
    "名称包含无效字符。",
  );
export const folderCoverSchema = z
  .object({ project_id: folderUUID, asset_id: folderUUID })
  .strict();
export type FolderCover = z.infer<typeof folderCoverSchema>;
const folderReceiptSchema = z
  .object({
    id: folderUUID,
    name: folderName,
    cover: folderCoverSchema.nullable(),
    cover_unavailable: z.boolean(),
    project_count: z.number().int().nonnegative().optional(),
    revision,
    is_delete: z.boolean(),
    delete_time: z.iso.datetime({ offset: true }).nullable(),
    create_time: z.iso.datetime({ offset: true }),
    update_time: z.iso.datetime({ offset: true }),
  })
  .refine((value) => !value.cover_unavailable || value.cover === null)
  .refine((value) => value.is_delete === (value.delete_time !== null));
export const folderSummarySchema = folderReceiptSchema.safeExtend({
  project_count: z.number().int().nonnegative(),
  is_delete: z.literal(false),
  delete_time: z.null(),
});
export type FolderSummary = z.infer<typeof folderSummarySchema>;
export const folderPageSchema = z
  .object({
    current_actor_id: folderUUID,
    current_org_id: folderUUID,
    items: z.array(folderSummarySchema).max(200),
    next_cursor: z.string().min(1).nullable(),
  })
  .refine(
    (value) =>
      new Set(value.items.map((item) => item.id)).size === value.items.length,
  );
export type FolderPage = z.infer<typeof folderPageSchema>;
export const folderMoveSchema = z
  .object({
    expected_project_revision: revision,
    expected_placement_revision: placementRevision,
    folder_id: folderUUID.nullable(),
    expected_folder_revision: placementRevision,
  })
  .strict()
  .refine((value) =>
    value.folder_id === null
      ? value.expected_folder_revision === 0
      : value.expected_folder_revision > 0,
  );
export const projectPlacementSchema = z
  .object({
    project_id: folderUUID,
    folder_id: folderUUID.nullable(),
    revision: placementRevision,
  })
  .refine((value) => value.folder_id === null || value.revision > 0);
const createSchema = z
  .object({ name: folderName, cover: folderCoverSchema.optional() })
  .strict();
const patchSchema = z
  .object({
    expected_revision: revision,
    name: folderName.optional(),
    cover: folderCoverSchema.nullable().optional(),
  })
  .strict()
  .refine((value) => value.name !== undefined || Object.hasOwn(value, "cover"));
export const folderCommandSchema = z.discriminatedUnion("action", [
  z.object({ action: z.literal("create"), body: createSchema }).strict(),
  z
    .object({
      action: z.literal("patch"),
      folderId: folderUUID,
      body: patchSchema,
    })
    .strict(),
  z
    .object({
      action: z.literal("recycle"),
      folderId: folderUUID,
      body: z.object({ expected_revision: revision }).strict(),
    })
    .strict(),
  z
    .object({
      action: z.literal("move"),
      projectId: folderUUID,
      body: folderMoveSchema,
    })
    .strict(),
]);
export type FolderCommand = z.infer<typeof folderCommandSchema>;
const changeSchema = z.object({
  folder: folderReceiptSchema.optional(),
  placement: projectPlacementSchema.optional(),
  recycled_project_ids: z
    .array(folderUUID)
    .refine((ids) => new Set(ids).size === ids.length)
    .optional(),
});
export function readFolderChange(command: FolderCommand, value: unknown) {
  const parsed = changeSchema.safeParse(value);
  if (!parsed.success) throw new ApiError(502, "invalid_response");
  const receipt = parsed.data;
  const folder = receipt.folder;
  let valid = false;
  if (command.action === "move") {
    valid =
      receipt.placement?.project_id === command.projectId &&
      receipt.placement.folder_id === command.body.folder_id &&
      receipt.placement.revision >= command.body.expected_placement_revision;
    valid &&=
      command.body.folder_id === null
        ? !folder
        : Boolean(
            folder &&
            !folder.is_delete &&
            folder.id === command.body.folder_id &&
            folder.revision >= command.body.expected_folder_revision,
          );
  } else if (folder && !receipt.placement) {
    valid =
      (command.action === "create" || folder.id === command.folderId) &&
      folder.is_delete === (command.action === "recycle");
    if (command.action !== "create")
      valid &&= folder.revision > command.body.expected_revision;
    if (command.action !== "recycle") {
      if (command.body.name !== undefined)
        valid &&= folder.name === command.body.name;
      if (Object.hasOwn(command.body, "cover"))
        valid &&=
          JSON.stringify(folder.cover) === JSON.stringify(command.body.cover);
    }
  }
  if (
    command.action !== "recycle" &&
    receipt.recycled_project_ids !== undefined
  )
    valid = false;
  if (!valid) throw new ApiError(502, "invalid_response");
  return receipt;
}
