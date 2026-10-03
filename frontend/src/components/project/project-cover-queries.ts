import { z } from "zod";
import * as media from "@/api/media";
import { ApiError } from "@/lib/request";
import { getFolderCoverPreview } from "./folder-queries";
import type { FolderScope } from "./folder-intent";

export const PROJECT_COVERS_KEY = ["project-covers"] as const;
const imagePage = z.object({
  items: z
    .array(
      z.object({
        id: z.uuid(),
        project_id: z.uuid(),
        kind: z.enum(["image", "video", "audio", "model", "document"]),
        file_name: z.string(),
        mime_type: z.string(),
        byte_size: z.number().int().positive(),
        width: z.number().int().positive().optional(),
        height: z.number().int().positive().optional(),
        revision: z.number().int().positive(),
      }),
    )
    .max(50),
  next_cursor: z.string().min(1).nullable(),
});
export function projectCoverPreviewKey(
  scope: FolderScope,
  projectId: string,
  assetId: string,
) {
  return [
    ...PROJECT_COVERS_KEY,
    scope.origin,
    scope.actorId,
    scope.orgId,
    projectId,
    assetId,
  ] as const;
}
export async function listProjectCoverImages(
  projectId: string,
  cursor?: string,
  signal?: AbortSignal,
) {
  const result = imagePage.safeParse(
    await media.listMediaAssets(
      { pid: projectId, limit: 50, cursor },
      { signal },
    ),
  );
  if (
    !result.success ||
    result.data.items.some((asset) => asset.project_id !== projectId)
  )
    throw new ApiError(502, "invalid_response");
  return {
    ...result.data,
    items: result.data.items.filter((asset) => asset.kind === "image"),
  };
}
export function getProjectCoverPreview(
  projectId: string,
  assetId: string,
  signal?: AbortSignal,
) {
  return getFolderCoverPreview(
    { project_id: projectId, asset_id: assetId },
    signal,
  );
}
