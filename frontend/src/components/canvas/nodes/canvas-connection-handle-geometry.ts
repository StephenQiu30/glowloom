/** Jimeng handle coordinates are world units; chrome stops counter-scaling below 50%. */
export type ConnectionHandleSide = "left" | "right";
export type HandlePoint = { x: number; y: number };

export function connectionHandleCounterScale(zoom: number): number {
  return 1 / Math.max(Number.isFinite(zoom) && zoom > 0 ? zoom : 1, 0.5);
}

export function connectionHandleRest(
  side: ConnectionHandleSide,
  zoom: number,
): HandlePoint {
  return {
    x: 30 + (side === "right" ? 22 : -22) * connectionHandleCounterScale(zoom),
    y: 60,
  };
}

export function connectionHandleTarget(
  side: ConnectionHandleSide,
  pointer: HandlePoint,
  rect: Pick<DOMRect, "left" | "top" | "width" | "height">,
): HandlePoint | null {
  if (rect.width <= 0 || rect.height <= 0) return null;
  const x = ((pointer.x - rect.left) / rect.width) * 60;
  const y = ((pointer.y - rect.top) / rect.height) * 120;
  if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
  return {
    x: Math.max(
      side === "right" ? 30 : -30,
      Math.min(side === "right" ? 90 : 30, x),
    ),
    y: Math.max(0, Math.min(120, y)),
  };
}

export function advanceConnectionHandle(
  current: HandlePoint,
  target: HandlePoint,
  elapsed: number | null,
) {
  const dt = elapsed === null ? 1000 / 60 : Math.max(0, Math.min(64, elapsed));
  const alpha = 1 - Math.exp(-dt / 80);
  const point = {
    x: current.x + (target.x - current.x) * alpha,
    y: current.y + (target.y - current.y) * alpha,
  };
  const settled =
    Math.abs(target.x - point.x) <= 0.05 &&
    Math.abs(target.y - point.y) <= 0.05;
  return { point: settled ? target : point, settled };
}

export function connectionHandleTransform(
  point: HandlePoint,
  counterScale: number,
): string {
  return `translate3d(${Number(point.x.toFixed(2))}px, ${Number(point.y.toFixed(2))}px, 0) scale(${counterScale}) translate(-50%, -50%)`;
}
