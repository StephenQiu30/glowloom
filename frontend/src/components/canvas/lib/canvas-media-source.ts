import type { CanvasNodeData } from "../types/canvas";

/** Persisted content references identify uploaded media; generated previews do not. */
export function isFixedCanvasMedia(node: CanvasNodeData) {
  return (
    ["image", "video", "audio"].includes(node.type) &&
    !!node.documentData?.fileReferences?.some(
      (reference) => reference.purpose === "CONTENT",
    )
  );
}
