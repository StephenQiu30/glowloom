"use client";
// View-window retention adapted from BeefTV ddaf5f9 (MIT); no node count cap.
import {
  Fragment,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";
import type { Node } from "../document";
import {
  connectionPath,
  buildRenderIndex,
  visibleNodes,
  visibleEdges,
  type ViewSize,
} from "../viewport/render-model";
import {
  CANVAS_NODE_DRAG_EVENT,
  CANVAS_VIEWPORT_EVENT,
  type ViewportTransform,
} from "../viewport/transform";
export function CanvasNodeLayer({
  containerRef,
  nodes,
  edges,
  viewport,
  size,
  selectedIDs,
  renderNode,
}: {
  containerRef: RefObject<HTMLDivElement | null>;
  nodes: Node[];
  edges: API.Edge[];
  viewport: ViewportTransform;
  size: ViewSize;
  selectedIDs: string[];
  renderNode: (node: Node, zoom: number) => ReactNode;
}) {
  const host = useRef<HTMLDivElement>(null);
  const viewportKey = `${viewport.x}/${viewport.y}/${viewport.k}`;
  const [windowState, setWindow] = useState(() => ({
    live: viewport,
    key: viewportKey,
    retained: new Set<string>(),
  }));
  const live = windowState.key === viewportKey ? windowState.live : viewport;
  const index = useMemo(() => buildRenderIndex(nodes), [nodes]);
  const [focused, setFocused] = useState<string>();
  useLayoutEffect(() => {
    const container =
      containerRef.current ??
      host.current?.closest<HTMLElement>("[data-canvas-viewport]") ??
      host.current?.parentElement;
    if (!container) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let next = viewport;
    const publish = () => {
      timer = undefined;
      setWindow((previous) => {
        const pinned = new Set(selectedIDs);
        if (focused) pinned.add(focused);
        const previousView =
          previous.key === viewportKey ? previous.live : viewport;
        return {
          live: next,
          key: viewportKey,
          retained: new Set(
            visibleNodes(
              index,
              previousView,
              size,
              pinned,
              previous.retained,
            ).map((node) => node.id),
          ),
        };
      });
    };
    const preview = (event: Event) => {
      next = (event as CustomEvent<ViewportTransform>).detail;
      if (!timer) timer = setTimeout(publish, 80);
    };
    const focus = (event: FocusEvent) => {
      const target =
        event.target instanceof Element
          ? event.target.closest<HTMLElement>("[data-node-id]")
          : null;
      setFocused(target?.dataset.nodeId);
    };
    const blur = (event: FocusEvent) => {
      if (!container.contains(event.relatedTarget as globalThis.Node | null))
        setFocused(undefined);
    };
    container.addEventListener(CANVAS_VIEWPORT_EVENT, preview);
    container.addEventListener("focusin", focus);
    container.addEventListener("focusout", blur);
    return () => {
      if (timer) clearTimeout(timer);
      container.removeEventListener(CANVAS_VIEWPORT_EVENT, preview);
      container.removeEventListener("focusin", focus);
      container.removeEventListener("focusout", blur);
    };
  }, [containerRef, viewportKey, viewport, index, size, selectedIDs, focused]);
  useLayoutEffect(() => {
    const container =
      containerRef.current ??
      host.current?.closest<HTMLElement>("[data-canvas-viewport]") ??
      host.current?.parentElement;
    if (!container) return;
    const preview = () => {
      const current = (id: string | undefined) => {
        const node = index.byID.get(id ?? "");
        if (!node) return;
        const element = container.querySelector<HTMLElement>(
          `[data-node-id="${node.id}"]`,
        );
        const value = (raw: string | undefined, fallback: number) => {
          const parsed = Number.parseFloat(raw ?? "");
          return Number.isFinite(parsed) ? parsed : fallback;
        };
        return {
          ...node,
          x: value(
            element?.style.getPropertyValue("--canvas-drag-x") ||
              element?.style.left,
            node.x,
          ),
          y: value(
            element?.style.getPropertyValue("--canvas-drag-y") ||
              element?.style.top,
            node.y,
          ),
          width: value(element?.style.width, node.width ?? 288),
          height: value(
            element?.style.getPropertyValue("--canvas-node-height"),
            node.height ?? 200,
          ),
        };
      };
      for (const edge of edges) {
        const path = host.current?.querySelector<SVGPathElement>(
          `[data-connection-id="${edge.id}"]`,
        );
        if (!path) continue;
        const a = current(edge.source_node_id),
          b = current(edge.target_node_id);
        if (a && b) path.setAttribute("d", connectionPath(a, b));
      }
    };
    // A remote render changes the SVG path too; keep it on the held preview
    // before paint even when the pointer has stopped producing events.
    preview();
    container.addEventListener(CANVAS_NODE_DRAG_EVENT, preview);
    return () => container.removeEventListener(CANVAS_NODE_DRAG_EVENT, preview);
  }, [containerRef, index, edges]);
  const pinned = new Set(selectedIDs);
  if (focused) pinned.add(focused);
  const visible = visibleNodes(index, live, size, pinned, windowState.retained);
  const lines = visibleEdges(edges, index.byID, live, size, index.hidden);
  return (
    <div ref={host} className="contents">
      <svg
        className="absolute top-0 left-0 size-px overflow-visible"
        aria-label="画布连线与参考关系"
        style={{ pointerEvents: "none" }}
      >
        {lines.map(({ edge, path }) => (
          <path
            key={edge.id}
            data-connection-id={edge.id}
            data-connection-kind={edge.edge_type}
            d={path}
            fill="none"
            stroke="var(--muted-foreground)"
            strokeOpacity={
              selectedIDs.includes(edge.source_node_id ?? "") ||
              selectedIDs.includes(edge.target_node_id ?? "")
                ? 0.7
                : 0.45
            }
            strokeWidth={1}
            strokeDasharray={
              edge.edge_type === "annotation" ? "4 4" : undefined
            }
            vectorEffect="non-scaling-stroke"
          />
        ))}
      </svg>
      {visible.map((node) => (
        <Fragment key={node.id}>{renderNode(node, live.k)}</Fragment>
      ))}
    </div>
  );
}
