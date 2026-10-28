import type { CanvasNodeData } from "../types/canvas";
import { nodeBounds } from "./canvas-node-geometry";

export function layoutCanvasSelection(
  nodes: CanvasNodeData[],
  selected: Set<string>,
  mode: "grid" | "smart",
): CanvasNodeData[] {
  const byId = new Map(nodes.map((node) => [node.id, node]));
  const selectedAncestor = (node: CanvasNodeData) => {
    let parent = node.metadata?.groupId;
    const visited = new Set<string>();
    while (parent && !visited.has(parent)) {
      if (selected.has(parent)) return parent;
      visited.add(parent);
      parent = byId.get(parent)?.metadata?.groupId;
    }
    return undefined;
  };
  const roots = nodes.filter(
    (node) => selected.has(node.id) && !selectedAncestor(node),
  );
  if (roots.length < 2) return nodes;
  const bounds = nodeBounds(roots),
    gap = 48;
  const columns = Math.ceil(Math.sqrt(roots.length));
  const cellWidth = Math.max(...roots.map((n) => n.width)) + gap;
  const cellHeight = Math.max(...roots.map((n) => n.height)) + gap;
  const targetWidth = Math.max(
    cellWidth - gap,
    Math.sqrt(
      roots.reduce((sum, n) => sum + (n.width + gap) * (n.height + gap), 0),
    ),
  );
  const offsets = new Map<string, { x: number; y: number }>();
  let x = bounds.left,
    y = bounds.top,
    rowHeight = 0;
  roots.forEach((node, index) => {
    if (mode === "grid") {
      x = bounds.left + (index % columns) * cellWidth;
      y = bounds.top + Math.floor(index / columns) * cellHeight;
    } else if (x > bounds.left && x + node.width > bounds.left + targetWidth) {
      x = bounds.left;
      y += rowHeight + gap;
      rowHeight = 0;
    }
    offsets.set(node.id, { x: x - node.position.x, y: y - node.position.y });
    if (mode === "smart") {
      x += node.width + gap;
      rowHeight = Math.max(rowHeight, node.height);
    }
  });
  return nodes.map((node) => {
    let offset = offsets.get(node.id);
    let parent = node.metadata?.groupId;
    const visited = new Set<string>();
    while (!offset && parent && !visited.has(parent)) {
      visited.add(parent);
      offset = offsets.get(parent);
      parent = byId.get(parent)?.metadata?.groupId;
    }
    return offset
      ? {
          ...node,
          position: {
            x: node.position.x + offset.x,
            y: node.position.y + offset.y,
          },
        }
      : node;
  });
}
