import type { CanvasNodeData } from "../types/canvas";
import { canvasConnectionMediaAssetId } from "../lib/canvas-timeline-connections";

/** Use the same pinned source identity as canvas-to-timeline connections. */
export function timelineCanvasAssets(
  nodes: CanvasNodeData[],
  targetID: string,
) {
  return nodes.flatMap((node) => {
    if (node.id === targetID) return [];
    try {
      return [
        {
          nodeID: node.id,
          assetID: canvasConnectionMediaAssetId(node),
          title: node.title,
          kind: node.type,
        },
      ];
    } catch {
      // Empty, ambiguous or locally replaced nodes have no importable source.
      return [];
    }
  });
}
