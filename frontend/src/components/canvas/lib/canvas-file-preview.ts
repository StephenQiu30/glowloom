import { getMediaPreview } from "@/api/media";
import { requirePreview } from "@/components/media/library";
import { readCanvasMedia } from "@/components/canvas/runtime/api/canvas-media-bff";
import * as files from "@/components/canvas/runtime/api/file-bff";
import { assetReviewRead } from "@/components/canvas/runtime/api/asset-review-bff";
import { unwrap } from "@/components/canvas/runtime/lib/request";

export type CanvasFilePreviewContext = {
  canvasId: string | number;
  projectId?: string;
  bindingRevision: string | number;
  snapshotId?: string | number;
};

/** Resolve a fresh read grant without changing the document's pinned version. */
export async function readCanvasFilePreviewGrant(
  reference: {
    fileId: string;
    version: string;
    publicAssetId?: string | number;
    publicRevision?: string | number;
  },
  context?: CanvasFilePreviewContext & { nodeId: string; referenceKey: string },
) {
  if (context?.projectId) {
    const value = await getMediaPreview({
      pid: context.projectId,
      asset_id: reference.fileId,
    });
    return {
      url: requirePreview(value, reference.fileId),
      expiresAt: Date.parse(value.expires_at!),
    };
  }
  if (context) {
    const grant = unwrap(
      await readCanvasMedia({
        ...context,
        fileId: reference.fileId,
        version: reference.version,
      }),
    );
    if (!grant.url?.startsWith("/api/file/content?ticket="))
      throw new Error("画布素材读取授权无效");
    return ticketGrant(grant.url, grant.expiresSeconds);
  }
  if (reference.publicAssetId && reference.publicRevision) {
    const grant = unwrap(
      await assetReviewRead({
        scope: "PUBLIC",
        id: reference.publicAssetId,
        revision: reference.publicRevision,
        purpose: "PREVIEW",
      }),
    );
    if (!grant.url?.startsWith("/api/file/content?ticket="))
      throw new Error("公共素材读取授权无效");
    return ticketGrant(grant.url, grant.expiresSeconds);
  }
  const file = unwrap(
    await files.getFileTransfer({ fileId: reference.fileId }),
  );
  if (file.status !== "READY" || file.version !== reference.version)
    throw new Error("画布引用的文件版本不可用");
  const grant = unwrap(
    await files.readFileTransfer({ fileId: reference.fileId }),
  );
  if (!grant.url?.startsWith("/api/file/content?ticket="))
    throw new Error("素材读取授权无效");
  return ticketGrant(grant.url, grant.expiresSeconds);
}

function ticketGrant(url: string, seconds?: number) {
  if (!seconds || !Number.isFinite(seconds) || seconds <= 0)
    throw new Error("素材授权有效期无效");
  return { url, expiresAt: Date.now() + seconds * 1000 };
}
