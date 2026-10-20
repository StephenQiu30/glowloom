// Screen-space composer placement; viewport changes never mutate node coordinates.
import type { ViewportTransform } from "../viewport/transform";
export function nodePanelPosition(
  node: { x: number; y: number; width: number; height: number },
  viewport: ViewportTransform,
  containerWidth: number,
  containerHeight = Number.POSITIVE_INFINITY,
  panelHeight = 224,
) {
  const width = Math.min(
    Math.max(0, containerWidth - (containerWidth >= 768 ? 92 : 32)),
    680,
  );
  const safeLeft = containerWidth >= 768 ? 76 : 16;
  const below = Math.max(
    64,
    viewport.y + (node.y + node.height) * viewport.k + 16,
  );
  // Leave room for the node's title as well as the gap above its shell.
  const above = Math.min(
    containerHeight - 16,
    viewport.y + node.y * viewport.k - 48,
  );
  const belowSpace = Math.max(0, containerHeight - below - 16);
  const aboveSpace = Math.max(0, above - 64);
  const placeAbove = belowSpace < panelHeight && aboveSpace > belowSpace;
  const maxHeight = placeAbove ? aboveSpace : belowSpace;
  const top = placeAbove
    ? Math.max(64, above - Math.min(panelHeight, aboveSpace))
    : below;
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
