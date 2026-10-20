// Screen-space composer placement; viewport changes never mutate node coordinates.
import type { ViewportTransform } from "../viewport/transform";
export function nodePanelPosition(
  node: { x: number; y: number; width: number; height: number },
  viewport: ViewportTransform,
  containerWidth: number,
  containerHeight = Number.POSITIVE_INFINITY,
  panelHeight = 216,
) {
  const width = Math.min(Math.max(0, containerWidth - 32), 680);
  const safeLeft = containerWidth >= 768 ? 76 : 16;
  const safeTop = containerWidth >= 768 ? 80 : 132;
  const nodeTop = viewport.y + node.y * viewport.k;
  const below = viewport.y + (node.y + node.height) * viewport.k + 20;
  const maxHeight = Math.max(0, containerHeight - safeTop - 16);
  const height = Math.min(panelHeight, maxHeight);
  const above = nodeTop - height - 20;
  const top =
    below + height <= containerHeight - 16
      ? below
      : above >= safeTop
        ? above
        : Math.max(safeTop, containerHeight - height - 16);
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
