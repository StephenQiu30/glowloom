// Screen coordinates shared by node placement, fit and floating node panels.
export function canvasWorkspaceRect(
  width: number,
  height: number,
  aiWidth = 0,
) {
  const viewportWidth = Number.isFinite(width) ? Math.max(1, width) : 1;
  const viewportHeight = Number.isFinite(height) ? Math.max(1, height) : 1;
  const ai = Number.isFinite(aiWidth) ? Math.max(0, aiWidth) : 0;
  const panelsOpen = ai > 0;

  function axis(size: number, before: number, after: number) {
    // Compress reservations on small screens instead of producing a negative
    // extent or placing the only reachable canvas point outside the viewport.
    const extent = Math.max(Math.min(96, size), size - before - after);
    const reserved = before + after;
    const leading =
      reserved > size - extent ? (before / reserved) * (size - extent) : before;
    return { leading, extent };
  }

  const horizontal = axis(
    viewportWidth,
    panelsOpen ? 76 : 0,
    ai > 0 ? ai + 24 : 0,
  );
  const vertical = axis(viewportHeight, panelsOpen ? 64 : 0, 0);
  return {
    left: horizontal.leading,
    top: vertical.leading,
    width: horizontal.extent,
    height: vertical.extent,
    centerX: horizontal.leading + horizontal.extent / 2,
    centerY: vertical.leading + vertical.extent / 2,
  };
}
