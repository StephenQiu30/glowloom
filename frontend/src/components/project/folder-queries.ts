import { z } from "zod";
import * as projects from "@/gen/api/projects";
import * as media from "@/gen/api/media";
import { ApiError } from "@/lib/request";
import { PROJECTS_KEY, listProjects } from "./queries";
import {
  folderPageSchema,
  folderUUID,
  readFolderChange,
  type FolderCover,
} from "./folder-model";
import {
  folderIntentSchema,
  type FolderIntent,
  type FolderScope,
} from "./folder-intent";

export const FOLDERS_KEY = [...PROJECTS_KEY, "folders"] as const;
export function folderCoverPreviewKey(scope: FolderScope, cover: FolderCover) {
  return [
    ...FOLDERS_KEY,
    "preview",
    scope.origin,
    scope.actorId,
    scope.orgId,
    cover.project_id,
    cover.asset_id,
  ] as const;
}
function parse<T>(schema: z.ZodType<T>, value: unknown): T {
  const checked = schema.safeParse(value);
  if (!checked.success) throw new ApiError(502, "invalid_response");
  return checked.data;
}
export async function listFolders(cursor?: string, signal?: AbortSignal) {
  const result = parse(
    folderPageSchema,
    await projects.listProjectFolders({ limit: 25, cursor }, { signal }),
  );
  if (result.items.length > 25) throw new ApiError(502, "invalid_response");
  return result;
}
export function requireFolderScope(
  page: { current_actor_id: string; current_org_id: string },
  scope: FolderScope,
) {
  if (
    page.current_actor_id !== scope.actorId ||
    page.current_org_id !== scope.orgId
  )
    throw new ApiError(409, "scope_changed");
}
export async function findFolder(
  id: string,
  scope: FolderScope,
  signal?: AbortSignal,
) {
  folderUUID.parse(id);
  const visited = new Set<string>();
  let cursor: string | undefined;
  for (;;) {
    const page = await listFolders(cursor, signal);
    requireFolderScope(page, scope);
    const found = page.items.find((item) => item.id === id);
    if (found) return found;
    if (!page.next_cursor) throw new ApiError(404, "not_found");
    if (visited.has(page.next_cursor))
      throw new ApiError(502, "invalid_response");
    visited.add(page.next_cursor);
    cursor = page.next_cursor;
  }
}
export async function findMovableProject(id: string, signal?: AbortSignal) {
  const visited = new Set<string>();
  let cursor: string | undefined;
  for (;;) {
    const page = await listProjects(
      { deleted: false, limit: 100, cursor },
      signal,
    );
    const found = page.items.find((item) => item.id === id);
    if (found) {
      if (found.status !== "active" || found.is_delete)
        throw new ApiError(409, "state_conflict");
      return found;
    }
    if (!page.next_cursor) throw new ApiError(404, "not_found");
    if (visited.has(page.next_cursor))
      throw new ApiError(502, "invalid_response");
    visited.add(page.next_cursor);
    cursor = page.next_cursor;
  }
}
export async function runFolderIntent(intent: FolderIntent) {
  const checked = folderIntentSchema.parse(intent);
  const options = {
    headers: { "Idempotency-Key": checked.key, Origin: checked.origin },
  };
  let response: unknown;
  switch (checked.action) {
    case "create":
      response = await projects.createProjectFolder(checked.body, options);
      break;
    case "patch":
      response = await projects.updateProjectFolder(
        { folder_id: checked.folderId },
        checked.body,
        options,
      );
      break;
    case "recycle":
      response = await projects.recycleProjectFolder(
        { folder_id: checked.folderId },
        checked.body,
        options,
      );
      break;
    case "move":
      response = await projects.moveProjectToFolder(
        { pid: checked.projectId },
        checked.body,
        options,
      );
      break;
  }
  return readFolderChange(checked, response);
}
const safeAsset = z.object({
  id: folderUUID,
  project_id: folderUUID,
  kind: z.enum(["image", "video", "audio", "model"]),
  file_name: z.string(),
  mime_type: z.string(),
  byte_size: z.number().int().positive(),
  width: z.number().int().positive().optional(),
  height: z.number().int().positive().optional(),
  duration_ms: z.number().int().nonnegative().optional(),
  revision: z.number().int().positive(),
});
export async function listCoverImages(
  projectId: string,
  cursor?: string,
  signal?: AbortSignal,
) {
  const page = parse(
    z.object({
      items: z
        .array(safeAsset.extend({ project_id: z.literal(projectId) }))
        .max(50),
      next_cursor: z.string().min(1).nullable(),
    }),
    await media.listMediaAssets(
      { pid: projectId, limit: 50, cursor },
      { signal },
    ),
  );
  return {
    ...page,
    items: page.items.filter((asset) => asset.kind === "image"),
  };
}
export async function getFolderCoverPreview(
  cover: FolderCover,
  signal?: AbortSignal,
) {
  const result = parse(
    z.object({
      asset: safeAsset.extend({
        id: z.literal(cover.asset_id),
        project_id: z.literal(cover.project_id),
        kind: z.literal("image"),
      }),
      url: z.url().refine((value) => {
        const url = new URL(value);
        return (
          ["http:", "https:"].includes(url.protocol) &&
          !url.username &&
          !url.password
        );
      }),
      expires_at: z.iso.datetime({ offset: true }),
    }),
    await media.getMediaPreview(
      { pid: cover.project_id, asset_id: cover.asset_id },
      { signal },
    ),
  );
  if (Date.parse(result.expires_at) <= Date.now() + 5000)
    throw new Error("封面预览授权已过期，请重新读取。");
  return result;
}
