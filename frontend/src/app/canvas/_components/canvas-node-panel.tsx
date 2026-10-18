"use client";
// BeefTV a2196d3: CanvasNodePanelOverlay in canvas-workspace-overlays.tsx (MIT).
// Uses the actual node rectangle during transient drag and viewport frames.
import { useLayoutEffect, useRef, type ReactNode, type RefObject } from "react";
import type { Node } from "./editor-state";
import { nodePanelPosition } from "./canvas-panel-position";
import {
  CANVAS_VIEWPORT_EVENT,
  CANVAS_NODE_DRAG_EVENT,
  type ViewportTransform,
} from "./canvas-viewport";

export function CanvasNodePanel({
  node,
  viewport,
  containerRef,
  children,
  rightInset = 0,
  bottomInset = 0,
}: {
  node: Node;
  viewport: ViewportTransform;
  containerRef: RefObject<HTMLDivElement | null>;
  children: ReactNode;
  rightInset?: number;
  bottomInset?: number;
}) {
  const panelRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const container = containerRef.current,
      panel = panelRef.current;
    if (!container || !panel) return;
    let liveViewport = viewport;
    const update = () => {
      const shell = container.querySelector<HTMLElement>(
        `[data-node-id="${node.id}"] .canvas-node-shell`,
      );
      const bounds = container.getBoundingClientRect();
      const rect = shell?.getBoundingClientRect();
      const screenNode =
        rect && container.clientWidth
          ? {
              x: rect.left - bounds.left,
              y: rect.top - bounds.top,
              width: rect.width,
              height: rect.height,
            }
          : {
              x: node.x,
              y: node.y,
              width: node.width ?? 288,
              height: node.height ?? 200,
            };
      const position = nodePanelPosition(
        screenNode,
        rect && container.clientWidth ? { x: 0, y: 0, k: 1 } : liveViewport,
        Math.max(80, (container.clientWidth || 1024) - rightInset),
        container.clientHeight
          ? Math.max(160, container.clientHeight - bottomInset)
          : Number.POSITIVE_INFINITY,
        panel.offsetHeight || 216,
      );
      panel.style.maxHeight = container.clientHeight
        ? `${Math.max(80, container.clientHeight - 96)}px`
        : "";
      panel.style.width = `${position.width}px`;
      // Avoid a transformed containing block: the shared editor expands in place.
      panel.style.left = `${position.left}px`;
      panel.style.top = `${position.top}px`;
    };
    const preview = (event: Event) => {
      liveViewport = (event as CustomEvent<ViewportTransform>).detail;
      update();
    };
    update();
    const observer =
      typeof ResizeObserver === "undefined"
        ? undefined
        : new ResizeObserver(update);
    observer?.observe(container);
    observer?.observe(panel);
    container.addEventListener(CANVAS_VIEWPORT_EVENT, preview);
    container.addEventListener(CANVAS_NODE_DRAG_EVENT, update);
    return () => {
      observer?.disconnect();
      container.removeEventListener(CANVAS_VIEWPORT_EVENT, preview);
      container.removeEventListener(CANVAS_NODE_DRAG_EVENT, update);
    };
  }, [
    containerRef,
    node.id,
    node.x,
    node.y,
    node.width,
    node.height,
    viewport,
    rightInset,
    bottomInset,
  ]);
  return (
    <div
      ref={panelRef}
      className="canvas-node-panel"
      data-canvas-node-panel={node.id}
      data-canvas-no-zoom
      onPointerDown={(event) => event.stopPropagation()}
    >
      {children}
    </div>
  );
}
