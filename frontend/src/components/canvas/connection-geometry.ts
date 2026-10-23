// Attribution and source revision are recorded in /licenses/canvas-sources.md.
// Upstream notices: /licenses/canvas-sources.md.
import type { Position } from "./types";

/** Right-facing output to left-facing input, including backward connections. */
export function canvasConnectionGeometry(start: Position, end: Position) {
  const distance = end.x - start.x;
  const offset = distance >= 0 ? distance / 2 : 6.25 * Math.sqrt(-distance);
  return {
    path: `M ${start.x} ${start.y} C ${start.x + offset} ${start.y}, ${end.x - offset} ${end.y}, ${end.x} ${end.y}`,
    bounds: {
      left: Math.min(start.x, end.x - offset),
      right: Math.max(start.x + offset, end.x),
      top: Math.min(start.y, end.y),
      bottom: Math.max(start.y, end.y),
    },
  };
}
