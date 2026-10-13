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
}: {
  node: Node;
  viewport: ViewportTransform;
  containerRef: RefObject<HTMLDivElement | null>;
  children: ReactNode;
}) {
  const panelRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const container = containerRef.current,
      panel = panelRef.current;
    if (!container || !panel) return;
    let liveViewport = viewport;
    const update = () => {
      const position = nodePanelPosition(
        {
          x: node.x,
          y: node.y,
          width: node.width ?? 288,
          height: node.height ?? 200,
        },
        liveViewport,
        container.clientWidth || 1024,
      );
      const shell = container.querySelector<HTMLElement>(
        `[data-node-id="${node.id}"] .canvas-node-shell`,
      );
      if (shell && container.clientWidth) {
        const rect = shell.getBoundingClientRect(),
          bounds = container.getBoundingClientRect();
        position.left = Math.max(
          16,
          Math.min(
            container.clientWidth - position.width - 16,
            rect.left - bounds.left + rect.width / 2 - position.width / 2,
          ),
        );
        position.top = rect.bottom - bounds.top + 16;
      }
      panel.style.width = `${position.width}px`;
      panel.style.transform = `translate3d(${position.left}px, ${position.top}px, 0)`;
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
