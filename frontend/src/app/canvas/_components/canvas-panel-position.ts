// BeefTV a2196d3: canvas-workspace-overlays.tsx, media keepBelowNode placement (MIT).
import type { ViewportTransform } from "./canvas-viewport";

export function nodePanelPosition(
  node: { x: number; y: number; width: number; height: number },
  viewport: ViewportTransform,
  containerWidth: number,
) {
  const width = Math.min(
    Math.max(0, containerWidth - 32),
    Math.min(920, Math.max(660, Math.round(node.width * viewport.k * 1.5))),
  );
  return {
    width,
    left: Math.max(
      16,
      Math.min(
        Math.max(16, containerWidth - width - 16),
        viewport.x + (node.x + node.width / 2) * viewport.k - width / 2,
      ),
    ),
    top: viewport.y + (node.y + node.height) * viewport.k + 16,
  };
}
