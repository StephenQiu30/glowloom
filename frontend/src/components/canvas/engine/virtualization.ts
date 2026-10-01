// Adapted from BeefTV use-canvas-render-model.ts and canvas-performance-mode.ts.
// Source: 0d9e9f48d407570cd431ad9730cdd522b06810c0 (MIT); see THIRD_PARTY_NOTICES.
import type {
  CanvasConnection,
  CanvasNodeData,
  ViewportTransform,
} from "../model";
import {
  buildCanvasSpatialIndex,
  canvasNodeBounds,
  type CanvasSpatialBounds,
} from "./spatial-index";
import { collapsedConnectionEndpoint } from "./frame";

export const CANVAS_MAX_RENDERED_NODES = 720;
export const CANVAS_MAX_RENDERED_CONNECTIONS = 5000;
type CanvasSize = { width: number; height: number };
export type CanvasNodeRenderOptions = {
  previouslyRenderedIds?: ReadonlySet<string>;
  interactingNodeIds?: ReadonlySet<string>;
  reduceMediaEffects?: boolean;
};
export type CanvasDisplayConnection = {
  edge: CanvasConnection;
  from: CanvasNodeData;
  to: CanvasNodeData;
};
export type CanvasConnectionRenderOptions = {
  selectedEdgeId?: string;
  interactingNodeIds?: ReadonlySet<string>;
  reduceMediaEffects?: boolean;
  dragPreview?: {
    nodeIds: ReadonlySet<string>;
    x: number;
    y: number;
  };
};

export function canvasNodeRenderBudget(scale: number) {
  return scale < 0.14 ? 280 : scale < 0.28 ? 420 : CANVAS_MAX_RENDERED_NODES;
}

export function canvasRenderBounds(
  viewport: ViewportTransform,
  size: CanvasSize,
  reduceMediaEffects = false,
) {
  const left = -viewport.x / viewport.k;
  const top = -viewport.y / viewport.k;
  const withPadding = (pixels: number): CanvasSpatialBounds => {
    const padding = pixels / viewport.k;
    return {
      left: left - padding,
      top: top - padding,
      right: left + size.width / viewport.k + padding,
      bottom: top + size.height / viewport.k + padding,
    };
  };
  return {
    enter: withPadding(reduceMediaEffects ? 128 : 192),
    retain: withPadding(reduceMediaEffects ? 640 : 384),
  };
}

export function createRenderIndex(nodes: CanvasNodeData[]) {
  const byId = new Map(nodes.map((node) => [node.id, node]));
  return buildCanvasSpatialIndex(
    nodes
      .filter((node) => collapsedConnectionEndpoint(node, nodes, byId) === node)
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
  size: CanvasSize,
  selected: ReadonlySet<string>,
  nodes: CanvasNodeData[],
  options: CanvasNodeRenderOptions = {},
) {
  const budget = canvasNodeRenderBudget(viewport.k);
  const bounds = canvasRenderBounds(viewport, size, options.reduceMediaEffects);
  const byId = new Map(nodes.map((node) => [node.id, node]));
  const visible = new Map<string, CanvasNodeData>();
  const add = (node: CanvasNodeData | undefined) => {
    if (
      node &&
      visible.size < budget &&
      collapsedConnectionEndpoint(node, nodes, byId) === node
    )
      visible.set(node.id, node);
  };

  // Forced cards consume the same DOM budget; interaction wins over a dense selection.
  options.interactingNodeIds?.forEach((id) => add(byId.get(id)));
  selected.forEach((id) => add(byId.get(id)));
  const forcedCount = visible.size;
  // Enter-region cards win over retain-only cards when the DOM budget is full.
  index.query(bounds.enter, budget + forcedCount).forEach(add);
  options.previouslyRenderedIds?.forEach((id) => {
    const node = byId.get(id);
    if (node && intersects(canvasNodeBounds(node), bounds.retain)) add(node);
  });
  const order = new Map(nodes.map((node, position) => [node.id, position]));
  return [...visible.values()].sort(
    (a, b) =>
      Number(a.type !== "group") - Number(b.type !== "group") ||
      a.zIndex - b.zIndex ||
      order.get(a.id)! - order.get(b.id)!,
  );
}

export function createConnectionRenderIndex(
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
) {
  const nodeById = new Map(nodes.map((node) => [node.id, node]));
  const byId = new Map<string, CanvasDisplayConnection>();
  const connectionsByNodeId = new Map<string, string[]>();
  const entries = connections.flatMap((edge) => {
    const from = collapsedConnectionEndpoint(
      nodeById.get(edge.fromNodeId),
      nodes,
      nodeById,
    );
    const to = collapsedConnectionEndpoint(
      nodeById.get(edge.toNodeId),
      nodes,
      nodeById,
    );
    if (!from || !to || from.id === to.id) return [];
    const value = { edge, from, to };
    byId.set(edge.id, value);
    for (const id of new Set([
      edge.fromNodeId,
      edge.toNodeId,
      from.id,
      to.id,
    ])) {
      const related = connectionsByNodeId.get(id);
      if (related) related.push(edge.id);
      else connectionsByNodeId.set(id, [edge.id]);
    }
    return [{ id: edge.id, bounds: connectionBounds(from, to), value }];
  });
  return { index: buildCanvasSpatialIndex(entries), byId, connectionsByNodeId };
}

export function visibleCanvasConnections(
  renderIndex: ReturnType<typeof createConnectionRenderIndex>,
  viewport: ViewportTransform,
  size: CanvasSize,
  options: CanvasConnectionRenderOptions = {},
) {
  const bounds = canvasRenderBounds(viewport, size, options.reduceMediaEffects);
  const forced = new Set<string>();
  if (options.selectedEdgeId) forced.add(options.selectedEdgeId);
  const addRelated = (id: string) =>
    renderIndex.connectionsByNodeId
      .get(id)
      ?.forEach((edgeId) => forced.add(edgeId));
  options.interactingNodeIds?.forEach(addRelated);
  options.dragPreview?.nodeIds.forEach(addRelated);
  const candidates = new Map<string, CanvasDisplayConnection>();
  forced.forEach((id) => {
    const value = renderIndex.byId.get(id);
    if (value) candidates.set(id, value);
  });
  renderIndex.index
    .query(bounds.retain, CANVAS_MAX_RENDERED_CONNECTIONS + forced.size)
    .forEach((value) => candidates.set(value.edge.id, value));
  const visible: CanvasDisplayConnection[] = [];
  for (const value of candidates.values()) {
    const from = dragPosition(value.from, options.dragPreview);
    const to = dragPosition(value.to, options.dragPreview);
    if (
      !forced.has(value.edge.id) &&
      !intersects(connectionBounds(from, to), bounds.retain)
    )
      continue;
    visible.push(
      from === value.from && to === value.to ? value : { ...value, from, to },
    );
    if (visible.length === CANVAS_MAX_RENDERED_CONNECTIONS) break;
  }
  return visible;
}

function dragPosition(
  node: CanvasNodeData,
  dragPreview: CanvasConnectionRenderOptions["dragPreview"],
) {
  return dragPreview?.nodeIds.has(node.id)
    ? {
        ...node,
        position: {
          x: node.position.x + dragPreview.x,
          y: node.position.y + dragPreview.y,
        },
      }
    : node;
}

function connectionBounds(
  from: CanvasNodeData,
  to: CanvasNodeData,
): CanvasSpatialBounds {
  const a = canvasNodeBounds(from);
  const b = canvasNodeBounds(to);
  return {
    left: Math.min(a.left, b.left),
    top: Math.min(a.top, b.top),
    right: Math.max(a.right, b.right),
    bottom: Math.max(a.bottom, b.bottom),
  };
}

function intersects(a: CanvasSpatialBounds, b: CanvasSpatialBounds) {
  return (
    a.right > b.left && a.left < b.right && a.bottom > b.top && a.top < b.bottom
  );
}

export function canvasDetailLevel(
  scale: number,
): "overview" | "compact" | "full" {
  return scale < 0.2 ? "overview" : scale < 0.5 ? "compact" : "full";
}
