"use client";
// BeefTV a2196d3: canvas-mini-map.tsx world bounds, margins and navigation (MIT).
import { isDraft, type Node } from "../document";
import type { ViewportTransform } from "./transform";
import { Button } from "@/components/ui/button";

export function CanvasMiniMap({
  nodes,
  viewport,
  width,
  height,
  onNavigate,
}: {
  nodes: Node[];
  viewport: ViewportTransform;
  width: number;
  height: number;
  onNavigate: (x: number, y: number) => void;
}) {
  const left = Math.min(0, ...nodes.map((n) => n.x)) - 500;
  const top = Math.min(0, ...nodes.map((n) => n.y)) - 500;
  const right =
    Math.max(500, ...nodes.map((n) => n.x + (n.width ?? 288))) + 500;
  const bottom =
    Math.max(500, ...nodes.map((n) => n.y + (n.height ?? 200))) + 500;
  const scale = Math.min(156 / (right - left), 114 / (bottom - top));
  const offsetX = (156 - (right - left) * scale) / 2,
    offsetY = (114 - (bottom - top) * scale) / 2;
  return (
    <Button
      variant="outline"
      type="button"
      className="canvas-mini-map h-auto p-0"
      data-canvas-no-zoom
      aria-label="在小地图定位画布"
      onClick={(event) => {
        const rect = event.currentTarget.getBoundingClientRect();
        if (!event.detail) {
          onNavigate((left + right) / 2, (top + bottom) / 2);
          return;
        }
        onNavigate(
          left +
            (((event.clientX - rect.left) * 156) / rect.width - offsetX) /
              scale,
          top +
            (((event.clientY - rect.top) * 114) / rect.height - offsetY) /
              scale,
        );
      }}
    >
      <svg
        className="size-full"
        viewBox="0 0 156 114"
        role="img"
        aria-label="节点位置与当前视口"
      >
        {nodes.map((node) => (
          <rect
            key={node.id}
            x={(node.x - left) * scale + offsetX}
            y={(node.y - top) * scale + offsetY}
            width={(node.width ?? 288) * scale}
            height={(node.height ?? 200) * scale}
            rx="2"
            fill="var(--canvas-node-muted)"
            opacity={isDraft(node) ? 0.65 : 0.3}
          />
        ))}
        <rect
          x={(-viewport.x / viewport.k - left) * scale + offsetX}
          y={(-viewport.y / viewport.k - top) * scale + offsetY}
          width={(width / viewport.k) * scale}
          height={(height / viewport.k) * scale}
          fill="var(--canvas-selection)"
          stroke="var(--canvas-node-text)"
          strokeWidth="1"
        />
      </svg>
    </Button>
  );
}
