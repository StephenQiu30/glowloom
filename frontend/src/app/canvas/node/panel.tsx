"use client";
// BeefTV a2196d3: CanvasNodePanelOverlay in canvas-workspace-overlays.tsx (MIT).
// Uses the actual node rectangle during transient drag and viewport frames.
import { useLayoutEffect, useRef, type ReactNode, type RefObject } from "react";
import type { Node } from "../document";
import { nodePanelPosition } from "./panel-position";
import {
  CANVAS_VIEWPORT_EVENT,
  CANVAS_NODE_DRAG_EVENT,
  type ViewportTransform,
} from "../viewport/transform";

export function CanvasNodePanel({
  node,
  viewport,
  containerRef,
  children,
  rightInset = 0,
  bottomInset = 0,
  onReveal,
}: {
  node: Node;
  viewport: ViewportTransform;
  containerRef: RefObject<HTMLDivElement | null>;
  children: ReactNode;
  rightInset?: number;
  bottomInset?: number;
  onReveal?: (dy: number) => void;
}) {
  const panelRef = useRef<HTMLDivElement>(null);
  const revealed = useRef<string | null>(null);
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
      const expanded = !!panel.querySelector('[data-expanded="true"]');
      panel.style.visibility =
        !expanded &&
        rect &&
        bounds.height &&
        (rect.bottom < bounds.top ||
          rect.top > bounds.bottom ||
          rect.right < bounds.left ||
          rect.left > bounds.right)
          ? "hidden"
          : "";
      const position = nodePanelPosition(
        screenNode,
        rect && container.clientWidth ? { x: 0, y: 0, k: 1 } : liveViewport,
        Math.max(80, (container.clientWidth || 1024) - rightInset),
        container.clientHeight
          ? Math.max(160, container.clientHeight - bottomInset)
          : Number.POSITIVE_INFINITY,
      );
      if (revealed.current !== node.id && container.clientHeight && onReveal) {
        revealed.current = node.id;
        const missing =
          position.top +
          Math.min(panel.scrollHeight || 280, 320) +
          56 -
          container.clientHeight;
        if (missing > 0) onReveal(-missing);
      }
      panel.style.maxHeight = Number.isFinite(position.maxHeight)
        ? `${position.maxHeight}px`
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
    onReveal,
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
