// Screen-to-world mapping and fit framing from the source canvas viewport lib.
// Zoom stays on this product's 8%–800% contract.
import type { CanvasViewport } from "../document";

export const MIN_ZOOM = 0.08;
export const MAX_ZOOM = 8;

export function screenToWorld(
  point: { x: number; y: number },
  viewport: CanvasViewport,
) {
  return {
    x: (point.x - viewport.x) / viewport.zoom,
    y: (point.y - viewport.y) / viewport.zoom,
  };
}

export function zoomAt(
  viewport: CanvasViewport,
  point: { x: number; y: number },
  scale: number,
): CanvasViewport {
  const zoom = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, scale));
  const world = screenToWorld(point, viewport);
  return {
    x: point.x - world.x * zoom,
    y: point.y - world.y * zoom,
    zoom,
  };
}

type Box = { x: number; y: number; width?: number; height?: number };

/** Fit the given nodes inside the visible frame. Selection may reach 100%; the whole canvas stays at 50%. */
export function fitNodes(
  nodes: Box[],
  frame: { left: number; top: number; width: number; height: number },
  selection = false,
): CanvasViewport {
  if (!nodes.length) return { x: 0, y: 0, zoom: 1 };
  const minX = Math.min(...nodes.map((node) => node.x));
  const minY = Math.min(...nodes.map((node) => node.y));
  const maxX = Math.max(...nodes.map((node) => node.x + (node.width ?? 288)));
  const maxY = Math.max(...nodes.map((node) => node.y + (node.height ?? 200)));
  const zoom = Math.max(
    MIN_ZOOM,
    Math.min(
      selection ? 1 : 0.5,
      (frame.width - 96) / (maxX - minX),
      (frame.height - 192) / (maxY - minY),
    ),
  );
  return {
    x: frame.left + frame.width / 2 - ((minX + maxX) / 2) * zoom,
    y: frame.top + frame.height / 2 - ((minY + maxY) / 2) * zoom,
    zoom,
  };
}
