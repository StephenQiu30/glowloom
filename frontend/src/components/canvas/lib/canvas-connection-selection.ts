import { canvasConnectionHandles } from "../constants/canvas-input-ports";
import type { CanvasNodeData } from "../types/canvas";

export type CanvasConnectionSelection = {
  anchorNodeId: string;
  sourceNodeIds: string[];
};

/** Freeze render order for the gesture; unsupported sources remain for batch validation. */
export function snapshotCanvasConnectionSelection(
  nodes: readonly Pick<CanvasNodeData, "id" | "type">[],
  selectedNodeIds: ReadonlySet<string>,
): CanvasConnectionSelection | null {
  const sources = nodes.filter((node) => selectedNodeIds.has(node.id));
  const eligible = sources.filter(
    (node) => canvasConnectionHandles(node.type).output,
  );
  if (!eligible.length) return null;
  return {
    anchorNodeId: eligible[Math.floor((eligible.length - 1) / 2)].id,
    sourceNodeIds: sources.map((node) => node.id),
  };
}
