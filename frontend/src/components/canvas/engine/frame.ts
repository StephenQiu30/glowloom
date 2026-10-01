// Adapted from BeefTV canvas-frame.ts; only geometry and grouping are retained.
import { CanvasNodeType, type CanvasNodeData, type Position } from "../model";
import {
  buildCanvasSpatialIndex,
  canvasNodeBounds,
  type CanvasSpatialIndex,
} from "./spatial-index";
export const FRAME_HEADER_HEIGHT = 36,
  FRAME_PADDING = 24;
export function isFrameNode(node?: CanvasNodeData | null) {
  return node?.type === CanvasNodeType.Frame;
}
export function getFrameChildIds(id: string, nodes: CanvasNodeData[]) {
  const children = new Set<string>();
  const pending = [id];
  while (pending.length) {
    const parent = pending.shift()!;
    for (const node of nodes)
      if (
        node.parentId === parent &&
        node.id !== id &&
        !children.has(node.id)
      ) {
        children.add(node.id);
        pending.push(node.id);
      }
  }
  return children;
}
/** 选择祖先时只提交祖先的 drop 关系，后代仍按同一 world delta 移动。 */
export function selectionRootNodes(
  nodes: CanvasNodeData[],
  ids: ReadonlySet<string>,
) {
  const byId = new Map(nodes.map((node) => [node.id, node]));
  return nodes.filter((node) => {
    if (!ids.has(node.id)) return false;
    const seen = new Set<string>();
    let parent = node.parentId;
    while (parent && !seen.has(parent)) {
      if (ids.has(parent)) return false;
      seen.add(parent);
      parent = byId.get(parent)?.parentId;
    }
    return true;
  });
}
export function moveNodesWithChildren(
  nodes: CanvasNodeData[],
  positions: ReadonlyMap<string, Position>,
) {
  const byId = new Map(nodes.map((node) => [node.id, node]));
  return nodes.map((node) => {
    let anchor = positions.has(node.id) ? node : undefined,
      parent = node.parentId;
    const seen = new Set<string>();
    while (parent && !seen.has(parent)) {
      seen.add(parent);
      const ancestor = byId.get(parent);
      if (ancestor && positions.has(parent)) anchor = ancestor;
      parent = ancestor?.parentId;
    }
    if (!anchor) return node;
    const target = positions.get(anchor.id)!;
    return {
      ...node,
      position: {
        x: node.position.x + target.x - anchor.position.x,
        y: node.position.y + target.y - anchor.position.y,
      },
    };
  });
}
export function collapsedConnectionEndpoint(
  node: CanvasNodeData | undefined,
  nodes: CanvasNodeData[],
  byId: ReadonlyMap<string, CanvasNodeData> = new Map(
    nodes.map((item) => [item.id, item]),
  ),
) {
  const seen = new Set<string>();
  let visible = node,
    parentId = node?.parentId;
  while (parentId && !seen.has(parentId)) {
    seen.add(parentId);
    const parent = byId.get(parentId);
    if (parent?.metadata?.frame?.collapsed) visible = parent;
    parentId = parent?.parentId;
  }
  return visible;
}
export function isNodeHiddenByCollapsedFrame(
  node: CanvasNodeData,
  nodes: CanvasNodeData[],
) {
  const byId = new Map(nodes.map((item) => [item.id, item])),
    seen = new Set<string>();
  let parentId = node.parentId;
  while (parentId && !seen.has(parentId)) {
    seen.add(parentId);
    const parent = byId.get(parentId);
    if (parent?.metadata?.frame?.collapsed) return true;
    parentId = parent?.parentId;
  }
  return false;
}
export function buildCanvasFrameDropIndex(nodes: CanvasNodeData[]) {
  return buildCanvasSpatialIndex(
    nodes.filter(isFrameNode).map((node) => ({
      id: node.id,
      bounds: canvasNodeBounds(node),
      value: node,
    })),
  );
}
export function findFrameDropTargetFromIndex(
  index: CanvasSpatialIndex<CanvasNodeData>,
  dragged: CanvasNodeData[],
  ids: Set<string>,
  offset: Position,
) {
  if (!dragged.length) return null;
  const centers = dragged.map((node) => ({
    x: node.position.x + offset.x + node.width / 2,
    y: node.position.y + offset.y + node.height / 2,
  }));
  const bounds = {
    left: Math.min(...centers.map((p) => p.x)) - 0.01,
    top: Math.min(...centers.map((p) => p.y)) - 0.01,
    right: Math.max(...centers.map((p) => p.x)) + 0.01,
    bottom: Math.max(...centers.map((p) => p.y)) + 0.01,
  };
  return (
    [...index.query(bounds)]
      .reverse()
      .find(
        (frame) =>
          !ids.has(frame.id) &&
          !dragged.some((node) => node.id === frame.id) &&
          !frame.metadata?.frame?.collapsed &&
          centers.every(
            (p) =>
              p.x >= frame.position.x &&
              p.x <= frame.position.x + frame.width &&
              p.y >= frame.position.y + FRAME_HEADER_HEIGHT &&
              p.y <= frame.position.y + frame.height,
          ),
      )?.id ?? null
  );
}
export function applyFrameDrop(
  nodes: CanvasNodeData[],
  ids: Set<string>,
  frameId: string | null,
) {
  const target = nodes.find((node) => node.id === frameId && isFrameNode(node));
  if (
    target &&
    [...ids].some(
      (id) => id === target.id || getFrameChildIds(id, nodes).has(target.id),
    )
  )
    throw new Error("分组关系不能形成循环。");
  const next = nodes.map((node) =>
    !ids.has(node.id) ? node : { ...node, parentId: target?.id },
  );
  if (!target) return next;
  const children = next.filter((node) => node.parentId === target.id);
  if (!children.length) return next;
  const left = Math.min(
    target.position.x,
    ...children.map((node) => node.position.x - FRAME_PADDING),
  );
  const top = Math.min(
    target.position.y,
    ...children.map(
      (node) => node.position.y - FRAME_HEADER_HEIGHT - FRAME_PADDING,
    ),
  );
  const right = Math.max(
    target.position.x + target.width,
    ...children.map((node) => node.position.x + node.width + FRAME_PADDING),
  );
  const bottom = Math.max(
    target.position.y + target.height,
    ...children.map((node) => node.position.y + node.height + FRAME_PADDING),
  );
  return next.map((node) =>
    node.id !== target.id
      ? node
      : {
          ...node,
          position: { x: left, y: top },
          width: right - left,
          height: bottom - top,
          metadata: {
            frame: {
              collapsed: false,
              expandedWidth: right - left,
              expandedHeight: bottom - top,
            },
          },
        },
  );
}
