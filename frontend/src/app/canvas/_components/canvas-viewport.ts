// Adapted from BeefTV ddaf5f9abc64b16567c28ea5399d8f2069c47832
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

export const CANVAS_GRAPHICS_VIEWPORT_EVENT =
  "canvas:graphics-viewport-preview";

const liveViewportElements = new WeakMap<
  HTMLDivElement,
  { worldLayer: HTMLElement | null }
>();

export function applyCanvasLiveViewport(
  container: HTMLDivElement | null,
  viewport: ViewportTransform,
  notify = true,
) {
  if (!container) return;
  const committedScale =
    Number(container.style.getPropertyValue("--canvas-committed-scale")) ||
    viewport.k;
  let elements = liveViewportElements.get(container);
  if (!elements) {
    elements = {
      worldLayer: container.querySelector<HTMLElement>(
        "[data-canvas-world-layer]",
      ),
    };
    liveViewportElements.set(container, elements);
  }
  const worldLayer = elements.worldLayer;
  if (worldLayer) {
    // 平移期间直接更新合成层，避免修改容器继承变量导致所有节点重新计算样式。
    worldLayer.style.transformOrigin = "0 0";
    worldLayer.style.transform = `translate3d(${viewport.x}px, ${viewport.y}px, 0) scale(${viewport.k / committedScale})`;
    worldLayer.style.willChange =
      container.dataset.canvasViewportInteracting === "true" ? "transform" : "";
  }
  // Keep the live camera coordinates observable to overlays and automation
  // while the world layer is moved through a compositor transform.
  container.style.setProperty("--canvas-live-x", String(viewport.x));
  container.style.setProperty("--canvas-live-y", String(viewport.y));
  container.style.setProperty("--canvas-live-scale", String(viewport.k));
  // 外置节点标题用同一帧逆倍率抵消世界层缩放，避免等待 React 提交后再校正尺寸。
  container.style.setProperty(
    "--canvas-live-inverse-scale",
    String(1 / Math.max(viewport.k, 0.05)),
  );
  // 图形层必须逐帧跟随 DOM 世界层；浮层和滚动通知仍可按原频率节流。
  container.dispatchEvent(
    new CustomEvent<ViewportTransform>(CANVAS_GRAPHICS_VIEWPORT_EVENT, {
      detail: viewport,
    }),
  );
  if (notify) {
    container.dispatchEvent(
      new CustomEvent<ViewportTransform>(CANVAS_VIEWPORT_EVENT, {
        detail: viewport,
      }),
    );
    // Overlay positioning must follow CSS transforms as well as layout changes.
    container.dispatchEvent(new Event("scroll"));
  }
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
