import { canvasNodeRenderHeight } from "../model";
// Adapted from BeefTV canvas-project-domain.ts 325–398; business generation removed.
import type { CanvasNodeData, Position } from "../model";
import { isNodeHiddenByCollapsedFrame } from "./frame";
export type NodeAlignmentContext = {
  movingBounds: { left: number; top: number; right: number; bottom: number };
  targets: { x: number[]; y: number[] }[];
};
export function createNodeAlignmentContext(
  nodes: CanvasNodeData[],
  positions: { id: string; x: number; y: number }[],
): NodeAlignmentContext | null {
  const movingIds = new Set(positions.map((node) => node.id)),
    initial = new Map(positions.map((node) => [node.id, node]));
  const moving = nodes.filter((node) => movingIds.has(node.id));
  if (!moving.length) return null;
  const left = Math.min(
      ...moving.map((node) => initial.get(node.id)?.x ?? node.position.x),
    ),
    top = Math.min(
      ...moving.map((node) => initial.get(node.id)?.y ?? node.position.y),
    );
  const right = Math.max(
      ...moving.map(
        (node) => (initial.get(node.id)?.x ?? node.position.x) + node.width,
      ),
    ),
    bottom = Math.max(
      ...moving.map(
        (node) =>
          (initial.get(node.id)?.y ?? node.position.y) +
          canvasNodeRenderHeight(node),
      ),
    );
  return {
    movingBounds: { left, top, right, bottom },
    targets: nodes
      .filter(
        (node) =>
          !movingIds.has(node.id) && !isNodeHiddenByCollapsedFrame(node, nodes),
      )
      .map((node) => ({
        x: [
          node.position.x,
          node.position.x + node.width / 2,
          node.position.x + node.width,
        ],
        y: [
          node.position.y,
          node.position.y + canvasNodeRenderHeight(node) / 2,
          node.position.y + canvasNodeRenderHeight(node),
        ],
      })),
  };
}
export function calculateNodeAlignment(
  context: NodeAlignmentContext | null,
  raw: Position,
  threshold: number,
) {
  if (!context)
    return {
      offset: raw,
      guides: {} as { vertical?: number; horizontal?: number },
    };
  const { left, top, right, bottom } = context.movingBounds;
  const movingX = [left + raw.x, (left + right) / 2 + raw.x, right + raw.x],
    movingY = [top + raw.y, (top + bottom) / 2 + raw.y, bottom + raw.y];
  let dx: number | undefined,
    dy: number | undefined,
    vertical: number | undefined,
    horizontal: number | undefined;
  context.targets.forEach((target) => {
    movingX.forEach((value, i) => {
      const delta = target.x[i] - value;
      if (
        Math.abs(delta) <= threshold &&
        (dx === undefined || Math.abs(delta) < Math.abs(dx))
      ) {
        dx = delta;
        vertical = target.x[i];
      }
    });
    movingY.forEach((value, i) => {
      const delta = target.y[i] - value;
      if (
        Math.abs(delta) <= threshold &&
        (dy === undefined || Math.abs(delta) < Math.abs(dy))
      ) {
        dy = delta;
        horizontal = target.y[i];
      }
    });
  });
  return {
    offset: { x: raw.x + (dx ?? 0), y: raw.y + (dy ?? 0) },
    guides: { vertical, horizontal },
  };
}
export function sameStringSet(a: Set<string>, b: Set<string>) {
  return a.size === b.size && [...a].every((id) => b.has(id));
}
