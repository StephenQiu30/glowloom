/** Convert screen motion through both canvas scale and timeline zoom. */
export function timelinePointerDelta(
  pixels: number,
  pixelsPerSecond: number,
  canvasScale = 1,
) {
  if (!Number.isFinite(pixels) || pixelsPerSecond <= 0 || canvasScale <= 0)
    return 0;
  return (pixels / canvasScale / pixelsPerSecond) * 1000;
}
export function timelineElementScale(element: HTMLElement) {
  const root = element.closest<HTMLElement>(".canvas-timeline");
  if (!root?.offsetWidth) return 1;
  return root.getBoundingClientRect().width / root.offsetWidth || 1;
}
