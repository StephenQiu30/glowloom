// Spatial render window adapted from BeefTV use-canvas-render-model.ts.
import type { CanvasNodeData, ViewportTransform } from "../model";
import { buildCanvasSpatialIndex, canvasNodeBounds } from "./spatial-index";
import { isNodeHiddenByCollapsedFrame } from "./frame";
export function createRenderIndex(nodes: CanvasNodeData[]) {
  return buildCanvasSpatialIndex(
    nodes
      .filter((node) => !isNodeHiddenByCollapsedFrame(node, nodes))
      .map((node) => ({
        id: node.id,
        bounds: canvasNodeBounds(node),
        value: node,
      })),
  );
}
export function visibleCanvasNodes(
  index: ReturnType<typeof createRenderIndex>,
  viewport: ViewportTransform,
  size: { width: number; height: number },
  selected: Set<string>,
  nodes: CanvasNodeData[],
) {
  const padding = 480 / viewport.k;
  const bounds = {
    left: -viewport.x / viewport.k - padding,
    top: -viewport.y / viewport.k - padding,
    right: (size.width - viewport.x) / viewport.k + padding,
    bottom: (size.height - viewport.y) / viewport.k + padding,
  };
  const visible = index.query(bounds);
  const ids = new Set(visible.map((node) => node.id));
  nodes.forEach((node) => {
    if (
      selected.has(node.id) &&
      !ids.has(node.id) &&
      !isNodeHiddenByCollapsedFrame(node, nodes)
    )
      visible.push(node);
  });
  return visible.sort(
    (a, b) =>
      Number(a.type !== "group") - Number(b.type !== "group") ||
      a.zIndex - b.zIndex,
  );
}
export function canvasDetailLevel(
  scale: number,
): "overview" | "compact" | "full" {
  return scale < 0.2 ? "overview" : scale < 0.5 ? "compact" : "full";
}
