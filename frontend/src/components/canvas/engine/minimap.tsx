import { canvasNodeRenderHeight } from "../model";
// World fit and interactive camera projection adapted from BeefTV canvas-mini-map.tsx.
import { useMemo, useRef, useState, useEffect } from "react";
import type { CanvasNodeData, ViewportTransform } from "../model";
import { getCanvasNodesBounds } from "./viewport";
import { subscribeCanvasViewportPreview } from "./viewport-dom";
import { isNodeHiddenByCollapsedFrame } from "./frame";
export function Minimap({
  nodes,
  viewport,
  size,
  container,
  onPreview,
  onCommit,
  disabled,
}: {
  nodes: CanvasNodeData[];
  viewport: ViewportTransform;
  size: { width: number; height: number };
  container: React.RefObject<HTMLDivElement | null>;
  onPreview: (viewport: ViewportTransform) => void;
  onCommit: (viewport: ViewportTransform) => void;
  disabled?: boolean;
}) {
  const [preview, setPreview] = useState<{
    base: ViewportTransform;
    camera: ViewportTransform;
  }>();
  const camera = preview?.base === viewport ? preview.camera : viewport;
  const current = useRef(viewport);
  const dragging = useRef(false);
  const root = useRef<SVGSVGElement>(null);
  useEffect(() => {
    current.current = viewport;
  }, [viewport]);
  useEffect(() => {
    if (container.current)
      return subscribeCanvasViewportPreview(container.current, (next) => {
        current.current = next;
        setPreview({ base: viewport, camera: next });
      });
  }, [container, viewport]);
  const display = useMemo(
    () => nodes.filter((node) => !isNodeHiddenByCollapsedFrame(node, nodes)),
    [nodes],
  );
  const projection = useMemo(() => {
    const bounds = getCanvasNodesBounds(display) ?? {
      left: -500,
      top: -500,
      right: 500,
      bottom: 500,
    };
    const x = bounds.left - 300,
      y = bounds.top - 300,
      width = bounds.right - bounds.left + 600,
      height = bounds.bottom - bounds.top + 600;
    const scale = Math.min(200 / width, 120 / height);
    return {
      x,
      y,
      scale,
      dx: (200 - width * scale) / 2,
      dy: (120 - height * scale) / 2,
    };
  }, [display]);
  function move(event: React.PointerEvent) {
    const rect = root.current?.getBoundingClientRect();
    if (!rect) return;
    const worldX =
        (event.clientX - rect.left - projection.dx) / projection.scale +
        projection.x,
      worldY =
        (event.clientY - rect.top - projection.dy) / projection.scale +
        projection.y;
    current.current = {
      x: size.width / 2 - worldX * current.current.k,
      y: size.height / 2 - worldY * current.current.k,
      k: current.current.k,
    };
    setPreview({ base: viewport, camera: current.current });
    onPreview(current.current);
  }
  return (
    <svg
      ref={root}
      width={200}
      height={120}
      aria-label="画布缩略地图"
      role="img"
      data-canvas-no-zoom
      className="touch-none rounded-xl bg-background/90 shadow-sm"
      onPointerDown={(event) => {
        if (disabled) return;
        event.stopPropagation();
        dragging.current = true;
        event.currentTarget.setPointerCapture(event.pointerId);
        move(event);
      }}
      onPointerMove={(event) => {
        if (dragging.current) move(event);
      }}
      onPointerUp={(event) => {
        if (dragging.current) {
          dragging.current = false;
          event.currentTarget.releasePointerCapture(event.pointerId);
          onCommit(current.current);
        }
      }}
      onPointerCancel={() => {
        dragging.current = false;
      }}
    >
      {display.map((node) => (
        <rect
          key={node.id}
          x={
            (node.position.x - projection.x) * projection.scale + projection.dx
          }
          y={
            (node.position.y - projection.y) * projection.scale + projection.dy
          }
          width={Math.max(2, node.width * projection.scale)}
          height={Math.max(2, canvasNodeRenderHeight(node) * projection.scale)}
          rx={2}
          fill={
            node.type === "group" ? "var(--muted)" : "var(--muted-foreground)"
          }
          opacity={0.6}
        />
      ))}
      <rect
        x={
          (-camera.x / camera.k - projection.x) * projection.scale +
          projection.dx
        }
        y={
          (-camera.y / camera.k - projection.y) * projection.scale +
          projection.dy
        }
        width={(size.width / camera.k) * projection.scale}
        height={(size.height / camera.k) * projection.scale}
        fill="none"
        stroke="var(--foreground)"
        strokeWidth={1.5}
      />
    </svg>
  );
}
