import type {
  CanvasNodeData,
  Position,
  ViewportTransform,
} from "@/components/canvas/types/canvas";

export const CANVAS_MIN_SCALE = 0.05;
export const CANVAS_MAX_SCALE = 5;
export const CANVAS_GRID_SIZE = 48;

export function canvasPoint(
  point: Position,
  viewport: ViewportTransform,
): Position {
  return {
    x: (point.x - viewport.x) / viewport.k,
    y: (point.y - viewport.y) / viewport.k,
  };
}

export function zoomAt(
  viewport: ViewportTransform,
  point: Position,
  scale: number,
): ViewportTransform {
  const k = Math.min(CANVAS_MAX_SCALE, Math.max(CANVAS_MIN_SCALE, scale));
  const world = canvasPoint(point, viewport);
  return { x: point.x - world.x * k, y: point.y - world.y * k, k };
}

export function fitCanvasNodes(
  nodes: Pick<CanvasNodeData, "position" | "width" | "height">[],
  size: { width: number; height: number },
): ViewportTransform {
  if (!nodes.length) return { x: size.width / 2, y: size.height / 2, k: 1 };
  const left = Math.min(...nodes.map((node) => node.position.x));
  const top = Math.min(...nodes.map((node) => node.position.y));
  const right = Math.max(...nodes.map((node) => node.position.x + node.width));
  const bottom = Math.max(
    ...nodes.map((node) => node.position.y + node.height),
  );
  // Reserve space for the top controls and bottom dock using the existing focus framing ratio.
  const k = Math.max(
    CANVAS_MIN_SCALE,
    Math.min(
      1,
      (size.width * 0.6) / (right - left || 1),
      (size.height * 0.6) / (bottom - top || 1),
    ),
  );
  return {
    x: size.width / 2 - ((left + right) / 2) * k,
    y: size.height / 2 - ((top + bottom) / 2) * k,
    k,
  };
}
