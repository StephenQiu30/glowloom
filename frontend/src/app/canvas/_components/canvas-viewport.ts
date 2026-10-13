// Adapted from BeefTV a2196d32782be6090beb003590ce11a956a423e1
// lib/canvas/canvas-live-viewport.ts and canvas-selection.ts (MIT).
export type ViewportTransform = { x: number; y: number; k: number };
export const CANVAS_VIEWPORT_EVENT = "canvas:viewport-preview";
export const CANVAS_NODE_DRAG_EVENT = "canvas:node-drag-preview";
export type SelectionArea = {
  x: number;
  y: number;
  width: number;
  height: number;
};

export function applyCanvasLiveViewport(
  container: HTMLDivElement | null,
  viewport: ViewportTransform,
) {
  if (!container) return;
  const world = container.querySelector<HTMLElement>(
    "[data-canvas-world-layer]",
  );
  if (world) {
    world.style.transform = `translate3d(${viewport.x}px, ${viewport.y}px, 0) scale(${viewport.k})`;
    world.style.willChange = container.dataset.canvasViewportInteracting
      ? "transform"
      : "";
  }
  container.style.setProperty("--canvas-live-x", String(viewport.x));
  container.style.setProperty("--canvas-live-y", String(viewport.y));
  container.style.setProperty("--canvas-live-scale", String(viewport.k));
  container.style.setProperty(
    "--canvas-live-inverse-scale",
    String(1 / Math.max(viewport.k, 0.05)),
  );
  container.dispatchEvent(
    new CustomEvent<ViewportTransform>(CANVAS_VIEWPORT_EVENT, {
      detail: viewport,
    }),
  );
}

export function resolveCanvasPointerIntent(input: {
  altKey: boolean;
  background: boolean;
  boxSelectEnabled: boolean;
  button: number;
  ctrlKey: boolean;
  metaKey: boolean;
  pointerType: string;
  shiftKey: boolean;
  spacePressed: boolean;
}): "pan" | "select" | "ignore" {
  if (input.pointerType === "touch") return input.background ? "pan" : "ignore";
  if (input.button === 1) return "pan";
  if (input.button !== 0 || !input.background) return "ignore";
  if (input.spacePressed) return "pan";
  return input.boxSelectEnabled ||
    input.altKey ||
    input.ctrlKey ||
    input.metaKey ||
    input.shiftKey
    ? "select"
    : "pan";
}

export function selectionArea(
  start: { x: number; y: number },
  end: { x: number; y: number },
  viewport: ViewportTransform,
): SelectionArea {
  return {
    x: (Math.min(start.x, end.x) - viewport.x) / viewport.k,
    y: (Math.min(start.y, end.y) - viewport.y) / viewport.k,
    width: Math.abs(end.x - start.x) / viewport.k,
    height: Math.abs(end.y - start.y) / viewport.k,
  };
}
