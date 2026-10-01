"use client";

import { MediaUploadDialog } from "@/components/canvas/media-upload-dialog";
import { uploadCanvasMedia } from "@/components/canvas/queries";
import { ApiError } from "@/lib/request";
import { listFolders, requireFolderScope } from "./folder-queries";
import type { FolderScope } from "./folder-intent";
import type { FolderCover } from "./folder-model";

export function ProjectFolderCoverUpload({
  projectId,
  scope,
  onClose,
  onSelected,
}: {
  projectId: string;
  scope: FolderScope;
  onClose: () => void;
  onSelected: (cover: FolderCover) => void;
}) {
  return (
    <MediaUploadDialog
      open
      target="folder-cover"
      remainingSlots={1}
      maximumFiles={1}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      upload={async (file, key, options) => {
        if (window.location.origin !== scope.origin)
          throw new ApiError(409, "scope_changed");
        requireFolderScope(await listFolders(undefined, options.signal), scope);
        const asset = await uploadCanvasMedia(projectId, file, key, options);
        if (asset.kind !== "image") throw new ApiError(502, "invalid_response");
        return asset;
      }}
      onImported={async (assets) => {
        if (
          assets.length !== 1 ||
          assets[0].kind !== "image" ||
          assets[0].project_id !== projectId
        )
          throw new ApiError(502, "invalid_response");
        onSelected({ project_id: projectId, asset_id: assets[0].id });
      }}
    />
  );
}
