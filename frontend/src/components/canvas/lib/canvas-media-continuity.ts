import type { CanvasNodeData } from "../types/canvas";

function contentIdentity(node: CanvasNodeData) {
  const ref = node.documentData?.fileReferences?.find(
    (value) => value.purpose === "CONTENT",
  );
  return (
    ref &&
    JSON.stringify([
      ref.referenceKey,
      ref.fileId,
      ref.version,
      ref.publicAssetId ?? null,
      ref.publicRevision ?? null,
    ])
  );
}

/** Same editor session only. Receipts own business fields; previews stay in memory. */
export function retainCanvasMediaPreviews(
  previous: CanvasNodeData[],
  incoming: CanvasNodeData[],
): CanvasNodeData[] {
  const before = new Map(previous.map((node) => [node.id, node]));
  let changed = false;
  const next = incoming.map((node) => {
    if (
      !["image", "video", "audio"].includes(node.type) ||
      node.metadata?.content
    )
      return node;
    const old = before.get(node.id);
    const identity = contentIdentity(node);
    if (
      !old ||
      old.type !== node.type ||
      !identity ||
      identity !== contentIdentity(old) ||
      !old.metadata?.content
    )
      return node;
    changed = true;
    return {
      ...node,
      metadata: {
        ...node.metadata,
        content: old.metadata.content,
        naturalWidth: old.metadata.naturalWidth,
        naturalHeight: old.metadata.naturalHeight,
      },
    };
  });
  return changed ? next : incoming;
}
