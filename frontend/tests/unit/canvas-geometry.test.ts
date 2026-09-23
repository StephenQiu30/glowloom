import { describe, expect, it } from "vitest";

import { fitCanvas, MAX_ZOOM, MIN_ZOOM, zoomAtPoint } from "@/components/canvas/canvas-geometry";

describe("canvas viewport", () => {
  it("keeps the world point below the cursor fixed while zooming", () => {
    const before = { x: 76, y: -42, k: 0.8 };
    const cursor = { x: 413, y: 207 };
    const world = { x: (cursor.x - before.x) / before.k, y: (cursor.y - before.y) / before.k };
    const after = zoomAtPoint(before, 1.6, cursor);

    expect(after.x + world.x * after.k).toBeCloseTo(cursor.x);
    expect(after.y + world.y * after.k).toBeCloseTo(cursor.y);
    expect(zoomAtPoint(before, 100, cursor).k).toBe(MAX_ZOOM);
    expect(zoomAtPoint(before, 0, cursor).k).toBe(MIN_ZOOM);
  });

  it("fits every node in the available viewport", () => {
    const nodes = [
      { x: -100, y: 40, width: 300, height: 150 },
      { x: 480, y: 300, width: 220, height: 120 },
    ];
    const size = { width: 1000, height: 700 };
    const viewport = fitCanvas(nodes, size);

    for (const node of nodes) {
      expect(node.x * viewport.k + viewport.x).toBeGreaterThanOrEqual(0);
      expect(node.y * viewport.k + viewport.y).toBeGreaterThanOrEqual(0);
      expect((node.x + node.width) * viewport.k + viewport.x).toBeLessThanOrEqual(size.width);
      expect((node.y + node.height) * viewport.k + viewport.y).toBeLessThanOrEqual(size.height);
    }
  });
});
