// Adapted from BeefTV ddaf5f9abc64b16567c28ea5399d8f2069c47832
// canvas-spatial-index.ts, use-canvas-render-model.ts and canvas-frame.ts (MIT).
// Uses Glowloom world coordinates and no rendered-node count truncation.
import { isEditableNode, type Node } from "../document";
import type { ViewportTransform } from "./transform";
type Bounds = { left: number; top: number; right: number; bottom: number };
export type ViewSize = { width: number; height: number };
import { buildCanvasSpatialIndex } from "./spatial-index";
const intersects = (a: Bounds, b: Bounds) =>
  a.right >= b.left &&
  a.left <= b.right &&
  a.bottom >= b.top &&
  a.top <= b.bottom;
export function nodeBounds(node: Node): Bounds {
  return {
    left: node.x,
    top: node.y - 40,
    right: node.x + (node.width ?? 288),
    bottom: node.y + (node.height ?? 200) + 24,
  };
}
export function viewBounds(
  viewport: ViewportTransform,
  size: ViewSize,
  padding = 0,
): Bounds {
  return {
    left: (-viewport.x - padding) / viewport.k,
    top: (-viewport.y - padding) / viewport.k,
    right: (size.width - viewport.x + padding) / viewport.k,
    bottom: (size.height - viewport.y + padding) / viewport.k,
  };
}
export function descendants(
  nodes: Node[],
  selected: ReadonlySet<string>,
): Set<string> {
  const children = new Map<string, string[]>();
  for (const node of nodes)
    if (node.parent_id) {
      const list = children.get(node.parent_id) ?? [];
      list.push(node.id);
      children.set(node.parent_id, list);
    }
  const ids = new Set(selected),
    queue = [...selected];
  for (let i = 0; i < queue.length; i++)
    for (const id of children.get(queue[i]) ?? [])
      if (!ids.has(id)) {
        ids.add(id);
        queue.push(id);
      }
  return ids;
}
export function moveSelection(
  nodes: Node[],
  selected: ReadonlySet<string>,
  dx: number,
  dy: number,
): Node[] {
  const ids = descendants(nodes, selected);
  if (nodes.some((node) => ids.has(node.id) && !isEditableNode(node)))
    throw new Error("所选分组包含只读节点，不能整体移动。");
  const clamp = (n: number) => Math.max(-1e6, Math.min(1e6, n));
  return nodes.map((node) =>
    ids.has(node.id)
      ? { ...node, x: clamp(node.x + dx), y: clamp(node.y + dy) }
      : node,
  );
}
export function buildRenderIndex(nodes: Node[]) {
  // Preserve legacy documents on save, but retired tools have no canvas UI.
  nodes = nodes.filter((node) => node.node_type !== "timeline");
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const hidden = descendants(
    nodes,
    new Set(
      nodes
        .filter((node) => node.node_type === "group" && node.config.collapsed)
        .map((node) => node.id),
    ),
  );
  for (const node of nodes)
    if (node.node_type === "group" && node.config.collapsed && !node.parent_id)
      hidden.delete(node.id);
  // A collapsed nested frame remains visible unless one of its ancestors is collapsed.
  for (const node of nodes)
    if (hidden.has(node.id)) {
      let parent = node.parent_id ? byID.get(node.parent_id) : undefined;
      const seen = new Set<string>();
      let collapsedAncestor = false;
      while (parent && !seen.has(parent.id)) {
        seen.add(parent.id);
        if (parent.config.collapsed) collapsedAncestor = true;
        parent = parent.parent_id ? byID.get(parent.parent_id) : undefined;
      }
      if (!collapsedAncestor) hidden.delete(node.id);
    }
  const entries = nodes.map((node) => ({
    id: node.id,
    value: node,
    bounds: nodeBounds(node),
  }));
  const spatial = buildCanvasSpatialIndex(entries);
  return {
    nodes,
    byID,
    hidden,
    query(bounds: Bounds) {
      // Keep huge legal cameras bounded; the original grid query has no coverage guard.
      const columns = Math.ceil((bounds.right - bounds.left) / 1024) + 1;
      const rows = Math.ceil((bounds.bottom - bounds.top) / 1024) + 1;
      if (columns * rows > 4096)
        return entries
          .filter((entry) => intersects(entry.bounds, bounds))
          .map((entry) => entry.value);
      return spatial.query(bounds);
    },
  };
}
export function visibleNodes(
  index: ReturnType<typeof buildRenderIndex>,
  viewport: ViewportTransform,
  size: ViewSize,
  pinned: ReadonlySet<string> = new Set(),
  retained: ReadonlySet<string> = new Set(),
): Node[] {
  const enter = viewBounds(viewport, size, 192),
    retain = viewBounds(viewport, size, 384);
  const found = new Set(
    index
      .query(retain)
      .filter(
        (node) =>
          !index.hidden.has(node.id) &&
          (retained.has(node.id) || intersects(nodeBounds(node), enter)),
      )
      .map((n) => n.id),
  );
  for (const id of pinned) if (index.byID.has(id)) found.add(id);
  return index.nodes.filter((node) => found.has(node.id));
}
export function visibleEdges(
  edges: API.Edge[],
  nodes: ReadonlyMap<string, Node>,
  viewport: ViewportTransform,
  size: ViewSize,
  hidden: ReadonlySet<string> = new Set(),
) {
  const bounds = viewBounds(viewport, size, 192);
  return edges.flatMap((edge) => {
    const a = nodes.get(edge.source_node_id ?? ""),
      b = nodes.get(edge.target_node_id ?? "");
    if (!a || !b || hidden.has(a.id) || hidden.has(b.id)) return [];
    const { x1, y1, x2, y2, c1, c2 } = connectionGeometry(a, b);
    if (
      !intersects(
        {
          left: Math.min(x1, x2, c1, c2),
          right: Math.max(x1, x2, c1, c2),
          top: Math.min(y1, y2),
          bottom: Math.max(y1, y2),
        },
        bounds,
      )
    )
      return [];
    return [
      {
        edge,
        path: connectionPath(a, b),
      },
    ];
  });
}

export function connectionPath(a: Node, b: Node): string {
  const { x1, y1, x2, y2, c1, c2 } = connectionGeometry(a, b);
  return `M ${x1} ${y1} C ${c1} ${y1}, ${c2} ${y2}, ${x2} ${y2}`;
}

function connectionGeometry(a: Node, b: Node) {
  const rightward = a.x + (a.width ?? 288) / 2 <= b.x + (b.width ?? 288) / 2;
  const direction = rightward ? 1 : -1;
  const x1 = rightward ? a.x + (a.width ?? 288) : a.x,
    y1 = a.y + (a.height ?? 200) / 2,
    x2 = rightward ? b.x : b.x + (b.width ?? 288),
    y2 = b.y + (b.height ?? 200) / 2;
  const bend = Math.max(50, Math.abs(x2 - x1) / 2);
  return {
    x1,
    y1,
    x2,
    y2,
    c1: x1 + direction * bend,
    c2: x2 - direction * bend,
  };
}
