import { describe, expect, it } from "vitest";
import { timelinePointerDelta } from "@/components/canvas/timeline/timeline-gesture";
describe("canvas-scaled timeline gestures", () => {
  it.each([0.5, 1, 2])(
    "keeps a one-second drag at canvas scale %s",
    (scale) => {
      expect(timelinePointerDelta(21.4 * scale, 21.4, scale)).toBeCloseTo(1000);
      expect(timelinePointerDelta(-21.4 * scale, 21.4, scale)).toBeCloseTo(
        -1000,
      );
    },
  );
  it("rejects invalid viewport measurements", () => {
    expect(timelinePointerDelta(10, 0)).toBe(0);
    expect(timelinePointerDelta(10, 21.4, 0)).toBe(0);
  });
});
