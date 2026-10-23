// Attribution and source revision are recorded in /licenses/canvas-sources.md.
// Upstream notices: /licenses/canvas-sources.md.
import { zoomAt } from "./viewport";
import type { ViewportTransform } from "./types";

type CanvasWheelInput = {
  deltaX: number;
  deltaY: number;
  deltaMode: number;
  ctrlKey: boolean;
  metaKey: boolean;
  shiftKey: boolean;
};

// Unmodified wheel gestures pan; modified wheel/pinch zooms around the cursor.
export function canvasWheelViewport(
  viewport: ViewportTransform,
  event: CanvasWheelInput,
  anchor: { x: number; y: number },
  pageHeight: number,
): ViewportTransform {
  const multiplier =
    event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? pageHeight : 1;
  const deltaX = event.deltaX * multiplier;
  const deltaY = event.deltaY * multiplier;
  const pinch = event.ctrlKey || event.metaKey;
  const rawY = Math.abs(event.deltaY);
  const mouseWheel =
    event.deltaMode !== 0 ||
    (rawY >= 80 &&
      [100, 120].some(
        (step) => Math.abs(rawY - Math.round(rawY / step) * step) < 1,
      ));
  if (!pinch) {
    const horizontal = event.shiftKey && Math.abs(deltaX) < 1;
    return {
      x: viewport.x - (horizontal ? deltaY : deltaX),
      y: viewport.y - (horizontal ? 0 : deltaY),
      k: viewport.k,
    };
  }
  return zoomAt(
    viewport,
    anchor,
    viewport.k * Math.pow(1.1, -deltaY / (pinch && !mouseWheel ? 24 : 72)),
  );
}
