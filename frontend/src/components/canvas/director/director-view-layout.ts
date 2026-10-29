/** CSS-pixel rectangles use a top-left origin; the renderer converts to bottom-left. */
export type DirectorViewRect = {
  x: number;
  y: number;
  width: number;
  height: number;
};
export function directorFitView(
  rect: DirectorViewRect,
  aspect: number,
): DirectorViewRect {
  const safeAspect = Number.isFinite(aspect) && aspect > 0 ? aspect : 16 / 9;
  const width = Math.min(rect.width, rect.height * safeAspect);
  const height = width / safeAspect;
  return {
    x: rect.x + (rect.width - width) / 2,
    y: rect.y + (rect.height - height) / 2,
    width,
    height,
  };
}
export function directorViewLayout(
  width: number,
  height: number,
  aspect: number,
  options: {
    split: boolean;
    compact: boolean;
    cameraMain: boolean;
    previewVisible: boolean;
    previewWidth: number;
  },
) {
  const whole = { x: 0, y: 0, width, height };
  const editor =
    options.split && !options.compact ? { ...whole, width: width / 2 } : whole;
  const main =
    options.cameraMain || options.compact
      ? directorFitView(editor, aspect)
      : editor;
  const safeAspect = Number.isFinite(aspect) && aspect > 0 ? aspect : 16 / 9;
  const floatingWidth = Math.min(
    options.previewWidth,
    width * 0.6,
    height * 0.5 * safeAspect,
  );
  const preview = options.compact
    ? directorFitView(whole, safeAspect)
    : options.split
      ? directorFitView(
          { x: width / 2, y: 0, width: width / 2, height },
          safeAspect,
        )
      : options.previewVisible
        ? {
            x: width - floatingWidth - 12,
            y: 48,
            width: floatingWidth,
            height: floatingWidth / safeAspect,
          }
        : null;
  return { editor, main, preview };
}
export function directorViewPoint(
  rect: DirectorViewRect,
  x: number,
  y: number,
): [number, number] | null {
  if (
    rect.width <= 0 ||
    rect.height <= 0 ||
    x < rect.x ||
    y < rect.y ||
    x > rect.x + rect.width ||
    y > rect.y + rect.height
  )
    return null;
  return [
    (2 * (x - rect.x)) / rect.width - 1,
    1 - (2 * (y - rect.y)) / rect.height,
  ];
}
