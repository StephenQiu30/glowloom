// Adapted from BeefTV ddaf5f9abc64b16567c28ea5399d8f2069c47832
// canvas-spatial-index.ts, use-canvas-render-model.ts and canvas-frame.ts (MIT).
// Uses Glowloom world coordinates and no rendered-node count truncation.
import { isEditableNode, type Node } from "./editor-state";
import type { ViewportTransform } from "./canvas-viewport";
type Bounds = { left: number; top: number; right: number; bottom: number };
export type ViewSize = { width: number; height: number };
const cellSize = 1024;
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
  const buckets = new Map<string, number[]>(),
    large: number[] = [];
  const entries = nodes.map((node) => ({ node, bounds: nodeBounds(node) }));
  for (let i = 0; i < entries.length; i++) {
    const b = entries[i].bounds;
    const x0 = Math.floor(b.left / cellSize),
      x1 = Math.floor(b.right / cellSize),
      y0 = Math.floor(b.top / cellSize),
      y1 = Math.floor(b.bottom / cellSize);
    if ((x1 - x0 + 1) * (y1 - y0 + 1) > 256) {
      large.push(i);
      continue;
    }
    for (let y = y0; y <= y1; y++)
      for (let x = x0; x <= x1; x++) {
        const key = `${x}:${y}`,
          list = buckets.get(key) ?? [];
        list.push(i);
        buckets.set(key, list);
      }
  }
  return {
    nodes,
    byID,
    hidden,
    query(bounds: Bounds) {
      const x0 = Math.floor(bounds.left / cellSize),
        x1 = Math.floor(bounds.right / cellSize),
        y0 = Math.floor(bounds.top / cellSize),
        y1 = Math.floor(bounds.bottom / cellSize);
      if ((x1 - x0 + 1) * (y1 - y0 + 1) > 4096)
        return entries
          .filter((e) => intersects(e.bounds, bounds))
          .map((e) => e.node);
      const found = new Set(large);
      for (let y = y0; y <= y1; y++)
        for (let x = x0; x <= x1; x++)
          for (const i of buckets.get(`${x}:${y}`) ?? []) found.add(i);
      return [...found]
        .sort((a, b) => a - b)
        .filter((i) => intersects(entries[i].bounds, bounds))
        .map((i) => entries[i].node);
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
    const x1 = a.x + (a.width ?? 288),
      y1 = a.y + (a.height ?? 200) / 2,
      x2 = b.x,
      y2 = b.y + (b.height ?? 200) / 2;
    const bend = Math.max(50, Math.abs(x2 - x1) / 2);
    if (
      !intersects(
        {
          left: Math.min(x1, x2 - bend),
          right: Math.max(x1 + bend, x2),
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
  const x1 = a.x + (a.width ?? 288),
    y1 = a.y + (a.height ?? 200) / 2,
    x2 = b.x,
    y2 = b.y + (b.height ?? 200) / 2;
  const bend = Math.max(50, Math.abs(x2 - x1) / 2);
  return `M ${x1} ${y1} C ${x1 + bend} ${y1}, ${x2 - bend} ${y2}, ${x2} ${y2}`;
}
