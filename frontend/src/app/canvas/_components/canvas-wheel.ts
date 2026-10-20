// Adapted from the local source project at 927a5186a6e0f422d247a5e10c50d48466c10382:
// frontend/src/components/canvas/lib/canvas-wheel.ts and canvas-viewport.ts.
// Keeps Glowloom's viewport identity, 8%-800% limits, and existing persistence owner.
import type { ViewportTransform } from "./canvas-viewport";

type WheelInput = Pick<
  WheelEvent,
  "deltaX" | "deltaY" | "deltaMode" | "ctrlKey" | "metaKey" | "shiftKey"
>;

export function canvasWheelViewport(
  viewport: ViewportTransform,
  event: WheelInput,
  anchor: { x: number; y: number },
  pageHeight: number,
): ViewportTransform {
  const multiplier =
    event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? pageHeight : 1;
  const deltaX = event.deltaX * multiplier;
  const deltaY = event.deltaY * multiplier;
  if (!event.ctrlKey && !event.metaKey) {
    const horizontal = event.shiftKey && Math.abs(deltaX) < 1;
    return {
      x: viewport.x - (horizontal ? deltaY : deltaX),
      y: viewport.y - (horizontal ? 0 : deltaY),
      k: viewport.k,
    };
  }
  const rawY = Math.abs(event.deltaY);
  const mouseWheel =
    event.deltaMode !== 0 ||
    (rawY >= 80 &&
      [100, 120].some(
        (step) => Math.abs(rawY - Math.round(rawY / step) * step) < 1,
      ));
  const k = Math.min(
    8,
    Math.max(
      0.08,
      viewport.k * Math.pow(1.1, -deltaY / (mouseWheel ? 72 : 24)),
    ),
  );
  if (k === viewport.k) return viewport;
  return {
    x: anchor.x - ((anchor.x - viewport.x) / viewport.k) * k,
    y: anchor.y - ((anchor.y - viewport.y) / viewport.k) * k,
    k,
  };
}
