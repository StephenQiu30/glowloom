// Viewport math adapted from basketikun/infinite-canvas (MIT, commit dab19adc).
// See UPSTREAM-LICENSE.md in this directory for attribution.

export type CanvasViewport = { x: number; y: number; k: number };
export type CanvasPoint = { x: number; y: number };
export type CanvasRect = CanvasPoint & { width: number; height: number };

export const MIN_ZOOM = 0.35;
export const MAX_ZOOM = 2.5;

export function clampZoom(zoom: number): number {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, zoom));
}

export function zoomAtPoint(
  viewport: CanvasViewport,
  nextZoom: number,
  point: CanvasPoint,
): CanvasViewport {
  const k = clampZoom(nextZoom);
  const worldX = (point.x - viewport.x) / viewport.k;
  const worldY = (point.y - viewport.y) / viewport.k;
  return { x: point.x - worldX * k, y: point.y - worldY * k, k };
}

export function fitCanvas(
  rects: CanvasRect[],
  size: { width: number; height: number },
  padding = 80,
): CanvasViewport {
  if (!rects.length || !size.width || !size.height) return { x: 0, y: 0, k: 1 };

  const left = Math.min(...rects.map((rect) => rect.x));
  const top = Math.min(...rects.map((rect) => rect.y));
  const right = Math.max(...rects.map((rect) => rect.x + rect.width));
  const bottom = Math.max(...rects.map((rect) => rect.y + rect.height));
  const width = Math.max(1, right - left);
  const height = Math.max(1, bottom - top);
  const k = clampZoom(
    Math.min((size.width - padding * 2) / width, (size.height - padding * 2) / height, 1),
  );

  return {
    x: (size.width - width * k) / 2 - left * k,
    y: (size.height - height * k) / 2 - top * k,
    k,
  };
}
