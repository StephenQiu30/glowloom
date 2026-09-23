// Minimap world-to-screen mapping adapted from basketikun/infinite-canvas (MIT, commit dab19adc).
// See UPSTREAM-LICENSE.md in this directory for attribution.

import { Button } from "@/components/ui/button";
import type { CanvasViewport } from "./canvas-geometry";
import type { CanvasNode } from "./canvas-model";

export function CanvasMiniMap({
  nodes,
  viewport,
  size,
  onNavigate,
}: {
  nodes: CanvasNode[];
  viewport: CanvasViewport;
  size: { width: number; height: number };
  onNavigate: (worldX: number, worldY: number) => void;
}) {
  const viewLeft = -viewport.x / viewport.k;
  const viewTop = -viewport.y / viewport.k;
  const viewWidth = size.width / viewport.k;
  const viewHeight = size.height / viewport.k;
  const left = Math.min(viewLeft, ...nodes.map((node) => node.x)) - 60;
  const top = Math.min(viewTop, ...nodes.map((node) => node.y)) - 60;
  const right = Math.max(viewLeft + viewWidth, ...nodes.map((node) => node.x + node.width)) + 60;
  const bottom = Math.max(viewTop + viewHeight, ...nodes.map((node) => node.y + node.height)) + 60;
  const scale = Math.min(176 / (right - left), 104 / (bottom - top));
  const mapWidth = (right - left) * scale;
  const mapHeight = (bottom - top) * scale;

  return (
    <Button
      aria-label="小地图，点击定位画布"
      className="absolute bottom-5 right-5 hidden h-32 w-48 rounded-xl bg-background/95 p-2 text-left shadow-lg backdrop-blur-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring md:block"
      onClick={(event) => {
        const bounds = event.currentTarget.getBoundingClientRect();
        onNavigate(
          left + (event.clientX - bounds.left - 8) / scale,
          top + (event.clientY - bounds.top - 8) / scale,
        );
      }}
      variant="ghost"
    >
      <span className="pointer-events-none absolute left-3 top-2 text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
        Map
      </span>
      <span
        className="pointer-events-none absolute bottom-2 left-2 overflow-hidden rounded-md bg-muted/70"
        style={{ width: mapWidth, height: mapHeight }}
      >
        {nodes.map((node) => (
          <span
            className={
              node.kind === "project"
                ? "absolute rounded-sm bg-foreground/75"
                : "absolute rounded-sm bg-foreground/35"
            }
            key={node.id}
            style={{
              left: (node.x - left) * scale,
              top: (node.y - top) * scale,
              width: Math.max(4, node.width * scale),
              height: Math.max(4, node.height * scale),
            }}
          />
        ))}
        <span
          className="absolute rounded-sm outline outline-1 outline-primary"
          style={{
            left: (viewLeft - left) * scale,
            top: (viewTop - top) * scale,
            width: viewWidth * scale,
            height: viewHeight * scale,
          }}
        />
      </span>
    </Button>
  );
}
