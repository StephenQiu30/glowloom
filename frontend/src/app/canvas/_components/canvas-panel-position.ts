// Screen-space composer placement; viewport changes never mutate node coordinates.
import type { ViewportTransform } from "./canvas-viewport";
export function nodePanelPosition(
  node: { x: number; y: number; width: number; height: number },
  viewport: ViewportTransform,
  containerWidth: number,
  containerHeight = Number.POSITIVE_INFINITY,
  panelHeight = 216,
) {
  const width = Math.min(Math.max(0, containerWidth - 32), 680);
  const safeLeft = containerWidth >= 768 ? 76 : 16;
  const nodeTop = viewport.y + node.y * viewport.k;
  const below = viewport.y + (node.y + node.height) * viewport.k + 20;
  const height = Math.min(panelHeight, Math.max(0, containerHeight - 96));
  const above = nodeTop - height - 20;
  const top =
    below + height <= containerHeight - 16
      ? below
      : above >= 80
        ? above
        : Math.max(80, containerHeight - height - 16);
  return {
    width,
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
