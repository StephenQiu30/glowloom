"use client";
import { MediaUploadDialog } from "@/components/canvas/media-upload-dialog";
import { uploadCanvasMedia } from "@/components/canvas/queries";
import { ApiError } from "@/lib/request";
import { listFolders, requireFolderScope } from "./folder-queries";
import type { FolderScope } from "./folder-intent";

export function ProjectCoverUpload({
  projectId,
  scope,
  onClose,
  onSelected,
}: {
  projectId: string;
  scope: FolderScope;
  onClose: () => void;
  onSelected: (assetId: string) => void;
}) {
  return (
    <MediaUploadDialog
      open
      target="project-cover"
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
        if (asset.kind !== "image" || asset.project_id !== projectId)
          throw new ApiError(502, "invalid_response");
        return asset;
      }}
      onImported={async (assets) => {
        if (
          assets.length !== 1 ||
          assets[0].kind !== "image" ||
          assets[0].project_id !== projectId
        )
          throw new ApiError(502, "invalid_response");
        onSelected(assets[0].id);
      }}
    />
  );
}
