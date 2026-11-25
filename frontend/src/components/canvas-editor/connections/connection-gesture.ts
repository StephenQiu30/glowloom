export type ConnectionGesture = { x: number; y: number; dragged: boolean };

/** Connection dragging has a 1 CSS px threshold, independent of node dragging. */
export function updateConnectionGesture(
  start: ConnectionGesture,
  point: { x: number; y: number },
): ConnectionGesture {
  return {
    ...start,
    dragged:
      start.dragged || (point.x - start.x) ** 2 + (point.y - start.y) ** 2 > 1,
  };
}
