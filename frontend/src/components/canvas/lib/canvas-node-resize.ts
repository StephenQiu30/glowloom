export type CanvasResizeHandle =
  | "top-left"
  | "top"
  | "top-right"
  | "right"
  | "bottom-right"
  | "bottom"
  | "bottom-left"
  | "left";

type ResizeRect = { x: number; y: number; width: number; height: number };

/** Keep the opposite edge/corner fixed; proportional side handles use the other axis's centre. */
export function resizeCanvasNode(
  rect: ResizeRect,
  handle: CanvasResizeHandle,
  dx: number,
  dy: number,
  ratio?: number,
): ResizeRect {
  const left = handle.includes("left"),
    right = handle.includes("right");
  const top = handle.includes("top"),
    bottom = handle.includes("bottom");
  const horizontal = left || right,
    vertical = top || bottom;
  let width = horizontal
    ? Math.max(220, rect.width + (left ? -dx : dx))
    : rect.width;
  let height = vertical
    ? Math.max(160, rect.height + (top ? -dy : dy))
    : rect.height;
  if (ratio && Number.isFinite(ratio) && ratio > 0) {
    if (horizontal && (!vertical || Math.abs(dx) >= Math.abs(dy)))
      height = width / ratio;
    else width = height * ratio;
    const grow = Math.max(1, 220 / width, 160 / height);
    width *= grow;
    height *= grow;
  }
  return {
    x: left
      ? rect.x + rect.width - width
      : !horizontal
        ? rect.x + (rect.width - width) / 2
        : rect.x,
    y: top
      ? rect.y + rect.height - height
      : !vertical
        ? rect.y + (rect.height - height) / 2
        : rect.y,
    width,
    height,
  };
}
