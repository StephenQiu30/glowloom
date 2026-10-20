// Screen-space composer placement; viewport changes never mutate node coordinates.
import type { ViewportTransform } from "../viewport/transform";
export function nodePanelPosition(
  node: { x: number; y: number; width: number; height: number },
  viewport: ViewportTransform,
  containerWidth: number,
  containerHeight = Number.POSITIVE_INFINITY,
) {
  const width = Math.min(
    Math.max(0, containerWidth - (containerWidth >= 768 ? 92 : 32)),
    960,
  );
  const safeLeft = containerWidth >= 768 ? 76 : 16;
  const top = viewport.y + (node.y + node.height) * viewport.k + 16;
  const maxHeight = Math.max(0, containerHeight - top - 16);
  return {
    width,
    maxHeight,
    left: Math.max(
      Math.min(safeLeft, Math.max(16, containerWidth - width - 16)),
      Math.min(
        Math.max(16, containerWidth - width - 16),
        viewport.x + (node.x + node.width / 2) * viewport.k - width / 2,
      ),
    ),
    top,
  };
}
