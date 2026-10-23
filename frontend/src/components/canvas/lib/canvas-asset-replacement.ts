import type { InsertAssetPayload } from "@/components/canvas/runtime/components/assets/picker/asset-picker-modal";
import type { CanvasNodeData } from "../types/canvas";
import { toCanvasGraph } from "./canvas-document-adapter";

/** Replace the primary fixed file while preserving the identity used by edges. */
export function replaceCanvasNodeAsset(
  node: CanvasNodeData,
  asset: InsertAssetPayload,
): CanvasNodeData {
  if (asset.kind === "text" || node.type !== asset.kind)
    throw new Error("请选择与当前节点类型相同的素材");
  if (!asset.fileId || !asset.fileVersion)
    throw new Error("请选择已上传的固定版本素材");
  const documentData = toCanvasGraph({
    nodes: [node],
    connections: [],
    backgroundMode: "dots",
    showImageInfo: false,
    viewport: { x: 0, y: 0, k: 1 },
  }).nodes[0];
  const references = documentData.fileReferences ?? [];
  const primary = references.find(
    (reference) => reference.purpose === "CONTENT",
  );
  const replacement = {
    referenceKey: primary?.referenceKey ?? "file-" + crypto.randomUUID(),
    purpose: "CONTENT",
    fileId: asset.fileId,
    version: asset.fileVersion,
    ...(asset.publicAssetId
      ? {
          publicAssetId: asset.publicAssetId,
          publicRevision: asset.publicRevision,
        }
      : {}),
  };
  return {
    ...node,
    documentData: {
      ...documentData,
      parameters: { mode: "MANUAL" },
      fileReferences: primary
        ? references
            .filter((reference) => reference.purpose !== "LAST_FRAME")
            .map((reference) =>
              reference.referenceKey === primary.referenceKey
                ? replacement
                : reference,
            )
        : [
            ...references.filter(
              (reference) => reference.purpose !== "LAST_FRAME",
            ),
            replacement,
          ],
    },
    metadata: {
      ...node.metadata,
      content: asset.kind === "image" ? asset.dataUrl : asset.url,
      generationOutput: undefined,
      images: undefined,
      primaryImageId: undefined,
      status: "success",
      errorDetails: undefined,
      storageKey: undefined,
      naturalWidth: undefined,
      naturalHeight: undefined,
      bytes: undefined,
      mimeType: undefined,
      durationMs: undefined,
      videoTaskId: undefined,
      videoTaskProvider: undefined,
    },
  };
}

/** Detach the uploaded content, keeping the node and its prompt available. */
export function removeCanvasNodeAsset(node: CanvasNodeData): CanvasNodeData {
  return {
    ...node,
    documentData: node.documentData && {
      ...node.documentData,
      parameters: { mode: node.type === "audio" ? "MANUAL" : "GENERATE" },
      fileReferences: [],
    },
    metadata: {
      ...node.metadata,
      content: undefined,
      generationOutput: undefined,
      images: undefined,
      primaryImageId: undefined,
      status: "idle",
      errorDetails: undefined,
      storageKey: undefined,
      naturalWidth: undefined,
      naturalHeight: undefined,
      bytes: undefined,
      mimeType: undefined,
      durationMs: undefined,
      videoTaskId: undefined,
      videoTaskProvider: undefined,
    },
  };
}
