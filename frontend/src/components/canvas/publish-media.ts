import { uploadMediaAsset } from "@/api/media";
import { validateUpload } from "@/components/media/library";
import type { CanvasEditorState } from "./lib/canvas-document-adapter";
export type CanvasMediaReceipt = {
  source: string;
  key: string;
  asset?: API.AssetSummary;
};
export async function publishCanvasMedia(
  state: CanvasEditorState,
  projectID: string,
  receipts: Map<string, CanvasMediaReceipt>,
  signal: AbortSignal,
): Promise<CanvasEditorState> {
  const nodes = [];
  for (const node of state.nodes) {
    const source = node.metadata?.content;
    if (
      !["image", "video", "audio"].includes(node.type) ||
      (node.documentData?.fileReferences?.some(
        (ref) => ref.purpose === "CONTENT",
      ) &&
        !source?.startsWith("data:") &&
        !source?.startsWith("blob:")) ||
      !source
    ) {
      nodes.push(node);
      continue;
    }
    if (!source.startsWith("data:") && !source.startsWith("blob:"))
      throw new Error("请从项目参考素材目录选择已授权的媒体");
    let receipt = receipts.get(node.id);
    if (!receipt || receipt.source !== source) {
      receipt = { source, key: crypto.randomUUID() };
      receipts.set(node.id, receipt);
    }
    if (!receipt.asset) {
      // eslint-disable-next-line no-restricted-syntax -- Read an already validated local data/blob URL, not a business API.
      const response = await fetch(source, { signal });
      const blob = await response.blob();
      const extension =
        blob.type === "image/png"
          ? "png"
          : blob.type === "image/jpeg"
            ? "jpg"
            : (blob.type.split("/")[1] ?? "bin");
      const file = new File([blob], `${node.title || "素材"}.${extension}`, {
        type: blob.type,
      });
      const error = validateUpload(file);
      if (error) throw new Error(error);
      const result = await uploadMediaAsset(
        { pid: projectID },
        { local_review_confirmed: true },
        file,
        { signal, headers: { "Idempotency-Key": receipt.key } },
      );
      if (
        !result.asset?.id ||
        result.asset.project_id !== projectID ||
        result.asset.kind !== node.type
      )
        throw new Error("素材上传回执无效");
      receipt.asset = result.asset;
    }
    const asset = receipt.asset;
    nodes.push({
      ...node,
      documentData: {
        ...(node.documentData ?? {
          id: node.id,
          type: node.type,
          nodeTypeVersion: 1,
          title: node.title,
          position: node.position,
          width: node.width,
          height: node.height,
          zIndex: 1,
        }),
        fileReferences: [
          {
            referenceKey:
              node.documentData?.fileReferences?.find(
                (ref) => ref.purpose === "CONTENT",
              )?.referenceKey ?? asset.id!,
            fileId: asset.id!,
            version: String(asset.revision),
            purpose: "CONTENT",
          },
        ],
      },
    });
  }
  return { ...state, nodes };
}
