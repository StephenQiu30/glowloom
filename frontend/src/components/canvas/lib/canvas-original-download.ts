import { assetReviewRead } from "@/components/canvas/runtime/api/asset-review-bff";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { startFileDownload } from "@/components/canvas/runtime/components/assets/lib/original-download";
import { downloadFixedFile } from "@/components/canvas/runtime/components/assets/lib/original-download";
import { CanvasNodeType, type CanvasNodeData } from "../types/canvas";

export async function downloadCanvasOriginal(
  node: CanvasNodeData,
  imageId?: string,
  signal?: AbortSignal,
) {
  if (
    ![
      CanvasNodeType.Image,
      CanvasNodeType.Video,
      CanvasNodeType.Audio,
    ].includes(node.type as CanvasNodeType)
  )
    throw new Error("请选择图片、视频或音频原件");
  const content =
    node.documentData?.fileReferences?.filter(
      (ref) => ref.purpose === "CONTENT",
    ) ?? [];
  const references = node.metadata?.generationOutput?.files ?? content;
  const selectedId = imageId ?? node.metadata?.primaryImageId;
  const reference = selectedId
    ? (content.find((ref) => ref.referenceKey === selectedId) ??
      (references.length === 1 && node.metadata?.images?.length === 1
        ? references[0]
        : undefined))
    : references[0];
  if (!reference) throw new Error("该素材尚未取得已发布的固定文件版本");
  if (
    "publicAssetId" in reference &&
    reference.publicAssetId &&
    reference.publicRevision
  ) {
    const grant = unwrap(
      await assetReviewRead(
        {
          scope: "PUBLIC",
          id: reference.publicAssetId,
          revision: reference.publicRevision,
          purpose: "DOWNLOAD",
        },
        { signal },
      ),
    );
    signal?.throwIfAborted();
    if (!grant.url) throw new Error("原件下载授权无效");
    startFileDownload(grant.url);
    return;
  }
  await downloadFixedFile(
    { fileId: reference.fileId, fileVersion: reference.version },
    signal,
  );
}
